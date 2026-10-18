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
	if uid != "" && storeDB != nil {
		for t := range clientProfileKeys {
			id := platformSettingID(platform, string(t))
			if v, ok, err := storeDB.GetSetting(uid, id); err == nil && ok {
				vals[string(t)] = v
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
		return "innie#exo", []string{"innie#exo", "innie#mpv", "innie#fvp"}, true
	case "windows", "linux":
		return "innie#mpv", []string{"innie#mpv", "innie#fvp", "outie#mpv", "outie#vlc"}, true
	case "macos":
		return "innie#mpv", []string{"innie#mpv", "innie#fvp", "outie#mpv", "outie#vlc", "outie#iina"}, true
	case "ios":
		return "innie#mpv", []string{"innie#mpv", "innie#fvp", "innie#html"}, true
	case "web":
		return "innie#html", []string{"innie#html", "innie#art", "innie#xg", "innie#zw"}, true
	default:
		return "", nil, false
	}
}

func clampListedPlayer(v, def string, allowed ...string) string {
	v = strings.TrimSpace(v)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return def
}

// SetClientProfileKeys 远端租户写入平台画像；返回仍应写普通 setting 的键。
func SetClientProfileKeys(kv map[string]string, platform string) map[string]string {
	uid := hostclient.DataOwner()
	if uid == "" || storeDB == nil {
		return kv
	}
	platform = normalizePlatform(platform)
	rest := map[string]string{}
	for k, v := range kv {
		t := Type(k)
		if clientProfileKeys[t] {
			_ = storeDB.SetSetting(uid, platformSettingID(platform, k), v)
			continue
		}
		rest[k] = v
	}
	return rest
}
