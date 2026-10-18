package player

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bobo/KOTV/internal/player/embed"
	appruntime "github.com/bobo/KOTV/internal/runtime"
	"github.com/bobo/KOTV/internal/settings"
	"github.com/bobo/KOTV/internal/source"
	"github.com/bobo/KOTV/internal/thunder"
)

// Progress 播放进度回调。
type Progress struct {
	PositionMs int64
	DurationMs int64
	URL        string
	HistoryKey string
}

var (
	mu           sync.Mutex
	current      *exec.Cmd
	ipcSock      string
	historyKey   string
	playURL      string
	onProgress   func(Progress)
	progressStop chan struct{}
	lastPosition int64
	lastDuration int64
)

// SetProgressHandler 设置进度回调（用于写入历史续播）。
func SetProgressHandler(fn func(Progress)) {
	mu.Lock()
	onProgress = fn
	mu.Unlock()
}

// PreferEmbed 当前设置是否应为页内嵌入（innie#mpv）。
func PreferEmbed() bool {
	player := settings.Get(settings.Player)
	if player == "" {
		player = "innie#mpv"
	}
	parts := strings.SplitN(player, "#", 2)
	mode := parts[0]
	name := "mpv"
	if len(parts) > 1 && parts[1] != "" {
		name = strings.ToLower(parts[1])
	}
	return mode == "innie" && name == "mpv"
}

// Play 严格按设置启动对应播放器；innie#mpv 且已挂载画面时走页内嵌入。
func Play(url string, histKey string) error {
	player := settings.Get(settings.Player)
	if player == "" {
		player = "innie#mpv"
	}
	parts := strings.SplitN(player, "#", 2)
	mode := parts[0]
	name := "mpv"
	if len(parts) > 1 && parts[1] != "" {
		name = strings.ToLower(parts[1])
	}
	if mode == "" {
		mode = "innie"
	}
	name = strings.ToLower(name)

	log.Printf("播放器设置=%s → mode=%s name=%s url=%s", player, mode, name, url)

	// 磁力 / 专用源：先展开为可播 HTTP。
	if thunder.Match(url) {
		stream, err := thunder.Fetch(url)
		if err != nil {
			return fmt.Errorf("磁力链接解析失败: %w", err)
		}
		log.Printf("thunder fetch → %s", stream)
		url = stream
	} else if source.Match(url) {
		stream, err := source.Fetch(url, nil)
		if err != nil {
			return fmt.Errorf("专用源解析失败: %w", err)
		}
		log.Printf("source fetch → %s", stream)
		url = stream
	} else if isEd2k(url) {
		if err := openSystemURL(url); err != nil {
			return fmt.Errorf("电驴链接需用系统下载器打开: %w", err)
		}
		mu.Lock()
		historyKey = histKey
		playURL = url
		mu.Unlock()
		return nil
	}

	if mode == "innie" && name == "mpv" {
		if !embed.HasSink() {
			return fmt.Errorf("页内播放器未就绪：请在详情/直播页播放")
		}
		if !embed.MPVAvailable() {
			return fmt.Errorf("未找到捆绑 libmpv：页内 MPV 由 Flutter 原生通道提供，或改选外部 MPV")
		}
		Stop() // 停掉旁路进程
		eng := embed.Controller(embed.EnsureMPV())
		// 切内核时只停另一个；同内核换台/回看由 Play 内部 stop+load，勿 StopAll。
		if prev := embed.Active(); prev != nil && prev != eng {
			prev.Stop()
		}
		embed.SetActive(eng)
		// 解码方式必须在打开片源前设置：MPV 的 hwdec 影响首帧，
		// VLC 的 :avcodec-hw 只能在建 media 时下发。不支持的内核会返回 false，无副作用。
		if enhanced, ok := eng.(embed.Enhanced); ok {
			mode := settings.Get(settings.PlayerDecode)
			if mode == "" {
				mode = "auto"
			}
			enhanced.SetDecodeMode(mode)
		}
		if err := eng.Play(url, histKey); err != nil {
			return fmt.Errorf("内嵌 %s 失败: %w", displayName(name), err)
		}
		if volume, err := strconv.Atoi(settings.Get(settings.PlayerVolume)); err == nil {
			eng.SetVolume(volume)
		}
		if enhanced, ok := eng.(embed.Enhanced); ok {
			if speed, err := strconv.ParseFloat(settings.Get(settings.PlayerSpeed), 64); err == nil && speed > 0 {
				enhanced.SetSpeed(speed)
			}
		}
		mu.Lock()
		historyKey = histKey
		playURL = url
		mu.Unlock()
		return nil
	}

	return launchExternal(name, url, histKey)
}

func displayName(name string) string {
	switch strings.ToLower(name) {
	case "vlc":
		return "VLC"
	case "mpv":
		return "MPV"
	case "iina":
		return "IINA"
	default:
		return name
	}
}

func launchExternal(name, url, histKey string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = "mpv"
	}
	mu.Lock()
	defer mu.Unlock()
	stopLocked()
	embed.StopAll()

	historyKey = histKey
	playURL = url
	lastPosition = -1
	lastDuration = 0

	mpvBin := resolveBin(findPlayer("mpv"))
	vlcBin := resolveBin(findPlayer("vlc"))
	sock := ""
	if name == "mpv" && histKey != "" && mpvBin != "" {
		sock = mpvNewIPCPath()
		mpvCleanupIPC(sock)
	}
	argv, dir, err := externalArgv(runtime.GOOS, name, url, mpvBin, vlcBin, appruntime.MacApp("VLC"), appruntime.MacApp("IINA"), sock)
	if err != nil {
		if sock != "" {
			mpvCleanupIPC(sock)
		}
		return err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	log.Printf("启动外部播放器: %s", strings.Join(argv, " "))
	if err := cmd.Start(); err != nil {
		if sock != "" {
			mpvCleanupIPC(sock)
		}
		return err
	}
	current = cmd
	ipcSock = sock
	if sock != "" {
		progressStop = make(chan struct{})
		go pollMPVProgress(sock, progressStop)
	}
	go func() {
		_ = cmd.Wait()
		mu.Lock()
		if current == cmd {
			flushProgressLocked()
			current = nil
			ipcSock = ""
		}
		mu.Unlock()
	}()
	return nil
}

func resolveBin(bin string) string {
	if bin == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil && resolved != "" {
		return resolved
	}
	return bin
}

// externalArgv 按系统拼外置播放器参数。mpvBin、vlcBin 是可执行文件，vlcApp、iinaApp 是 .app 目录。
// ipcSock 非空时 MPV 挂进度通道。
func externalArgv(goos, name, url, mpvBin, vlcBin, vlcApp, iinaApp, ipcSock string) (argv []string, dir string, err error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mpv", "":
		if mpvBin == "" {
			return nil, "", fmt.Errorf("未找到 MPV")
		}
		args := []string{mpvBin, "--force-window=yes", "--keep-open=yes", "--idle=no"}
		if ipcSock != "" {
			args = append(args, "--input-ipc-server="+ipcSock)
		}
		args = append(args, url)
		// 工作目录放在二进制旁，.app 里的库按可执行文件相对路径加载。
		return args, filepath.Dir(mpvBin), nil
	case "vlc":
		if goos == "darwin" {
			app := appBundle(vlcBin)
			if app == "" {
				app = vlcApp
			}
			if app == "" {
				return nil, "", fmt.Errorf("未找到 VLC")
			}
			return []string{"open", "-a", app, url}, "", nil
		}
		if vlcBin == "" {
			return nil, "", fmt.Errorf("未找到 VLC")
		}
		return []string{vlcBin, url}, "", nil
	case "iina":
		if goos != "darwin" {
			return nil, "", fmt.Errorf("IINA 仅支持 macOS")
		}
		if iinaApp == "" {
			return nil, "", fmt.Errorf("未安装 IINA")
		}
		return []string{"open", "-a", iinaApp, url}, "", nil
	default:
		return nil, "", fmt.Errorf("未知播放器: %s", name)
	}
}

func appBundle(bin string) string {
	const marker = ".app/"
	idx := strings.Index(bin, marker)
	if idx < 0 {
		return ""
	}
	return bin[:idx+len(".app")]
}

func pollMPVProgress(sock string, stop <-chan struct{}) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if mpvIPCReady(sock) {
			break
		}
		select {
		case <-stop:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			pos, dur := mpvGetTime(sock)
			if pos < 0 {
				continue
			}
			select {
			case <-stop:
				return
			default:
			}
			mu.Lock()
			if ipcSock != sock {
				mu.Unlock()
				return
			}
			fn := onProgress
			key := historyKey
			u := playURL
			posMs := int64(pos * 1000)
			durMs := int64(dur * 1000)
			lastPosition = posMs
			lastDuration = durMs
			mu.Unlock()
			if fn != nil && key != "" {
				fn(Progress{
					PositionMs: posMs,
					DurationMs: durMs,
					URL:        u,
					HistoryKey: key,
				})
			}
		}
	}
}

func mpvGetTime(sock string) (pos, dur float64) {
	return mpvIPCGetTime(sock)
}

func flushProgressLocked() {
	if onProgress == nil || historyKey == "" || lastPosition < 0 {
		return
	}
	fn := onProgress
	p := Progress{
		PositionMs: lastPosition,
		DurationMs: lastDuration,
		URL:        playURL,
		HistoryKey: historyKey,
	}
	// 回调可能写数据库或刷新 UI，不应在 player 全局锁内同步执行。
	go fn(p)
}

// Stop 停止旁路与内嵌播放器。
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	stopLocked()
	embed.StopAll()
}

func stopLocked() {
	if progressStop != nil {
		close(progressStop)
		progressStop = nil
	}
	flushProgressLocked()
	if current != nil && current.Process != nil {
		_ = current.Process.Kill()
		current = nil
	}
	if ipcSock != "" {
		mpvCleanupIPC(ipcSock)
		ipcSock = ""
	}
}

// findPlayer 返回可执行路径；找不到返回空字符串。
func findPlayer(name string) string {
	switch strings.ToLower(name) {
	case "mpv", "":
		return appruntime.MPV()
	case "vlc":
		return appruntime.VLC()
	default:
		return ""
	}
}

// ExternalPlay 用外部播放器打开 URL（供 Flutter 切到 outie#* 时调用）。不挂进度通道。
func ExternalPlay(url, name string) error {
	url = strings.TrimSpace(url)
	name = strings.ToLower(strings.TrimSpace(name))
	if url == "" {
		return fmt.Errorf("empty url")
	}
	if name == "" {
		name = "mpv"
	}
	return launchExternal(name, url, "")
}

// Available 返回当前机器上可用的播放器说明（设置页展示用）。
func Available() map[string]bool {
	return map[string]bool{
		"vlc":       findPlayer("vlc") != "" || appruntime.MacApp("VLC") != "",
		"mpv":       findPlayer("mpv") != "",
		"iina":      appruntime.MacApp("IINA") != "",
		"embed_fvp": true,
		// Flutter 页内 MPV 走原生通道，库与 fvp/mdk 同目录。
		"embed_mpv": true,
	}
}

func isEd2k(u string) bool {
	u = strings.ToLower(strings.TrimSpace(u))
	return strings.HasPrefix(u, "ed2k:")
}

func isTorrentLike(u string) bool {
	return thunder.Match(u) || isEd2k(u)
}

func openSystemURL(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}
