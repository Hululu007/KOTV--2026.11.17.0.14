package settings

import (
	"strings"

	"github.com/bobo/KOTV/internal/hostclient"
)

// 远端租户按「用户 + 前端平台」隔离的设置键（播放器等与客户端能力相关）。
// 存 setting 表：id = p:<platform>:<key>，owner = DataOwner()。
var clientProfileKeys = map[Type]bool{
	Player:             true,
	PlayerLive:         true,
	PlayerSpeed:        true,
	PlayerScale:        true,
	PlayerDecode:       true,
	PlayerRender:       true,
	PlayerFailover:     true,
	PlayerVolume:       true,
	PlayerAmbient:      true,
	PlayerStableVolume: true,
	UA:                 true,
	MpvVulkan:          true,
	MpvGpuNext:         true,
	MpvDirect:          true,
	MpvConf:            true,
}

func platformSettingID(platform, key string) string {
	return "p:" + normalizePlatform(platform) + ":" + key
}

func normalizePlatform(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	switch p {
	case "android", "ios", "windows", "macos", "linux", "web":
		return p
	default:
		if p == "" {
			return "unknown"
		}
		return p
	}
}

// OverlayClientProfile 按「用户+前端平台」覆盖播放器等键，并按前端能力钳制选项。
func OverlayClientProfile(vals map[string]string, platform string) {
	if vals == nil {
		return
	}
	platform = normalizePlatform(platform)
	uid := hostclient.DataOwner()
	loaded := map[Type]bool{}
	if storeDB != nil && (uid != "" || platform == "ios") {
		for t := range clientProfileKeys {
			if uid == "" && t != Player && t != PlayerLive {
				continue
			}
			id := platformSettingID(platform, string(t))
			if v, ok, err := storeDB.GetSetting(uid, id); err == nil && ok {
				vals[string(t)] = v
				loaded[t] = true
			}
		}
	}
	if platform == "ios" {
		if def, _, ok := playerChoices(platform); ok {
			if !loaded[Player] {
				vals[string(Player)] = def
			}
			if !loaded[PlayerLive] {
				vals[string(PlayerLive)] = def
			}
		}
	}
	clampPlayerKeysForPlatform(vals, platform)
}

func clampPlayerKeysForPlatform(vals map[string]string, platform string) {
	for _, key := range []Type{Player, PlayerLive} {
		vals[string(key)] = strings.TrimSpace(vals[string(key)])
	}
	def, allowed, ok := playerChoices(platform)
	if !ok {
		return
	}
	for _, key := range []Type{Player, PlayerLive} {
		vals[string(key)] = clampListedPlayer(vals[string(key)], def, allowed...)
	}
}

func playerChoices(platform string) (def string, allowed []string, ok bool) {
	switch platform {
	case "android":
		return "innie#exo", []string{"innie#exo", "innie#mpv", "innie#fvp", "outie#view"}, true
	case "windows", "linux", "macos":
		// 桌面外置与安卓相同：仅 outie#view，由系统打开。
		return "innie#mpv", []string{"innie#mpv", "innie#fvp", "outie#view"}, true
	case "ios":
		return "innie#html", []string{"innie#html", "innie#mpv", "innie#fvp"}, true
	case "web":
		return "innie#html", []string{"innie#html", "innie#art", "innie#xg", "innie#zw"}, true
	default:
		return "", nil, false
	}
}

func clampListedPlayer(v, def string, allowed ...string) string {
	v = strings.TrimSpace(v)
	// 旧桌面外置名迁到统一的「外部播放器」。
	switch v {
	case "outie#mpv", "outie#vlc", "outie#iina":
		v = "outie#view"
	}
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return def
}

// SetClientProfileKeys 远端租户写入平台画像；返回仍应写普通 setting 的键。
func SetClientProfileKeys(kv map[string]string, platform string) map[string]string {
	if storeDB == nil {
		return kv
	}
	platform = normalizePlatform(platform)
	uid := hostclient.DataOwner()
	if uid == "" && platform != "ios" {
		return kv
	}
	rest := map[string]string{}
	for k, v := range kv {
		t := Type(k)
		if !clientProfileKeys[t] || (uid == "" && t != Player && t != PlayerLive) {
			rest[k] = v
			continue
		}
		_ = storeDB.SetSetting(uid, platformSettingID(platform, k), v)
	}
	return rest
}
