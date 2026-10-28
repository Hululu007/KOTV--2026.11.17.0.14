package download

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/bobo/KOTV/internal/paths"
	"github.com/bobo/KOTV/internal/settings"
)

func destDir() string {
	d := strings.TrimSpace(settings.Get(settings.DownloadDir))
	if d == "" {
		d = paths.Downloads()
	}
	return paths.Ensure(d)
}

func classifyOn() bool { return settings.Bool(settings.DownloadClassify, true) }

// nameTemplate 把设置里的规则 ID（或旧版手写模板）收成路径模板。
func nameTemplate() string {
	r := strings.TrimSpace(settings.Get(settings.DownloadNameRule))
	switch r {
	case "", "title_flag_ep":
		return "{title}/{flag}_{ep}.{ext}"
	case "title_ep":
		return "{title}/{ep}.{ext}"
	case "flag_ep":
		return "{flag}_{ep}.{ext}"
	case "title_ep_flat":
		return "{title}_{ep}.{ext}"
	}
	if strings.Contains(r, "{") {
		return r
	}
	return "{title}/{flag}_{ep}.{ext}"
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "未命名"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteByte('_')
		// 下载列表按「集名$路径#下一集」播放串打开文件，# / $ / ＄ 会把路径拆坏。
		case r == '#' || r == '$' || r == '＄':
			b.WriteByte('_')
		case unicode.IsControl(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "未命名"
	}
	return out
}

func extFromURL(raw, fallback string) string {
	raw = strings.Split(raw, "?")[0]
	ext := strings.ToLower(filepath.Ext(raw))
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" || len(ext) > 5 {
		if fallback != "" {
			return fallback
		}
		return "mp4"
	}
	if ext == "m3u8" || ext == "m3u" {
		return "mp4"
	}
	return ext
}

// ResolvePath 按命名规则生成落盘绝对路径。
func ResolvePath(title, flag, ep, ext string) string {
	title = sanitizeName(title)
	flag = sanitizeName(flag)
	ep = sanitizeName(ep)
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if ext == "" {
		ext = "mp4"
	}
	rel := nameTemplate()
	rel = strings.ReplaceAll(rel, "{title}", title)
	rel = strings.ReplaceAll(rel, "{flag}", flag)
	rel = strings.ReplaceAll(rel, "{ep}", ep)
	rel = strings.ReplaceAll(rel, "{ext}", ext)
	rel = filepath.FromSlash(rel)
	if !classifyOn() {
		base := filepath.Base(rel)
		if !strings.Contains(base, ".") {
			base = flag + "_" + ep + "." + ext
		}
		rel = base
	}
	dir := destDir()
	full := filepath.Join(dir, rel)
	_ = paths.Ensure(filepath.Dir(full))
	return full
}
