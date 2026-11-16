package localproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBridgeHealthy_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	if !BridgeHealthy(srv.URL, time.Second) {
		t.Fatal("expected healthy")
	}
}

func TestBridgeHealthy_Fail(t *testing.T) {
	if BridgeHealthy("http://127.0.0.1:1", 50*time.Millisecond) {
		t.Fatal("expected unhealthy")
	}
}

func TestBridgeClient_SameInstance(t *testing.T) {
	a := BridgeClient()
	b := BridgeClient()
	if a != b {
		t.Fatal("BridgeClient must reuse singleton")
	}
}
