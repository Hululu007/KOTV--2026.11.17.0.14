package m3u8

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bobo/KOTV/internal/settings"
)

var headClient = &http.Client{Timeout: 4 * time.Second}

// FilterPlaylist 去广告开着时过滤节目单，相对切片按这一次响应的最终地址补全。
// 过滤把片长砍崩时退回补全后的原文。关闭或不是节目单时原样返回。
func FilterPlaylist(content, base string, headers map[string]string) string {
	if !settings.IsAdFilterEnabled() || !strings.HasPrefix(strings.TrimSpace(content), "#EXTM3U") {
		return content
	}
	text := absolutizeURIs(content, base)
	text = absolutizeTagURIs(text, base)
	cfg := loadFilterConfig()
	var lms []time.Time
	if cfg.Mode == ModeSmart && cfg.UseLastModified && strings.Contains(text, "#EXT-X-DISCONTINUITY") {
		lms = sampleChunkLastModifieds(text, headers)
	}
	filtered := NewFilter(cfg).Apply(text, lms...)
	filtered = absolutizeURIs(filtered, base)
	filtered = absolutizeTagURIs(filtered, base)
	if countSegments(filtered) < 2 && countSegments(text) >= 2 {
		return text
	}
	if dur := sumExtinf(text); dur >= 120 && sumExtinf(filtered) < dur*0.5 {
		return text
	}
	return filtered
}

func loadFilterConfig() FilterConfig {
	cfg := DefaultConfig()
	raw := settings.GetM3U8FilterConfigJSON()
	if raw == "" {
		return cfg
	}
	// 旧 ltxlong JSON：映射到新档位，丢弃序号启发式字段。
	if strings.Contains(raw, "violentFilterModeFlag") || strings.Contains(raw, "tsNameLenExtend") {
		if strings.Contains(raw, `"violentFilterModeFlag":true`) || strings.Contains(raw, `"violentFilterModeFlag": true`) {
			cfg.Mode = ModeMild
		} else {
			cfg.Mode = ModeSmart
		}
		return cfg
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	return cfg.normalized()
}

// sampleChunkLastModifieds 对每个 DISCONTINUITY 段抽一个媒体 URI 做 HEAD，取 Last-Modified。
func sampleChunkLastModifieds(content string, headers map[string]string) []time.Time {
	parts := strings.Split(content, "#EXT-X-DISCONTINUITY")
	out := make([]time.Time, len(parts))
	var wg sync.WaitGroup
	for i, part := range parts {
		uri := firstMediaURI(part)
		if uri == "" {
			continue
		}
		wg.Add(1)
		go func(idx int, u string) {
			defer wg.Done()
			if t, ok := headLastModified(u, headers); ok {
				out[idx] = t
			}
		}(i, uri)
	}
	wg.Wait()
	return out
}

func firstMediaURI(chunk string) string {
	for _, line := range strings.Split(chunk, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		low := strings.ToLower(trim)
		if strings.Contains(low, ".ts") ||
			strings.Contains(low, ".m4s") ||
			strings.Contains(low, ".mp4") ||
			strings.Contains(low, ".jpg") ||
			strings.Contains(low, ".jpeg") ||
			strings.Contains(low, ".png") {
			return trim
		}
	}
	return ""
}

func headLastModified(u string, headers map[string]string) (time.Time, bool) {
	do := func(method string) (time.Time, bool) {
		req, err := http.NewRequest(method, u, nil)
		if err != nil {
			return time.Time{}, false
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", settings.PlayUA())
		}
		if method == http.MethodGet {
			req.Header.Set("Range", "bytes=0-0")
		}
		resp, err := headClient.Do(req)
		if err != nil {
			return time.Time{}, false
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return time.Time{}, false
		}
		lm := resp.Header.Get("Last-Modified")
		if lm == "" {
			return time.Time{}, false
		}
		t, err := http.ParseTime(lm)
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	}
	if t, ok := do(http.MethodHead); ok {
		return t, true
	}
	// 部分 CDN 拒 HEAD，退到 Range GET。
	return do(http.MethodGet)
}

func absolutizeURIs(content, base string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		lines[i] = resolveURL(base, trim)
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
		return `URI="` + resolveURL(base, raw) + `"`
	})
}

func resolveURL(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	bu, err := url.Parse(base)
	if err != nil {
		return ref
	}
	ru, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return bu.ResolveReference(ru).String()
}

func countSegments(content string) int {
	n := 0
	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		n++
	}
	return n
}

func sumExtinf(content string) float64 {
	var sum float64
	for _, m := range extinfRe.FindAllStringSubmatch(content, -1) {
		if len(m) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		sum += v
	}
	return sum
}
