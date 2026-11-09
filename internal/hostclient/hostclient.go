// Package hostclient 在一次 HTTP/爬虫调用的 goroutine 上绑定前端 clientId / userId / username，
// 供 JAR bridge 请求携带，并把界面消息与每用户运行时路由回正确客户端。
//
// userId：鉴权 UUID（Scope / DB owner / 独立引擎桶）。
// username：登录名（Path.tv 的 /TV/s_u_<用户名>/、下载人读子目录）。
// 脚本文件（JAR/Py/JS 下载缓存）全局共享；仅「远端租户」请求使用独立 JVM/Py/JS 引擎。
package hostclient

import (
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type binding struct {
	ClientID   string
	UserID     string
	Username   string // 远端登录名；仅 Path.tv / 下载子目录等人读路径用
	Platform   string
	PublicBase string // 非本机请求：scheme://host[:port]，供代理 URL 改写
	AuthToken  string // 登录令牌；供 Node OPENWEB 远端 URL 附带 ?token=
	Dedicated  bool   // 远端租户：独立引擎；本机/未开鉴权：共享引擎
}

var byG sync.Map // goroutine id -> binding

func goid() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := string(buf[:n])
	s = strings.TrimPrefix(s, "goroutine ")
	i := strings.IndexByte(s, ' ')
	if i <= 0 {
		return 0
	}
	id, _ := strconv.ParseUint(s[:i], 10, 64)
	return id
}

// Enter 绑定当前 goroutine 的 clientId（共享引擎）。
func Enter(id string) func() {
	return EnterSession(id, "", false)
}

// EnterSession 绑定 clientId/userId（测试/同线程嵌套）。子 goroutine 须用 Bind(Snapshot())，
// 否则 Username/PublicBase 等不会带过去，Path.tv 会误落 /TV/。
func EnterSession(clientID, userID string, dedicated bool) func() {
	clientID = strings.TrimSpace(clientID)
	userID = strings.TrimSpace(userID)
	if clientID == "" && userID == "" && !dedicated {
		return func() {}
	}
	return Bind(Request{
		ClientID:  clientID,
		UserID:    userID,
		Dedicated: dedicated,
	})
}

// Request 一次 HTTP 请求绑定到 goroutine 的身份与上下文。
type Request struct {
	ClientID   string
	UserID     string
	Username   string // 远端登录名；Path.tv 用
	Dedicated  bool   // 远端租户：独立运行时
	Platform   string // 前端平台（android/windows/…）
	PublicBase string // 非本机请求的对外根地址（可带媒体钥匙前缀），供改写代理地址；本机为空
	AuthToken  string // 登录令牌；Node OPENWEB 远端改写时写入 ?token=
}

// Bind 把 req 绑定到当前 goroutine，返回的函数恢复之前的绑定。
func Bind(req Request) func() {
	userID := strings.TrimSpace(req.UserID)
	gid := goid()
	prev, had := byG.Load(gid)
	byG.Store(gid, binding{
		ClientID:   strings.TrimSpace(req.ClientID),
		UserID:     userID,
		Username:   strings.TrimSpace(req.Username),
		Platform:   strings.ToLower(strings.TrimSpace(req.Platform)),
		PublicBase: strings.TrimRight(strings.TrimSpace(req.PublicBase), "/"),
		AuthToken:  strings.TrimSpace(req.AuthToken),
		Dedicated:  req.Dedicated && userID != "",
	})
	return func() {
		if !had {
			byG.Delete(gid)
			return
		}
		byG.Store(gid, prev)
	}
}

// Current 返回当前 goroutine 绑定的 clientId（无则空串）。
// 新代码优先用 ScopeID() 做会话隔离。
func Current() string {
	v, ok := byG.Load(goid())
	if !ok {
		return ""
	}
	if b, ok := v.(binding); ok {
		return b.ClientID
	}
	s, _ := v.(string)
	return s
}

// CurrentUserID 返回当前 goroutine 绑定的 userId（鉴权 UUID；无则空串）。
func CurrentUserID() string {
	v, ok := byG.Load(goid())
	if !ok {
		return ""
	}
	if b, ok := v.(binding); ok {
		return b.UserID
	}
	return ""
}

// CurrentUsername 返回当前 goroutine 绑定的登录名（Path.tv / 人读路径；无则空串）。
func CurrentUsername() string {
	v, ok := byG.Load(goid())
	if !ok {
		return ""
	}
	if b, ok := v.(binding); ok {
		return b.Username
	}
	return ""
}

// CurrentPlatform 当前请求前端平台。
func CurrentPlatform() string {
	v, ok := byG.Load(goid())
	if !ok {
		return ""
	}
	if b, ok := v.(binding); ok {
		return b.Platform
	}
	return ""
}

// PublicBase 当前请求的对外根地址；本机请求通常为空。
func PublicBase() string {
	v, ok := byG.Load(goid())
	if !ok {
		return ""
	}
	if b, ok := v.(binding); ok {
		return b.PublicBase
	}
	return ""
}

// AuthToken 当前请求登录令牌；本机免登录通常为空。
func AuthToken() string {
	v, ok := byG.Load(goid())
	if !ok {
		return ""
	}
	if b, ok := v.(binding); ok {
		return b.AuthToken
	}
	return ""
}

// Snapshot 当前 goroutine 绑定的完整请求上下文（无绑定时零值）。
func Snapshot() Request {
	v, ok := byG.Load(goid())
	if !ok {
		return Request{}
	}
	b, ok := v.(binding)
	if !ok {
		return Request{}
	}
	return Request{
		ClientID:   b.ClientID,
		UserID:     b.UserID,
		Username:   b.Username,
		Dedicated:  b.Dedicated,
		Platform:   b.Platform,
		PublicBase: b.PublicBase,
		AuthToken:  b.AuthToken,
	}
}

// ScopeID 会话/队列/媒体/遥控隔离键（不是站点 cookie 落盘路径）。
// 远端租户（dedicated）：u:<userId>；本机（含本机已登录/admin）只按 c:<clientId>。
// cookie 落盘：本机 /TV/，远端 /TV/s_u_<用户名>/（Util.tvScopeId / paths.TVDirName）。
func ScopeID() string {
	if DedicatedRuntime() {
		if u := CurrentUserID(); u != "" {
			return "u:" + u
		}
	}
	if c := Current(); c != "" {
		return "c:" + c
	}
	return ""
}

// ResolveScopeKey 从请求参数解析隔离键（本机免登录也全功能）。
// 优先级：userId → scopeId → clientId（含 X-Kotv-Client-Id）。
// - userId：裸值 → u:<id>（远端租户）
// - scopeId：已带 u:/c: 原样；裸值按 userId 处理
// - clientId：已带 u:/c: 原样（JAR 可能传 ScopeID）；裸值 → c:<id>（本机多窗口）
func ResolveScopeKey(userID, scopeID, clientID string) string {
	if s, ok := scopeOrPrefix(userID, "u:"); ok {
		return s
	}
	if s, ok := scopeOrPrefix(scopeID, "u:"); ok {
		return s
	}
	if s, ok := scopeOrPrefix(clientID, "c:"); ok {
		return s
	}
	return ""
}

func scopeOrPrefix(raw, barePrefix string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if strings.HasPrefix(raw, "u:") || strings.HasPrefix(raw, "c:") {
		return raw, true
	}
	return barePrefix + raw, true
}

func idFromScope(scope, prefix string) string {
	scope = strings.TrimSpace(scope)
	if strings.HasPrefix(scope, prefix) {
		return strings.TrimPrefix(scope, prefix)
	}
	return ""
}

// UserIDFromScope 从 ScopeID 取出裸 userId；非 u: 前缀返回空。
func UserIDFromScope(scope string) string {
	return idFromScope(scope, "u:")
}

// ClientIDFromScope 从 ScopeID 取出裸 clientId；非 c: 前缀返回空。
func ClientIDFromScope(scope string) string {
	return idFromScope(scope, "c:")
}

// RuntimeUserID 用于 JVM/Py/JS 引擎分桶：仅远端租户返回 userId，本机共享返回空。
func RuntimeUserID() string {
	v, ok := byG.Load(goid())
	if !ok {
		return ""
	}
	b, ok := v.(binding)
	if !ok || !b.Dedicated {
		return ""
	}
	return b.UserID
}

// DataOwner 历史/收藏数据桶：直接用当前请求已绑定的 userId。
// Web/远端登录后中间件写入 userId；本机 PC·安卓免登录则为空（共享默认桶）。
// 不是二次鉴权，只是复用登录身份做数据分桶。
func DataOwner() string {
	return strings.TrimSpace(CurrentUserID())
}

// DedicatedRuntime 当前请求是否走独立 JVM/Py/JS（远端租户）；Go 引擎始终只有一个。
func DedicatedRuntime() bool {
	return RuntimeUserID() != ""
}
