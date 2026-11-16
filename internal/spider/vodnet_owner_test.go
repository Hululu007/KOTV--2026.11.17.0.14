package spider

import (
	"testing"

	"github.com/bobo/KOTV/internal/hostclient"
)

func TestGoNetConfigPerOwner(t *testing.T) {
	vodNetMu.Lock()
	oldHdr := vodHdrByOwner
	oldHost := vodHostByOwner
	vodHdrByOwner = map[string][]vodHeaderRule{}
	vodHostByOwner = map[string]map[string]string{}
	vodNetMu.Unlock()
	t.Cleanup(func() {
		vodNetMu.Lock()
		vodHdrByOwner = oldHdr
		vodHostByOwner = oldHost
		vodNetMu.Unlock()
	})

	hdrAlice := []byte(`[{"host":"a.example","header":{"X-User":"alice"}}]`)
	hdrBob := []byte(`[{"host":"a.example","header":{"X-User":"bob"}}]`)
	hostsAlice := []byte(`["cdn.example=1.1.1.1"]`)
	hostsBob := []byte(`["cdn.example=2.2.2.2"]`)

	bind := func(uid, cid string) func() {
		return hostclient.Bind(hostclient.Request{UserID: uid, ClientID: cid, Dedicated: true})
	}

	unAlice := bind("alice", "c1")
	setGoNetConfig(hdrAlice, hostsAlice)
	unAlice()

	unBob := bind("bob", "c2")
	setGoNetConfig(hdrBob, hostsBob)
	unBob()

	unAlice = bind("alice", "c1")
	got := InjectVodHeaders("https://a.example/v.m3u8", nil)
	if got["X-User"] != "alice" {
		t.Fatalf("alice headers: %v", got)
	}
	rewritten := RewriteVodURLHost("https://cdn.example/x")
	if rewritten != "https://1.1.1.1/x" {
		t.Fatalf("alice host rewrite: %s", rewritten)
	}
	unAlice()

	unBob = bind("bob", "c2")
	got = InjectVodHeaders("https://a.example/v.m3u8", nil)
	if got["X-User"] != "bob" {
		t.Fatalf("bob headers: %v", got)
	}
	rewritten = RewriteVodURLHost("https://cdn.example/x")
	if rewritten != "https://2.2.2.2/x" {
		t.Fatalf("bob host rewrite: %s", rewritten)
	}
	unBob()

	clearGoNetConfigForUser("alice")
	unAlice = bind("alice", "c1")
	got = InjectVodHeaders("https://a.example/v.m3u8", nil)
	if got["X-User"] != "" {
		t.Fatalf("alice should be cleared: %v", got)
	}
	unAlice()

	unBob = bind("bob", "c2")
	got = InjectVodHeaders("https://a.example/v.m3u8", nil)
	if got["X-User"] != "bob" {
		t.Fatalf("bob should remain: %v", got)
	}
	unBob()
}
