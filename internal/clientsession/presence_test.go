package clientsession

import (
	"testing"
	"time"
)

func TestPresenceLeave_CallsOnExpire(t *testing.T) {
	var got []string
	p := NewPresence(func(uid string) { got = append(got, uid) })
	t.Cleanup(p.Stop)
	p.Ping("alice")
	p.Leave("alice")
	if len(got) != 1 || got[0] != "alice" {
		t.Fatalf("Leave onExpire got %v", got)
	}
	// 已离开的不再出现在 lastPing，二次 Leave 仍触发 onExpire（幂等释放）。
	p.Leave("alice")
	if len(got) != 2 {
		t.Fatalf("second Leave should still onExpire, got %v", got)
	}
}

func TestPresenceSweep_ExpiresStale(t *testing.T) {
	var got []string
	p := NewPresence(func(uid string) { got = append(got, uid) })
	t.Cleanup(p.Stop)
	p.Ping("stale")
	p.Ping("fresh")
	p.mu.Lock()
	p.lastPing["stale"] = time.Now().Add(-2 * time.Minute)
	p.mu.Unlock()
	p.sweep()
	if len(got) != 1 || got[0] != "stale" {
		t.Fatalf("sweep should expire only stale, got %v", got)
	}
	p.mu.Lock()
	_, staleOK := p.lastPing["stale"]
	_, freshOK := p.lastPing["fresh"]
	p.mu.Unlock()
	if staleOK {
		t.Fatal("stale should be removed")
	}
	if !freshOK {
		t.Fatal("fresh must remain")
	}
}
