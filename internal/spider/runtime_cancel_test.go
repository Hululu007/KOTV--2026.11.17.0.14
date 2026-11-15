package spider

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	goruntime "runtime"
	"strconv"
	"testing"
	"time"

	"github.com/bobo/KOTV/internal/hostclient"
)

func TestSoftCancelForClient_EmptyNoop(t *testing.T) {
	SoftCancelForClient("") // 无 Scope 时不得打断全部会话
}

func TestNodePostJSON_InterruptCancelsAndStops(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(3 * time.Second)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 {
		t.Fatalf("bad port: %v", u.Port())
	}
	pkg := &nodePkg{id: "inflight-cancel-test", port: port}

	leave := hostclient.Enter("test-client-node")
	defer leave()
	snap := hostclient.Snapshot()
	scope := hostclient.ScopeID() // c:<clientId>

	errCh := make(chan error, 1)
	go func() {
		defer hostclient.Bind(snap)()
		_, err := pkg.PostJSON("/play", map[string]any{"a": 1})
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler not started")
	}
	InterruptNodeForClient(scope)
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrNodeInterrupted) {
			t.Fatalf("got %v want ErrNodeInterrupted", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PostJSON did not return after interrupt")
	}
	if pkg.Port() != 0 {
		t.Fatalf("port still %d after Stop", pkg.Port())
	}
}

func TestRestartSharedRuntime_StopsNode(t *testing.T) {
	h := DefaultNodeHost()
	pkg := &nodePkg{id: "restart-shared-node", port: 8888}
	h.mu.Lock()
	h.pkgs[pkg.id] = pkg
	h.mu.Unlock()
	t.Cleanup(func() {
		h.mu.Lock()
		delete(h.pkgs, pkg.id)
		h.mu.Unlock()
	})
	HardRestartSharedRuntimes()
	if pkg.Port() != 0 {
		t.Fatalf("node port=%d after hard restart", pkg.Port())
	}
}

func TestPySpiderInterrupt_DesktopBusyDoesNotDeadlock(t *testing.T) {
	if goruntime.GOOS == "android" {
		t.Skip("android interrupt 走 bridge")
	}
	s := &pySpider{}
	s.activeClient.Store("c1")
	// 模拟 run() 持有 mu 等待 IPC
	s.mu.Lock()
	done := make(chan struct{})
	go func() {
		s.interrupt()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		s.mu.Unlock()
		t.Fatal("interrupt deadlocked on mu")
	}
	s.mu.Unlock()

	s.activeClient.Store("")
	s.interrupt() // 空闲 slot 不杀
}

func TestInterruptScriptSpiders_PerSlotClient(t *testing.T) {
	jsPyMu.Lock()
	saved := jsPy
	jsPy = map[string]Spider{}
	jsPyMu.Unlock()
	t.Cleanup(func() {
		jsPyMu.Lock()
		jsPy = saved
		jsPyMu.Unlock()
	})

	a := &pySpider{}
	a.activeClient.Store("c-a")
	a.epoch.Store(1)
	b := &pySpider{}
	b.activeClient.Store("c-b")
	b.epoch.Store(1)
	pool := &pyPool{}
	pool.slots = []*pySpider{a, b}
	jsPyMu.Lock()
	jsPy["pool"] = pool
	jsPyMu.Unlock()

	InterruptScriptSpidersForClient("c-a")
	if a.epoch.Load() == 1 {
		t.Fatal("slot a should be interrupted")
	}
	if b.epoch.Load() != 1 {
		t.Fatal("slot b must not be interrupted for other client")
	}
}
