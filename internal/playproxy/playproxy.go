package playproxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/localproxy"
	"github.com/bobo/KOTV/internal/m3u8"
	"github.com/bobo/KOTV/internal/settings"
	"github.com/bobo/KOTV/internal/util"
)

// 将带自定义 Header 的远端媒体转成本地可播地址，供 VLC/MPV 无 header 能力时使用。

type entry struct {
	URL       string
	Headers   map[string]string
	ExpiresAt time.Time
	Filter    bool
}

var (
	mu      sync.Mutex
	entries = map[string]entry{}
	portFn  func() int
	// rxBytes 代理从远端拉到的累计字节（供缓冲浮层测速，与播放器解耦）。
	rxBytes atomic.Uint64
)

// RxBytes 返回代理累计下行字节。
func RxBytes() uint64 { return rxBytes.Load() }

// SetPortFunc 注入本地服务端口读取函数。
func SetPortFunc(fn func() int) { portFn = fn }

// RegisterAlways 登记 /proxy/play。filter 为真时，这条透传才过滤节目单。
func RegisterAlways(rawURL string, headers map[string]string, filter bool) string {
	if strings.TrimSpace(rawURL) == "" {
		return ""
	}
	return register(rawURL, headers, filter)
}

// NeedsLocalRelay 回环地址是否标了非引擎端口。这种端口只在跑引擎的机器上听得到。
func NeedsLocalRelay(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || !util.IsLoopbackHost(u.Hostname()) {
		return false
	}
	p := u.Port()
	if p == "" {
		return false
	}
	return p != strconv.Itoa(enginePort())
}

func enginePort() int {
	return localproxy.PortOrDefault(portFn)
}

func register(rawURL string, headers map[string]string, filter bool) string {
	if rawURL == "" {
		return ""
	}
	if strings.Contains(rawURL, "/proxy/play?") || strings.Contains(rawURL, "/proxy/cached_m3u8") || strings.Contains(rawURL, "/proxy/rolling_m3u8") {
		return rawURL
	}
	id := newID()
	cp := make(map[string]string, len(headers))
	for k, v := range headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		cp[k] = v
	}
	now := time.Now()
	mu.Lock()
	for k, e := range entries {
		if now.After(e.ExpiresAt) {
			delete(entries, k)
		}
	}
	trimEntriesHalfLocked(entriesMax)
	entries[id] = entry{URL: rawURL, Headers: cp, ExpiresAt: now.Add(6 * time.Hour), Filter: filter}
	mu.Unlock()
	return fmt.Sprintf("%s/proxy/play?id=%s", LocalHTTPBase(), id)
}

const entriesMax = 256

// trimEntriesHalfLocked 达上限时删一半，并 kill 对应 splice（调用方已持 mu）。
func trimEntriesHalfLocked(max int) {
	if len(entries) < max {
		return
	}
	n := len(entries) / 2
	killed := make([]string, 0, n)
	for k := range entries {
		if n <= 0 {
			break
		}
		delete(entries, k)
		killed = append(killed, k)
		n--
	}
	for _, id := range killed {
		killSpliceByKey(id)
	}
}

// Clear 清空播放代理登记表与活跃 splice 槽（清理缓存工具）。
func Clear() {
	spliceMu.Lock()
	holds := make([]*spliceHold, 0, len(spliceLive))
	for _, h := range spliceLive {
		holds = append(holds, h)
	}
	spliceLive = map[string]*spliceHold{}
	spliceMu.Unlock()
	for _, h := range holds {
		if h != nil {
			h.kill()
		}
	}
	mu.Lock()
	entries = map[string]entry{}
	mu.Unlock()
}

// LocalHTTPBase 本机默认 127.0.0.1:port；远端请求则用 hostclient.PublicBase。
func LocalHTTPBase() string {
	if b := hostclient.PublicBase(); b != "" {
		return b
	}
	if p := enginePort(); p > 0 && p != localproxy.DefaultPort {
		return fmt.Sprintf("http://127.0.0.1:%d", p)
	}
	return localproxy.LocalBaseURL()
}

// PublicizeURL 把地址改成当前请求的客户端打得开的形式。
// 回环且端口不是本引擎时，由本机代拉后给出已有的 /proxy/play。
// 本引擎路径上的回环或内网主机换成前端填写的连接根。其它地址保持不变。
// 本机请求（PublicBase 为空）原样返回。headers 可选，只在代拉时附上。
func PublicizeURL(raw string, headers ...map[string]string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if hostclient.PublicBase() != "" && NeedsLocalRelay(raw) {
		var hdr map[string]string
		if len(headers) > 0 {
			hdr = headers[0]
		}
		raw = register(raw, hdr, false)
	}
	return rewritePublicHost(raw)
}

func rewritePublicHost(raw string) string {
	raw = strings.TrimSpace(raw)
	base := hostclient.PublicBase()
	if raw == "" || base == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || !IsEngineRoute(u.Path) {
		return raw
	}
	pb, err := url.Parse(base)
	if err != nil || pb.Host == "" {
		return raw
	}
	if !strings.EqualFold(u.Hostname(), pb.Hostname()) && !isRewriteableProxyHost(u.Hostname()) {
		return raw
	}
	prefix := ""
	if IsMediaPath(u.Path) {
		prefix = strings.TrimRight(pb.Path, "/")
	}
	if u.Scheme == pb.Scheme && u.Host == pb.Host && prefix == "" {
		return raw
	}
	u.Scheme, u.Host = pb.Scheme, pb.Host
	if prefix != "" {
		u.Path = prefix + u.Path
		if u.RawPath != "" {
			u.RawPath = prefix + u.RawPath
		}
	}
	return u.String()
}

func isRewriteableProxyHost(host string) bool {
	if strings.TrimSpace(host) == "" || util.IsLoopbackHost(host) {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

var engineRoutes atomic.Pointer[[]string]

// SetEngineRoutes 记下引擎自己处理的路由（server 注册路由时调用）。以 / 结尾的是前缀；nil 表示清空。
func SetEngineRoutes(patterns []string) {
	if patterns == nil {
		engineRoutes.Store(nil)
		return
	}
	engineRoutes.Store(&patterns)
}

// IsEngineRoute 引擎自己处理的路径（静态页面以外）。路由尚未注册时只认媒体地址。
func IsEngineRoute(path string) bool {
	routes := engineRoutes.Load()
	if routes == nil {
		return IsMediaPath(path)
	}
	for _, p := range *routes {
		if path == p || (strings.HasSuffix(p, "/") && strings.HasPrefix(path, p)) {
			return true
		}
	}
	return false
}

// MediaKeyPrefix 媒体钥匙在地址里的路径前缀：/k/<钥匙>/proxy/…
const MediaKeyPrefix = "/k/"

// IsMediaPath 播放器、图片加载直接请求的媒体地址；凭媒体钥匙可以只读访问。
func IsMediaPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	return path == "/proxy" || strings.HasPrefix(path, "/proxy/") ||
		strings.HasPrefix(path, "/file/") || strings.HasPrefix(path, "/image/") || path == "/tvbus"
}

// SplitMediaKey 拆开 /k/<钥匙>/<路径>；不是这种形式时 ok 为 false。
func SplitMediaKey(path string) (key, rest string, ok bool) {
	if !strings.HasPrefix(path, MediaKeyPrefix) {
		return "", "", false
	}
	key, rest, found := strings.Cut(strings.TrimPrefix(path, MediaKeyPrefix), "/")
	if !found || key == "" {
		return "", "", false
	}
	return key, "/" + rest, true
}

// WithMediaKey 给交给其它设备（电视、投屏接收端）的本引擎媒体地址加上媒体钥匙；
// 本机回环地址、已带钥匙或不是媒体的地址原样返回。
func WithMediaKey(raw, key string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || key == "" || util.IsLoopbackHost(u.Hostname()) || !IsMediaPath(u.Path) {
		return raw
	}
	u.Path = MediaKeyPrefix + key + u.Path
	if u.RawPath != "" {
		u.RawPath = MediaKeyPrefix + key + u.RawPath
	}
	return u.String()
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func lookup(id string) (entry, bool) {
	mu.Lock()
	defer mu.Unlock()
	e, ok := entries[id]
	if !ok {
		return entry{}, false
	}
	if time.Now().After(e.ExpiresAt) {
		delete(entries, id)
		return entry{}, false
	}
	return e, true
}

// Resolve 若为本地 /proxy/play 地址，返回原始 URL 与登记 Headers；否则原样返回。
func Resolve(proxyOrRaw string) (rawURL string, headers map[string]string) {
	proxyOrRaw = strings.TrimSpace(proxyOrRaw)
	if proxyOrRaw == "" {
		return "", nil
	}
	id := playID(proxyOrRaw)
	if id == "" {
		return proxyOrRaw, nil
	}
	e, ok := lookup(id)
	if !ok || e.URL == "" {
		return proxyOrRaw, nil
	}
	cp := make(map[string]string, len(e.Headers))
	for k, v := range e.Headers {
		cp[k] = v
	}
	return e.URL, cp
}

// RegisteredFilter 这条透传登记是否要过滤节目单。不是 /proxy/play 时为假。
func RegisteredFilter(proxyOrRaw string) bool {
	id := playID(proxyOrRaw)
	if id == "" {
		return false
	}
	e, ok := lookup(id)
	return ok && e.Filter
}

func playID(proxyOrRaw string) string {
	proxyOrRaw = strings.TrimSpace(proxyOrRaw)
	if proxyOrRaw == "" || !strings.Contains(proxyOrRaw, "/proxy/play?") {
		return ""
	}
	id := ""
	if i := strings.Index(proxyOrRaw, "id="); i >= 0 {
		id = proxyOrRaw[i+3:]
		if j := strings.IndexAny(id, "&?#"); j >= 0 {
			id = id[:j]
		}
	}
	return id
}

// Handle 代理拉流并转发 Range，注入登记的请求头。
// 节目单里的切片用同一 id，另带 u 指向绝对地址，请求头仍用这条登记。
func Handle(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	e, ok := lookup(id)
	if !ok || e.URL == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if target := strings.TrimSpace(r.URL.Query().Get("u")); target != "" {
		abs, ok := allowedChild(e.URL, target)
		if !ok {
			http.Error(w, "bad url", http.StatusBadRequest)
			return
		}
		e.URL = abs
	}
	rangeHdr := strings.TrimSpace(r.Header.Get("Range"))
	serveEntry(w, r, e, rangeHdr, id)
}

// 播放器发下一次 Range 时，上一次连到爬虫的 TCP 往往还没关。
// 先关掉上一次，再拨这一次，爬虫才能继续送数据。
var (
	spliceMu   sync.Mutex
	spliceLive = map[string]*spliceHold{}
)

type spliceHold struct {
	key    string
	ctx    context.Context
	cancel context.CancelFunc

	mu   sync.Mutex
	conn net.Conn
	dead bool
}

func beginSplice(key string) *spliceHold {
	ctx, cancel := context.WithCancel(context.Background())
	slot := &spliceHold{key: key, ctx: ctx, cancel: cancel}
	spliceMu.Lock()
	prev := spliceLive[key]
	spliceLive[key] = slot
	spliceMu.Unlock()
	if prev != nil {
		prev.kill()
	}
	return slot
}

// killSpliceByKey 半裁登记表时结束对应活跃 splice。子请求的键是 id|地址。
func killSpliceByKey(key string) {
	if key == "" {
		return
	}
	prefix := key + "|"
	spliceMu.Lock()
	holds := make([]*spliceHold, 0, 1)
	for k, h := range spliceLive {
		if k == key || strings.HasPrefix(k, prefix) {
			delete(spliceLive, k)
			if h != nil {
				holds = append(holds, h)
			}
		}
	}
	spliceMu.Unlock()
	for _, h := range holds {
		h.kill()
	}
}

func (s *spliceHold) kill() {
	s.mu.Lock()
	s.dead = true
	cancel := s.cancel
	c := s.conn
	s.conn = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if c != nil {
		_ = c.Close()
	}
}

func (s *spliceHold) bind(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return false
	}
	s.conn = c
	return true
}

func (s *spliceHold) takeConn() net.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.conn
	s.conn = nil
	return c
}

func (s *spliceHold) alive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.dead
}

func (s *spliceHold) release() {
	s.kill()
	spliceMu.Lock()
	if spliceLive[s.key] == s {
		delete(spliceLive, s.key)
	}
	spliceMu.Unlock()
}

func serveEntry(w http.ResponseWriter, r *http.Request, e entry, rangeHdr, playID string) {
	// 回环播放口按原始 TCP 转发状态行、响应头和正文。
	// 本机播放器直连时走的就是这条连接，不再经 http.Client 重封装。
	if tryRawSplice(w, r, e, rangeHdr, playID) {
		return
	}
	resp, err := e.fetch(r, rangeHdr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	prefix := make([]byte, len("#EXTM3U"))
	n, _ := io.ReadFull(resp.Body, prefix)
	head := prefix[:n]
	if !bytes.Equal(head, []byte("#EXTM3U")) {
		writePassthrough(w, resp, io.MultiReader(bytes.NewReader(head), resp.Body))
		return
	}
	rest, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	raw := append(append([]byte{}, head...), rest...)
	final := e.URL
	if resp.Request != nil && resp.Request.URL != nil && resp.Request.URL.String() != "" {
		final = resp.Request.URL.String()
	}
	body := string(raw)
	if e.Filter {
		body = m3u8.FilterPlaylist(body, final, e.Headers)
	}
	body = rewritePlaylist(body, final, playID, hasHeaders(e.Headers))
	writePlaylist(w, resp, []byte(body))
}

func hasHeaders(headers map[string]string) bool {
	for k, v := range headers {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// playlistURIRe 匹配标签行里的 URI="…"。
var playlistURIRe = regexp.MustCompile(`(?i)\bURI="([^"]+)"`)

// rewritePlaylist 把节目单里的地址补成绝对地址。
// 需要带上登记的请求头，或切片本身在回环上时，改写成同一条透传。
func rewritePlaylist(content, base, id string, inject bool) string {
	if !strings.HasPrefix(strings.TrimSpace(content), "#EXTM3U") {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		lines[i] = playlistRef(base, trim, id, inject)
	}
	text := strings.Join(lines, "\n")
	if !strings.Contains(strings.ToUpper(text), "URI=") {
		return text
	}
	return playlistURIRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := playlistURIRe.FindStringSubmatch(m)
		if len(sub) < 2 || sub[1] == "" || strings.HasPrefix(sub[1], "data:") {
			return m
		}
		return `URI="` + playlistRef(base, sub[1], id, inject) + `"`
	})
}

func playlistRef(base, ref, id string, inject bool) string {
	abs := util.AbsHTTPRef(base, ref)
	if strings.Contains(abs, "/proxy/play?") {
		return abs
	}
	if id == "" || !mustProxyChild(abs, inject) {
		return abs
	}
	return proxyChild(id, abs)
}

func mustProxyChild(abs string, inject bool) bool {
	if !strings.HasPrefix(abs, "http://") && !strings.HasPrefix(abs, "https://") {
		return false
	}
	if inject {
		return true
	}
	u, err := url.Parse(abs)
	if err != nil {
		return false
	}
	return util.IsLoopbackHost(u.Hostname())
}

func proxyChild(id, abs string) string {
	u, err := url.Parse(LocalHTTPBase() + "/proxy/play")
	if err != nil {
		return abs
	}
	q := u.Query()
	q.Set("id", id)
	q.Set("u", abs)
	u.RawQuery = q.Encode()
	return u.String()
}

// allowedChild 只允许 http(s)。公网登记不能再被节目单指去回环或内网。
func allowedChild(parent, target string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	if util.IsLoopbackHost(u.Hostname()) || isPrivateIP(u.Hostname()) {
		p, err := url.Parse(parent)
		if err != nil || p.Host == "" || (!util.IsLoopbackHost(p.Hostname()) && !isPrivateIP(p.Hostname())) {
			return "", false
		}
	}
	return u.String(), true
}

func isPrivateIP(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}

func writePlaylist(w http.ResponseWriter, resp *http.Response, body []byte) {
	copyResponseHeaders(w, resp.Header)
	w.Header().Del("Content-Length")
	w.Header().Del("Content-Range")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func writePassthrough(w http.ResponseWriter, resp *http.Response, body io.Reader) {
	copyResponseHeaders(w, resp.Header)
	w.WriteHeader(resp.StatusCode)
	dst := io.Writer(w)
	if f, ok := w.(http.Flusher); ok {
		dst = flushWriter{w: w, f: f}
	}
	_, _ = io.Copy(countingWriter{w: dst}, body)
}

// copyResponseHeaders 原样转发响应头。去掉逐跳头，不补 Accept-Ranges。
func copyResponseHeaders(w http.ResponseWriter, h http.Header) {
	hop := map[string]struct{}{
		"connection":          {},
		"keep-alive":          {},
		"proxy-authenticate":  {},
		"proxy-authorization": {},
		"te":                  {},
		"trailer":             {},
		"transfer-encoding":   {},
		"upgrade":             {},
	}
	for _, c := range h.Values("Connection") {
		for _, token := range strings.Split(c, ",") {
			if t := strings.ToLower(strings.TrimSpace(token)); t != "" {
				hop[t] = struct{}{}
			}
		}
	}
	for k, vals := range h {
		if _, skip := hop[strings.ToLower(k)]; skip {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if n > 0 && fw.f != nil {
		fw.f.Flush()
	}
	return n, err
}

// errUseHTTPClient 表示这条地址要走 TLS 或跨出回环，改由 http.Client 处理。
var errUseHTTPClient = errors.New("use http client")

type rawHead struct {
	statusLine string
	statusCode int
	header     http.Header
}

func tryRawSplice(w http.ResponseWriter, r *http.Request, e entry, rangeHdr, playID string) bool {
	if !canRawSplice(e.URL) {
		return false
	}
	if _, ok := w.(http.Hijacker); !ok {
		return false
	}
	key := playID
	if key == "" {
		key = e.URL
	} else {
		key = playID + "|" + e.URL
	}
	slot := beginSplice(key)
	defer slot.release()
	br, head, peek, _, err := openRawUpstream(slot, e, rangeHdr, clientSpliceUA(r, e.Headers), r.Header.Get("If-Range"), r.Header.Get("Connection"), r.Header.Get("Accept-Encoding"))
	if errors.Is(err, errUseHTTPClient) {
		return false
	}
	if err != nil {
		if !slot.alive() {
			return true
		}
		http.Error(w, "连接本机播放端口失败", http.StatusBadGateway)
		return true
	}
	if bytes.Equal(peek, []byte("#EXTM3U")) {
		return false
	}
	spliceToClient(w, slot, br, head, peek)
	return true
}

func canRawSplice(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return false
	}
	return NeedsLocalRelay(raw)
}

// clientSpliceUA 登记里已有 UA 时返回空，调用方改用登记值。
func clientSpliceUA(r *http.Request, headers map[string]string) string {
	if r == nil {
		return ""
	}
	for k := range headers {
		if strings.EqualFold(k, "user-agent") && strings.TrimSpace(headers[k]) != "" {
			return ""
		}
	}
	return r.Header.Get("User-Agent")
}

func openRawUpstream(slot *spliceHold, e entry, rangeHdr, ua, ifRange, connectionHdr, acceptEncoding string) (*bufio.Reader, rawHead, []byte, string, error) {
	current := e.URL
	for hop := 0; hop < 10; hop++ {
		if !slot.alive() {
			return nil, rawHead{}, nil, "", context.Canceled
		}
		u, err := url.Parse(current)
		if err != nil || u.Scheme != "http" || u.Host == "" {
			return nil, rawHead{}, nil, "", errUseHTTPClient
		}
		conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(slot.ctx, "tcp", u.Host)
		if err != nil {
			return nil, rawHead{}, nil, "", err
		}
		tcpNoDelay(conn)
		if !slot.bind(conn) {
			conn.Close()
			return nil, rawHead{}, nil, "", context.Canceled
		}
		if werr := writeRawRequest(conn, u, e.Headers, rangeHdr, ua, ifRange, connectionHdr, acceptEncoding); werr != nil {
			_ = slot.takeConn()
			conn.Close()
			return nil, rawHead{}, nil, "", werr
		}
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		br := bufio.NewReaderSize(conn, 64*1024)
		head, err := readRawHead(br)
		if err != nil {
			_ = slot.takeConn()
			conn.Close()
			return nil, rawHead{}, nil, "", err
		}
		if head.statusCode >= 300 && head.statusCode < 400 {
			loc := strings.TrimSpace(head.header.Get("Location"))
			if c := slot.takeConn(); c != nil {
				c.Close()
			}
			if loc == "" {
				return nil, rawHead{}, nil, "", fmt.Errorf("重定向缺少 Location")
			}
			current = util.AbsHTTPRef(current, loc)
			continue
		}
		want := len("#EXTM3U")
		if cl, ok := contentLength(head.header); ok && cl < int64(want) {
			want = int(cl)
		}
		peek := make([]byte, want)
		n, rerr := io.ReadFull(br, peek)
		if rerr != nil && rerr != io.EOF && rerr != io.ErrUnexpectedEOF {
			if c := slot.takeConn(); c != nil {
				c.Close()
			}
			return nil, rawHead{}, nil, "", rerr
		}
		peek = peek[:n]
		if !bytes.Equal(peek, []byte("#EXTM3U")) {
			_ = conn.SetReadDeadline(time.Time{})
		}
		return br, head, peek, current, nil
	}
	return nil, rawHead{}, nil, "", fmt.Errorf("重定向次数过多")
}

func writeRawRequest(conn net.Conn, u *url.URL, headers map[string]string, rangeHdr, ua, ifRange, connectionHdr, acceptEncoding string) error {
	uri := u.RequestURI()
	if uri == "" {
		uri = "/"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "GET %s HTTP/1.1\r\n", uri)
	fmt.Fprintf(&b, "Host: %s\r\n", u.Host)
	for k, v := range headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		if skipUpstreamHeader(k) {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\r\n", http.CanonicalHeaderKey(k), v)
	}
	if rangeHdr == "" {
		rangeHdr = headerValue(headers, "range")
	}
	if rangeHdr != "" {
		fmt.Fprintf(&b, "Range: %s\r\n", rangeHdr)
	}
	if ua == "" {
		ua = headerValue(headers, "user-agent")
	}
	if ua != "" {
		fmt.Fprintf(&b, "User-Agent: %s\r\n", ua)
	}
	if ifRange == "" {
		ifRange = headerValue(headers, "if-range")
	}
	if ifRange != "" {
		fmt.Fprintf(&b, "If-Range: %s\r\n", ifRange)
	}
	// 播放器请求里的 Connection、Accept-Encoding 原样带上。没有就不写。
	if connectionHdr != "" {
		fmt.Fprintf(&b, "Connection: %s\r\n", connectionHdr)
	}
	if acceptEncoding != "" {
		fmt.Fprintf(&b, "Accept-Encoding: %s\r\n", acceptEncoding)
	}
	b.WriteString("\r\n")
	_, err := io.WriteString(conn, b.String())
	return err
}

func readRawHead(br *bufio.Reader) (rawHead, error) {
	var head rawHead
	var total int
	line, err := br.ReadString('\n')
	if err != nil {
		return head, err
	}
	total += len(line)
	head.statusLine = line
	if !strings.HasSuffix(head.statusLine, "\n") {
		head.statusLine += "\r\n"
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 {
		return head, fmt.Errorf("状态行无法识别")
	}
	head.statusCode, err = strconv.Atoi(fields[1])
	if err != nil {
		return head, err
	}
	head.header = make(http.Header)
	for {
		line, err = br.ReadString('\n')
		if err != nil {
			return head, err
		}
		total += len(line)
		if total > 1<<20 {
			return head, fmt.Errorf("响应头过大")
		}
		trim := strings.TrimRight(line, "\r\n")
		if trim == "" {
			return head, nil
		}
		k, v, ok := strings.Cut(trim, ":")
		if !ok {
			continue
		}
		head.header.Add(strings.TrimSpace(k), strings.TrimSpace(v))
	}
}

func spliceToClient(w http.ResponseWriter, slot *spliceHold, br *bufio.Reader, head rawHead, peek []byte) {
	hj := w.(http.Hijacker)
	client, _, err := hj.Hijack()
	if err != nil {
		slot.kill()
		http.Error(w, "连接本机播放端口失败", http.StatusBadGateway)
		return
	}
	tcpNoDelay(client)
	_ = client.SetDeadline(time.Time{})
	go func() {
		_, _ = io.Copy(io.Discard, client)
		slot.kill()
	}()
	defer client.Close()
	defer slot.kill()

	var buf bytes.Buffer
	buf.WriteString(head.statusLine)
	hdr := filterSpliceHeaders(head.header)
	hdr.Set("Access-Control-Allow-Origin", "*")
	_ = hdr.Write(&buf)
	buf.WriteString("\r\n")
	if _, err := client.Write(buf.Bytes()); err != nil {
		return
	}
	body := bufio.NewReader(io.MultiReader(bytes.NewReader(peek), br))
	copyBuf := make([]byte, 256*1024)
	_ = copyUpstreamBody(countingWriter{w: client}, body, head.header, copyBuf)
}

func copyUpstreamBody(dst io.Writer, br *bufio.Reader, h http.Header, buf []byte) error {
	if transferChunked(h) {
		return copyChunked(dst, br)
	}
	if n, ok := contentLength(h); ok {
		_, err := io.CopyBuffer(dst, io.LimitReader(br, n), buf)
		return err
	}
	_, err := io.CopyBuffer(dst, br, buf)
	return err
}

func transferChunked(h http.Header) bool {
	for _, v := range h.Values("Transfer-Encoding") {
		for _, p := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(p), "chunked") {
				return true
			}
		}
	}
	return false
}

func contentLength(h http.Header) (int64, bool) {
	v := strings.TrimSpace(h.Get("Content-Length"))
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func copyChunked(dst io.Writer, br *bufio.Reader) error {
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		if _, err := dst.Write([]byte(line)); err != nil {
			return err
		}
		hexPart := strings.TrimSpace(line)
		if i := strings.IndexByte(hexPart, ';'); i >= 0 {
			hexPart = strings.TrimSpace(hexPart[:i])
		}
		size, err := strconv.ParseInt(hexPart, 16, 64)
		if err != nil {
			return err
		}
		if size == 0 {
			for {
				line, err = br.ReadString('\n')
				if err != nil {
					return err
				}
				if _, err := dst.Write([]byte(line)); err != nil {
					return err
				}
				if strings.TrimRight(line, "\r\n") == "" {
					return nil
				}
			}
		}
		if _, err := io.CopyN(dst, br, size); err != nil {
			return err
		}
		crlf := make([]byte, 2)
		if _, err := io.ReadFull(br, crlf); err != nil {
			return err
		}
		if _, err := dst.Write(crlf); err != nil {
			return err
		}
	}
}

func filterSpliceHeaders(h http.Header) http.Header {
	hop := map[string]struct{}{
		"keep-alive":          {},
		"proxy-authenticate":  {},
		"proxy-authorization": {},
		"te":                  {},
		"trailer":             {},
		"upgrade":             {},
	}
	for _, c := range h.Values("Connection") {
		for _, token := range strings.Split(c, ",") {
			if t := strings.ToLower(strings.TrimSpace(token)); t != "" {
				hop[t] = struct{}{}
			}
		}
	}
	out := make(http.Header, len(h))
	for k, vals := range h {
		if _, skip := hop[strings.ToLower(k)]; skip {
			continue
		}
		for _, v := range vals {
			out.Add(k, v)
		}
	}
	return out
}

func headerValue(headers map[string]string, name string) string {
	for k, v := range headers {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func tcpNoDelay(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
}

func (e entry) fetch(r *http.Request, rangeHdr string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, e.URL, nil)
	if err != nil {
		return nil, err
	}
	applyUpstreamHeaders(req, e.Headers)
	applyPlaybackHeaders(req, r, e.Headers, rangeHdr)
	client := &http.Client{
		// 不设 Timeout：整片可能很长。关闭自动解压，避免声明 gzip 后把 Content-Length 拿掉。
		Transport: playTransport,
		// 网盘 CDN 常 302 到另一域名；默认 Client 跨域会剥 Cookie → 403 HTML →
		// 播放器报 invalid or unsupported media。
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			applyUpstreamHeaders(next, e.Headers)
			applyPlaybackHeaders(next, r, e.Headers, rangeHdr)
			return nil
		},
	}
	return client.Do(req)
}

var playTransport = newPlayTransport()

func newPlayTransport() http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{DisableCompression: true}
	}
	tr := base.Clone()
	tr.DisableCompression = true
	return tr
}

func applyUpstreamHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" || skipUpstreamHeader(k) {
			continue
		}
		req.Header.Set(k, v)
	}
}

// skipUpstreamHeader 这些头单独写。Host 用上游地址，UA、Range、If-Range 在后面按登记和这次请求补。
func skipUpstreamHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "host", "content-length", "content-type", "connection", "transfer-encoding", "accept-encoding", "range", "if-range", "user-agent":
		return true
	default:
		return false
	}
}

// applyPlaybackHeaders 把这次播放请求补到上游。
// 登记里有 UA 就用登记的；没有才用播放器这次的，再没有且目标不是回环时用设置里的播放 UA。
// Range、If-Range 这次请求有就用这次的，没有再用登记的。Accept-Encoding 只在播放器带了时写上。
func applyPlaybackHeaders(req *http.Request, r *http.Request, registered map[string]string, rangeHdr string) {
	if rangeHdr == "" {
		rangeHdr = headerValue(registered, "range")
	}
	if rangeHdr != "" {
		req.Header.Set("Range", rangeHdr)
	}
	ua := ""
	if r != nil {
		ua = clientSpliceUA(r, registered)
	}
	if ua == "" {
		ua = headerValue(registered, "user-agent")
	}
	if ua == "" && req.URL != nil && !util.IsLoopbackHost(req.URL.Hostname()) {
		ua = settings.PlayUA()
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	ifRange := ""
	if r != nil {
		ifRange = strings.TrimSpace(r.Header.Get("If-Range"))
	}
	if ifRange == "" {
		ifRange = headerValue(registered, "if-range")
	}
	if ifRange != "" {
		req.Header.Set("If-Range", ifRange)
	}
	if r == nil {
		return
	}
	if ae := strings.TrimSpace(r.Header.Get("Accept-Encoding")); ae != "" {
		req.Header.Set("Accept-Encoding", ae)
	}
	conn := strings.ToLower(r.Header.Get("Connection"))
	if strings.Contains(conn, "close") {
		req.Close = true
	}
}

type countingWriter struct {
	w io.Writer
}

func (c countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		rxBytes.Add(uint64(n))
	}
	return n, err
}
