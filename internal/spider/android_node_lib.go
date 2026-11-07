package spider

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bobo/KOTV/internal/paths"
)

// 安卓 libnode.so 按需下载（不进 APK）。启动器 libkotv_node.so 已链到它。
const (
	androidNodeMobileVer = "v18.20.4"
	androidLibNodeBase   = "https://github.com/nodejs-mobile/nodejs-mobile/releases/download/" +
		androidNodeMobileVer + "/nodejs-mobile-" + androidNodeMobileVer + "-android.zip"
)

var (
	androidLibNodeOnce sync.Once
	androidLibNodeErr  error
	androidLibNodeDir  string
)

// ensureAndroidLibNode 下载并解出当前 ABI 的 libnode.so 到应用可写目录，返回其所在目录（供 LD_LIBRARY_PATH）。
func ensureAndroidLibNode() (libDir string, err error) {
	if runtime.GOOS != "android" {
		return "", nil
	}
	androidLibNodeOnce.Do(func() {
		androidLibNodeDir, androidLibNodeErr = fetchAndroidLibNode()
	})
	return androidLibNodeDir, androidLibNodeErr
}

func fetchAndroidLibNode() (string, error) {
	abi := androidABI()
	if abi == "" {
		return "", fmt.Errorf("无法识别安卓 ABI")
	}
	dir := paths.Ensure(filepath.Join(paths.CacheRoot(), "node-lib", abi))
	dest := filepath.Join(dir, "libnode.so")
	if st, err := os.Stat(dest); err == nil && st.Size() > 1024*1024 {
		return dir, nil
	}
	log.Printf("android node: 下载 libnode.so (%s)…", abi)
	tmpZip := filepath.Join(paths.CacheRoot(), "node-lib", "nodejs-mobile-android.zip")
	_ = os.MkdirAll(filepath.Dir(tmpZip), 0o755)
	if err := downloadFile(androidLibNodeBase, tmpZip); err != nil {
		return "", fmt.Errorf("下载 libnode 失败: %w", err)
	}
	// 只抽出当前 ABI，避免整包解压占满存储。
	member := "bin/" + abi + "/libnode.so"
	if err := unzipOne(tmpZip, member, dest); err != nil {
		return "", fmt.Errorf("解压 libnode.so 失败: %w", err)
	}
	_ = os.Chmod(dest, 0o755)
	log.Printf("android node: libnode.so 就绪 %s", dest)
	return dir, nil
}

func androidABI() string {
	// 优先 Flutter/Kotlin 注入的 ABI；否则按 GOARCH 猜测。
	if v := strings.TrimSpace(os.Getenv("KOTV_ANDROID_ABI")); v != "" {
		return v
	}
	switch runtime.GOARCH {
	case "arm64":
		return "arm64-v8a"
	case "arm":
		return "armeabi-v7a"
	case "amd64":
		return "x86_64"
	case "386":
		return "x86"
	default:
		return ""
	}
}

func downloadFile(url, dest string) error {
	if st, err := os.Stat(dest); err == nil && st.Size() > 1024 {
		return nil
	}
	partial := dest + ".partial"
	_ = os.Remove(partial)
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(partial)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(partial)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(partial)
		return closeErr
	}
	return os.Rename(partial, dest)
}
