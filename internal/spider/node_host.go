package spider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bobo/KOTV/internal/localproxy"
	"github.com/bobo/KOTV/internal/paths"
	"github.com/bobo/KOTV/internal/playproxy"
	appruntime "github.com/bobo/KOTV/internal/runtime"
	"github.com/bobo/KOTV/internal/util"

	_ "embed"
)

//go:embed noderunner.js
var nodeRunnerSrc string

const (
	nodeStartTimeout   = 45 * time.Second
	nodeConfigTimeout  = 15 * time.Second
	nodeCallTimeout    = 45 * time.Second
	nodeRestartBackoff = 2 * time.Second
	nodeMaxBackoff     = 30 * time.Second
)

var (
	defaultNodeHost     *NodeHost
	defaultNodeHostOnce sync.Once
)

// NodeHost 管理猫爪 Node 源包：下载、启动、反代与消息回调。
type NodeHost struct {
	mu       sync.Mutex
	pkgs     map[string]*nodePkg // id → pkg
	apiIndex map[string]string   // /spider/... → pkgID
	runner   string
}

type nodePkg struct {
	id, pkgURL, dir, dataDir string
	indexJS, configJS        string

	mu       sync.Mutex
	cmd      *exec.Cmd
	port     int
	stopping bool
	backoff  time.Duration
	cancel   context.CancelFunc
}

// DefaultNodeHost 进程内单例。
func DefaultNodeHost() *NodeHost {
	defaultNodeHostOnce.Do(func() {
		defaultNodeHost = &NodeHost{
			pkgs:     map[string]*nodePkg{},
			apiIndex: map[string]string{},
		}
	})
	return defaultNodeHost
}

// MsgPort 引擎本机端口，供 node 回调 /msg。
func (h *NodeHost) MsgPort() int {
	return localproxy.Port()
}

// LoadPackage 下载并启动源包，返回 GET /config 的 JSON 正文。
func (h *NodeHost) LoadPackage(pkgURL string) (string, error) {
	pkgURL = strings.TrimSpace(pkgURL)
	if pkgURL == "" {
		return "", fmt.Errorf("空的 Node 源包地址")
	}
	if _, err := appruntime.RequireNode(); err != nil {
		return "", err
	}
	pkg, err := h.Ensure(pkgURL)
	if err != nil {
		return "", err
	}
	return pkg.FetchConfig()
}

// Ensure 确保源包已下载并有进程在跑。
func (h *NodeHost) Ensure(pkgURL string) (*nodePkg, error) {
	id := nodePkgID(pkgURL)
	h.mu.Lock()
	pkg, ok := h.pkgs[id]
	if !ok {
		dir := filepath.Join(paths.NodeCache(), id)
		pkg = &nodePkg{
			id:      id,
			pkgURL:  pkgURL,
			dir:     dir,
			dataDir: filepath.Join(dir, "data"),
			indexJS: filepath.Join(dir, "index.js"),
			configJS: filepath.Join(dir, "index.config.js"),
			backoff: nodeRestartBackoff,
		}
		h.pkgs[id] = pkg
	} else {
		pkg.pkgURL = pkgURL
	}
	h.mu.Unlock()

	if err := os.MkdirAll(pkg.dataDir, 0o755); err != nil {
		return nil, err
	}
	if err := h.ensureRunner(); err != nil {
		return nil, err
	}
	if err := pkg.download(pkgURL); err != nil {
		return nil, err
	}
	if err := pkg.start(h); err != nil {
		return nil, err
	}
	return pkg, nil
}

func (h *NodeHost) ensureRunner() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	dest := filepath.Join(paths.NodeCache(), "_kotv_noderunner.js")
	cur, _ := os.ReadFile(dest)
	if !bytes.Equal(cur, []byte(nodeRunnerSrc)) {
		if err := os.WriteFile(dest, []byte(nodeRunnerSrc), 0o644); err != nil {
			return err
		}
	}
	h.runner = dest
	return nil
}

func nodePkgID(pkgURL string) string {
	u, err := url.Parse(strings.TrimSpace(pkgURL))
	if err == nil {
		u.User = nil
		u.Fragment = ""
		// 统一成 index.js 形态，避免 .md5 与正式地址分成两个包。
		p := strings.ToLower(u.Path)
		if strings.HasSuffix(p, ".md5") {
			u.Path = u.Path[:len(u.Path)-4]
		}
		pkgURL = u.String()
	}
	sum := md5.Sum([]byte(pkgURL))
	return hex.EncodeToString(sum[:])[:12]
}

func (p *nodePkg) download(pkgURL string) error {
	base := util.NodePackageBaseURL(pkgURL)
	files := []struct {
		name string
		dest string
	}{
		{"index.js", p.indexJS},
		{"index.config.js", p.configJS},
	}
	for _, f := range files {
		fileURL := strings.TrimRight(base, "/") + "/" + f.name
		md5URL := fileURL + ".md5"
		want, err := fetchNodeText(md5URL, pkgURL)
		if err != nil {
			// index.config.js.md5 可选：没有配置文件时跳过。
			if f.name == "index.config.js" {
				log.Printf("node pkg %s: 无 %s，跳过", p.id, f.name)
				_ = os.Remove(f.dest)
				continue
			}
			return fmt.Errorf("下载 %s.md5 失败: %w", f.name, err)
		}
		want = strings.TrimSpace(strings.ToLower(want))
		if fields := strings.Fields(want); len(fields) > 0 {
			want = fields[0]
		}
		if len(want) != 32 {
			return fmt.Errorf("%s.md5 无效: %q", f.name, want)
		}
		if st, err := os.Stat(f.dest); err == nil && st.Size() > 0 {
			if sum := fileMD5(f.dest); sum == want {
				continue
			}
		}
		body, err := fetchNodeBytes(fileURL, pkgURL)
		if err != nil {
			return fmt.Errorf("下载 %s 失败: %w", f.name, err)
		}
		sum := md5.Sum(body)
		got := hex.EncodeToString(sum[:])
		if got != want {
			return fmt.Errorf("%s md5 不匹配: got %s want %s", f.name, got, want)
		}
		if err := writeFileAtomic(f.dest, body); err != nil {
			return err
		}
	}
	return nil
}

// fetchNodeText/Bytes：仅对原始源包主机带 Basic；跨主机 302 剥 userinfo。
func fetchNodeText(rawURL, authURL string) (string, error) {
	b, err := fetchNodeBytes(rawURL, authURL)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func fetchNodeBytes(rawURL, authURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", util.OkHttpUA)
	applyNodeAuth(req, authURL)

	client := &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			orig := via[0].URL.Hostname()
			if req.URL.Hostname() != orig {
				req.URL.User = nil
				req.Header.Del("Authorization")
			} else {
				applyNodeAuth(req, authURL)
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return body, nil
}

func applyNodeAuth(req *http.Request, authURL string) {
	if req == nil || authURL == "" {
		return
	}
	u, err := url.Parse(authURL)
	if err != nil || u.User == nil {
		return
	}
	if req.URL != nil && req.URL.Hostname() != u.Hostname() {
		return
	}
	userInfo := u.User.String()
	if userInfo == "" {
		return
	}
	req.Header.Set("Authorization", util.BasicAuthHeader(userInfo))
}

func (p *nodePkg) start(h *NodeHost) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil && p.cmd.ProcessState == nil && p.port > 0 {
		return nil
	}
	return p.startLocked(h)
}

func (p *nodePkg) startLocked(h *NodeHost) error {
	p.stopLocked()
	nodeBin, err := appruntime.RequireNode()
	if err != nil {
		return err
	}
	freePort, err := pickFreeLocalPort()
	if err != nil {
		return err
	}
	args := []string{h.runner, p.indexJS}
	if st, err := os.Stat(p.configJS); err == nil && st.Size() > 0 {
		args = append(args, p.configJS)
	}
	cmd := exec.Command(nodeBin, args...)
	cmd.Dir = p.dir
	cmd.Env = appruntime.WithNativeLibDirEnv(append(os.Environ(),
		"HOST=127.0.0.1",
		fmt.Sprintf("DEV_HTTP_PORT=%d", freePort),
		"NODE_PATH="+p.dataDir,
		fmt.Sprintf("KOTV_MSG_PORT=%d", h.MsgPort()),
	), nodeBin)
	setChildProcAttrs(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd
	p.stopping = false

	portCh := make(chan int, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "NODE_PORT=") {
				var port int
				if _, err := fmt.Sscanf(line, "NODE_PORT=%d", &port); err == nil && port > 0 {
					select {
					case portCh <- port:
					default:
					}
				}
			} else if line != "" {
				log.Printf("[node:%s] %s", p.id, line)
			}
		}
	}()
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if line := strings.TrimSpace(sc.Text()); line != "" {
				log.Printf("[node:%s] %s", p.id, line)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), nodeStartTimeout)
	defer cancel()
	select {
	case port := <-portCh:
		p.port = port
		p.backoff = nodeRestartBackoff
		log.Printf("node pkg %s listening on 127.0.0.1:%d (%s)", p.id, port, util.MaskURLCredentials(p.pkgURL))
		go p.watch(h)
		return nil
	case <-ctx.Done():
		p.stopLocked()
		return fmt.Errorf("等待 NODE_PORT 超时")
	}
}

func (p *nodePkg) watch(h *NodeHost) {
	cmd := p.cmd
	if cmd == nil {
		return
	}
	err := cmd.Wait()
	p.mu.Lock()
	same := p.cmd == cmd
	stopping := p.stopping
	p.mu.Unlock()
	if !same || stopping {
		return
	}
	log.Printf("node pkg %s 进程退出: %v，将重启", p.id, err)
	p.mu.Lock()
	backoff := p.backoff
	if backoff < nodeRestartBackoff {
		backoff = nodeRestartBackoff
	}
	p.backoff = backoff * 2
	if p.backoff > nodeMaxBackoff {
		p.backoff = nodeMaxBackoff
	}
	p.cmd = nil
	p.port = 0
	p.mu.Unlock()
	time.Sleep(backoff)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopping {
		return
	}
	if err := p.startLocked(h); err != nil {
		log.Printf("node pkg %s 重启失败: %v", p.id, err)
	}
}

func (p *nodePkg) stopLocked() {
	p.stopping = true
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if p.cmd != nil && p.cmd.Process != nil {
		killProcessTree(p.cmd.Process)
	}
	p.cmd = nil
	p.port = 0
}

// Stop 停止单个源包进程。
func (p *nodePkg) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

// Stop 按 id 停止。
func (h *NodeHost) Stop(pkgID string) {
	h.mu.Lock()
	pkg := h.pkgs[pkgID]
	h.mu.Unlock()
	if pkg != nil {
		pkg.Stop()
	}
}

// StopAll 停止全部 Node 源包。
func (h *NodeHost) StopAll() {
	h.mu.Lock()
	pkgs := make([]*nodePkg, 0, len(h.pkgs))
	for _, p := range h.pkgs {
		pkgs = append(pkgs, p)
	}
	h.mu.Unlock()
	for _, p := range pkgs {
		p.Stop()
	}
}

// ClearAll 停进程并清空缓存目录与索引。
func (h *NodeHost) ClearAll() {
	h.StopAll()
	h.mu.Lock()
	h.pkgs = map[string]*nodePkg{}
	h.apiIndex = map[string]string{}
	h.mu.Unlock()
	dir := paths.NodeCache()
	_ = os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0o755)
}

// RegisterPackageAPIs 把 /config 里 video.sites 的 api 全部挂到该包。
func (h *NodeHost) RegisterPackageAPIs(pkgID string, configJSON string) {
	var root struct {
		Sites []struct {
			API string `json:"api"`
		} `json:"sites"`
		Video *struct {
			Sites []struct {
				API string `json:"api"`
			} `json:"sites"`
		} `json:"video"`
	}
	if err := json.Unmarshal([]byte(configJSON), &root); err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	add := func(api string) {
		api = strings.TrimSpace(api)
		if strings.HasPrefix(api, "/spider/") {
			h.apiIndex[api] = pkgID
		}
	}
	for _, s := range root.Sites {
		add(s.API)
	}
	if root.Video != nil {
		for _, s := range root.Video.Sites {
			add(s.API)
		}
	}
}

// KnowsAPI 该 api 是否已登记为 Node 站。
func (h *NodeHost) KnowsAPI(api string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.apiIndex[strings.TrimSpace(api)]
	return ok
}

// PkgIDForAPI 返回站点对应的源包 id。
func (h *NodeHost) PkgIDForAPI(api string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.apiIndex[strings.TrimSpace(api)]
}

// GetPkg 按 id 取包。
func (h *NodeHost) GetPkg(id string) *nodePkg {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pkgs[id]
}

// ActivePkg 返回任一已起端口的源包；没有则 nil。
// 配置中心页内 fetch('/website/…') 打引擎根路径时，用它反代到当前 Node。
func (h *NodeHost) ActivePkg() *nodePkg {
	h.mu.Lock()
	defer h.mu.Unlock()
	var pkg *nodePkg
	for _, p := range h.pkgs {
		if p != nil && p.Port() > 0 {
			pkg = p
		}
	}
	return pkg
}

// ProxyWebsite 把 /website… 原样反代到 ActivePkg；无就绪包返回 false。
func (h *NodeHost) ProxyWebsite(w http.ResponseWriter, r *http.Request) bool {
	pkg := h.ActivePkg()
	if pkg == nil {
		return false
	}
	path := r.URL.Path
	if path == "" {
		path = "/website"
	}
	pkg.Proxy(w, r, path)
	return true
}

// ReleaseOther 切站时保留 keepAPI 所属包，停掉其它。
func (h *NodeHost) ReleaseOther(keepAPI string) {
	keepID := ""
	if strings.HasPrefix(strings.TrimSpace(keepAPI), "/spider/") {
		keepID = h.PkgIDForAPI(keepAPI)
	}
	h.mu.Lock()
	pkgs := make([]*nodePkg, 0, len(h.pkgs))
	for id, p := range h.pkgs {
		if keepID != "" && id == keepID {
			continue
		}
		pkgs = append(pkgs, p)
	}
	h.mu.Unlock()
	for _, p := range pkgs {
		p.Stop()
	}
}

func (p *nodePkg) baseURL() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.port <= 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", p.port)
}

func (p *nodePkg) Port() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.port
}

// FetchConfig GET /config。
func (p *nodePkg) FetchConfig() (string, error) {
	base := p.baseURL()
	if base == "" {
		return "", fmt.Errorf("node 未就绪")
	}
	client := &http.Client{Timeout: nodeConfigTimeout}
	resp, err := client.Get(base + "/config")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("GET /config http %d", resp.StatusCode)
	}
	DefaultNodeHost().RegisterPackageAPIs(p.id, string(b))
	return string(b), nil
}

// PostJSON POST 路径，返回响应正文。
func (p *nodePkg) PostJSON(path string, body any) (string, error) {
	base := p.baseURL()
	if base == "" {
		return "", fmt.Errorf("node 未就绪")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: nodeCallTimeout}
	resp, err := client.Post(base+path, "application/json; charset=utf-8", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("POST %s http %d: %s", path, resp.StatusCode, truncatePyLog(string(b)))
	}
	out := strings.TrimSpace(string(b))
	if out == "" {
		return "{}", nil
	}
	return out, nil
}

// Proxy 反代到 node 进程。path 为去掉 /node/<pkg> 后的路径。
func (p *nodePkg) Proxy(w http.ResponseWriter, r *http.Request, path string) {
	base := p.baseURL()
	if base == "" {
		http.Error(w, "node 未就绪", http.StatusBadGateway)
		return
	}
	target, err := url.Parse(base)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("node proxy %s: %v", p.id, err)
		http.Error(rw, "node 反代失败", http.StatusBadGateway)
	}
	orig := r.URL.Path
	r.URL.Path = path
	if r.URL.RawPath != "" {
		r.URL.RawPath = path
	}
	r.Host = target.Host
	proxy.ServeHTTP(w, r)
	r.URL.Path = orig
}

var (
	reLocalNodeURL = regexp.MustCompile(`(?i)https?://(?:127\.0\.0\.1|localhost):(\d+)`)
	reJS2P         = regexp.MustCompile(`(?i)js2p://_WEB_`)
)

// nodeProxyBase 本包在引擎上的 /node/<pkg> 根。
// PublicBase 空时为 127.0.0.1；远端则为客户端可达根（含媒体钥匙路径）。
func (p *nodePkg) nodeProxyBase() string {
	loop := fmt.Sprintf("http://127.0.0.1:%d/node/%s", localproxy.Port(), p.id)
	return playproxy.PublicizeURL(loop)
}

// rewriteToNodeProxy 把本包回环地址与 js2p://_WEB_ 换成 nodeProxyBase（播放 JSON / 弹幕 / OPENWEB 共用）。
func (p *nodePkg) rewriteToNodeProxy(raw string) string {
	if raw == "" {
		return raw
	}
	engine := p.nodeProxyBase()
	port := p.Port()
	if port > 0 {
		ps := fmt.Sprintf("%d", port)
		raw = strings.ReplaceAll(raw, "http://127.0.0.1:"+ps, engine)
		raw = strings.ReplaceAll(raw, "http://localhost:"+ps, engine)
		raw = reLocalNodeURL.ReplaceAllStringFunc(raw, func(m string) string {
			sub := reLocalNodeURL.FindStringSubmatch(m)
			if len(sub) == 2 && sub[1] == ps {
				return engine
			}
			return m
		})
	}
	return reJS2P.ReplaceAllString(raw, engine)
}

// RewritePlayJSON 把播放 JSON 里的本机 node 地址与 js2p://_WEB_ 改成引擎 /node/<pkg>/…。
func (p *nodePkg) RewritePlayJSON(raw string) string {
	return p.rewriteToNodeProxy(raw)
}

// RewriteNodeURL 把指向本包 node 端口的 URL 改成远端客户端可达的 /node/<pkg>/…。
func (p *nodePkg) RewriteNodeURL(raw string) string {
	return p.rewriteToNodeProxy(strings.TrimSpace(raw))
}

// profilePath 源包 profile.json。
func (p *nodePkg) profilePath() string {
	return filepath.Join(p.dataDir, "profile.json")
}

func (p *nodePkg) queryProfile() any {
	b, err := os.ReadFile(p.profilePath())
	if err != nil || len(b) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	return v
}

func (p *nodePkg) saveProfile(profile any) error {
	b, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return writeFileAtomic(p.profilePath(), b)
}