package settings

import (
	"path/filepath"
	"testing"

	"github.com/bobo/KOTV/internal/database"
)

func TestClampPlayerMigratesOldDesktopExternal(t *testing.T) {
	for _, plat := range []string{"windows", "linux", "macos"} {
		vals := map[string]string{
			string(Player):     "outie#iina",
			string(PlayerLive): "outie#vlc",
		}
		clampPlayerKeysForPlatform(vals, plat)
		if vals[string(Player)] != "outie#view" || vals[string(PlayerLive)] != "outie#view" {
			t.Fatalf("%s migrate: %v", plat, vals)
		}
		vals[string(Player)] = "outie#mpv"
		vals[string(PlayerLive)] = "outie#view"
		clampPlayerKeysForPlatform(vals, plat)
		if vals[string(Player)] != "outie#view" || vals[string(PlayerLive)] != "outie#view" {
			t.Fatalf("%s keep view: %v", plat, vals)
		}
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
	// 安卓不允许旧桌面外置名；outie#mpv 迁 view 后仍在白名单。
	if vals[string(Player)] != "outie#view" || vals[string(PlayerLive)] != "outie#view" {
		t.Fatalf("android: %v", vals)
	}
	vals = map[string]string{
		string(Player):     "outie#view",
		string(PlayerLive): "outie#view",
	}
	clampPlayerKeysForPlatform(vals, "windows")
	if vals[string(Player)] != "outie#view" || vals[string(PlayerLive)] != "outie#view" {
		t.Fatalf("windows: %v", vals)
	}
}
