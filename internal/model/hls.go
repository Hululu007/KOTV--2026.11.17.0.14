package model

import (
	"net/url"
	"path"
	"strings"
)

// URLIsHls 按协议、路径和查询判断地址是不是 HLS。
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
	if strings.Contains(raw, "=m3u8") {
		return true
	}
	if strings.Contains(raw, "=mpd") {
		return false
	}
	// 任一路径段含 m3u8（getM3u8、/m3u8/xxx）；仅查询参数里的 id=2.m3u8 不算。
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == "" {
			continue
		}
		lower := strings.ToLower(seg)
		if lower == "m3u8" || strings.Contains(lower, "m3u8") {
			return true
		}
		ext := strings.ToLower(path.Ext(seg))
		if ext == ".m3u8" || ext == ".php" {
			return true
		}
	}
	return false
}

