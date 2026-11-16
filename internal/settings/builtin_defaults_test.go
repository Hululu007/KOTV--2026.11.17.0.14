package settings

import (
	"path/filepath"
	"testing"

	"github.com/bobo/KOTV/internal/database"
	"github.com/bobo/KOTV/internal/hostclient"
)

func TestRemoteGetMissUsesBuiltinNotHostMemory(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "set.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	BindDB(db)
	t.Cleanup(func() { BindDB(nil) })

	builtin := defaultValue(string(Theme))
	if builtin == "" {
		t.Fatal("theme builtin empty")
	}
	hostVal := "dark"
	if hostVal == builtin {
		hostVal = "light"
	}
	Set(Theme, hostVal)
	if Get(Theme) != hostVal {
		t.Fatalf("host Get: got %q want %q", Get(Theme), hostVal)
	}

	defer hostclient.Bind(hostclient.Request{UserID: "u-remote", Dedicated: true})()
	got := Get(Theme)
	if got != builtin {
		t.Fatalf("remote miss: got %q want builtin %q (not host %q)", got, builtin, hostVal)
	}
}
