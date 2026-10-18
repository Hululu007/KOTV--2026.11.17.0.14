package settings

import "testing"

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
