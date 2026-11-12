package m3u8

import (
	"regexp"
	"strings"

	"github.com/bobo/KOTV/internal/settings"
	"github.com/bobo/KOTV/internal/util"
)

// FilterPlaylist 去广告开启且正文为节目单时：先补全相对地址，再 Process。
// 关闭、非节目单、或判不出广告时不改切片（仍可能只做地址补全）。
func FilterPlaylist(content, base string) string {
	if !settings.IsAdFilterEnabled() || !strings.HasPrefix(strings.TrimSpace(content), "#EXTM3U") {
		return content
	}
	return Process(absolutizePlaylist(content, base))
}

func absolutizePlaylist(content, base string) string {
	text := absolutizeURIs(content, base)
	return absolutizeTagURIs(text, base)
}

func absolutizeURIs(content, base string) string {
	lines := splitLines(content)
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if !isSegmentURI(trim) {
			continue
		}
		lines[i] = util.AbsHTTPRef(base, trim)
	}
	return strings.Join(lines, "\n")
}

// tagURIRe 匹配 #EXT-X-KEY / #EXT-X-MAP 等行里的 URI="…"。
var tagURIRe = regexp.MustCompile(`(?i)\bURI="([^"]+)"`)

func absolutizeTagURIs(content, base string) string {
	if base == "" || !strings.Contains(strings.ToUpper(content), "URI=") {
		return content
	}
	return tagURIRe.ReplaceAllStringFunc(content, func(m string) string {
		sub := tagURIRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		raw := sub[1]
		if raw == "" || strings.HasPrefix(raw, "data:") {
			return m
		}
		if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
			return m
		}
		return `URI="` + util.AbsHTTPRef(base, raw) + `"`
	})
}
