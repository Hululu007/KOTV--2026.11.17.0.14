package spider

import "testing"

func TestCurrentPyCallScope(t *testing.T) {
	jsPyMu.Lock()
	saved := jsPy
	jsPy = map[string]Spider{}
	jsPyMu.Unlock()
	t.Cleanup(func() {
		jsPyMu.Lock()
		jsPy = saved
		jsPyMu.Unlock()
	})

	if got, conflict := CurrentPyCallScope(); got != "" || conflict {
		t.Fatalf("empty got %q conflict %v", got, conflict)
	}

	phone := &pySpider{}
	phone.activeClient.Store("c:phone")
	idle := &pySpider{}
	pool := &pyPool{slots: []*pySpider{phone, idle}}
	jsPyMu.Lock()
	jsPy["pool"] = pool
	jsPyMu.Unlock()
	if got, conflict := CurrentPyCallScope(); got != "c:phone" || conflict {
		t.Fatalf("local got %q conflict %v", got, conflict)
	}

	remote := &pySpider{}
	remote.activeClient.Store("u:user")
	jsPyMu.Lock()
	jsPy["remote"] = remote
	jsPyMu.Unlock()
	if got, conflict := CurrentPyCallScope(); got != "" || !conflict {
		t.Fatalf("conflict got %q conflict %v", got, conflict)
	}

	phone.activeClient.Store("u:user")
	if got, conflict := CurrentPyCallScope(); got != "u:user" || conflict {
		t.Fatalf("same user got %q conflict %v", got, conflict)
	}
}

func TestRecentPyCallScopeKeepsLastCaller(t *testing.T) {
	recentPyMu.Lock()
	saved := recentPyScopeID
	recentPyScopeID = ""
	recentPyMu.Unlock()
	t.Cleanup(func() {
		recentPyMu.Lock()
		recentPyScopeID = saved
		recentPyMu.Unlock()
	})

	noteRecentPyScope("")
	if got := RecentPyCallScope(); got != "" {
		t.Fatalf("empty note kept %q", got)
	}
	noteRecentPyScope(" c:phone ")
	noteRecentPyScope("")
	if got := RecentPyCallScope(); got != "c:phone" {
		t.Fatalf("got %q", got)
	}
	noteRecentPyScope("u:user")
	if got := RecentPyCallScope(); got != "u:user" {
		t.Fatalf("replaced got %q", got)
	}
}
