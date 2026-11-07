package spider

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	appruntime "github.com/bobo/KOTV/internal/runtime"
)

// 端到端：设 KOTV_CAT_NODE_URL=http://user:pass@host/index.js.md5 时实跑样例源包。
func TestNodePackageE2E_Optional(t *testing.T) {
	pkgURL := strings.TrimSpace(os.Getenv("KOTV_CAT_NODE_URL"))
	if pkgURL == "" {
		t.Skip("set KOTV_CAT_NODE_URL to run")
	}
	if appruntime.Node() == "" {
		t.Skip("no bundled/runtime Node")
	}

	h := DefaultNodeHost()
	t.Cleanup(func() { h.StopAll() })

	start := time.Now()
	cfgJSON, err := h.LoadPackage(pkgURL)
	if err != nil {
		t.Fatalf("LoadPackage: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(cfgJSON), &root); err != nil {
		t.Fatalf("config json: %v", err)
	}
	video, _ := root["video"].(map[string]any)
	sites, _ := video["sites"].([]any)
	if len(sites) == 0 {
		t.Fatalf("no video.sites")
	}
	var api string
	for _, s := range sites {
		m, _ := s.(map[string]any)
		a, _ := m["api"].(string)
		key, _ := m["key"].(string)
		if strings.HasPrefix(a, "/spider/") && !strings.Contains(key, "douban") && !strings.Contains(key, "baseset") {
			api = a
			break
		}
	}
	if api == "" {
		t.Fatalf("no usable spider api")
	}
	sp := Get("e2e", api, "", "")
	if err := sp.Init(""); err != nil {
		t.Fatalf("init: %v", err)
	}
	home, err := sp.HomeContent(true)
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if !strings.Contains(home, "list") && !strings.Contains(home, "class") {
		t.Fatalf("unexpected home: %s", home[:min(200, len(home))])
	}
	search, err := sp.SearchContent("流浪地球", false, "1")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	t.Logf("api=%s home=%dB search=%dB cost=%s", api, len(home), len(search), time.Since(start))
}
