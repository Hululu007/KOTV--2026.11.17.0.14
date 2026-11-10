package player

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/bobo/KOTV/internal/player/embed"
	"github.com/bobo/KOTV/internal/settings"
	"github.com/bobo/KOTV/internal/source"
	"github.com/bobo/KOTV/internal/thunder"
)

// ParsePlayer 拆开 mode#name。名字空着时按 mpv；模式空着时保持空。
func ParsePlayer(raw string) (mode, name string) {
	parts := strings.SplitN(raw, "#", 2)
	mode = parts[0]
	name = "mpv"
	if len(parts) > 1 && parts[1] != "" {
		name = parts[1]
	}
	return mode, strings.ToLower(name)
}

// Play 严格按设置启动对应播放器；innie#mpv 且已挂载画面时走页内嵌入。
func Play(url string, histKey string) error {
	player := settings.Get(settings.Player)
	if player == "" {
		player = "innie#mpv"
	}
	mode, name := ParsePlayer(player)
	if mode == "" {
		mode = "innie"
	}

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
		return nil
	}

	if mode == "innie" && name == "mpv" {
		if !embed.HasSink() {
			return fmt.Errorf("页内播放器未就绪：请在详情/直播页播放")
		}
		if !embed.MPVAvailable() {
			return fmt.Errorf("未找到捆绑 libmpv：页内 MPV 由 Flutter 原生通道提供，或改用外部播放器")
		}
		Stop()
		eng := embed.Controller(embed.EnsureMPV())
		// 切内核时只停另一个；同内核换台/回看由 Play 内部 stop+load，勿 StopAll。
		if prev := embed.Active(); prev != eng {
			prev.Stop()
		}
		embed.SetActive(eng)
		// 解码方式必须在打开片源前设置：MPV 的 hwdec 影响首帧。
		if enhanced, ok := eng.(embed.Enhanced); ok {
			mode := settings.Get(settings.PlayerDecode)
			if mode == "" {
				mode = "auto"
			}
			enhanced.SetDecodeMode(mode)
		}
		if err := eng.Play(url, histKey); err != nil {
			return fmt.Errorf("内嵌 MPV 失败: %w", err)
		}
		if volume, err := strconv.Atoi(settings.Get(settings.PlayerVolume)); err == nil {
			eng.SetVolume(volume)
		}
		if enhanced, ok := eng.(embed.Enhanced); ok {
			if speed, err := strconv.ParseFloat(settings.Get(settings.PlayerSpeed), 64); err == nil && speed > 0 {
				enhanced.SetSpeed(speed)
			}
		}
		return nil
	}

	// 外置：桌面走系统打开；安卓由 Flutter 原生选择器，一般不经这里。
	return ExternalPlay(url)
}

// Stop 停止页内嵌入播放器。
func Stop() {
	embed.StopAll()
}

// Available 返回当前机器上可用的播放器说明（设置页展示用）。
// view：桌面由系统打开；安卓是否另有视频应用由 Flutter 再探测。
func Available() map[string]bool {
	return map[string]bool{
		"view":      true,
		"embed_fvp": true,
		// Flutter 页内 MPV 走原生通道，库与 fvp/mdk 同目录。
		"embed_mpv": true,
	}
}

// ExternalPlay 用系统打开 URL（供 Flutter 切到 outie#view 时调用）。
func ExternalPlay(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("empty url")
	}
	Stop()
	log.Printf("系统打开外部播放: %s", url)
	return openSystemURL(url)
}

func isEd2k(u string) bool {
	u = strings.ToLower(strings.TrimSpace(u))
	return strings.HasPrefix(u, "ed2k:")
}

// openSystemURL 按平台用系统关联打开（macOS open / Windows FileProtocolHandler / Linux xdg-open）。
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
