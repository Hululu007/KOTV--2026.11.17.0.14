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
	reconcilePackageCacheOnce = sync.Once{}

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

func TestPackageCaches_PublicNoUID(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KOTV_DATA_DIR", root)
	t.Setenv("KOTV_CACHE_DIR", "")
	reconcilePackageCacheOnce = sync.Once{}

	cache := CacheRoot()
	for _, p := range []string{JarCache(), PyCache(), JsCache(), NodeCache()} {
		if filepath.Dir(p) != cache {
			t.Fatalf("package body must be under CacheRoot: %s (cache=%s)", p, cache)
		}
		if strings.Contains(p, filepath.Join("run", "alice")) || strings.Contains(p, filepath.Join("run", "shared")) {
			t.Fatalf("package body must not use run/<uid>: %s", p)
		}
	}
	jvm := UserJVMDir("alice")
	if !strings.Contains(jvm, filepath.Join("run", "alice", "jvm")) {
		t.Fatalf("UserJVMDir is prefs/files only: %s", jvm)
	}
	if jvm == cache || strings.HasPrefix(JarCache(), jvm+string(filepath.Separator)) {
		t.Fatal("JarCache must not live under UserJVMDir")
	}
}

func TestCacheRoot_PrefersKOTVCacheDir(t *testing.T) {
	root := t.TempDir()
	osCache := filepath.Join(root, "os-cache")
	data := filepath.Join(root, "data")
	t.Setenv("KOTV_CACHE_DIR", osCache)
	t.Setenv("KOTV_DATA_DIR", data)
	reconcilePackageCacheOnce = sync.Once{}

	legacyJar := filepath.Join(data, "cache", "jar")
	if err := os.MkdirAll(legacyJar, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyJar, "shared.jar"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := CacheRoot()
	if filepath.Clean(got) != filepath.Clean(osCache) {
		t.Fatalf("CacheRoot=%s want %s (must match Path/getCacheDir)", got, osCache)
	}
	if filepath.Dir(JarCache()) != filepath.Clean(osCache) {
		t.Fatalf("JarCache under %s", JarCache())
	}
	migrated := filepath.Join(osCache, "jar", "shared.jar")
	if b, err := os.ReadFile(migrated); err != nil || string(b) != "x" {
		t.Fatalf("legacy DATA/cache/jar migrate: %v %q", err, b)
	}
}

func TestReconcilePackageCacheLayout(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KOTV_DATA_DIR", root)
	reconcilePackageCacheOnce = sync.Once{}

	nodeMis := filepath.Join(root, "cache", "js", "pkg", "abc123def456")
	if err := os.MkdirAll(nodeMis, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeMis, "index.js"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	jarMis := filepath.Join(root, "cache", "run", "alice", "jvm", "cache", "jar")
	if err := os.MkdirAll(jarMis, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jarMis, "deadbeef.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	_ = CacheRoot()

	migratedNode := filepath.Join(root, "cache", "node", "abc123def456", "index.js")
	if b, err := os.ReadFile(migratedNode); err != nil || string(b) != "ok" {
		t.Fatalf("node migrate: %v %q", err, b)
	}
	migratedJar := filepath.Join(root, "cache", "jar", "deadbeef.jar")
	if b, err := os.ReadFile(migratedJar); err != nil || string(b) != "jar" {
		t.Fatalf("jar migrate: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(root, "cache", "run", "alice", "jvm", "cache", "jar")); !os.IsNotExist(err) {
		t.Fatalf("legacy jvm/cache/jar should be gone: %v", err)
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
