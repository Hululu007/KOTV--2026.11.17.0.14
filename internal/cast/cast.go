// Package cast 统一发现并投送媒体到局域网设备。
// 支持：DLNA/UPnP MediaRenderer、Google Chromecast、局域网 App（/device + /action?do=cast）。
package cast

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bobo/KOTV/internal/dlna"
	"github.com/bobo/KOTV/internal/model"
	"github.com/bobo/KOTV/internal/util"
	castdns "github.com/vishen/go-chromecast/dns"
)

// Protocol 投屏协议。
type Protocol string

const (
	ProtocolDLNA       Protocol = "DLNA"
	ProtocolChromecast Protocol = "Chromecast"
)

// Device 可投屏设备。
type Device struct {
	Name     string
	Protocol Protocol
	Detail   string // 辅助信息（地址等）

	dlna     dlna.Device
	castIP   string
	castPort int
	appBase  string
}

// deviceKey 同一设备的去重键（协议 + 地址）。
func (d Device) deviceKey() string {
	return string(d.Protocol) + "|" + d.Detail
}

// DiscoverEach 并行搜索 DLNA、Chromecast 与局域网 App（/device）。
// 每发现一台新设备立即回调 onFound（同一设备只回调一次），返回全部设备（按发现顺序）。
// Chromecast 与 App 逐台回调；DLNA 由底层库在搜索窗口结束后一并返回。
func DiscoverEach(timeout time.Duration, onFound func(Device)) ([]Device, error) {
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	var (
		mu   sync.Mutex
		out  []Device
		seen = map[string]bool{}
		errs []error
		wg   sync.WaitGroup
	)
	emit := func(d Device) {
		mu.Lock()
		k := d.deviceKey()
		if seen[k] {
			mu.Unlock()
			return
		}
		seen[k] = true
		out = append(out, d)
		mu.Unlock()
		if onFound != nil {
			onFound(d)
		}
	}
	run := func(label string, discover func(emit func(Device)) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := discover(emit); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", label, err))
				mu.Unlock()
			}
		}()
	}
	run("DLNA", func(emit func(Device)) error {
		devs, err := dlna.Discover(timeout)
		for _, d := range devs {
			emit(Device{Name: d.Name, Protocol: ProtocolDLNA, Detail: d.Location, dlna: d})
		}
		return err
	})
	run("Chromecast", func(emit func(Device)) error { return discoverChromecast(timeout, emit) })
	run("App", func(emit func(Device)) error { return discoverAppPeers(timeout, emit) })
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(out) == 0 && len(errs) > 0 {
		return nil, errs[0]
	}
	for _, e := range errs {
		log.Printf("cast discover: %v", e)
	}
	rememberDiscover(out)
	return append([]Device(nil), out...), nil
}

var (
	lastDiscoverMu sync.Mutex
	lastDiscover   []Device
)

func rememberDiscover(devs []Device) {
	lastDiscoverMu.Lock()
	lastDiscover = append([]Device(nil), devs...)
	lastDiscoverMu.Unlock()
}

// discoverChromecast mDNS 每收到一台设备就 emit（去重由 DiscoverEach 负责）。
func discoverChromecast(timeout time.Duration, emit func(Device)) error {
	// Android：Go net.Interfaces 常因 netlinkrib 失败；有 KOTV_LAN_IP 时仍尝试发现。
	if runtime.GOOS == "android" {
		lan := util.LanIP()
		ifaces, err := net.Interfaces()
		usable := 0
		if err != nil {
			if lan == "" {
				log.Printf("cast: skip Chromecast on Android (interfaces: %v)", err)
				return nil
			}
			log.Printf("cast: Android interfaces failed (%v); try discover with LanIP=%s", err, lan)
		} else {
			for _, iface := range ifaces {
				if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 || iface.Flags&net.FlagLoopback != 0 {
					continue
				}
				usable++
			}
			if usable == 0 && lan == "" {
				log.Printf("cast: skip Chromecast on Android (no multicast interfaces)")
				return nil
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ch, err := castdns.DiscoverCastDNSEntries(ctx, nil)
	if err != nil {
		msg := err.Error()
		if runtime.GOOS == "android" && (strings.Contains(msg, "interfaces: []") || strings.Contains(msg, "failed to join")) {
			log.Printf("cast: Chromecast discover soft-fail on Android: %v", err)
			return nil
		}
		return err
	}
	// 同一台设备可能从多个地址广播，按 UUID 去重。
	seenUUID := map[string]bool{}
	for e := range ch {
		if e.UUID != "" {
			if seenUUID[e.UUID] {
				continue
			}
			seenUUID[e.UUID] = true
		}
		name := strings.TrimSpace(e.DeviceName)
		if name == "" {
			name = strings.TrimSpace(e.Device)
		}
		if name == "" {
			name = e.GetAddr()
		}
		emit(Device{
			Name:     name,
			Protocol: ProtocolChromecast,
			Detail:   fmt.Sprintf("%s:%d", e.GetAddr(), e.GetPort()),
			castIP:   e.GetAddr(),
			castPort: e.GetPort(),
		})
	}
	return nil
}

// CastOptions 投送附加参数（App 协议需要 config/device/history）。
type CastOptions struct {
	Title       string
	Headers     map[string]string
	PositionMs  int64
	ConfigJSON  string
	DeviceJSON  string
	HistoryJSON string
}

// CastWithOpts 投送；headers 写入 DIDL，positionMs 起播续看。
// Chromecast 失败时，若同 IP 存在 DLNA 设备则自动回退；used 为实际成功的协议设备。
func CastWithOpts(d Device, mediaURL string, opt CastOptions) (Device, error) {
	// 回环改写由 toolCast 统一完成；此处只做空串校验。
	mediaURL = strings.TrimSpace(mediaURL)
	if mediaURL == "" && d.Protocol != ProtocolApp {
		return d, fmt.Errorf("媒体地址为空")
	}
	title := strings.TrimSpace(opt.Title)
	switch d.Protocol {
	case ProtocolDLNA:
		err := dlna.CastWith(d.dlna, mediaURL, dlna.CastOptions{
			Title:      title,
			Headers:    opt.Headers,
			PositionMs: opt.PositionMs,
		})
		return d, err
	case ProtocolChromecast:
		err := castChromecast(d, mediaURL, opt.PositionMs)
		if err == nil {
			return d, nil
		}
		if alt, ok := findDLNAByHost(d.castIP); ok {
			log.Printf("cast: Chromecast 失败，回退 DLNA %s (%s): %v", alt.Name, alt.Detail, err)
			if err2 := dlna.CastWith(alt.dlna, mediaURL, dlna.CastOptions{
				Title:      title,
				Headers:    opt.Headers,
				PositionMs: opt.PositionMs,
			}); err2 == nil {
				return alt, nil
			} else {
				log.Printf("cast: DLNA 回退仍失败: %v", err2)
			}
		}
		return d, fmt.Errorf("%w\n\n%s", err, chromecastHint())
	case ProtocolApp:
		hist := strings.TrimSpace(opt.HistoryJSON)
		if hist == "" {
			return d, fmt.Errorf("App 投屏缺少 history")
		}
		err := castAppPeer(d, appCastPayload{
			Config:  opt.ConfigJSON,
			Device:  opt.DeviceJSON,
			History: hist,
		})
		return d, err
	default:
		return d, fmt.Errorf("不支持的协议: %s", d.Protocol)
	}
}

func findDLNAByHost(host string) (Device, bool) {
	host = strings.TrimSpace(host)
	if host == "" {
		return Device{}, false
	}
	lastDiscoverMu.Lock()
	cached := append([]Device(nil), lastDiscover...)
	lastDiscoverMu.Unlock()
	for _, d := range cached {
		if d.Protocol != ProtocolDLNA {
			continue
		}
		if hostMatchesLocation(host, d.Detail) {
			return d, true
		}
	}
	devs, err := dlna.Discover(3 * time.Second)
	if err != nil {
		return Device{}, false
	}
	for _, d := range devs {
		if hostMatchesLocation(host, d.Location) {
			return Device{
				Name:     d.Name,
				Protocol: ProtocolDLNA,
				Detail:   d.Location,
				dlna:     d,
			}, true
		}
	}
	return Device{}, false
}

func hostMatchesLocation(host, location string) bool {
	uHost := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		uHost = h
	}
	loc := strings.TrimSpace(location)
	if loc == "" {
		return false
	}
	// Location 形如 http://192.168.1.2:49152/desc.xml
	if i := strings.Index(loc, "://"); i >= 0 {
		loc = loc[i+3:]
	}
	if j := strings.IndexAny(loc, "/:"); j >= 0 {
		loc = loc[:j]
	}
	return strings.EqualFold(loc, uHost)
}

func chromecastHint() string {
	return "提示：若 Chrome 能投同一台电视，可再试一次 Chromecast；仍失败请选同设备的 DLNA（控制更完整）。"
}

func castChromecast(d Device, mediaURL string, positionMs int64) error {
	if d.castIP == "" || d.castPort == 0 {
		return fmt.Errorf("设备无效")
	}
	var last error
	for attempt := 1; attempt <= 2; attempt++ {
		if attempt > 1 {
			time.Sleep(800 * time.Millisecond)
			log.Printf("cast: Chromecast 重试 %d/2 → %s:%d", attempt, d.castIP, d.castPort)
		}
		last = loadURLOnChromecast(d.castIP, d.castPort, mediaURL, contentTypeForURL(mediaURL), positionMs)
		if last == nil {
			return nil
		}
		msg := strings.ToLower(last.Error())
		if strings.Contains(msg, "connection reset") || strings.Contains(msg, "refused") {
			break
		}
	}
	return fmt.Errorf("Chromecast 投送: %w", friendlyChromecastErr(last))
}

func friendlyChromecastErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "cc1ad845") && strings.Contains(low, "deadline"):
		return fmt.Errorf("无法启动默认播放器（协议库超时）。若 Chrome 能投，多半是应答较慢，请再试或改用 DLNA")
	case strings.Contains(low, "启动默认播放器超时"):
		return err
	case strings.Contains(low, "deadline exceeded"):
		return fmt.Errorf("等待设备响应超时")
	case strings.Contains(low, "connection reset"):
		return fmt.Errorf("连接被设备断开（会话被占用或协议握手失败）")
	default:
		return err
	}
}

func contentTypeForURL(u string) string {
	if model.URLIsHls(u) {
		return model.FormatHLS
	}
	low := strings.ToLower(u)
	switch {
	case strings.Contains(low, ".mpd"):
		return "application/dash+xml"
	case strings.Contains(low, ".mp4"):
		return "video/mp4"
	case strings.Contains(low, ".webm"):
		return "video/webm"
	case strings.Contains(low, ".mkv"):
		return "video/x-matroska"
	default:
		return "video/mp4"
	}
}

// Label 列表展示用。
func (d Device) Label() string {
	if d.Protocol == "" {
		return d.Name
	}
	return fmt.Sprintf("%s · %s", d.Name, d.Protocol)
}
