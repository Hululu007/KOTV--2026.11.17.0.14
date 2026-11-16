package spider

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJSInterrupt_CancelsHTTP(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			_, _ = w.Write([]byte("late"))
		}
	}))
	t.Cleanup(srv.Close)

	s := &jsSpider{key: "irq", api: "x.js"}
	ctx, end := s.beginHTTP()
	done := make(chan map[string]interface{}, 1)
	go func() {
		done <- doJSRequest(ctx, srv.URL, jsHTTPRequest{Method: "GET", Timeout: 10000})
		end()
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler not started")
	}
	s.interrupt()
	select {
	case out := <-done:
		if code, ok := out["code"].(string); !ok || code != "" {
			t.Fatalf("want empty code after cancel, got %#v", out["code"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("doJSRequest did not return after interrupt")
	}
}

func TestJSInterrupt_WakesWaiters(t *testing.T) {
	s := &jsSpider{key: "wake", api: "x.js"}
	wake := s.addWait()
	go func() {
		time.Sleep(30 * time.Millisecond)
		s.interrupt()
	}()
	select {
	case <-wake:
	case <-time.After(2 * time.Second):
		t.Fatal("wait channel should close on interrupt")
	}
}

func TestBeginScopeHTTP_Cancel(t *testing.T) {
	ctx, end := BeginScopeHTTP("c:test")
	defer end()
	cancelScopeHTTP("c:test")
	select {
	case <-ctx.Done():
	default:
		t.Fatal("scope http should be cancelled")
	}
}

func TestBumpJarRefreshGen_ClearsAllTenants(t *testing.T) {
	const path = "/hard-restart.jar"
	keyA := "user-a\x01" + path
	keyB := "user-b\x01" + path
	parseJarNotified.Store(keyA, struct{}{})
	parseJarNotified.Store(keyB, struct{}{})
	jarRecentByUser.Store("user-a", path)
	t.Cleanup(func() {
		parseJarNotified.Delete(keyA)
		parseJarNotified.Delete(keyB)
		jarRecentByUser.Delete("user-a")
	})
	bumpJarRefreshGen()
	if _, ok := parseJarNotified.Load(keyA); ok {
		t.Fatal("tenant A notified should clear on hard restart bump")
	}
	if _, ok := parseJarNotified.Load(keyB); ok {
		t.Fatal("tenant B notified should clear on hard restart bump")
	}
	if _, ok := jarRecentByUser.Load("user-a"); ok {
		t.Fatal("jarRecent should clear")
	}
}
