package m3u8

import (
	"regexp"
	"strings"

	"github.com/bobo/KOTV/internal/settings"
	"github.com/bobo/KOTV/internal/util"
)

// FilterPlaylist 去广告开着时过滤节目单，相对切片按这一次响应的最终地址补全。
// 关闭或不是节目单时原样返回。判断不了广告时只补全地址，不改切片。
func FilterPlaylist(content, base string, _ map[string]string) string {
	if !settings.IsAdFilterEnabled() || !strings.HasPrefix(strings.TrimSpace(content), "#EXTM3U") {
		return content
	}
	text := absolutizeTagURIs(absolutizeURIs(content, base), base)
	return Process(text)
}

func absolutizeURIs(content, base string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
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
