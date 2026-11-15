package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const appName = "KOTV"

// Root 返回用户数据根目录。
// Android：优先 KOTV_DATA_DIR / KOTV_CACHE_DIR（由 Flutter launcher 注入应用可写目录）。
// Windows：%APPDATA%/KOTV。缓存统一落在 Root()/cache/...。
func Root() string {
	if v := strings.TrimSpace(os.Getenv("KOTV_DATA_DIR")); v != "" {
		return ensure(v)
	}
	if v := strings.TrimSpace(os.Getenv("KOTV_CACHE_DIR")); v != "" {
		return ensure(v)
	}
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return ensure(filepath.Join(base, appName))
	case "darwin":
		home, _ := os.UserHomeDir()
		return ensure(filepath.Join(home, "Library", "Application Support", appName))
	case "android":
		// 未注入环境变量时回落到 cwd（通常为应用私有目录）。
		cwd, _ := os.Getwd()
		if cwd != "" {
			return ensure(filepath.Join(cwd, "kotv-cache"))
		}
		return ensure(filepath.Join("/data/local/tmp", appName))
	default:
		home, _ := os.UserHomeDir()
		return ensure(filepath.Join(home, ".local", "share", appName))
	}
}

func Data() string { return ensure(filepath.Join(Root(), "data")) }

// CacheRoot 所有可再生缓存的统一根：{Root}/cache
func CacheRoot() string { return ensure(filepath.Join(Root(), "cache")) }

// Assets 返回本地 assets 资源根，对应 bridge AssetManager 的 {root}/assets（供 assets:// 使用）。
func Assets() string { return ensure(filepath.Join(Root(), "assets")) }
func DB() string     { return filepath.Join(ensure(filepath.Join(Root(), "db")), "tv.db") }

// 下列缓存均在 CacheRoot 下，与 bridge 的 Root/cache/* 保持一致，避免 data/cache 再套一层。
func JarCache() string { return ensure(filepath.Join(CacheRoot(), "jar")) }
func PyCache() string  { return ensure(filepath.Join(CacheRoot(), "py")) }
func PicCache() string { return ensure(filepath.Join(CacheRoot(), "pic")) }
func LogDir() string   { return ensure(filepath.Join(Data(), "log")) }
func JsCache() string { return ensure(filepath.Join(CacheRoot(), "js")) }

// NodeCache 猫爪 Node 源包包体（index.js / index.config.js）。
// 与 jar / py / js 同级挂在 CacheRoot 下：cache/{jar,py,js,node}/。
// 误放到 cache/js/pkg/ 的包体在首次访问时迁回此处。
func NodeCache() string {
	dst := ensure(filepath.Join(CacheRoot(), "node"))
	migrateMisplacedNodePkgCache(dst)
	return dst
}
func EpgCache() string  { return ensure(filepath.Join(CacheRoot(), "epg")) }
func HttpCache() string {
	return ensure(filepath.Join(CacheRoot(), "http"))
}
func SubCache() string { return ensure(filepath.Join(CacheRoot(), "sub")) }

// Ensure 确保目录存在并返回路径。
func Ensure(p string) string { return ensure(p) }

func JarPath(md5 string) string {
	return filepath.Join(JarCache(), md5+".jar")
}

// Downloads 主机上的默认下载目录（设置 downloadDir 为空时使用）。
// 下载落在跑服务的那台机器上：Windows/macOS/Linux → ~/Downloads；
// Android → …/Download。均优先系统可见的「下载」文件夹。
// iOS 无本机服务进程，不会在本机执行本函数；远端下载仍走对方主机的 Downloads()。
// 公共目录不可用时回落 Root()/downloads。
func Downloads() string {
	if p := publicDownloadsDir(); p != "" {
		return p
	}
	return ensure(filepath.Join(Root(), "downloads"))
}

func publicDownloadsDir() string {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		return ensureHomeDownloads()
	case "android":
		ext := externalStorageRoot()
		if ext == "" {
			return ""
		}
		// DIRECTORY_DOWNLOADS：安卓是 Download（单数）。
		pub := filepath.Join(ext, "Download")
		if err := os.MkdirAll(pub, 0o755); err != nil {
			return ""
		}
		return pub
	default:
		return ""
	}
}

func ensureHomeDownloads() string {
	home := ""
	if runtime.GOOS == "windows" {
		home = strings.TrimSpace(os.Getenv("USERPROFILE"))
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return ensure(filepath.Join(home, "Downloads"))
}

func ensure(p string) string {
	_ = os.MkdirAll(p, 0o755)
	return p
}

var migrateMisplacedNodePkgOnce sync.Once

// migrateMisplacedNodePkgCache 把误嵌在 cache/js/pkg/* 的包体迁回 cache/node/*。
func migrateMisplacedNodePkgCache(dst string) {
	migrateMisplacedNodePkgOnce.Do(func() {
		misplaced := filepath.Join(CacheRoot(), "js", "pkg")
		st, err := os.Stat(misplaced)
		if err != nil || !st.IsDir() {
			return
		}
		ents, err := os.ReadDir(misplaced)
		if err != nil {
			return
		}
		for _, e := range ents {
			src := filepath.Join(misplaced, e.Name())
			target := filepath.Join(dst, e.Name())
			if _, err := os.Stat(target); err == nil {
				continue
			}
			if err := os.Rename(src, target); err != nil {
				continue
			}
		}
		if left, err := os.ReadDir(misplaced); err == nil && len(left) == 0 {
			_ = os.Remove(misplaced)
		}
	})
}
