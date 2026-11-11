package model

import (
	"net/url"
	"strings"
)

// URLIsHls 无网络时的强信号快路径（path / =m3u8），不是正文探测。
// 客户端副本：Dart kotvUrlIsHls、Kotlin KotvMpvHls.urlLooksLikeHls。
//  1. 只认 path 段名含 m3u8；查询值 id=2.m3u8 不算。
//  2. 整串大小写不敏感匹配 =m3u8；=mpd 则否。
//  3. 任意 .php 不算；伪装节目单靠引擎正文嗅探或播放失败再开。
//  4. rtsp/rtspt 否；data: 仅 mime 含 mpegurl/m3u8 才是。
func URLIsHls(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return strings.Contains(strings.ToLower(raw), "=m3u8")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "rtsp" || scheme == "rtspt" {
		return false
	}
	if scheme == "data" {
		return dataURLLooksHls(raw)
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "=m3u8") {
		return true
	}
	if strings.Contains(lower, "=mpd") {
		return false
	}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == "" {
			continue
		}
		if strings.Contains(strings.ToLower(seg), "m3u8") {
			return true
		}
	}
	return false
}

func dataURLLooksHls(raw string) bool {
	body := raw
	if len(body) > 5 {
		body = body[5:]
	}
	mime := body
	if i := strings.IndexByte(body, ','); i >= 0 {
		mime = body[:i]
	}
	mime = strings.ToLower(strings.TrimSpace(mime))
	return strings.Contains(mime, "mpegurl") || strings.Contains(mime, "m3u8")
}
