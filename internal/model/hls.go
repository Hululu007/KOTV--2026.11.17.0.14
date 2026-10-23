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
	seg := lastNonEmptySegment(u.Path)
	if seg == "" {
		return false
	}
	if strings.EqualFold(seg, "m3u8") {
		return true
	}
	ext := strings.ToLower(path.Ext(seg))
	return ext == ".m3u8" || ext == ".php"
}

func lastNonEmptySegment(p string) string {
	parts := strings.Split(p, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return ""
}
