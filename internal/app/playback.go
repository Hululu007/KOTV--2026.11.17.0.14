package app

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/bobo/KOTV/internal/database"
	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/localproxy"
	"github.com/bobo/KOTV/internal/paths"
	"github.com/bobo/KOTV/internal/playproxy"
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
// 引擎本机绝对路径也算（含 Windows 盘符路径，如下载目录里的文件）。
func IsLocalMediaURL(u string) bool {
	u = strings.TrimSpace(u)
	if u == "" {
		return false
	}
	low := strings.ToLower(u)
	if strings.HasPrefix(low, "file:") || strings.HasPrefix(low, "content:") {
		return true
	}
	if strings.HasPrefix(u, "/") || filepath.IsAbs(u) {
		_, ok := localMediaExt[strings.ToLower(filepath.Ext(u))]
		return ok
	}
	return false
}

// IsEphemeralPlayURL 是否为引擎登记的临时代理 HTTP 地址（id 会过期，勿写入历史/跨会话复用）。
// 播放器只认 URL，不能改成引擎内函数回调；登记表在引擎内存，对外仍是可打开的 HTTP。
func IsEphemeralPlayURL(u string) bool {
	u = strings.ToLower(strings.TrimSpace(u))
	return strings.Contains(u, "/proxy/play?") ||
		strings.Contains(u, "/proxy/cached_m3u8") ||
		strings.Contains(u, "/proxy/rolling_m3u8") ||
		strings.Contains(u, "/proxy/hls/") ||
		strings.Contains(u, "/proxy/bt/")
}

// PreparePlaybackURL 将媒体地址转成当前会话可播的地址（不正文嗅探）。
func (a *App) PreparePlaybackURL(raw string, headers map[string]string) string {
	play, _, _ := a.PreparePlayback(raw, headers)
	return play
}

// PreparePlayback 点播地址准备：HLS 预登记透传（Direct 为原址）；不发嗅探请求。
// 完整「嗅探写 format + 透传」请用 planPlayback（APIPlay）。
func (a *App) PreparePlayback(raw string, headers map[string]string) (string, string, map[string]string) {
	_, isHls := decidePlayFormat(raw, "", headers, false)
	return a.preparePlayback(raw, headers, true, isHls)
}

// PrepareLivePlayback 直播开播、回看。不走去广告，也不因 HLS 改走透传。
func (a *App) PrepareLivePlayback(raw string, headers map[string]string) (string, string, map[string]string) {
	return a.preparePlayback(raw, headers, false, false)
}

// PreparePlaybackListed 直播列表批量用。与开播同一规则，不拉流、不嗅探。
func (a *App) PreparePlaybackListed(raw string, headers map[string]string) (string, string, map[string]string) {
	return a.PrepareLivePlayback(raw, headers)
}

func (a *App) preparePlayback(raw string, headers map[string]string, vod, isHls bool) (string, string, map[string]string) {
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
	if !thunder.Match(raw) {
		if localproxy.IsSpiderProxyURL(raw) {
			return playproxy.PublicizeURL(raw, headers), "", headers
		}
		if vod {
			if reg, filter := shouldRegisterVodHls(isHls); reg {
				return registerPlayback(raw, headers, filter)
			}
		}
	}
	return playproxy.PublicizeURL(raw, headers), "", headers
}

// registerPlayback 登记透传；Direct 为登记前的公网原址（远端经 Publicize）。
func registerPlayback(raw string, headers map[string]string, filter bool) (string, string, map[string]string) {
	direct := playproxy.PublicizeURL(raw, headers)
	play := playproxy.RegisterAlways(raw, headers, filter)
	return playproxy.PublicizeURL(play, headers), direct, headers
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
