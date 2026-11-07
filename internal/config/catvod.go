package config

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"
)

// NormalizeCatVodJSON 把猫影视结构（video.sites）并入顶层 sites，供 model.Api 解码。
// 非影视段 read/comic/music/pan 的站点数计入 ignoredNonVideo，不并入 sites。
// 同时存在顶层 sites 与 video.sites 时优先 video.sites；顶层为空时用 video.sites。
func NormalizeCatVodJSON(data string) (cleaned string, ignoredNonVideo int) {
	data = strings.TrimSpace(data)
	if data == "" {
		return data, 0
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &root); err != nil || root == nil {
		return data, 0
	}
	videoSites, hasVideo := extractSites(root["video"])
	topSites, hasTop := extractTopSites(root["sites"])

	for _, key := range []string{"read", "comic", "music", "pan"} {
		if sites, ok := extractSites(root[key]); ok {
			ignoredNonVideo += len(sites)
		}
	}

	switch {
	case hasVideo && (!hasTop || len(topSites) == 0):
		raw, err := json.Marshal(videoSites)
		if err == nil {
			root["sites"] = raw
		}
	case hasVideo && hasTop:
		// 两边都有时优先 video.sites
		raw, err := json.Marshal(videoSites)
		if err == nil {
			root["sites"] = raw
		}
	}

	out, err := json.Marshal(root)
	if err != nil {
		return data, ignoredNonVideo
	}
	return string(out), ignoredNonVideo
}

func extractTopSites(raw json.RawMessage) ([]json.RawMessage, bool) {
	raw = trimRaw(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return nil, false
	}
	var sites []json.RawMessage
	if err := json.Unmarshal(raw, &sites); err != nil {
		return nil, false
	}
	return sites, true
}

func extractSites(section json.RawMessage) ([]json.RawMessage, bool) {
	section = trimRaw(section)
	if len(section) == 0 {
		return nil, false
	}
	if section[0] == '[' {
		var sites []json.RawMessage
		if err := json.Unmarshal(section, &sites); err != nil {
			return nil, false
		}
		return sites, true
	}
	if section[0] != '{' {
		return nil, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(section, &obj); err != nil {
		return nil, false
	}
	return extractTopSites(obj["sites"])
}

func trimRaw(raw json.RawMessage) json.RawMessage {
	return json.RawMessage(strings.TrimSpace(string(raw)))
}

// IsNodePackageURL 配置地址是否为猫爪 Node 源包（路径以 index.js 或 index.js.md5 结尾）。
func IsNodePackageURL(u string) bool {
	u = strings.TrimSpace(u)
	if u == "" {
		return false
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	p := strings.ToLower(path.Clean("/" + strings.TrimPrefix(parsed.Path, "/")))
	p = strings.TrimSuffix(p, "/")
	return strings.HasSuffix(p, "/index.js") || strings.HasSuffix(p, "/index.js.md5") ||
		p == "/index.js" || p == "/index.js.md5"
}
