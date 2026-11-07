package runtime

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bobo/KOTV/internal/localproxy"
)

// OS 本机平台：macos / windows / linux / android。前端用同一套写法（另有 ios、web）。
func OS() string {
	if runtime.GOOS == "darwin" {
		return "macos"
	}
	return runtime.GOOS
}

// Arch 本机架构：x64 / arm64 / arm / x86 / riscv64。前端用同一套写法。
func Arch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	default:
		return runtime.GOARCH
	}
}

// Platform「平台-架构」，如 macos-x64；打包目录名与日志用。
func Platform() string { return OS() + "-" + Arch() }

var (
	mu       sync.RWMutex
	baseDirs []string
)

// SetBaseDirs 覆盖搜索根（测试用）。
func SetBaseDirs(dirs ...string) {
	mu.Lock()
	baseDirs = append([]string{}, dirs...)
	mu.Unlock()
}

// Roots 返回 runtime 搜索根目录列表。
func Roots() []string {
	mu.RLock()
	if len(baseDirs) > 0 {
		out := append([]string{}, baseDirs...)
		mu.RUnlock()
		return out
	}
	mu.RUnlock()

	var roots []string
	plat := Platform()

	// 1) 可执行文件旁：./runtime
	if exe, err := os.Executable(); err == nil {
		if real, err2 := filepath.EvalSymlinks(exe); err2 == nil {
			exe = real
		}
		dir := filepath.Dir(exe)
		roots = append(roots,
			filepath.Join(dir, "runtime"),
			dir,
			// MacOS → ../Resources/runtime；Resources/engine → ../runtime
			filepath.Clean(filepath.Join(dir, "..", "runtime")),
			filepath.Clean(filepath.Join(dir, "..", "Resources", "runtime")),
		)
		// macOS .app：向上找到 Contents，再取 Resources/runtime
		if runtime.GOOS == "darwin" {
			p := dir
			for i := 0; i < 6; i++ {
				if filepath.Base(p) == "Contents" {
					roots = append(roots, filepath.Join(p, "Resources", "runtime"))
					break
				}
				next := filepath.Dir(p)
				if next == p {
					break
				}
				p = next
			}
		}
	}

	// 2) 环境变量（优先）
	if p := os.Getenv("KOTV_RUNTIME"); p != "" {
		roots = append([]string{p}, roots...)
	}

	// 3) 开发态：仓库内 runtime/
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots,
			filepath.Join(cwd, "runtime"),
			filepath.Join(cwd, "dist", "runtime"),
			filepath.Join(cwd, "dist", "KOTV-"+plat, "runtime"),
		)
	}

	return uniqueExistingParents(roots)
}

func uniqueExistingParents(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func firstExisting(candidates ...string) string {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			_ = os.Chmod(c, 0o755)
			return c
		}
	}
	return ""
}

// firstPresent 只判断系统里已有的文件，不改权限。
func firstPresent(candidates ...string) string {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func underRoots(rel ...string) []string {
	var out []string
	for _, root := range Roots() {
		for _, r := range rel {
			out = append(out, filepath.Join(root, r))
		}
	}
	return out
}

// Java 返回捆绑 JRE 中的 java（不回落系统 JAVA_HOME / PATH）。
func Java() string {
	return firstExisting(underRoots(
		filepath.Join("jre", "bin", "java"),
		filepath.Join("jre", "bin", "java.exe"),
		filepath.Join("jre", "Contents", "Home", "bin", "java"), // macOS Temurin 布局
	)...)
}

// JVMLib 返回进程内执行 JAR 所需的捆绑 JVM 动态库。
func JVMLib() string {
	return firstExisting(underRoots(
		filepath.Join("jre", "bin", "server", "jvm.dll"),
		filepath.Join("jre", "lib", "server", "libjvm.dylib"),
		filepath.Join("jre", "lib", "server", "libjvm.so"),
		filepath.Join("jre", "Contents", "Home", "lib", "server", "libjvm.dylib"),
	)...)
}

// Python 返回捆绑 Python（不回落系统 PATH）。
func Python() string {
	return firstExisting(underRoots(
		filepath.Join("python", "bin", "python3"),
		filepath.Join("python", "bin", "python"),
		filepath.Join("python", "python.exe"),
		filepath.Join("python", "python3.exe"),
		filepath.Join("python", "python3"),
		filepath.Join("python", "python"),
	)...)
}

// Node 返回捆绑 Node（不回落系统 PATH）。
// 桌面：runtime/node/node(.exe)；安卓：nativeLibraryDir/libkotv_node.so（同目录需有 libnode.so）。
func Node() string {
	if runtime.GOOS == "android" {
		cands := []string{}
		if dir := strings.TrimSpace(os.Getenv("KOTV_NATIVE_LIB_DIR")); dir != "" {
			cands = append(cands, filepath.Join(dir, "libkotv_node.so"))
		}
		if exe, err := os.Executable(); err == nil {
			cands = append(cands, filepath.Join(filepath.Dir(exe), "libkotv_node.so"))
		}
		cands = append(cands, underRoots("libkotv_node.so")...)
		return firstExisting(cands...)
	}
	return firstExisting(underRoots(
		filepath.Join("node", "node"),
		filepath.Join("node", "node.exe"),
	)...)
}

// RequireNode 返回捆绑 Node 路径；没有则返回统一错误文案。
func RequireNode() (string, error) {
	bin := Node()
	if bin == "" {
		return "", fmt.Errorf("本机没有 Node 运行时")
	}
	return bin, nil
}

// Chromium 返回 Chrome / chrome-headless-shell 路径（捆绑优先）。
func Chromium() string {
	cands := underRoots(
		filepath.Join("chromium", "chrome-headless-shell"),
		filepath.Join("chromium", "chrome-headless-shell.exe"),
		filepath.Join("chromium", "chrome.exe"),
		filepath.Join("chromium", "chrome"),
		filepath.Join("chromium", "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing"),
		filepath.Join("chromium", "Chromium.app", "Contents", "MacOS", "Chromium"),
		filepath.Join("chrome", "chrome"),
		filepath.Join("chrome", "chrome.exe"),
	)
	if p := firstExisting(cands...); p != "" {
		return p
	}
	// 系统 Chrome
	switch runtime.GOOS {
	case "darwin":
		for _, p := range []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		} {
			if firstExisting(p) != "" {
				return p
			}
		}
	case "windows":
		for _, p := range []string{
			filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		} {
			if firstExisting(p) != "" {
				return p
			}
		}
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// FFmpeg 返回 ffmpeg 路径。
func FFmpeg() string {
	cands := underRoots(
		filepath.Join("ffmpeg", "ffmpeg"),
		filepath.Join("ffmpeg", "ffmpeg.exe"),
		filepath.Join("ffmpeg", "bin", "ffmpeg"),
		filepath.Join("ffmpeg", "bin", "ffmpeg.exe"),
	)
	if p := firstExisting(cands...); p != "" {
		return p
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return ""
}

func programFilesBases() []string {
	return []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")}
}

func homeApplications() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Applications")
}

func macAppDirs() []string {
	dirs := []string{"/Applications"}
	if h := homeApplications(); h != "" {
		dirs = append(dirs, h)
	}
	return dirs
}

func macAppBinary(app, bin string) []string {
	out := make([]string, 0, 2)
	for _, root := range macAppDirs() {
		out = append(out, filepath.Join(root, app+".app", "Contents", "MacOS", bin))
	}
	return out
}

// MacApp 返回已安装的 .app 目录。非 macOS 或不存在时为空。
func MacApp(name string) string {
	name = strings.TrimSpace(name)
	if runtime.GOOS != "darwin" || name == "" {
		return ""
	}
	for _, root := range macAppDirs() {
		p := filepath.Join(root, name+".app")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return ""
}

// VLC 仅探测系统安装的外部播放器。
func VLC() string {
	switch runtime.GOOS {
	case "darwin":
		if p := firstPresent(macAppBinary("VLC", "VLC")...); p != "" {
			return p
		}
	case "windows":
		for _, base := range programFilesBases() {
			if p := firstPresent(filepath.Join(base, "VideoLAN", "VLC", "vlc.exe")); p != "" {
				return p
			}
		}
	}
	if p, err := exec.LookPath("vlc"); err == nil {
		return p
	}
	return ""
}

// MPV 返回外部 mpv 可执行文件（发行包目录优先，再系统安装，再 PATH）。
func MPV() string {
	cands := underRoots(
		filepath.Join("mpv", "mpv"),
		filepath.Join("mpv", "mpv.exe"),
		filepath.Join("mpv", "mpv.app", "Contents", "MacOS", "mpv"),
		filepath.Join("mpv", "bin", "mpv"),
	)
	if p := firstExisting(cands...); p != "" {
		return p
	}
	switch runtime.GOOS {
	case "darwin":
		sys := []string{"/opt/homebrew/bin/mpv", "/usr/local/bin/mpv"}
		sys = append(sys, macAppBinary("mpv", "mpv")...)
		if p := firstPresent(sys...); p != "" {
			return p
		}
	case "windows":
		for _, base := range programFilesBases() {
			if p := firstPresent(filepath.Join(base, "mpv", "mpv.exe")); p != "" {
				return p
			}
		}
	}
	if p, err := exec.LookPath("mpv"); err == nil {
		return p
	}
	return ""
}

// LibMPV 返回页内 MPV 动态库（与 fvp/mdk 同目录，不在 runtime）。
func LibMPV() string {
	cands := underRoots(
		"mpv-2.dll",
		"libmpv-2.dll",
		"libmpv.dylib",
		filepath.Join("lib", "libmpv.so.2"),
		filepath.Join("lib", "libmpv.so"),
		"libmpv.so.2",
		filepath.Join("..", "Frameworks", "libmpv.dylib"),
	)
	return firstExisting(cands...)
}

// BridgeJAR 返回 spider-bridge.jar。
func BridgeJAR() string {
	cands := underRoots(
		filepath.Join("bridge", "spider-bridge.jar"),
		"spider-bridge.jar",
	)
	cands = append(cands, "bridge/spider-bridge.jar", "spider-bridge.jar")
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		cands = append(cands,
			filepath.Join(dir, "bridge", "spider-bridge.jar"),
			filepath.Join(dir, "spider-bridge.jar"),
		)
	}
	return firstExisting(cands...)
}

// Status 汇总捆绑/系统运行时状态（不含 jvm；展示顺序由客户端固定）。
func Status() map[string]string {
	// Android：只报 bridge/quickjs；不报桌面 chromium/ffmpeg 等（播放走 Exo/media_kit/fvp）。
	if runtime.GOOS == "android" {
		st := "android-bridge(down)"
		if v := probeAndroidSpider(); v != "" {
			st = v
		}
		return map[string]string{
			"platform": OS(),
			"arch":     Arch(),
			"java":     st,
			"python":   st,
			"node":     androidNodeStatus(),
			"quickjs":  "embedded(CGO)",
			"bridge":   st,
		}
	}
	return map[string]string{
		"platform": OS(),
		"arch":     Arch(),
		"java":     orMissing(Java()),
		"python":   orMissing(Python()),
		// 与 java/python 一样只报路径，勿在 Status 里 exec node -v（会闪 Win 控制台）。
		"node":     orMissing(Node()),
		"quickjs":  "embedded(CGO)",
		"bridge":   orMissing(BridgeJAR()),
		"chromium": orMissing(Chromium()),
		"ffmpeg":   orMissing(FFmpeg()),
		"mpv":      orMissing(MPV()),
	}
}

// androidNodeStatus 与同屏 bridge 项一致：有则报 libkotv_node，不 exec、不报绝对路径。
func androidNodeStatus() string {
	if Node() == "" {
		return "(missing)"
	}
	return "libkotv_node"
}

// WithNativeLibDirEnv 给子进程补上 native 库搜索路径（安卓 libkotv_node.so 旁需 libnode.so）。
// 桌面原样返回 env。
func WithNativeLibDirEnv(env []string, bin string) []string {
	if runtime.GOOS != "android" {
		return env
	}
	dirs := make([]string, 0, 2)
	if d := strings.TrimSpace(os.Getenv("KOTV_NATIVE_LIB_DIR")); d != "" {
		dirs = append(dirs, d)
	}
	if bin = strings.TrimSpace(bin); bin != "" {
		if d := filepath.Dir(bin); d != "" && d != "." {
			dup := false
			for _, x := range dirs {
				if x == d {
					dup = true
					break
				}
			}
			if !dup {
				dirs = append(dirs, d)
			}
		}
	}
	if len(dirs) == 0 {
		return env
	}
	joined := strings.Join(dirs, string(os.PathListSeparator))
	const key = "LD_LIBRARY_PATH="
	out := make([]string, 0, len(env)+1)
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, key) {
			found = true
			cur := strings.TrimPrefix(e, key)
			if cur == "" {
				out = append(out, key+joined)
			} else {
				out = append(out, key+joined+string(os.PathListSeparator)+cur)
			}
			continue
		}
		out = append(out, e)
	}
	if !found {
		out = append(out, key+joined)
	}
	return out
}

func probeAndroidSpider() string {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(localproxy.DefaultBridgeBaseURL() + "/health")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	return fmt.Sprintf("android-bridge(:%d)", localproxy.DefaultBridgePort)
}

func orMissing(p string) string {
	if p == "" {
		return "(missing)"
	}
	return p
}
