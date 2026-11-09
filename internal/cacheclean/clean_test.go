package cacheclean

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSweepRootKeepsUIAndTV(t *testing.T) {
	root := t.TempDir()
	mustMk := func(name string) string {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	mustFile := func(dir, name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ui := mustMk("ui")
	tv := mustMk("TV")
	db := mustMk("db")
	cache := mustMk("data")
	mustFile(ui, "client.json", `{"clientId":"abc"}`)
	mustFile(tv, ".cookie", "keep")
	mustFile(db, "x.db", "db")
	mustFile(cache, "tmp.bin", "gone")

	if _, err := sweepRootKeepConfigAndDB(root); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(ui, "client.json"),
		filepath.Join(tv, ".cookie"),
		filepath.Join(db, "x.db"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("should keep %s: %v", p, err)
		}
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("data/ should be removed, err=%v", err)
	}
}
