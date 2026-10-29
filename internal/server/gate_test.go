package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/bobo/KOTV/internal/auth"
	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/settings"
)

func TestGate(t *testing.T) {
	t.Setenv("KOTV_DATA_DIR", t.TempDir())
	settings.Set(settings.RemoteAuth, "true")
	defer settings.Set(settings.RemoteAuth, "false")

	var seenBase string
	var seenAccess access
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenBase, seenAccess = hostclient.PublicBase(), requestAccess(r)
		w.WriteHeader(http.StatusOK)
	})
	s := New()
	s.buildMux(fstest.MapFS{"index.html": {Data: []byte("x")}})
	h := s.gate(ok)
	do := func(method, path, addr string, hdr map[string]string) int {
		r := httptest.NewRequest(method, "http://192.168.1.8:9978"+path, nil)
		r.RemoteAddr = addr
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		seenBase, seenAccess = "", -1
		h.ServeHTTP(w, r)
		return w.Code
	}
	const lan, local = "192.168.1.20:5000", "127.0.0.1:5000"
	proto := map[string]string{"X-Kotv-Protocol": "1"}
	key := auth.MediaKey()

	if c := do("GET", "/api/v1/home", local, nil); c != 200 || seenAccess != accessFull || seenBase != "" {
		t.Fatalf("local api: %d %v %q", c, seenAccess, seenBase)
	}
	if c := do("GET", "/api/v1/home", lan, proto); c != 401 {
		t.Fatalf("remote api without login: %d", c)
	}
	if c := do("GET", "/api/v1/home", local, map[string]string{"X-Forwarded-For": "203.0.113.9", "X-Kotv-Protocol": "1"}); c != 401 {
		t.Fatalf("reverse-proxied request must need login: %d", c)
	}
	for _, p := range []string{"/api/v1/health", "/api/v1/auth/status", "/", "/js/script.js"} {
		if c := do("GET", p, lan, proto); c != 200 || seenAccess != accessPublic {
			t.Fatalf("public %s: %d %v", p, c, seenAccess)
		}
	}
	for _, p := range []string{"/proxy/play?id=1", "/file/a.mp4", "/image/x", "/action", "/media", "/device", "/upload", "/ui", "/library", "/cache"} {
		if c := do("GET", p, lan, nil); c != 401 {
			t.Fatalf("remote %s without login: %d", p, c)
		}
	}
	if c := do("GET", "/k/"+key+"/proxy/play?id=1", lan, nil); c != 200 || seenAccess != accessMedia || seenBase != "http://192.168.1.8:9978/k/"+key {
		t.Fatalf("media key: %d %v %q", c, seenAccess, seenBase)
	}
	if c := do("GET", "/k/"+key+"/api/v1/home", lan, proto); c != 403 {
		t.Fatalf("media key on api: %d", c)
	}
	if c := do("POST", "/k/"+key+"/image/x", lan, nil); c != 403 {
		t.Fatalf("media key write: %d", c)
	}
	if c := do("GET", "/k/AAAA/proxy/play?id=1", lan, nil); c != 401 {
		t.Fatalf("bad media key: %d", c)
	}

	settings.Set(settings.RemoteAuth, "false")
	if c := do("GET", "/api/v1/home", lan, proto); c != 403 {
		t.Fatalf("remote auth off: %d", c)
	}
	if c := do("GET", "/api/v1/health", lan, nil); c != 200 {
		t.Fatalf("health with remote auth off: %d", c)
	}
	if c := do("GET", "/k/"+key+"/proxy/play?id=1", lan, nil); c != 200 {
		t.Fatalf("media key works without remote auth (tv cast): %d", c)
	}
}

func TestRouteNeeds(t *testing.T) {
	s := New()
	s.buildMux(fstest.MapFS{"index.html": {Data: []byte("x")}})
	want := map[need][]string{
		needPublic: {"/health", "/api/v1/health", "/api/v1/net", "/api/v1/auth/status", "/api/v1/auth/login", "/api/v1/auth/register", "/", "/js/script.js", "/admin/"},
		needUser:   {"/api/v1/home", "/api/v1/auth/me", "/library", "/ui", "/proxy/play", "/file/a.mp4", "/image/x", "/tvbus", "/unknown"},
		needAdmin:  {"/action", "/media", "/device", "/upload", "/newFolder", "/delFolder", "/delFile", "/cache", "/java", "/parse", "/api/v1/admin/users", "/api/v1/admin/users/1/password", "/api/v1/fs"},
		needLocal:  {"/api/v1/shutdown"},
	}
	for n, paths := range want {
		for _, p := range paths {
			r := httptest.NewRequest(http.MethodGet, "http://h"+p, nil)
			if got := s.routeNeed(r); got != n && !(p == "/unknown" && got == needPublic) {
				t.Fatalf("%s: need %d, want %d", p, got, n)
			}
		}
	}
}
