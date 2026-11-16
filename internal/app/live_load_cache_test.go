package app

import "testing"

func TestLiveLoadScopeCacheKey_IsolatesScopes(t *testing.T) {
	url := "http://live.example/list.m3u"
	a := liveLoadScopeCacheKey("u:alice", url)
	b := liveLoadScopeCacheKey("u:bob", url)
	if a == b {
		t.Fatal("same url under different scopes must not share cache key")
	}
	if a != liveLoadScopeCacheKey("u:alice", url) {
		t.Fatal("same scope+url must be stable")
	}
	host := liveLoadScopeCacheKey("", url)
	if host == a {
		t.Fatal("host empty scope must differ from dedicated scope")
	}
}
