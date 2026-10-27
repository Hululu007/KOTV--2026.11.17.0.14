package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// BrowseRoot 引擎侧可选目录根（供下载目录等浏览）。
type BrowseRoot struct {
	Name string
	Path string
}

// BrowseRoots 返回引擎本机可浏览根目录（绝对路径）。
func BrowseRoots() []BrowseRoot {
	seen := map[string]struct{}{}
	var out []BrowseRoot
	add := func(name, p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		abs, err := filepath.Abs(filepath.Clean(p))
		if err != nil {
			return
		}
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			return
		}
		key := abs
		if runtime.GOOS == "windows" {
			key = strings.ToLower(abs)
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, BrowseRoot{Name: name, Path: abs})
	}

	add("下载", Downloads())
	if home, err := os.UserHomeDir(); err == nil {
		add("用户目录", home)
	}
	if ext := externalStorageRoot(); ext != "" {
		add("内部存储", ext)
	}
	add("应用数据", Data())
	return out
}

// UnderBrowseRoot 路径是否落在任一浏览根下（含根自身）。
func UnderBrowseRoot(p string) bool {
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(p)))
	if err != nil || abs == "" {
		return false
	}
	for _, r := range BrowseRoots() {
		root := r.Path
		if abs == root || strings.HasPrefix(abs, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// BrowseParent 返回可继续浏览的父目录；越出浏览根则空串（回到根列表）。
func BrowseParent(dir string) string {
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(dir)))
	if err != nil || abs == "" {
		return ""
	}
	for _, r := range BrowseRoots() {
		if abs == r.Path {
			return ""
		}
	}
	parent := filepath.Dir(abs)
	if parent == abs || !UnderBrowseRoot(parent) {
		return ""
	}
	return parent
}
