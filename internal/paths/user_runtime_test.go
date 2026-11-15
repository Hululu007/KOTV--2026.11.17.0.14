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

func TestMigrateLegacyNodePkgCache(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KOTV_DATA_DIR", root)
	// 重置 Once：换临时根后需能再迁（测试专用；生产进程只迁一次）。
	migrateLegacyNodePkgOnce = sync.Once{}

	legacyDir := filepath.Join(root, "cache", "node", "abc123def456")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(legacyDir, "index.js")
	if err := os.WriteFile(marker, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := NodeCache()
	want := filepath.Join(root, "cache", "js", "pkg")
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
