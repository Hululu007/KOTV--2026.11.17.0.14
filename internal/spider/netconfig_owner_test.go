package spider

import (
	"testing"
)

func TestCurrentNetConfigFor_FiltersByOwner(t *testing.T) {
	netConfigMu.Lock()
	oldDef := netConfigJSON
	oldBy := netConfigByClient
	oldProxy := userProxyJSON
	oldProxyBy := userProxyByClient
	oldLocal := localProxyJSON
	netConfigJSON = []byte(`{"def":1}`)
	netConfigByClient = map[string][]byte{
		"u:alice": []byte(`{"alice":1}`),
		"u:bob":   []byte(`{"bob":1}`),
		"c:win":   []byte(`{"local":1}`),
	}
	userProxyJSON = []byte(`{"proxy":"def"}`)
	userProxyByClient = map[string][]byte{
		"u:alice": []byte(`{"proxy":"alice"}`),
		"u:bob":   []byte(`{"proxy":"bob"}`),
	}
	localProxyJSON = []byte(`{"localProxy":1}`)
	netConfigMu.Unlock()
	t.Cleanup(func() {
		netConfigMu.Lock()
		netConfigJSON = oldDef
		netConfigByClient = oldBy
		userProxyJSON = oldProxy
		userProxyByClient = oldProxyBy
		localProxyJSON = oldLocal
		netConfigMu.Unlock()
	})

	shared := currentNetConfigFor("")
	got := map[string]bool{}
	for _, b := range shared {
		got[string(b)] = true
	}
	if !got[`{"def":1}`] || !got[`{"proxy":"def"}`] || !got[`{"localProxy":1}`] || !got[`{"local":1}`] {
		t.Fatalf("shared missing defaults/local c: : %v", got)
	}
	if got[`{"alice":1}`] || got[`{"bob":1}`] {
		t.Fatalf("shared must not include tenant net: %v", got)
	}

	alice := currentNetConfigFor("alice")
	got = map[string]bool{}
	for _, b := range alice {
		got[string(b)] = true
	}
	if !got[`{"alice":1}`] || !got[`{"proxy":"alice"}`] || !got[`{"localProxy":1}`] {
		t.Fatalf("alice missing own net: %v", got)
	}
	if got[`{"bob":1}`] || got[`{"proxy":"bob"}`] || got[`{"def":1}`] {
		t.Fatalf("alice must not get others: %v", got)
	}
}
