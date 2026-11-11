package model

import (
	"bytes"
	"strings"
)

const (
	// FormatHLS 回包与客户端预强制共用的 HLS mime。
	FormatHLS = "application/x-mpegURL"
	// FormatDASH DASH mime。
	FormatDASH = "application/dash+xml"
)

// FormatIsHls 站源/嗅探/回包里的 format 是否表示 HLS。
func FormatIsHls(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "application/x-mpegurl", "application/vnd.apple.mpegurl",
		"audio/mpegurl", "audio/x-mpegurl":
		return true
	default:
		return false
	}
}

// FormatIsDash 站源/嗅探/回包里的 format 是否表示 DASH。
func FormatIsDash(format string) bool {
	f := strings.ToLower(strings.TrimSpace(format))
	return f == FormatDASH || f == "application/dash+xml" || f == "dash" || f == "mpd"
}

// ClassifyPlayPrefix 根据正文前缀与可选 Content-Type 判定播放容器 mime。
// 命中 HLS/DASH 时返回规范 mime；无法判定或明确非清单时返回空串。
func ClassifyPlayPrefix(head []byte, contentType string) string {
	trimmed := trimPlayPrefix(head)
	if len(trimmed) >= 7 && bytes.Equal(trimmed[:7], []byte("#EXTM3U")) {
		return FormatHLS
	}
	ct := mimeTypeOnly(contentType)
	lower := bytes.ToLower(trimmed)
	if lookLikeDash(lower, ct) {
		return FormatDASH
	}
	// CT 声称 HLS，且正文不像明确的其它容器时采信。
	if contentTypeLooksHls(ct) && !looksLikeNonManifest(trimmed) {
		return FormatHLS
	}
	return ""
}

func trimPlayPrefix(head []byte) []byte {
	head = bytes.TrimPrefix(head, []byte{0xEF, 0xBB, 0xBF})
	return bytes.TrimLeft(head, " \t\r\n")
}

func mimeTypeOnly(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct
}

func contentTypeLooksHls(ct string) bool {
	switch ct {
	case "application/x-mpegurl", "application/vnd.apple.mpegurl",
		"audio/mpegurl", "audio/x-mpegurl":
		return true
	default:
		return false
	}
}

func lookLikeDash(lower []byte, ct string) bool {
	if ct == FormatDASH || strings.Contains(ct, "dash+xml") {
		return true
	}
	n := len(lower)
	if n > 256 {
		n = 256
	}
	return bytes.Contains(lower[:n], []byte("<mpd"))
}

func looksLikeNonManifest(head []byte) bool {
	if len(head) >= 3 && head[0] == 'F' && head[1] == 'L' && head[2] == 'V' {
		return true
	}
	if len(head) >= 8 && bytes.Equal(head[4:8], []byte("ftyp")) {
		return true
	}
	if len(head) >= 8 && bytes.Equal(head[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return true
	}
	return false
}
