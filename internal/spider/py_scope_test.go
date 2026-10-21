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

	if got := CurrentPyCallScope(); got != "" {
		t.Fatalf("empty got %q", got)
	}

	phone := &pySpider{}
	phone.activeClient.Store("c:phone")
	idle := &pySpider{}
	pool := &pyPool{slots: []*pySpider{phone, idle}}
	jsPyMu.Lock()
	jsPy["pool"] = pool
	jsPyMu.Unlock()
	if got := CurrentPyCallScope(); got != "c:phone" {
		t.Fatalf("local got %q", got)
	}

	remote := &pySpider{}
	remote.activeClient.Store("u:user")
	jsPyMu.Lock()
	jsPy["remote"] = remote
	jsPyMu.Unlock()
	if got := CurrentPyCallScope(); got != "" {
		t.Fatalf("conflict got %q", got)
	}

	phone.activeClient.Store("u:user")
	if got := CurrentPyCallScope(); got != "u:user" {
		t.Fatalf("same user got %q", got)
	}
}
