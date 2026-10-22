package app

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/bobo/KOTV/internal/database"
	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/localproxy"
	"github.com/bobo/KOTV/internal/paths"
	"github.com/bobo/KOTV/internal/playproxy"
	"github.com/bobo/KOTV/internal/settings"
	"github.com/bobo/KOTV/internal/thunder"
)

// 本地媒体后缀（csp_Local / 推送绝对路径）。嗅探规则只认 http(s)，这里单独放行。
var localMediaExt = map[string]struct{}{
	".mp4": {}, ".mkv": {}, ".avi": {}, ".mov": {}, ".flv": {}, ".webm": {},
	".ts": {}, ".m2ts": {}, ".mts": {}, ".m4v": {}, ".wmv": {}, ".iso": {},
	".rmvb": {}, ".rm": {}, ".3gp": {}, ".f4v": {}, ".asf": {}, ".ogv": {},
	".mp3": {}, ".m4a": {}, ".aac": {}, ".flac": {}, ".wav": {}, ".ogg": {},
	".m3u8": {}, ".mpd": {},
}

// IsLocalMediaURL 是否为本地文件/内容 URI；是则直喂播放器，不要包 HTTP 代理。
func IsLocalMediaURL(u string) bool {
	u = strings.TrimSpace(u)
	if u == "" {
		return false
	}
	low := strings.ToLower(u)
	if strings.HasPrefix(low, "file:") || strings.HasPrefix(low, "content:") {
		return true
	}
	if strings.HasPrefix(u, "/") {
		ext := strings.ToLower(path.Ext(u))
		if ext == "" {
			return false
		}
		_, ok := localMediaExt[ext]
		return ok
	}
	return false
}

// IsEphemeralPlayURL 是否为进程内临时代理地址（不应写入历史或跨会话复用）。
func IsEphemeralPlayURL(u string) bool {
	u = strings.ToLower(strings.TrimSpace(u))
	return strings.Contains(u, "/proxy/play?") ||
		strings.Contains(u, "/proxy/cached_m3u8") ||
		strings.Contains(u, "/proxy/rolling_m3u8") ||
		strings.Contains(u, "/proxy/hls/") ||
		strings.Contains(u, "/proxy/bt/")
}

// PreparePlaybackURL 将媒体地址转成当前会话可播的地址。
func (a *App) PreparePlaybackURL(raw string, headers map[string]string) string {
	play, _, _ := a.PreparePlayback(raw, headers)
	return play
}

// PreparePlayback 返回透传地址、播放器能自己打开的原地址，以及请求头。
// 点播在去广告打开且是 m3u8 时，只给 /proxy/play，过滤发生在这条透传里。
// 其它时候原地址留给带得了请求头的播放器；带不了的用 /proxy/play。
func (a *App) PreparePlayback(raw string, headers map[string]string) (string, string, map[string]string) {
	return a.preparePlayback(raw, headers, prepareVOD)
}

// PrepareLivePlayback 直播开播、回看。不走去广告。有请求头时登记透传，同时把原地址留给带得了头的播放器。
func (a *App) PrepareLivePlayback(raw string, headers map[string]string) (string, string, map[string]string) {
	return a.preparePlayback(raw, headers, prepareLive)
}

// PreparePlaybackListed 直播列表批量用。有请求头的线路返回空串，由开播接口再登记，列表本身不拉流。
func (a *App) PreparePlaybackListed(raw string, headers map[string]string) (string, string, map[string]string) {
	return a.preparePlayback(raw, headers, prepareList)
}

type prepareMode int

const (
	prepareVOD prepareMode = iota
	prepareLive
	prepareList
)

func (a *App) preparePlayback(raw string, headers map[string]string, mode prepareMode) (string, string, map[string]string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || IsEphemeralPlayURL(raw) {
		return raw, "", headers
	}
	if IsLocalMediaURL(raw) {
		return exposeLocalFile(raw), "", headers
	}
	raw = localproxy.ConvertScheme(raw)
	if playproxy.ShouldExpandSpiderMediaProxy() {
		if media, hdrs, ok := playproxy.ExpandSpiderMediaProxy(raw, headers); ok {
			raw = media
			headers = hdrs
		}
	}
	direct := ""
	if !thunder.Match(raw) {
		if localproxy.IsSpiderProxyURL(raw) {
			return playproxy.PublicizeURL(raw, headers), "", headers
		}
		headed := hasPlaybackHeaders(headers)
		filter := mode == prepareVOD && vodAdFilter(raw)
		if mode == prepareList {
			if headed {
				return "", "", headers
			}
		} else if filter || headed {
			before := raw
			raw = playproxy.RegisterAlways(raw, headers, filter)
			if !filter {
				direct = directPlayable(before)
			}
		}
	}
	return playproxy.PublicizeURL(raw, headers), direct, headers
}

func vodAdFilter(raw string) bool {
	return settings.IsAdFilterEnabled() && strings.Contains(strings.ToLower(raw), "m3u8")
}

func hasPlaybackHeaders(headers map[string]string) bool {
	for k, v := range headers {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// directPlayable 播放器自己打得开的 http(s)。回环和本机代理留空。
func directPlayable(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "::1" || host == "127.0.0.1" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return ""
	}
	if u.Path == "/proxy" || strings.HasPrefix(u.Path, "/proxy/") {
		return ""
	}
	return raw
}

// exposeLocalFile 远端前端播放时，把本机文件改成 /file/ 地址。本机请求保持原路径。content: 不改。
func exposeLocalFile(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || hostclient.PublicBase() == "" {
		return raw
	}
	if strings.HasPrefix(strings.ToLower(raw), "content:") {
		return raw
	}
	if !IsLocalMediaURL(raw) {
		return raw
	}
	resolved := paths.ResolveMediaPath(raw)
	if resolved == "" {
		return raw
	}
	rel := strings.TrimPrefix(filepath.ToSlash(resolved), "/")
	parts := strings.Split(rel, "/")
	enc := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		enc = append(enc, url.PathEscape(p))
	}
	if len(enc) == 0 {
		return raw
	}
	return strings.TrimRight(hostclient.PublicBase(), "/") + "/file/" + strings.Join(enc, "/")
}

// PlayHistory 用历史里保存的原始媒体地址起播（会重新走 m3u8/代理，不依赖旧缓存 id）。
func (a *App) PlayHistory(h database.History) error {
	if strings.TrimSpace(h.EpisodeURL) == "" {
		return fmt.Errorf("历史无播放地址")
	}
	if IsEphemeralPlayURL(h.EpisodeURL) {
		return fmt.Errorf("历史地址已过期")
	}
	url := a.PreparePlaybackURL(h.EpisodeURL, nil)
	if url == "" {
		return fmt.Errorf("无法准备播放地址")
	}
	return PlayURLWithHistory(url, h.Key)
}
