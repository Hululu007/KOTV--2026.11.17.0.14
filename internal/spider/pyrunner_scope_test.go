package spider

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDesktopUtilScopeReadsCall(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3")
	}
	py, err := filepath.Abs("pyrunner.py")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(py); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "empty.py")
	if err := os.WriteFile(scriptPath, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `
import importlib.util, os, sys
sys.argv = ["pyrunner", os.environ["KOTV_TEST_SCRIPT"], "site", "", "http://127.0.0.1:9", os.environ["KOTV_TEST_CACHE"]]
spec = importlib.util.spec_from_file_location("kotv_pyrunner_scope", r"""` + py + `""")
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
import java
util = java.jclass("com.github.catvod.utils.Util")

def check(scope, sid, uid, cid):
    mod._bind_ui_scope(scope)
    got = (util.scopeId(), util.userId(), util.clientId())
    want = (sid, uid, cid)
    if got != want:
        raise SystemExit("scope %r got %r want %r" % (scope, got, want))

check("u:user-1", "u:user-1", "user-1", "u:user-1")
check("c:abc", "c:abc", "", "c:abc")
check("", "", "", "")
check("plain", "plain", "", "plain")
mod._bind_ui_scope("u:deadbeef", "alice")
if util.username() != "alice" or util.tvScopeId() != "u:alice":
    raise SystemExit("username/tvScopeId got %r %r" % (util.username(), util.tvScopeId()))
print("ok")
`
	cmd := exec.Command("python3", "-c", script)
	cmd.Env = append(os.Environ(),
		"PYTHONDONTWRITEBYTECODE=1",
		"KOTV_TEST_SCRIPT="+scriptPath,
		"KOTV_TEST_CACHE="+cache,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python: %v\n%s", err, out)
	}
}
