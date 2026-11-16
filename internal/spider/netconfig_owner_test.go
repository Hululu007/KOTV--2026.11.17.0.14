package spider

import (
	"testing"
)

func TestCurrentAllNetConfig_IncludesEveryScope(t *testing.T) {
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

	all := currentAllNetConfig()
	got := map[string]bool{}
	for _, b := range all {
		got[string(b)] = true
	}
	for _, want := range []string{
		`{"def":1}`, `{"proxy":"def"}`, `{"localProxy":1}`, `{"local":1}`,
		`{"alice":1}`, `{"bob":1}`, `{"proxy":"alice"}`, `{"proxy":"bob"}`,
	} {
		if !got[want] {
			t.Fatalf("single JVM replay missing %s: %v", want, got)
		}
	}
}

func TestClearNetConfigForUser(t *testing.T) {
	netConfigMu.Lock()
	oldBy := netConfigByClient
	oldProxyBy := userProxyByClient
	netConfigByClient = map[string][]byte{
		"u:alice": []byte(`{"alice":1}`),
		"u:bob":   []byte(`{"bob":1}`),
		"c:win":   []byte(`{"local":1}`),
	}
	userProxyByClient = map[string][]byte{
		"u:alice": []byte(`{"proxy":"alice"}`),
		"u:bob":   []byte(`{"proxy":"bob"}`),
	}
	netConfigMu.Unlock()
	t.Cleanup(func() {
		netConfigMu.Lock()
		netConfigByClient = oldBy
		userProxyByClient = oldProxyBy
		netConfigMu.Unlock()
	})

	clearNetConfigForUser("alice")
	netConfigMu.Lock()
	defer netConfigMu.Unlock()
	if _, ok := netConfigByClient["u:alice"]; ok {
		t.Fatal("alice net should be gone")
	}
	if _, ok := userProxyByClient["u:alice"]; ok {
		t.Fatal("alice proxy should be gone")
	}
	if _, ok := netConfigByClient["u:bob"]; !ok {
		t.Fatal("bob net must remain")
	}
	if _, ok := netConfigByClient["c:win"]; !ok {
		t.Fatal("local c: net must remain")
	}
}
