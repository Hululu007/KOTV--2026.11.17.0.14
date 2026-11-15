package spider

import (
	"encoding/json"
	"log"
	"strings"
	"sync"

	"github.com/bobo/KOTV/internal/hostclient"
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
		u = rewriteNodePkgURL(u, req.Prefix)
		if u != "" {
			payload, _ := json.Marshal(map[string]string{"url": u})
			jsPostMsg("DANMU:"+string(payload), "")
		}
		return map[string]any{"ok": true}

	case "getPlayInfo":
		// 引擎进程内一份全局 playInfo；多远端客户端未隔离。
		info := currentNodePlayInfo()
		return map[string]any{
			"title":       info.Title,
			"episodeName": info.EpisodeName,
			"flag":        info.Flag,
		}

	case "queryProfile":
		pkg := resolveNodePkg(req.Prefix)
		if pkg == nil {
			return nil
		}
		return pkg.queryProfile()

	case "saveProfile":
		pkg := resolveNodePkg(req.Prefix)
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
		}
		emitOpenWeb(u, req.Prefix)
		return map[string]any{"ok": true}

	case "sniff":
		// CatPawOpen ffm3u8 等会调；当前宿主无嗅探管线，返回 null。
		return nil

	default:
		if action != "" {
			log.Printf("node msg: 未知 action %q", action)
		}
		return nil
	}
}

// resolveNodePkg 按 api prefix 选包；prefix 空则取任一已起端口的包。
// /msg 等无 Bind 回调时扫全部用户桶（与 findNodePkgByLocalPort 同构）。
func resolveNodePkg(prefix string) *nodePkg {
	prefix = strings.TrimSpace(prefix)
	want := strings.TrimSuffix(prefix, "/")

	resolveIn := func(h *NodeHost) *nodePkg {
		if h == nil {
			return nil
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		var anyReady *nodePkg
		for _, p := range h.pkgs {
			if p.Port() > 0 {
				anyReady = p
			}
		}
		if want == "" {
			return anyReady
		}
		if id := h.apiIndex[want]; id != "" {
			if p := h.pkgs[id]; p != nil {
				return p
			}
		}
		for api, id := range h.apiIndex {
			if strings.HasPrefix(want, api) || strings.HasPrefix(api, want) {
				if p := h.pkgs[id]; p != nil {
					return p
				}
			}
		}
		return nil
	}

	// 有 Bind 时优先当前桶，避免跨用户误命中。
	if hostclient.RuntimeUserID() != "" || hostclient.ScopeID() != "" {
		if p := resolveIn(nodeHostForCurrent()); p != nil {
			return p
		}
	}
	for _, h := range snapshotNodeHosts() {
		if p := resolveIn(h); p != nil {
			return p
		}
	}
	return nil
}

// resolveNodePkgByLocalPort 按 URL 里 127.0.0.1|localhost:端口 匹配已起的包（跨用户桶，端口全局唯一）。
func resolveNodePkgByLocalPort(raw string) *nodePkg {
	sub := reLocalNodeURL.FindStringSubmatch(raw)
	if len(sub) != 2 {
		return nil
	}
	return findNodePkgByLocalPort(sub[1])
}

