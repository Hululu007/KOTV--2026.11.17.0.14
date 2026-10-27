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

func classifyOn() bool {
	v := strings.ToLower(strings.TrimSpace(settings.Get(settings.DownloadClassify)))
	return v == "" || v == "true" || v == "1"
}

func nameRule() string {
	r := strings.TrimSpace(settings.Get(settings.DownloadNameRule))
	if r == "" {
		return "{title}/{flag}_{ep}.{ext}"
	}
	return r
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
	rule := nameRule()
	rel := rule
	rel = strings.ReplaceAll(rel, "{title}", title)
	rel = strings.ReplaceAll(rel, "{flag}", flag)
	rel = strings.ReplaceAll(rel, "{ep}", ep)
	rel = strings.ReplaceAll(rel, "{ext}", ext)
	rel = filepath.FromSlash(rel)
	if !classifyOn() {
		// 关掉按片名分目录时，压成单层文件名。
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
