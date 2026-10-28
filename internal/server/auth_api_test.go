package server

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/bobo/KOTV/internal/auth"
	"github.com/bobo/KOTV/internal/settings"
)

func TestProbeOnlyPath(t *testing.T) {
	t.Parallel()
	if !probeOnlyPath("/api/v1/health") || !probeOnlyPath("/api/v1/auth/status") {
		t.Fatal("probe paths should be allowed")
	}
	if probeOnlyPath("/api/v1/auth/login") || probeOnlyPath("/api/v1/home") {
		t.Fatal("login/home must not be probe-only")
	}
}

func TestProtocolMismatch(t *testing.T) {
	t.Parallel()
	req := func(addr, path, proto string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "http://h:9978"+path, nil)
		r.RemoteAddr = addr
		if proto != "" {
			r.Header.Set("X-Kotv-Protocol", proto)
		}
		return r
	}
	const lan = "192.168.1.8:5000"
	if protocolMismatch(req("127.0.0.1:5000", "/api/v1/home", "")) {
		t.Fatal("loopback is never checked")
	}
	if protocolMismatch(req(lan, "/api/v1/health", "")) {
		t.Fatal("health is never checked")
	}
	if !protocolMismatch(req(lan, "/api/v1/home", "")) || !protocolMismatch(req(lan, "/api/v1/home", "0")) {
		t.Fatal("missing or old protocol must be rejected")
	}
	if protocolMismatch(req(lan, "/api/v1/home", strconv.Itoa(ProtocolVersion))) {
		t.Fatal("same protocol must pass")
	}
}

func TestRemoteAccessAllowed(t *testing.T) {
	settings.Set(settings.RemoteAuth, "false")
	defer settings.Set(settings.RemoteAuth, "false")

	loop := &http.Request{RemoteAddr: "127.0.0.1:12345"}
	if !remoteAccessAllowed(loop) {
		t.Fatal("loopback always allowed")
	}
	lan := &http.Request{RemoteAddr: "192.168.1.8:54321"}
	if remoteAccessAllowed(lan) {
		t.Fatal("LAN without remoteAuth must be denied")
	}
	webLan := &http.Request{
		RemoteAddr: "192.168.1.8:54321",
		Header:     http.Header{"X-Kotv-Client-Platform": []string{"web"}},
	}
	if !remoteAccessAllowed(webLan) {
		t.Fatal("Web same-origin must be allowed without remoteAuth")
	}
	if !authRequiredFor(webLan) {
		t.Fatal("Web must still require login")
	}

	settings.Set(settings.RemoteAuth, "true")
	if !remoteAccessAllowed(lan) {
		t.Fatal("LAN with remoteAuth must be allowed")
	}
	if !auth.RemoteAuthEnabled() {
		t.Fatal("remoteAuth should read true")
	}
}

func TestWebUiQueryPlatform(t *testing.T) {
	settings.Set(settings.RemoteAuth, "false")
	defer settings.Set(settings.RemoteAuth, "false")

	req, err := http.NewRequest(http.MethodGet, "http://192.168.1.8:9978/ui?role=ui&platform=web", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "192.168.1.8:54321"
	if !isWebClient(req) {
		t.Fatal("ui query platform should count as web")
	}
	if !remoteAccessAllowed(req) {
		t.Fatal("web ui socket should be allowed without remoteAuth")
	}
	if !authRequiredFor(req) {
		t.Fatal("web ui still requires login")
	}
}

func TestRequestPublicBasePrefersClientEngine(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://192.168.0.113:9978/api/v1/play", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "192.168.0.113:9978"
	req.Header.Set("X-Forwarded-Host", "10.0.0.8:9978")
	req.Header.Set("X-Kotv-Engine-Base", "https://tv.example.com:8443/ignored")
	if got := requestPublicBase(req); got != "https://tv.example.com:8443" {
		t.Fatalf("client base=%q", got)
	}
	req.Header.Set("X-Kotv-Engine-Base", "http://127.0.0.1:9978")
	if got := requestPublicBase(req); got != "http://10.0.0.8:9978" {
		t.Fatalf("loopback client base should fall through, got %q", got)
	}
}
