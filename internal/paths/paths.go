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

// CacheRoot 爬虫包体与可再生缓存的公共根（无 uid）：
//   - 若注入了 KOTV_CACHE_DIR（安卓 = Context.getCacheDir），直接用它，
//     与宿主 Path.jar/js/py 同根，避免 Go 与 JVM 各下一份；
//   - 否则 {Root}/cache（桌面）。
// 首次访问时收拢历史误放的包体。
func CacheRoot() string {
	p := ensure(cacheRootPath())
	reconcilePackageCacheLayout(p)
	return p
}

func cacheRootPath() string {
	if v := strings.TrimSpace(os.Getenv("KOTV_CACHE_DIR")); v != "" {
		return v
	}
	return filepath.Join(Root(), "cache")
}

// Assets 返回本地 assets 资源根，对应 bridge AssetManager 的 {root}/assets（供 assets:// 使用）。
func Assets() string { return ensure(filepath.Join(Root(), "assets")) }
func DB() string     { return filepath.Join(ensure(filepath.Join(Root(), "db")), "tv.db") }

// 下列为爬虫包体目录：一律 CacheRoot 下公共无 uid（与 JVM Path.jar/js/py 同根）。
func JarCache() string { return ensure(filepath.Join(CacheRoot(), "jar")) }
func PyCache() string  { return ensure(filepath.Join(CacheRoot(), "py")) }
func PicCache() string { return ensure(filepath.Join(CacheRoot(), "pic")) }
func LogDir() string   { return ensure(filepath.Join(Data(), "log")) }
func JsCache() string  { return ensure(filepath.Join(CacheRoot(), "js")) }

// NodeCache 猫爪 Node 源包包体（index.js / index.config.js），与 jar/py/js 同级。
func NodeCache() string { return ensure(filepath.Join(CacheRoot(), "node")) }
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

var reconcilePackageCacheOnce sync.Once

// reconcilePackageCacheLayout 一次性收拢历史误放的爬虫包体到公共 CacheRoot/{jar,py,js,node}：
//  1. js/pkg/* → node/*（曾误嵌在 js 下）
//  2. run/<uid|shared>/jvm/cache/{jar,js,py}/* → {jar,js,py}/*
//     （旧桌面 stub 把 kotv.cache.dir 指到 UserJVMDir）
//  3. 若 CacheRoot=KOTV_CACHE_DIR：把 DATA/cache/{jar,py,js,node} 迁过来
//     （安卓曾 Go 落 files/KOTV/cache、Path 落 cacheDir，双根各一份）
func reconcilePackageCacheLayout(cacheRoot string) {
	reconcilePackageCacheOnce.Do(func() {
		moveChildrenIfAbsent(
			filepath.Join(cacheRoot, "js", "pkg"),
			filepath.Join(cacheRoot, "node"),
		)
		runRoot := filepath.Join(cacheRoot, "run")
		if ents, err := os.ReadDir(runRoot); err == nil {
			for _, e := range ents {
				if !e.IsDir() {
					continue
				}
				legacy := filepath.Join(runRoot, e.Name(), "jvm", "cache")
				for _, name := range []string{"jar", "js", "py"} {
					moveChildrenIfAbsent(
						filepath.Join(legacy, name),
						filepath.Join(cacheRoot, name),
					)
				}
				removeDirIfEmpty(filepath.Join(legacy, "jar"))
				removeDirIfEmpty(filepath.Join(legacy, "js"))
				removeDirIfEmpty(filepath.Join(legacy, "py"))
				removeDirIfEmpty(legacy)
			}
		}
		if data := strings.TrimSpace(os.Getenv("KOTV_DATA_DIR")); data != "" {
			legacyDataCache := filepath.Join(data, "cache")
			if filepath.Clean(legacyDataCache) != filepath.Clean(cacheRoot) {
				for _, name := range []string{"jar", "py", "js", "node"} {
					moveChildrenIfAbsent(
						filepath.Join(legacyDataCache, name),
						filepath.Join(cacheRoot, name),
					)
				}
			}
		}
	})
}

// moveChildrenIfAbsent 把 src 下子项迁到 dst（dst 已有同名则跳过），迁空后删 src。
func moveChildrenIfAbsent(src, dst string) {
	st, err := os.Stat(src)
	if err != nil || !st.IsDir() {
		return
	}
	_ = os.MkdirAll(dst, 0o755)
	ents, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, e := range ents {
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())
		if _, err := os.Stat(to); err == nil {
			continue
		}
		_ = os.Rename(from, to)
	}
	removeDirIfEmpty(src)
}

func removeDirIfEmpty(dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil || len(ents) != 0 {
		return
	}
	_ = os.Remove(dir)
}
