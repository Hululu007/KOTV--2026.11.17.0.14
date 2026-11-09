package spider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestActivePkgAndProxyRootAlias(t *testing.T) {
	h := &NodeHost{pkgs: map[string]*nodePkg{}}
	if h.ActivePkg() != nil {
		t.Fatal("empty host should have no active pkg")
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9978/website/api/status", nil)
	if h.ProxyRootAlias(rr, req) {
		t.Fatal("ProxyRootAlias should miss without alias")
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/website/api/status" {
			t.Fatalf("upstream path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	port := mustLocalPort(t, upstream.URL)
	p := &nodePkg{id: "webpkg", port: port}
	h.pkgs["webpkg"] = p
	if h.ActivePkg() != p {
		t.Fatal("ActivePkg should return ready pkg")
	}
	h.RememberNodeRootAlias("website")
	if !h.MatchNodeRootAlias("/website/api/status") {
		t.Fatal("expected alias match")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9978/website/api/status", nil)
	if !h.ProxyRootAlias(rr, req) {
		t.Fatal("ProxyRootAlias should succeed")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body=%q", got)
	}
}

func mustLocalPort(t *testing.T, raw string) int {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if _, err := fmt.Sscanf(u.Port(), "%d", &n); err != nil || n == 0 {
		t.Fatalf("port from %s", raw)
	}
	return n
}
