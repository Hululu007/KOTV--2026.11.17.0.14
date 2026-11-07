package spider

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
)

// NodePlayInfo 当前点播起播信息，供 getPlayInfo 回调。
type NodePlayInfo struct {
	Title       string `json:"title"`
	EpisodeName string `json:"episodeName"`
	Flag        string `json:"flag"`
}

var (
	playInfoMu       sync.RWMutex
	currentPlayInfo  NodePlayInfo
	playInfoProvider func() NodePlayInfo
)

// SetPlayInfoProvider 由 App 注册，返回当前播放 title/episodeName/flag。
func SetPlayInfoProvider(fn func() NodePlayInfo) {
	playInfoMu.Lock()
	playInfoProvider = fn
	playInfoMu.Unlock()
}

// SetCurrentPlayInfo 点播起播时记下三项。
func SetCurrentPlayInfo(title, episodeName, flag string) {
	playInfoMu.Lock()
	currentPlayInfo = NodePlayInfo{
		Title:       strings.TrimSpace(title),
		EpisodeName: strings.TrimSpace(episodeName),
		Flag:        strings.TrimSpace(flag),
	}
	playInfoMu.Unlock()
}

func currentNodePlayInfo() NodePlayInfo {
	playInfoMu.RLock()
	fn := playInfoProvider
	cached := currentPlayInfo
	playInfoMu.RUnlock()
	if fn != nil {
		if info := fn(); info.Title != "" || info.EpisodeName != "" || info.Flag != "" {
			return info
		}
	}
	return cached
}

// HandleNodeMsg 处理猫爪 messageToDart：toast / danmuPush / getPlayInfo /
// queryProfile / saveProfile / openInternalWebview。未知 action 返回 null。
// 返回值会作为 HTTP JSON 响应体写回 node。
func HandleNodeMsg(body []byte) any {
	var req struct {
		Action string          `json:"action"`
		Opt    json.RawMessage `json:"opt"`
		Message string         `json:"message"`
		URL     string         `json:"url"`
		Profile json.RawMessage `json:"profile"`
		Prefix  string         `json:"prefix"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("node msg: 无效 JSON: %v", err)
		return nil
	}
	action := strings.TrimSpace(req.Action)
	opt := map[string]any{}
	if len(req.Opt) > 0 {
		_ = json.Unmarshal(req.Opt, &opt)
	}

	switch action {
	case "toast":
		msg := strings.TrimSpace(req.Message)
		if msg == "" {
			msg, _ = opt["message"].(string)
			msg = strings.TrimSpace(msg)
		}
		if msg != "" {
			jsPostMsg(msg, "")
		}
		return map[string]any{"ok": true}

	case "danmuPush":
		u := strings.TrimSpace(req.URL)
		if u == "" {
			u, _ = opt["url"].(string)
			u = strings.TrimSpace(u)
		}
		u = rewriteDanmuURL(u, req.Prefix)
		if u != "" {
			// 前端后续识别；当前 PostMsgHost 未处理时会当 toast，故用明确前缀。
			payload, _ := json.Marshal(map[string]string{"url": u})
			jsPostMsg("DANMU:"+string(payload), "")
		}
		return map[string]any{"ok": true}

	case "getPlayInfo":
		info := currentNodePlayInfo()
		return map[string]any{
			"title":       info.Title,
			"episodeName": info.EpisodeName,
			"flag":        info.Flag,
		}

	case "queryProfile":
		pkg := latestNodePkg()
		if pkg == nil {
			return nil
		}
		return pkg.queryProfile()

	case "saveProfile":
		pkg := latestNodePkg()
		if pkg == nil {
			return nil
		}
		var profile any
		if len(req.Profile) > 0 {
			_ = json.Unmarshal(req.Profile, &profile)
		} else if v, ok := opt["profile"]; ok {
			profile = v
		} else if len(req.Opt) > 0 {
			_ = json.Unmarshal(req.Opt, &profile)
		}
		if err := pkg.saveProfile(profile); err != nil {
			log.Printf("node saveProfile: %v", err)
			return nil
		}
		return map[string]any{"ok": true}

	case "openInternalWebview":
		u := strings.TrimSpace(req.URL)
		if u == "" {
			u, _ = opt["url"].(string)
			u = strings.TrimSpace(u)
		}
		if u != "" {
			// UI: 弹窗协议需带 id；另用 OPENWEB: 给前端后续接系统浏览器/内置页。
			jsPostMsg("OPENWEB:"+u, "")
		}
		return map[string]any{"ok": true}

	case "sniff":
		// 计划外；有调用时记日志并回 null，避免源包卡死。
		log.Printf("node msg: 未实现 sniff")
		return nil

	default:
		if action != "" {
			log.Printf("node msg: 未知 action %q", action)
		}
		return nil
	}
}

func rewriteDanmuURL(u, prefix string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return u
	}
	h := DefaultNodeHost()
	h.mu.Lock()
	var pkg *nodePkg
	for _, p := range h.pkgs {
		if p.Port() > 0 {
			pkg = p
		}
	}
	// 若 prefix 能对应 api，优先该包
	if prefix != "" {
		if id := h.apiIndex[strings.TrimSuffix(prefix, "/")]; id != "" {
			if p := h.pkgs[id]; p != nil {
				pkg = p
			}
		}
		// prefix 形如 /spider/key/type
		for api, id := range h.apiIndex {
			if strings.HasPrefix(prefix, api) || strings.HasPrefix(api, prefix) {
				if p := h.pkgs[id]; p != nil {
					pkg = p
					break
				}
			}
		}
	}
	h.mu.Unlock()
	if pkg == nil {
		return u
	}
	return pkg.RewriteNodeURL(u)
}

func latestNodePkg() *nodePkg {
	h := DefaultNodeHost()
	h.mu.Lock()
	defer h.mu.Unlock()
	var last *nodePkg
	for _, p := range h.pkgs {
		last = p
	}
	return last
}
