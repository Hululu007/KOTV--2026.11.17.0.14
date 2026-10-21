package spider

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobo/KOTV/internal/hostclient"
)

func TestCacheJarLocalFile(t *testing.T) {
	dir := t.TempDir()
	jar := filepath.Join(dir, "spider.jar")
	// 超过 downloadBinary 的 64 字节门槛；本地直载也要非空。
	payload := make([]byte, 128)
	for i := range payload {
		payload[i] = byte(i)
	}
	if err := os.WriteFile(jar, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	raw := (&url.URL{Scheme: "file", Path: filepath.ToSlash(jar)}).String()
	got, err := cacheJar(raw, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(jar) {
		t.Fatalf("got %q want %q", got, jar)
	}

	// 相对本地配置目录
	cfg := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfg, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = cacheJar("spider.jar", "file://"+filepath.ToSlash(cfg), false)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(jar) {
		t.Fatalf("relative got %q want %q", got, jar)
	}
}

func TestJavaInvokePayloadKeepsLong(t *testing.T) {
	raw := []byte(`{"op":"call","handle":1,"name":"set","args":[{"t":"n","v":9223372036854775807}]}`)
	payload, err := javaInvokePayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "9223372036854775807") {
		t.Fatalf("long lost: %s", payload)
	}
	var decoded struct {
		Method string          `json:"method"`
		Args   json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Method != "javaInvoke" || string(decoded.Args) != string(raw) {
		t.Fatalf("payload = %s", payload)
	}
}

func TestJavaInvokePayloadUsesBodyScopeWhenUnbound(t *testing.T) {
	raw := []byte(`{"op":"call","clientId":"u:user-1"}`)
	payload, err := javaInvokePayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		ClientID string          `json:"clientId"`
		UserID   string          `json:"userId"`
		RemoteUI bool            `json:"remoteUi"`
		Args     json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ClientID != "u:user-1" || decoded.UserID != "user-1" || !decoded.RemoteUI {
		t.Fatalf("scope lost: %s", payload)
	}
	if string(decoded.Args) != string(raw) {
		t.Fatalf("args rewritten: %s", decoded.Args)
	}

	raw = []byte(`{"op":"call","clientId":"c:abc"}`)
	payload, err = javaInvokePayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	decoded = struct {
		ClientID string          `json:"clientId"`
		UserID   string          `json:"userId"`
		RemoteUI bool            `json:"remoteUi"`
		Args     json.RawMessage `json:"args"`
	}{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ClientID != "c:abc" || decoded.UserID != "" || decoded.RemoteUI {
		t.Fatalf("client scope: %s", payload)
	}
}

func TestJavaInvokePayloadKeepsGoroutineScope(t *testing.T) {
	done := hostclient.EnterSession("local-win", "", false)
	defer done()
	raw := []byte(`{"op":"call","clientId":"u:other"}`)
	payload, err := javaInvokePayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		ClientID string `json:"clientId"`
		UserID   string `json:"userId"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ClientID != "c:local-win" || decoded.UserID != "" {
		t.Fatalf("goroutine scope replaced: %s", payload)
	}
}
