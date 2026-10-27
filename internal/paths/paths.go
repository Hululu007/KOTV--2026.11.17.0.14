package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
func JsCache() string  { return ensure(filepath.Join(CacheRoot(), "js")) }
func EpgCache() string { return ensure(filepath.Join(CacheRoot(), "epg")) }
func HttpCache() string {
	return ensure(filepath.Join(CacheRoot(), "http"))
}
func SubCache() string { return ensure(filepath.Join(CacheRoot(), "sub")) }

// Ensure 确保目录存在并返回路径。
func Ensure(p string) string { return ensure(p) }

func JarPath(md5 string) string {
	return filepath.Join(JarCache(), md5+".jar")
}

// Downloads 引擎默认下载目录（设置 downloadDir 为空时使用）。
// 各平台一律优先系统「下载」文件夹（资源管理器 / Finder / 文件管理器可见）：
// Windows/macOS/Linux → ~/Downloads；Android → …/Download。
// 只有没有公共下载或建目录失败时，才落到应用私有 Root()/downloads（如 iOS）。
func Downloads() string {
	if p := publicDownloadsDir(); p != "" {
		return p
	}
	return ensure(filepath.Join(Root(), "downloads"))
}

func publicDownloadsDir() string {
	switch runtime.GOOS {
	case "windows":
		base := strings.TrimSpace(os.Getenv("USERPROFILE"))
		if base == "" {
			base, _ = os.UserHomeDir()
		}
		if base == "" {
			return ""
		}
		return ensure(filepath.Join(base, "Downloads"))
	case "darwin", "linux":
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return ""
		}
		return ensure(filepath.Join(home, "Downloads"))
	case "android":
		if ext := externalStorageRoot(); ext != "" {
			// DIRECTORY_DOWNLOADS：安卓是 Download（单数）。
			pub := filepath.Join(ext, "Download")
			if err := os.MkdirAll(pub, 0o755); err == nil {
				return pub
			}
		}
		return ""
	default:
		return ""
	}
}

func ensure(p string) string {
	_ = os.MkdirAll(p, 0o755)
	return p
}
