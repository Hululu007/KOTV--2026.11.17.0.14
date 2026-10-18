package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bobo/KOTV/internal/database"
	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/settings"
)

const maxSyncBody = 8 << 20 // 8 MiB

// SyncHandler 局域网同步回调。
type SyncHandler struct {
	ValidatePair func(code string) bool
	Export       func(typ string) (json.RawMessage, error)
	Import       func(typ string, mode int, body []byte) error
	// ImportTVTargets 导入 FormBody 的 targets JSON（已做 key 归一）。
	ImportTVTargets func(typ string, force bool, targetsJSON string) error
	// ExportTVForm 导出兼容的 form 字段（config/targets 或 targets/configs）。
	ExportTVForm func(typ string) (url.Values, error)
	// LoadConfig 按地址加载点播配置。仅在地址与当前源不同时调用。
	LoadConfig func(url, name string) error
}

func (s *Server) SetSyncHandler(h *SyncHandler) {
	s.mu.Lock()
	s.syncHandler = h
	s.mu.Unlock()
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request, q map[string]string) {
	s.mu.RLock()
	h := s.syncHandler
	s.mu.RUnlock()
	if h == nil {
		http.Error(w, "sync not available", http.StatusServiceUnavailable)
		return
	}
	typ := strings.TrimSpace(q["type"])
	if typ != "history" && typ != "keep" {
		http.Error(w, "invalid type", http.StatusBadRequest)
		return
	}
	mode, _ := strconv.Atoi(strings.TrimSpace(q["mode"]))
	if mode < 0 || mode > 2 {
		http.Error(w, "invalid mode", http.StatusBadRequest)
		return
	}
	pair := strings.TrimSpace(q["pair"])
	force := strings.EqualFold(strings.TrimSpace(q["force"]), "true")
	targets := strings.TrimSpace(q["targets"])
	deviceJSON := strings.TrimSpace(q["device"])
	isTV := targets != "" || deviceJSON != "" ||
		strings.TrimSpace(q["config"]) != "" || strings.TrimSpace(q["configs"]) != ""

	// —— FormBody 协议（device/config/targets，无 pair）——
	if isTV && pair == "" {
		_, hasConfig := q["config"]
		if err := s.handleTVSync(h, typ, mode, force, targets, deviceJSON, strings.TrimSpace(q["config"]), strings.TrimSpace(q["configs"]), hasConfig); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
		return
	}

	// —— KOTV 配对协议 ——
	if h.ValidatePair == nil || pair == "" || !h.ValidatePair(pair) {
		http.Error(w, "invalid pair code", http.StatusForbidden)
		return
	}
	if mode == 0 {
		if h.Export == nil {
			http.Error(w, "export not available", http.StatusServiceUnavailable)
			return
		}
		data, err := h.Export(typ)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
		return
	}
	if h.Import == nil {
		http.Error(w, "import not available", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSyncBody))
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	if len(body) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	if err := h.Import(typ, mode, body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) handleTVSync(h *SyncHandler, typ string, mode int, force bool, targets, deviceJSON, configJSON, configsJSON string, hasConfig bool) error {
	// mode 0/2：把本机数据推回 device。mode 0/1：导入对方 targets，必要时先换源。
	if (mode == 0 || mode == 2) && deviceJSON != "" {
		if h.ExportTVForm == nil {
			return fmt.Errorf("外部导出暂不可用")
		}
		form, err := h.ExportTVForm(typ)
		if err != nil {
			return err
		}
		go pushTVSync(deviceJSON, typ, form)
	}
	if mode != 0 && mode != 1 {
		return nil
	}
	url, name, load, skipImport := syncLoadPlan(typ, settings.Get(settings.VOD), hasConfig, configJSON, configsJSON)
	if skipImport {
		return nil
	}
	if load {
		go s.finishSyncAfterLoad(h, typ, force, targets, url, name)
		return nil
	}
	return importTVTargets(h, typ, force, targets)
}

func (s *Server) finishSyncAfterLoad(h *SyncHandler, typ string, force bool, targets, url, name string) {
	if h.LoadConfig == nil {
		log.Printf("sync load: 换源暂不可用")
		return
	}
	if err := h.LoadConfig(url, name); err != nil {
		log.Printf("sync load: %v", err)
		return
	}
	if err := importTVTargets(h, typ, force, targets); err != nil {
		log.Printf("sync import: %v", err)
	}
}

func importTVTargets(h *SyncHandler, typ string, force bool, targets string) error {
	if targets == "" {
		return nil
	}
	if h.ImportTVTargets == nil {
		return fmt.Errorf("外部导入暂不可用")
	}
	return h.ImportTVTargets(typ, force, targets)
}

// syncLoadPlan 决定本次同步要不要先换点播源。
// 历史：带了 config 但 url 为空则不导入；url 与当前地址不同才加载。
// 收藏：仅当前地址为空时加载 configs 的第一项。
func syncLoadPlan(typ, current string, hasConfig bool, configJSON, configsJSON string) (url, name string, load, skipImport bool) {
	current = strings.TrimSpace(current)
	switch typ {
	case "history":
		if !hasConfig {
			return "", "", false, false
		}
		u, n, ok := parseHistoryConfig(configJSON)
		if !ok || u == "" {
			return "", "", false, true
		}
		if u != current {
			return u, n, true, false
		}
		return u, n, false, false
	case "keep":
		if current != "" {
			return "", "", false, false
		}
		u, n := firstKeepConfig(configsJSON)
		if u == "" {
			return "", "", false, false
		}
		return u, n, true, false
	default:
		return "", "", false, false
	}
}

func parseHistoryConfig(raw string) (url, name string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	var one struct {
		URL  string `json:"url"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &one); err != nil {
		return "", "", false
	}
	return strings.TrimSpace(one.URL), strings.TrimSpace(one.Name), true
}

func firstKeepConfig(raw string) (url, name string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	var items []struct {
		URL  string `json:"url"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil || len(items) == 0 {
		return "", ""
	}
	return strings.TrimSpace(items[0].URL), strings.TrimSpace(items[0].Name)
}

func pushTVSync(deviceJSON, typ string, form url.Values) {
	var dev struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal([]byte(deviceJSON), &dev); err != nil || strings.TrimSpace(dev.IP) == "" {
		log.Printf("sync push: bad device json")
		return
	}
	base := strings.TrimRight(strings.TrimSpace(dev.IP), "/")
	endpoint := base + "/action?do=sync&mode=0&type=" + url.QueryEscape(typ)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.PostForm(endpoint, form)
	if err != nil {
		log.Printf("sync push to %s: %v", base, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Printf("sync push HTTP %d: %s", resp.StatusCode, string(b))
	}
}

// NewAppSyncHandler 构造默认同步处理器（兼容 KOTV pair + FormBody）。
func NewAppSyncHandler(db *database.DB, validatePair func(string) bool) *SyncHandler {
	return &SyncHandler{
		ValidatePair: validatePair,
		Export: func(typ string) (json.RawMessage, error) {
			owner := hostclient.DataOwner()
			switch typ {
			case "history":
				items, err := db.ListAllHistory(owner)
				if err != nil {
					return nil, err
				}
				return json.Marshal(items)
			case "keep":
				items, err := db.ListAllKeep(owner, database.KeepTypeVod)
				if err != nil {
					return nil, err
				}
				return json.Marshal(items)
			default:
				return nil, http.ErrNotSupported
			}
		},
		Import: func(typ string, mode int, body []byte) error {
			owner := hostclient.DataOwner()
			switch typ {
			case "history":
				var items []database.History
				if err := json.Unmarshal(body, &items); err != nil {
					return err
				}
				normalizeHistoryKeys(items)
				if err := db.ImportHistory(owner, items, mode); err != nil {
					return err
				}
				return nil
			case "keep":
				var items []database.Keep
				if err := json.Unmarshal(body, &items); err != nil {
					return err
				}
				if err := db.ImportKeep(owner, items, mode); err != nil {
					return err
				}
				return nil
			default:
				return http.ErrNotSupported
			}
		},
		ImportTVTargets: func(typ string, force bool, targetsJSON string) error {
			mode := 1
			if force {
				mode = 2
			}
			owner := hostclient.DataOwner()
			body := []byte(targetsJSON)
			switch typ {
			case "history":
				var items []database.History
				if err := json.Unmarshal(body, &items); err != nil {
					return err
				}
				normalizeHistoryKeys(items)
				if err := db.ImportHistory(owner, items, mode); err != nil {
					return err
				}
				return nil
			case "keep":
				var items []database.Keep
				if err := json.Unmarshal(body, &items); err != nil {
					return err
				}
				if err := db.ImportKeep(owner, items, mode); err != nil {
					return err
				}
				return nil
			default:
				return http.ErrNotSupported
			}
		},
		ExportTVForm: func(typ string) (url.Values, error) {
			form := url.Values{}
			owner := hostclient.DataOwner()
			switch typ {
			case "history":
				items, err := db.ListAllHistory(owner)
				if err != nil {
					return nil, err
				}
				for i := range items {
					items[i].Key = database.ExternalHistoryKey(items[i].Key)
				}
				b, err := json.Marshal(items)
				if err != nil {
					return nil, err
				}
				cfgURL := settings.Get(settings.VOD)
				cfg, _ := json.Marshal(map[string]any{"url": cfgURL, "type": 0})
				form.Set("config", string(cfg))
				form.Set("targets", string(b))
			case "keep":
				items, err := db.ListAllKeep(owner, database.KeepTypeVod)
				if err != nil {
					return nil, err
				}
				b, err := json.Marshal(items)
				if err != nil {
					return nil, err
				}
				cfgURL := settings.Get(settings.VOD)
				cfgs, _ := json.Marshal([]map[string]any{{"url": cfgURL, "type": 0}})
				form.Set("targets", string(b))
				form.Set("configs", string(cfgs))
			default:
				return nil, http.ErrNotSupported
			}
			return form, nil
		},
	}
}

func normalizeHistoryKeys(items []database.History) {
	for i := range items {
		items[i].Key = database.NormalizeExternalHistoryKey(items[i].Key)
		if items[i].Position < 0 {
			items[i].Position = 0
		}
		if items[i].Opening < 0 {
			items[i].Opening = 0
		}
		if items[i].Ending < 0 {
			items[i].Ending = 0
		}
	}
}
