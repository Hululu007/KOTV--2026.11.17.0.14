package paths

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// TV 站点凭证目录（与 bridge Util.tvDirName / Path.tv 一致）：
// 本机 → {root}/TV/；远端 → {root}/TV/s_u_<用户名>/。

// TVRoots 可能存放 /TV 的根（数据根；安卓另加外置存储）。
func TVRoots() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "" || p == "." {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	add(filepath.Join(Root(), "TV"))
	if ext := externalStorageRoot(); ext != "" {
		add(filepath.Join(ext, "TV"))
	}
	return out
}

// TVDirName 与 Util.tvDirName 一致：返回 "u_<用户名>"；本机/空返回 ""。
func TVDirName(username string) string {
	name := TVUsername(username)
	if name == "" {
		return ""
	}
	return "u_" + name
}

// TVUserSeg 目录名 "s_u_<用户名>"；空用户名返回 ""。
func TVUserSeg(username string) string {
	seg := TVDirName(username)
	if seg == "" {
		return ""
	}
	return "s_" + seg
}

// TVUsername 抽出可落盘用户名（与 Util.tvUsernameOf / sanitize 对齐）。
func TVUsername(scopeOrName string) string {
	t := strings.TrimSpace(scopeOrName)
	if t == "" || strings.HasPrefix(t, "c:") || strings.HasPrefix(t, "c_") {
		return ""
	}
	switch {
	case strings.HasPrefix(t, "u:"):
		t = t[2:]
	case strings.HasPrefix(t, "s_u_"):
		t = t[4:]
	case strings.HasPrefix(t, "u_"):
		t = t[2:]
	}
	return sanitizeTVUsername(t)
}

func sanitizeTVUsername(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '#', '$':
			b.WriteByte('_')
		default:
			if unicode.IsControl(r) {
				continue
			}
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	for strings.HasPrefix(out, ".") {
		out = strings.TrimSpace(strings.TrimPrefix(out, "."))
	}
	if len([]rune(out)) > 64 {
		rs := []rune(out)
		out = string(rs[:64])
	}
	return strings.TrimSpace(out)
}

// RemoveTVUserDirs 删除各 TV 根下该用户的 s_u_<用户名> 目录。
func RemoveTVUserDirs(username string) {
	seg := TVUserSeg(username)
	if seg == "" {
		return
	}
	for _, root := range TVRoots() {
		_ = os.RemoveAll(filepath.Join(root, seg))
	}
}
