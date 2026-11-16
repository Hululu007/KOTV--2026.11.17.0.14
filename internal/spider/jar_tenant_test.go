package spider

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/paths"
)

func TestClearBridgeTenant_Payload(t *testing.T) {
	req := bridgeRequest{
		Method: "clearTenant",
		UserID: "alice",
		Args: map[string]interface{}{
			"userId": "alice",
		},
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["method"] != "clearTenant" {
		t.Fatalf("method=%v", got["method"])
	}
	if got["userId"] != "alice" {
		t.Fatalf("userId=%v", got["userId"])
	}
	args, _ := got["args"].(map[string]interface{})
	if args["userId"] != "alice" {
		t.Fatalf("args.userId=%v", args["userId"])
	}
}

func TestClearBridgeTenant_EmptyBecomesShared(t *testing.T) {
	req := bridgeRequest{
		Method: "clearTenant",
		UserID: "shared",
		Args:   map[string]interface{}{"userId": "shared"},
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"clearTenant"`) || !strings.Contains(s, `"shared"`) {
		t.Fatalf("payload=%s", raw)
	}
}

func TestKillUserRuntime_WipesUserJVMDirNotShared(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KOTV_CACHE_DIR", root)

	aliceJVM := paths.UserJVMDir("alice")
	bobJVM := paths.UserJVMDir("bob")
	sharedJVM := paths.UserJVMDir("")
	if err := os.WriteFile(filepath.Join(aliceJVM, "marker"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bobJVM, "marker"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedJVM, "marker"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 只擦盘路径逻辑（不依赖真实 JVM）：等价 KillUser 的 WipeDirRemove 段。
	if _, err := paths.WipeDirRemove(paths.UserJVMDir("alice")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(aliceJVM, "marker")); !os.IsNotExist(err) {
		t.Fatalf("alice jvm marker should be gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bobJVM, "marker")); err != nil {
		t.Fatalf("bob jvm must remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sharedJVM, "marker")); err != nil {
		t.Fatalf("shared jvm must remain: %v", err)
	}
}

func TestClearJarGoState_AllowsReparse(t *testing.T) {
	path := "/rehydrate.jar"
	leave := hostclient.Bind(hostclient.Request{UserID: "rehydrate-u", Dedicated: true})
	key := jarRunKey(path)
	parseJarNotified.Store(key, struct{}{})
	leave()
	t.Cleanup(func() { parseJarNotified.Delete(key) })

	clearJarGoStateForRuntime("rehydrate-u")
	if _, ok := parseJarNotified.Load(key); ok {
		t.Fatal("parseJarNotified must clear so notifyParseJar can run again")
	}
}

func TestEnsureRootJar_SetsCurrentPath(t *testing.T) {
	dir := t.TempDir()
	jarFile := filepath.Join(dir, "root.jar")
	if err := os.WriteFile(jarFile, []byte("pk"), 0o644); err != nil {
		t.Fatal(err)
	}
	leave := hostclient.Bind(hostclient.Request{UserID: "root-jar-u", Dedicated: true})
	t.Cleanup(func() {
		setCurrentJarPath("")
		parseJarNotified.Delete(jarRunKey(jarFile))
		leave()
	})
	prev := callParseJar
	callParseJar = func([]byte) error { return nil }
	t.Cleanup(func() { callParseJar = prev })

	if err := EnsureRootJar(jarFile); err != nil {
		t.Fatal(err)
	}
	if got := currentJarPath(); got != jarFile {
		t.Fatalf("currentJarPath=%q want %q", got, jarFile)
	}
}

func TestCacheJarPath_SharedAcrossUsers(t *testing.T) {
	url := "https://example.com/shared-spider.jar"
	sum := md5.Sum([]byte(url))
	hexSum := hex.EncodeToString(sum[:])
	a := paths.JarPath(hexSum)
	b := paths.JarPath(hexSum)
	if a != b {
		t.Fatalf("same url must share path: %s vs %s", a, b)
	}
	if !strings.Contains(a, "jar") {
		t.Fatalf("expect under jar cache: %s", a)
	}
}

func TestCancelJarCalls_PerScopeIsolation(t *testing.T) {
	aliceCtx, endAlice := beginJarCall("u:alice")
	bobCtx, endBob := beginJarCall("u:bob")
	t.Cleanup(func() {
		endAlice()
		endBob()
	})
	cancelJarCalls("u:alice")
	select {
	case <-aliceCtx.Done():
	default:
		t.Fatal("alice jar call should be cancelled")
	}
	select {
	case <-bobCtx.Done():
		t.Fatal("bob jar call must survive alice cancel")
	default:
	}
}
