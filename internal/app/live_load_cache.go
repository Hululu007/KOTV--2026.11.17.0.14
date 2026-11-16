package app

import "strings"

// liveLoadScopeCacheKey 直播 load DTO 缓存键：Scope + 源 URL，避免跨用户复用带媒体钥匙的播放地址。
func liveLoadScopeCacheKey(scope, url string) string {
	return strings.TrimSpace(scope) + "\x00" + strings.TrimSpace(url)
}
