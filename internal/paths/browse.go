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
// Windows：各盘符 + 下载/用户目录；Android：内部存储与 /storage 下卷；桌面：下载/家目录/应用数据。
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

	switch runtime.GOOS {
	case "windows":
		for _, d := range windowsDriveRoots() {
			add(d.Name, d.Path)
		}
		add("下载", Downloads())
		if home, err := os.UserHomeDir(); err == nil {
			add("用户目录", home)
		}
	case "android":
		if ext := externalStorageRoot(); ext != "" {
			add("内部存储", ext)
		}
		for _, d := range androidVolumeRoots() {
			add(d.Name, d.Path)
		}
		add("下载", Downloads())
		add("应用数据", Data())
	default:
		add("下载", Downloads())
		if home, err := os.UserHomeDir(); err == nil {
			add("用户目录", home)
		}
		add("应用数据", Data())
		for _, d := range unixMediaRoots() {
			add(d.Name, d.Path)
		}
	}
	return out
}

func windowsDriveRoots() []BrowseRoot {
	var out []BrowseRoot
	for c := 'A'; c <= 'Z'; c++ {
		p := string(c) + `:\`
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, BrowseRoot{Name: string(c) + ":", Path: p})
		}
	}
	return out
}

func androidVolumeRoots() []BrowseRoot {
	ents, err := os.ReadDir("/storage")
	if err != nil {
		return nil
	}
	var out []BrowseRoot
	for _, e := range ents {
		name := e.Name()
		if name == "" || name == "self" || name == "emulated" || name == "." || name == ".." {
			continue
		}
		p := filepath.Join("/storage", name)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, BrowseRoot{Name: name, Path: p})
		}
	}
	return out
}

func unixMediaRoots() []BrowseRoot {
	home, _ := os.UserHomeDir()
	cands := []string{"/media", "/mnt", "/Volumes"}
	if home != "" {
		cands = append(cands, filepath.Join("/media", filepath.Base(home)))
	}
	var out []BrowseRoot
	seen := map[string]struct{}{}
	for _, root := range cands {
		ents, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "" || strings.HasPrefix(name, ".") {
				continue
			}
			p := filepath.Join(root, name)
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, BrowseRoot{Name: name, Path: p})
		}
	}
	return out
}

// UnderBrowseRoot 路径是否落在任一浏览根下（含根自身）。
func UnderBrowseRoot(p string) bool {
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(p)))
	if err != nil || abs == "" {
		return false
	}
	for _, r := range BrowseRoots() {
		if PathUnderRoot(abs, r.Path) {
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
		if PathUnderRoot(abs, r.Path) && PathUnderRoot(r.Path, abs) {
			// abs 与某浏览根相同
			return ""
		}
	}
	parent := filepath.Dir(abs)
	if parent == abs || !UnderBrowseRoot(parent) {
		return ""
	}
	return parent
}
