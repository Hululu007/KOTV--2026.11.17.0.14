package spider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/localproxy"
	"github.com/bobo/KOTV/internal/paths"
	appruntime "github.com/bobo/KOTV/internal/runtime"
)

// ErrJavaBridgeInterrupted 表示调用被换源/关闭主动打断，不应再重试同一请求。
var ErrJavaBridgeInterrupted = errors.New("JAR 调用已中断")

// 桌面：独立 java -jar --serve（HTTP 多路）；超时后 Kill 并允许重建。
// 整次 jar 调用墙钟（含多次 OkHttp）；Android/桌面同为 120s。
const javaBridgeCallTimeout = 120 * time.Second

func jarBridgeCallTimeout() time.Duration {
	return javaBridgeCallTimeout
}

type javaBridgeClient struct {
	ownerUID   string // RuntimeUserID；空=本机共享
	mu         sync.Mutex
	cmd        *exec.Cmd
	baseURL    string // http://127.0.0.1:port
	stderrFile *os.File
	proc       atomic.Pointer[os.Process]
	epoch      atomic.Uint64
}

var javaBridge javaBridgeClient // ownerUID 空=共享

// Android SpiderService 最近一次 /jar/call 是否成功。失败后下次进详情重新 parseJar。
var androidJarReachable atomic.Bool

// 远端已登录用户：每人独立 JVM；空 userId 走共享 javaBridge（本机）。
// 不能把多租户塞进同一 JVM：bridge 内 Prefers（kotv.cache.dir）、spiders 缓存键、
// recentJar 均未按 userId 隔离；共用会串 Prefers / 爬虫实例 / proxy 解析。
// 同一 JVM 内已可装多份 jar（loaders 按路径）；缺的是「多用户」隔离，不是「多 jar」。
var (
	userBridgesMu sync.Mutex
	userBridges   = map[string]*javaBridgeClient{}
)

func bridgeForUser(userID string) *javaBridgeClient {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return &javaBridge
	}
	userBridgesMu.Lock()
	defer userBridgesMu.Unlock()
	if b, ok := userBridges[userID]; ok {
		return b
	}
	b := &javaBridgeClient{ownerUID: userID}
	userBridges[userID] = b
	return b
}

func bridgeForCurrent() *javaBridgeClient {
	// 脚本（jar 文件）全局共享；仅 RuntimeUserID 非空时用独立 JVM。
	return bridgeForUser(hostclient.RuntimeUserID())
}

// HostSpiderReady 远端主机爬虫面是否可用（安卓=本机 9979；桌面=捆绑 runtime 可用即可）。
func HostSpiderReady() (ok bool, errMsg string) {
	if runtime.GOOS == "android" {
		client := &http.Client{Timeout: 800 * time.Millisecond}
		resp, err := client.Get(localproxy.DefaultBridgeBaseURL() + "/health")
		if err != nil {
			return false, fmt.Sprintf("安卓 SpiderService(:%d) 不可达：请保持 App/前台服务运行", localproxy.DefaultBridgePort)
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			return false, fmt.Sprintf("安卓 SpiderService 异常: HTTP %d", resp.StatusCode)
		}
		return true, ""
	}
	return true, ""
}

// 点播配置的网络参数（headers/proxy/hosts/doh），在每个新 worker 启动时重放。
var (
	netConfigMu       sync.Mutex
	netConfigJSON     []byte // 空 clientId 默认桶（兼容）
	netConfigByClient map[string][]byte
	userProxyJSON     []byte
	userProxyByClient map[string][]byte
	localProxyJSON    []byte // 本机 HTTP 实听端口 → bridge Proxy.set
	netPrimed         bool   // Android HTTP 用
)

// 进行中的 JAR HTTP 调用，按 ScopeID 取消 Go 侧等待（进程回收见 callCtx）。
type jarInflight struct {
	id       uint64
	clientID string // 实为 ScopeID
	cancel   context.CancelFunc
}

var (
	jarInflightMu sync.Mutex
	jarInflightQ  = map[uint64]*jarInflight{}
	jarCallSeq    atomic.Uint64
)

func beginJarCall(clientID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	id := jarCallSeq.Add(1)
	jarInflightMu.Lock()
	jarInflightQ[id] = &jarInflight{id: id, clientID: clientID, cancel: cancel}
	jarInflightMu.Unlock()
	return ctx, func() {
		cancel()
		jarInflightMu.Lock()
		delete(jarInflightQ, id)
		jarInflightMu.Unlock()
	}
}

func cancelJarCalls(clientID string) {
	jarInflightMu.Lock()
	defer jarInflightMu.Unlock()
	for _, g := range jarInflightQ {
		if clientID == "" || g.clientID == clientID {
			g.cancel()
		}
	}
}

// cancelSharedJarCalls 只取消本机共享侧 JAR in-flight（非 u: 租户），不动远端用户。
func cancelSharedJarCalls() {
	jarInflightMu.Lock()
	defer jarInflightMu.Unlock()
	for _, g := range jarInflightQ {
		if hostclient.UserIDFromScope(g.clientID) == "" {
			g.cancel()
		}
	}
}

// clearNetConfigForUser 删掉该远端用户 Scope 下的点播网络/代理配置桶，避免离开后仍被新 JVM 重放。
func clearNetConfigForUser(userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	netConfigMu.Lock()
	defer netConfigMu.Unlock()
	purge := func(m map[string][]byte) {
		if m == nil {
			return
		}
		for cid := range m {
			if hostclient.UserIDFromScope(cid) == userID {
				delete(m, cid)
			}
		}
	}
	purge(netConfigByClient)
	purge(userProxyByClient)
}

// SetNetConfig 把点播配置里的 headers/proxy/hosts/doh 下发到 bridge OkHttp（按当前 ScopeID）。
// 各用户当前源可不同，故 net 按 Scope 分桶；空 Scope 写默认桶。
func SetNetConfig(headers, proxy, hosts, doh []byte) {
	setGoNetConfig(headers, hosts)
	cid := hostclient.ScopeID()
	args := map[string]json.RawMessage{}
	add := func(name string, raw []byte) {
		trimmed := strings.TrimSpace(string(raw))
		if trimmed == "" || trimmed == "null" {
			return
		}
		args[name] = json.RawMessage(raw)
	}
	add("headers", headers)
	add("proxy", proxy)
	add("hosts", hosts)
	add("doh", doh)
	if cid != "" {
		b, _ := json.Marshal(cid)
		args["clientId"] = b
	}

	req := map[string]interface{}{"method": "configNet", "args": args, "clientId": cid}
	payload, err := json.Marshal(req)
	if err != nil {
		return
	}
	netConfigMu.Lock()
	if netConfigByClient == nil {
		netConfigByClient = map[string][]byte{}
	}
	netConfigByClient[cid] = payload
	if cid == "" {
		netConfigJSON = payload
	}
	netPrimed = false
	netConfigMu.Unlock()
	pushBridgeIfAlive(payload)
}

// SetUserProxy 把 KOTV 用户代理设置下发到 bridge（按 ScopeID 分桶）。
func SetUserProxy(spec string) {
	spec = strings.TrimSpace(spec)
	url := ""
	if !(spec == "" || spec == "false" || strings.HasPrefix(spec, "false#")) {
		if i := strings.Index(spec, "#"); i >= 0 {
			url = strings.TrimSpace(spec[i+1:])
		} else {
			url = spec
		}
	}
	cid := hostclient.ScopeID()
	args := map[string]string{"url": url}
	req := map[string]interface{}{"method": "configProxy", "args": args, "clientId": cid}
	payload, err := json.Marshal(req)
	if err != nil {
		return
	}
	netConfigMu.Lock()
	if userProxyByClient == nil {
		userProxyByClient = map[string][]byte{}
	}
	userProxyByClient[cid] = payload
	if cid == "" {
		userProxyJSON = payload
	}
	netPrimed = false
	netConfigMu.Unlock()
	pushBridgeIfAlive(payload)
}

// SetLocalProxyPort 本机 HTTP 实听端口下发到 bridge：System.setProperty + Proxy.set。
func SetLocalProxyPort(port int) {
	if port <= 0 {
		return
	}
	req := map[string]interface{}{"method": "configLocalProxy", "args": map[string]int{"port": port}}
	payload, err := json.Marshal(req)
	if err != nil {
		return
	}
	netConfigMu.Lock()
	localProxyJSON = payload
	netPrimed = false
	netConfigMu.Unlock()
	pushBridgeIfAlive(payload)
}

func currentNetConfig() [][]byte {
	return currentNetConfigFor(hostclient.RuntimeUserID())
}

// currentNetConfigFor 只回放属于该运行时桶的网络配置，避免新 JVM 灌入其它租户的 headers/proxy。
func currentNetConfigFor(ownerUID string) [][]byte {
	ownerUID = strings.TrimSpace(ownerUID)
	netConfigMu.Lock()
	defer netConfigMu.Unlock()
	var out [][]byte
	seen := map[string]struct{}{}
	add := func(payload []byte) {
		if len(payload) == 0 {
			return
		}
		key := string(payload)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, payload)
	}
	scopeOwned := func(cid string) bool {
		uid := hostclient.UserIDFromScope(cid)
		if ownerUID == "" {
			// 本机共享 JVM：默认桶 + 本机 c: 窗口；不含远端 u: 租户。
			return uid == ""
		}
		return uid == ownerUID
	}
	if ownerUID == "" {
		add(netConfigJSON)
		add(userProxyJSON)
	}
	for cid, payload := range netConfigByClient {
		if scopeOwned(cid) {
			add(payload)
		}
	}
	for cid, payload := range userProxyByClient {
		if scopeOwned(cid) {
			add(payload)
		}
	}
	add(localProxyJSON)
	return out
}

// JavaInvoke 把 Python 的一次 classpath 调用转到当前 JVM。
// 参数保持原始 JSON，避免大整数经过 map 后变成浮点。
func JavaInvoke(body []byte) ([]byte, error) {
	payload, err := javaInvokePayload(body)
	if err != nil {
		return nil, err
	}
	raw, err := callJavaBridge(payload)
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
}

func javaInvokePayload(body []byte) ([]byte, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("java 请求不是 JSON")
	}
	meta := bridgeRequest{Method: "javaInvoke"}
	fillBridgeClient(&meta)
	if meta.ClientID == "" {
		var probe struct {
			ClientID string `json:"clientId"`
		}
		_ = json.Unmarshal(body, &probe)
		if scope := strings.TrimSpace(probe.ClientID); scope != "" {
			meta.ClientID = scope
			if uid := hostclient.UserIDFromScope(scope); uid != "" {
				meta.UserID = uid
				meta.RemoteUI = true
			}
		}
	}
	return json.Marshal(struct {
		Method   string          `json:"method"`
		Args     json.RawMessage `json:"args"`
		ClientID string          `json:"clientId,omitempty"`
		UserID   string          `json:"userId,omitempty"`
		Username string          `json:"username,omitempty"`
		RemoteUI bool            `json:"remoteUi,omitempty"`
		JarGen   uint64          `json:"jarGen"`
	}{
		Method:   meta.Method,
		Args:     body,
		ClientID: meta.ClientID,
		UserID:   meta.UserID,
		Username: meta.Username,
		RemoteUI: meta.RemoteUI,
		JarGen:   meta.JarGen,
	})
}

// callJavaBridge：桌面走独立 java HTTP bridge；Android 走 Native Service HTTP。可并发。
func callJavaBridge(payload []byte) (string, error) {
	cid := hostclient.ScopeID()
	ctx, end := beginJarCall(cid)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, jarBridgeCallTimeout())
	defer cancel()

	if runtime.GOOS == "android" {
		if err := primeAndroidNetConfigOnce(); err != nil {
			return "", err
		}
		return androidPostJarCtx(ctx, payload)
	}
	return bridgeForCurrent().callCtx(ctx, payload)
}

// interruptJavaBridgeForClient 取消进行中的 JAR HTTP（OkHttp + Go 客户端）。
// 不直接杀 JVM；被取消的 callCtx 返回时会 stopLocked 回收共享/用户 bridge。
func interruptJavaBridgeForClient(clientID string) {
	cancelJarCalls(clientID)
	softCancelJavaBridge(clientID)
}

// restartSharedRuntime 硬重启本机共享 JVM/Py/JS/Node（不动远端用户桶，不删脚本磁盘缓存）。
// 对外请用 HardRestartSharedRuntimes。
func restartSharedRuntime() {
	clearJarGoStateForRuntime("")
	cancelSharedJarCalls()
	// 杀共享 bridge，绝不碰 userBridges。
	softCancelSharedBridgeOnly()
	if runtime.GOOS == "android" {
		client := &http.Client{Timeout: 2 * time.Second}
		req, err := http.NewRequest(http.MethodPost, localproxy.DefaultBridgeBaseURL()+"/jar/interrupt", bytes.NewReader([]byte("{}")))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			if resp, err := client.Do(req); err == nil {
				_ = resp.Body.Close()
			}
		}
	} else {
		javaBridge.epoch.Add(1)
		javaBridge.mu.Lock()
		javaBridge.stopLocked()
		javaBridge.mu.Unlock()
	}
	clearSharedJsPy()
	clearSharedJarSpiders()
	clearSharedNodeHost()
}

func softCancelSharedBridgeOnly() {
	postJavaBridgeCancel([]*javaBridgeClient{&javaBridge}, "")
}

func clearSharedJarSpiders() {
	dropJarSpiders("\x01", false)
}

// javaBridgesForCancel 选 SoftCancel 应通知的 JVM：
//   - clientID 空（SoftCancelAll）：共享 + 全部用户桶（与当前 Bind 无关）
//   - u:<uid>：仅该用户桶（尚未创建则空，绝不回落共享）
//   - c:… / 其它：本机共享桶
func javaBridgesForCancel(clientID string) []*javaBridgeClient {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return snapshotJavaBridges()
	}
	userBridgesMu.Lock()
	defer userBridgesMu.Unlock()
	if uid := hostclient.UserIDFromScope(clientID); uid != "" {
		if b := userBridges[uid]; b != nil {
			return []*javaBridgeClient{b}
		}
		return nil
	}
	return []*javaBridgeClient{&javaBridge}
}

func softCancelJavaBridge(clientID string) {
	postJavaBridgeCancel(javaBridgesForCancel(clientID), clientID)
}

func postJavaBridgeCancel(targets []*javaBridgeClient, clientID string) {
	body, _ := json.Marshal(map[string]string{"clientId": clientID})
	payload, _ := json.Marshal(map[string]interface{}{
		"method":   "cancelClient",
		"args":     map[string]string{"clientId": clientID},
		"clientId": clientID,
	})
	if runtime.GOOS == "android" {
		_, _ = androidPostJarShort(payload)
		return
	}
	for _, br := range targets {
		if br == nil {
			continue
		}
		base := br.currentBaseURL()
		if base == "" {
			continue
		}
		_ = postJarPath(base, "/jar/cancel", body, 2*time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, _ = br.httpPostCtx(ctx, payload)
		cancel()
	}
}

// snapshotJavaBridges 共享 + 全部用户 JVM（只读快照）。
func snapshotJavaBridges() []*javaBridgeClient {
	userBridgesMu.Lock()
	defer userBridgesMu.Unlock()
	out := make([]*javaBridgeClient, 0, 1+len(userBridges))
	out = append(out, &javaBridge)
	for _, b := range userBridges {
		out = append(out, b)
	}
	return out
}

// ShutdownJavaBridge 引擎退出时杀掉共享与所有用户 JVM。
func ShutdownJavaBridge() {
	if runtime.GOOS == "android" {
		return
	}
	javaBridge.epoch.Add(1)
	javaBridge.mu.Lock()
	javaBridge.stopLocked()
	javaBridge.mu.Unlock()
	userBridgesMu.Lock()
	all := make([]*javaBridgeClient, 0, len(userBridges))
	for id, b := range userBridges {
		all = append(all, b)
		delete(userBridges, id)
	}
	userBridgesMu.Unlock()
	for _, b := range all {
		b.epoch.Add(1)
		b.mu.Lock()
		b.stopLocked()
		b.mu.Unlock()
	}
}

func (w *javaBridgeClient) currentBaseURL() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.baseURL
}

func (w *javaBridgeClient) pushIfAlive(payload []byte) {
	base := w.currentBaseURL()
	if base == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = w.httpPostCtx(ctx, payload)
}

func (w *javaBridgeClient) callCtx(ctx context.Context, payload []byte) (string, error) {
	startEpoch := w.epoch.Load()
	var transportErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", ErrJavaBridgeInterrupted
		}
		if w.epoch.Load() != startEpoch {
			return "", ErrJavaBridgeInterrupted
		}
		if err := w.ensureStarted(); err != nil {
			return "", err
		}
		out, err := w.httpPostCtx(ctx, payload)
		if err == nil {
			if w.epoch.Load() != startEpoch {
				return "", ErrJavaBridgeInterrupted
			}
			return out, nil
		}
		transportErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// 超时或软取消放弃：回收 JVM，下次 ensureStarted 重建（与传输失败同路）。
			w.mu.Lock()
			w.stopLocked()
			w.mu.Unlock()
			return "", ErrJavaBridgeInterrupted
		}
		if w.epoch.Load() != startEpoch {
			return "", ErrJavaBridgeInterrupted
		}
		// 传输失败：停掉并重试一次（重建 JVM）。
		w.mu.Lock()
		w.stopLocked()
		w.mu.Unlock()
	}
	return "", fmt.Errorf("Java bridge 异常退出，自动重启后仍不可用: %w", transportErr)
}

func (w *javaBridgeClient) ensureStarted() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.startLocked()
}

func (w *javaBridgeClient) httpPostCtx(ctx context.Context, payload []byte) (string, error) {
	w.mu.Lock()
	base := w.baseURL
	w.mu.Unlock()
	if base == "" {
		return "", fmt.Errorf("Java bridge 未就绪")
	}
	return postJarPathCtx(ctx, base, "/jar/call", payload)
}

func (w *javaBridgeClient) aliveLocked() bool {
	return w.cmd != nil && w.cmd.Process != nil && w.cmd.ProcessState == nil && w.baseURL != ""
}

func (w *javaBridgeClient) startLocked() error {
	if w.aliveLocked() {
		return nil
	}
	w.stopLocked()

	java, err := findJava()
	if err != nil {
		return err
	}
	bridgeJar := findBridgeJar()
	if bridgeJar == "" {
		return fmt.Errorf("未找到 spider-bridge.jar，请运行 bridge 目录下的构建脚本")
	}

	port, err := pickFreeLocalPort()
	if err != nil {
		return err
	}

	proxyPort := localproxy.Port()
	// data.dir=应用根（TV 凭证仍按用户名分目录）；cache.dir=每用户 JVM 可变态（Prefers 等）。
	dataDir := paths.Root()
	cacheDir := paths.UserJVMDir(w.ownerUID)
	cmd := exec.Command(java,
		"-Djava.awt.headless=true",
		fmt.Sprintf("-Dkotv.data.dir=%s", dataDir),
		fmt.Sprintf("-Dkotv.cache.dir=%s", cacheDir),
		fmt.Sprintf("-Dkotv.bridge.port=%d", port),
		"-Djava.net.useSystemProxies=false",
		"-DsocksProxyHost=",
		"-DsocksProxyPort=",
		"-Dhttp.proxyHost=",
		"-Dhttp.proxyPort=",
		"-Dhttps.proxyHost=",
		"-Dhttps.proxyPort=",
		"-Dftp.proxyHost=",
		"-Dftp.proxyPort=",
		fmt.Sprintf("-Dkotv.proxy.port=%d", proxyPort),
		"-Dsun.net.client.defaultConnectTimeout=8000",
		"-Dsun.net.client.defaultReadTimeout=10000",
		"-Dfile.encoding=UTF-8",
		"--add-opens=java.base/java.lang=ALL-UNNAMED",
		"--add-opens=java.base/java.util=ALL-UNNAMED",
		"-jar", bridgeJar,
		"--serve",
		fmt.Sprintf("--http-port=%d", port),
	)
	cmd.Dir = filepath.Dir(bridgeJar)
	cmd.Env = append(filterProxyEnv(os.Environ()), fmt.Sprintf("KOTV_PROXY_PORT=%d", proxyPort))
	_ = os.MkdirAll(paths.LogDir(), 0o755)
	stderrName := "java-bridge-" + paths.RuntimeOwnerLabel(w.ownerUID) + ".err.log"
	stderrPath := filepath.Join(paths.LogDir(), stderrName)
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		cmd.Stderr = stderrFile
	} else {
		cmd.Stderr = os.Stderr
		stderrFile = nil
	}
	setChildProcAttrs(cmd)

	if err := cmd.Start(); err != nil {
		if stderrFile != nil {
			_ = stderrFile.Close()
		}
		return fmt.Errorf("启动 Java bridge 失败: %w", err)
	}

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	w.cmd = cmd
	w.baseURL = base
	w.stderrFile = stderrFile
	w.proc.Store(cmd.Process)
	log.Printf("java bridge started pid=%d owner=%q jar=%s url=%s cache=%s", cmd.Process.Pid, w.ownerUID, bridgeJar, base, cacheDir)

	if err := waitBridgeHealthy(base, 15*time.Second); err != nil {
		hint := readBridgeErrTail(stderrPath)
		w.stopLocked()
		if hint != "" {
			return fmt.Errorf("等待 Java bridge HTTP 就绪失败: %w（详见 %s：%s）", err, stderrPath, hint)
		}
		return fmt.Errorf("等待 Java bridge HTTP 就绪失败: %w（详见 %s）", err, stderrPath)
	}

	for _, cfg := range currentNetConfigFor(w.ownerUID) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := postJarPathCtx(ctx, base, "/jar/call", cfg)
		cancel()
		if err != nil {
			hint := readBridgeErrTail(stderrPath)
			w.stopLocked()
			if hint != "" {
				return fmt.Errorf("下发网络配置到 Java bridge 失败: %w（详见 %s：%s）", err, stderrPath, hint)
			}
			return fmt.Errorf("下发网络配置到 Java bridge 失败: %w（详见 %s）", err, stderrPath)
		}
	}
	return nil
}

func pickFreeLocalPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || addr.Port <= 0 {
		return 0, fmt.Errorf("无法分配本地端口")
	}
	return addr.Port, nil
}

func waitBridgeHealthy(base string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	var last error
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 400 {
				return nil
			}
			last = fmt.Errorf("http %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	return last
}

func readBridgeErrTail(path string) string {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return ""
	}
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		s = s[len(s)-240:]
	}
	s = strings.ReplaceAll(s, "\n", " | ")
	return s
}

func (w *javaBridgeClient) stopLocked() {
	w.proc.Store(nil)
	w.baseURL = ""
	if w.cmd != nil {
		if w.cmd.Process != nil && w.cmd.ProcessState == nil {
			killProcessTree(w.cmd.Process)
		}
		_ = w.cmd.Wait()
	}
	if w.stderrFile != nil {
		_ = w.stderrFile.Close()
	}
	w.cmd = nil
	w.stderrFile = nil
}

func findJava() (string, error) {
	if p := appruntime.Java(); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("未找到捆绑 Java：请运行 ./scripts/prepare-runtime.sh 准备 runtime/jre")
}

func filterProxyEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		el := strings.ToLower(e)
		if strings.HasPrefix(el, "http_proxy=") || strings.HasPrefix(el, "https_proxy=") ||
			strings.HasPrefix(el, "all_proxy=") || strings.HasPrefix(el, "socks") ||
			strings.HasPrefix(el, "java_tool_options=") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// trimCString 去掉残留的 \0（TrimSpace 不会去掉）。
func trimCString(s string) string {
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func pushBridgeIfAlive(payload []byte) {
	if runtime.GOOS == "android" {
		client := &http.Client{Timeout: 300 * time.Millisecond}
		resp, err := client.Get(localproxy.DefaultBridgeBaseURL() + "/health")
		if err != nil {
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			return
		}
		_, _ = androidCallJavaBridgeShort(payload)
		netConfigMu.Lock()
		netPrimed = true
		netConfigMu.Unlock()
		return
	}
	bridgeForCurrent().pushIfAlive(payload)
}

func primeAndroidNetConfigOnce() error {
	netConfigMu.Lock()
	if netPrimed {
		netConfigMu.Unlock()
		return nil
	}
	// 必须包含当前 Scope 的配置：多用户/多客户端时 headers/hosts/doh 按 Scope 分桶，
	// 只重放默认桶会导致当前会话的 Referer/UA/DoH 丢失，站点请求被反爬拦成空页。
	cid := hostclient.ScopeID()
	cfgs := make([][]byte, 0, 3)
	push := func(b []byte) {
		if len(b) == 0 {
			return
		}
		for _, seen := range cfgs {
			if string(seen) == string(b) {
				return
			}
		}
		cfgs = append(cfgs, append([]byte(nil), b...))
	}
	push(netConfigJSON)
	if cid != "" && netConfigByClient != nil {
		push(netConfigByClient[cid])
	}
	push(userProxyJSON)
	push(localProxyJSON)
	netPrimed = true
	netConfigMu.Unlock()

	for _, cfg := range cfgs {
		if _, err := androidCallJavaBridge(cfg); err != nil {
			netConfigMu.Lock()
			netPrimed = false
			netConfigMu.Unlock()
			return fmt.Errorf("下发网络配置到 Android jar bridge 失败: %w", err)
		}
	}
	return nil
}

func androidCallJavaBridge(payload []byte) (string, error) {
	return androidPostJar(payload, 20*time.Second)
}

func androidCallJavaBridgeShort(payload []byte) (string, error) {
	return androidPostJarShort(payload)
}

func androidPostJarShort(payload []byte) (string, error) {
	return androidPostJar(payload, 2*time.Second)
}

func androidPostJar(payload []byte, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return androidPostJarCtx(ctx, payload)
}

func androidPostJarCtx(ctx context.Context, payload []byte) (string, error) {
	out, err := postJarPathCtx(ctx, localproxy.DefaultBridgeBaseURL(), "/jar/call", payload)
	if err != nil {
		androidJarReachable.Store(false)
		forgetJarRecent(hostclient.RuntimeUserID())
		return "", err
	}
	androidJarReachable.Store(true)
	return out, nil
}

func postJarPath(base, path string, payload []byte, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := postJarPathCtx(ctx, base, path, payload)
	return err
}

func postJarPathCtx(ctx context.Context, base, path string, payload []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	client := &http.Client{Timeout: jarBridgeCallTimeout()}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	s := strings.TrimSpace(string(b))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("jar call failed: http=%d %s", resp.StatusCode, s)
	}
	if path == "/jar/call" && s == "" {
		return "", fmt.Errorf("jar call empty response")
	}
	return s, nil
}
