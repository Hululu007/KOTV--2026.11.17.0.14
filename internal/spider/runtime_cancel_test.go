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

func TestSoftCancelAll_DoesNotPanic(t *testing.T) {
	// 全局软取消须可空跑（清缓存路径）；与 SoftCancelForClient("") 的 no-op 不同。
	SoftCancelAll()
}

func TestCancelSharedJarCalls_LeavesUserInflight(t *testing.T) {
	sharedCtx, endShared := beginJarCall("c:local")
	userCtx, endUser := beginJarCall("u:alice")
	t.Cleanup(func() {
		endShared()
		endUser()
	})
	cancelSharedJarCalls()
	select {
	case <-sharedCtx.Done():
	default:
		t.Fatal("shared jar call should be cancelled")
	}
	select {
	case <-userCtx.Done():
		t.Fatal("user jar call must survive shared cancel")
	default:
	}
}

func TestForgetUserRuntimeState_Consolidated(t *testing.T) {
	leave := hostclient.EnterSession("c-forget", "forget-u", true)
	path := "/forget.jar"
	key := jarRunKey(path)
	parseJarNotified.Store(key, struct{}{})
	jarPathByUser.Store("forget-u", path)
	configBaseByClient.Store("u:forget-u", "http://ex/")
	leave()
	jsRedirectsMu.Lock()
	jsRedirectsByUser["forget-u"] = nil // placeholder presence
	jsRedirectsMu.Unlock()
	netConfigMu.Lock()
	if netConfigByClient == nil {
		netConfigByClient = map[string][]byte{}
	}
	netConfigByClient["u:forget-u"] = []byte(`{}`)
	netConfigMu.Unlock()
	t.Cleanup(func() {
		parseJarNotified.Delete(key)
		clearJSRedirectsForUser("forget-u")
		clearNetConfigForUser("forget-u")
	})

	forgetUserRuntimeState("forget-u")
	if _, ok := parseJarNotified.Load(key); ok {
		t.Fatal("jar go state should be cleared")
	}
	jsRedirectsMu.Lock()
	_, redirOK := jsRedirectsByUser["forget-u"]
	jsRedirectsMu.Unlock()
	if redirOK {
		t.Fatal("js redirects should be cleared")
	}
	netConfigMu.Lock()
	_, netOK := netConfigByClient["u:forget-u"]
	netConfigMu.Unlock()
	if netOK {
		t.Fatal("net config should be cleared")
	}
}

func TestJavaBridgesForCancel_LocalVsRemote(t *testing.T) {
	userBridgesMu.Lock()
	saved := userBridges
	alice := &javaBridgeClient{ownerUID: "alice"}
	bob := &javaBridgeClient{ownerUID: "bob"}
	userBridges = map[string]*javaBridgeClient{"alice": alice, "bob": bob}
	userBridgesMu.Unlock()
	t.Cleanup(func() {
		userBridgesMu.Lock()
		userBridges = saved
		userBridgesMu.Unlock()
	})

	all := javaBridgesForCancel("")
	if len(all) != 3 || all[0] != &javaBridge {
		t.Fatalf("SoftCancelAll targets=%d want shared+alice+bob", len(all))
	}
	got := map[*javaBridgeClient]bool{}
	for _, b := range all {
		got[b] = true
	}
	if !got[alice] || !got[bob] {
		t.Fatal("SoftCancelAll must include every user bridge")
	}

	onlyAlice := javaBridgesForCancel("u:alice")
	if len(onlyAlice) != 1 || onlyAlice[0] != alice {
		t.Fatalf("u:alice targets=%v", onlyAlice)
	}
	if targets := javaBridgesForCancel("u:missing"); len(targets) != 0 {
		t.Fatalf("missing user must not fall back to shared, got %d", len(targets))
	}
	local := javaBridgesForCancel("c:window-1")
	if len(local) != 1 || local[0] != &javaBridge {
		t.Fatalf("local c: scope must hit shared only")
	}

	// SoftCancelAll 不得受当前 Bind 影响（即便绑在远端用户上）。
	leave := hostclient.Bind(hostclient.Request{UserID: "alice", Dedicated: true})
	defer leave()
	allUnderBind := javaBridgesForCancel("")
	if len(allUnderBind) != 3 {
		t.Fatalf("SoftCancelAll under dedicated Bind got %d want 3", len(allUnderBind))
	}
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
	h := sharedNodeHost
	pkg := &nodePkg{id: "inflight-cancel-test", port: port, ownerUID: "", host: h}
	h.mu.Lock()
	h.pkgs[pkg.id] = pkg
	h.mu.Unlock()
	t.Cleanup(func() {
		h.mu.Lock()
		delete(h.pkgs, pkg.id)
		h.mu.Unlock()
	})

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
	interruptNodeForClient(scope)
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

func TestRestartSharedRuntime_StopsSharedNodeOnly(t *testing.T) {
	shared := sharedNodeHost
	sharedPkg := &nodePkg{id: "restart-shared-node", port: 8888, ownerUID: ""}
	shared.mu.Lock()
	shared.pkgs[sharedPkg.id] = sharedPkg
	shared.mu.Unlock()

	user := nodeHostForUser("user-keep-node")
	userPkg := &nodePkg{id: "restart-user-node", port: 7777, ownerUID: "user-keep-node", host: user}
	user.mu.Lock()
	user.pkgs[userPkg.id] = userPkg
	user.mu.Unlock()

	t.Cleanup(func() {
		shared.mu.Lock()
		delete(shared.pkgs, sharedPkg.id)
		shared.mu.Unlock()
		destroyUserNodeHost("user-keep-node")
	})

	HardRestartSharedRuntimes()
	if sharedPkg.Port() != 0 {
		t.Fatalf("shared node port=%d after hard restart", sharedPkg.Port())
	}
	if userPkg.Port() != 7777 {
		t.Fatalf("user node should survive shared restart, port=%d", userPkg.Port())
	}
}

func TestNodeHostPerUserIsolation(t *testing.T) {
	u1 := nodeHostForUser("iso-u1")
	u2 := nodeHostForUser("iso-u2")
	if u1 == u2 || u1 == sharedNodeHost || u2 == sharedNodeHost {
		t.Fatal("dedicated users must get distinct hosts")
	}
	p1 := &nodePkg{id: "same-pkg", port: 1001, ownerUID: "iso-u1", host: u1}
	p2 := &nodePkg{id: "same-pkg", port: 1002, ownerUID: "iso-u2", host: u2}
	u1.mu.Lock()
	u1.pkgs[p1.id] = p1
	u1.mu.Unlock()
	u2.mu.Lock()
	u2.pkgs[p2.id] = p2
	u2.mu.Unlock()
	t.Cleanup(func() {
		destroyUserNodeHost("iso-u1")
		destroyUserNodeHost("iso-u2")
	})

	destroyUserNodeHost("iso-u1")
	if p1.Port() != 0 {
		t.Fatal("KillUser-equivalent should stop only u1 pkg")
	}
	if p2.Port() != 1002 {
		t.Fatal("u2 pkg must remain after destroying u1")
	}
	if nodeHostForUser("iso-u1") == u1 {
		t.Fatal("destroyed user host should be recreated on next access")
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

	interruptScriptSpidersForClient("c-a")
	if a.epoch.Load() == 1 {
		t.Fatal("slot a should be interrupted")
	}
	if b.epoch.Load() != 1 {
		t.Fatal("slot b must not be interrupted for other client")
	}
}
