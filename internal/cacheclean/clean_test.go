package cacheclean

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSweepRootKeepsClientJSONAndUserTV(t *testing.T) {
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
	sc := mustMk(filepath.Join("TV", "s_c_deadbeef"))
	suUUID := mustMk(filepath.Join("TV", "s_u_0123456789abcdef0123456789abcdef"))
	su := mustMk(filepath.Join("TV", "s_u_user1"))
	db := mustMk("db")
	cache := mustMk("data")
	mustFile(ui, "client.json", `{"clientId":"abc"}`)
	mustFile(ui, "junk.log", "gone")
	mustFile(tv, ".cookie", "keep")
	mustFile(sc, ".old", "drop")
	mustFile(suUUID, ".old-uuid", "drop")
	mustFile(su, ".user", "keep-user")
	mustFile(db, "x.db", "db")
	mustFile(cache, "tmp.bin", "gone")

	if _, err := sweepRootKeepConfigAndDB(root); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(ui, "client.json"),
		filepath.Join(tv, ".cookie"),
		filepath.Join(su, ".user"),
		filepath.Join(db, "x.db"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("should keep %s: %v", p, err)
		}
	}
	for _, p := range []string{
		filepath.Join(ui, "junk.log"),
		sc,
		suUUID,
		cache,
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("should remove %s, err=%v", p, err)
		}
	}
}
