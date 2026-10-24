package settings

import (
	"path/filepath"
	"testing"

	"github.com/bobo/KOTV/internal/database"
)

func TestClampPlayerDropsIINAOffMac(t *testing.T) {
	for _, plat := range []string{"windows", "linux"} {
		vals := map[string]string{
			string(Player):     "outie#iina",
			string(PlayerLive): "outie#iina",
		}
		clampPlayerKeysForPlatform(vals, plat)
		if vals[string(Player)] != "innie#mpv" || vals[string(PlayerLive)] != "innie#mpv" {
			t.Fatalf("%s kept iina: %v", plat, vals)
		}
		vals[string(Player)] = "outie#vlc"
		vals[string(PlayerLive)] = "outie#mpv"
		clampPlayerKeysForPlatform(vals, plat)
		if vals[string(Player)] != "outie#vlc" || vals[string(PlayerLive)] != "outie#mpv" {
			t.Fatalf("%s dropped desktop players: %v", plat, vals)
		}
	}
	vals := map[string]string{
		string(Player):     "outie#iina",
		string(PlayerLive): "outie#vlc",
	}
	clampPlayerKeysForPlatform(vals, "macos")
	if vals[string(Player)] != "outie#iina" || vals[string(PlayerLive)] != "outie#vlc" {
		t.Fatalf("macos: %v", vals)
	}
}

func TestIOSPlayerDefaultIgnoresHostMPV(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "set.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	BindDB(db)
	t.Cleanup(func() { BindDB(nil) })

	vals := map[string]string{
		string(Player):     "innie#mpv",
		string(PlayerLive): "innie#mpv",
	}
	OverlayClientProfile(vals, "ios")
	if vals[string(Player)] != "innie#html" || vals[string(PlayerLive)] != "innie#html" {
		t.Fatalf("ios without profile: %v", vals)
	}

	if err := db.SetSetting("", "p:ios:player", "innie#mpv"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting("", "p:ios:playerLive", "innie#mpv"); err != nil {
		t.Fatal(err)
	}
	vals = map[string]string{
		string(Player):     "innie#html",
		string(PlayerLive): "innie#html",
	}
	OverlayClientProfile(vals, "ios")
	if vals[string(Player)] != "innie#mpv" || vals[string(PlayerLive)] != "innie#mpv" {
		t.Fatalf("ios kept profile: %v", vals)
	}
}

func TestClampAndroidExternalView(t *testing.T) {
	vals := map[string]string{
		string(Player):     "outie#view",
		string(PlayerLive): "outie#mpv",
	}
	clampPlayerKeysForPlatform(vals, "android")
	if vals[string(Player)] != "outie#view" || vals[string(PlayerLive)] != "innie#exo" {
		t.Fatalf("android: %v", vals)
	}
	vals = map[string]string{
		string(Player):     "outie#view",
		string(PlayerLive): "outie#view",
	}
	clampPlayerKeysForPlatform(vals, "windows")
	if vals[string(Player)] != "innie#mpv" || vals[string(PlayerLive)] != "innie#mpv" {
		t.Fatalf("windows: %v", vals)
	}
}
