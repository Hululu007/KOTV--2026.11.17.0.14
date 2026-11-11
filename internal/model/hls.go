package model

import (
	"net/url"
	"path"
	"strings"
)

// URLIsHls 地址是否像 HLS（仅强信号快路径）。
// 引擎侧优先站源 format / 正文嗅探写准 format；本函数只作无网络时的预判。
// 与 Dart kotvMpvUrlIsHls、Kotlin KotvMpvHls.urlLooksLikeHls 同一套。
// 扩展名与路径段只看 path（查询里的 id=2.m3u8 不算）；任意 .php 不算。
func URLIsHls(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return strings.Contains(raw, "=m3u8")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "rtsp" || scheme == "rtspt" {
		return false
	}
	if scheme == "data" {
		body := ""
		if len(raw) > 5 {
			body = raw[5:]
		}
		return !strings.HasPrefix(body, "application/dash+xml")
	}
	// file=m3u8 / type=m3u8；id=2.m3u8 不含子串 =m3u8，不会误中。
	if strings.Contains(raw, "=m3u8") {
		return true
	}
	if strings.Contains(raw, "=mpd") {
		return false
	}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == "" {
			continue
		}
		lower := strings.ToLower(seg)
		if lower == "m3u8" || strings.Contains(lower, "m3u8") {
			return true
		}
		if strings.ToLower(path.Ext(seg)) == ".m3u8" {
			return true
		}
	}
	return false
}

