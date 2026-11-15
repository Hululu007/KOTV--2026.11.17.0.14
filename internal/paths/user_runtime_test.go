package paths

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestUserRuntimeDirs(t *testing.T) {
	if RuntimeOwnerLabel("") != "shared" {
		t.Fatalf("empty -> shared")
	}
	if RuntimeOwnerLabel("u1") != "u1" {
		t.Fatalf("uid label")
	}
	shared := UserRuntimeDir("")
	user := UserRuntimeDir("alice")
	if !strings.Contains(shared, filepath.Join("run", "shared")) {
		t.Fatalf("shared dir=%s", shared)
	}
	if !strings.Contains(user, filepath.Join("run", "alice")) {
		t.Fatalf("user dir=%s", user)
	}
	if UserJVMDir("alice") != filepath.Join(user, "jvm") {
		t.Fatalf("jvm subdir")
	}
	if UserPyWorkDir("alice") != filepath.Join(user, "py") {
		t.Fatalf("py subdir")
	}
	if UserChromiumDir("alice") != filepath.Join(user, "chromium") {
		t.Fatalf("chromium subdir")
	}
	if UserJsLocalDir("alice") != filepath.Join(user, "js-local") {
		t.Fatalf("js-local subdir")
	}
	if UserNodeDataDir("alice", "pkg1") != filepath.Join(user, "node", "pkg1", "data") {
		t.Fatalf("node data subdir")
	}
	if !strings.HasSuffix(JsModuleCache(), filepath.Join("js", "mod")) &&
		!strings.Contains(JsModuleCache(), filepath.Join("js", "mod")) {
		t.Fatalf("js module cache=%s", JsModuleCache())
	}
}

func TestNodeCache_SiblingOfJarPy(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KOTV_DATA_DIR", root)
	migrateMisplacedNodePkgOnce = sync.Once{}

	got := NodeCache()
	want := filepath.Join(root, "cache", "node")
	if got != want {
		t.Fatalf("NodeCache=%s want %s (应与 jar/py 同级)", got, want)
	}
	jar := JarCache()
	py := PyCache()
	js := JsCache()
	if filepath.Dir(got) != filepath.Dir(jar) || filepath.Dir(got) != filepath.Dir(py) || filepath.Dir(got) != filepath.Dir(js) {
		t.Fatalf("package caches must share CacheRoot: node=%s jar=%s py=%s js=%s", got, jar, py, js)
	}
}

func TestMigrateMisplacedNodePkgCache(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KOTV_DATA_DIR", root)
	migrateMisplacedNodePkgOnce = sync.Once{}

	misDir := filepath.Join(root, "cache", "js", "pkg", "abc123def456")
	if err := os.MkdirAll(misDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(misDir, "index.js")
	if err := os.WriteFile(marker, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := NodeCache()
	want := filepath.Join(root, "cache", "node")
	if got != want {
		t.Fatalf("NodeCache=%s want %s", got, want)
	}
	migrated := filepath.Join(want, "abc123def456", "index.js")
	b, err := os.ReadFile(migrated)
	if err != nil || string(b) != "ok" {
		t.Fatalf("migrated content: %v %q", err, b)
	}
}

func TestWipeAndRemoveUserRuntime(t *testing.T) {
	jvm := UserJVMDir("wipe-u1")
	marker := filepath.Join(jvm, "pref.dat")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := WipeUserRuntimeSubdirs("jvm")
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected freed >0, got %d", n)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker should be gone: %v", err)
	}
	if st, err := os.Stat(jvm); err != nil || !st.IsDir() {
		t.Fatalf("jvm dir should remain: %v", err)
	}
	raw := filepath.Join(userRuntimeRootPath(), "wipe-u1")
	if err := RemoveUserRuntimeDir("wipe-u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(raw); !os.IsNotExist(err) {
		t.Fatalf("user runtime dir should be removed: %v", err)
	}
	if err := RemoveUserRuntimeDir(""); err != nil {
		t.Fatal(err)
	}
}
