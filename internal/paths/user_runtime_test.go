package paths

import (
	"path/filepath"
	"strings"
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
