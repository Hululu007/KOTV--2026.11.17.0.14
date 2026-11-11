package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bobo/KOTV/internal/model"
	"github.com/bobo/KOTV/internal/settings"
	"github.com/bobo/KOTV/internal/util"
)

const playSniffBytes = 512
const playSniffTimeout = 3 * time.Second

// playMode 区分点播透传与直播直连；format 判定（含正文嗅探）统一走 planPlayback。
type playMode int

const (
	playModeVod playMode = iota // 点播：HLS 预登记 /proxy/play
	playModeLive                // 直播开播/回看：不因 HLS 透传
)

// playPlan 一次开播的地址与 format 决策结果。
type playPlan struct {
	Play, Direct string
	Headers      map[string]string
	Format       string
}

// sniffPlayFormatFn 单测可替换，避免真实网络。
var sniffPlayFormatFn = sniffPlayFormatHTTP

// planPlayback 开播唯一入口：format（站源 → URL 强信号 → 正文嗅探）+ 可播地址。
// 列表 PreparePlaybackListed 不走此处，避免海量请求。
func (a *App) planPlayback(raw, format string, headers map[string]string, mode playMode) playPlan {
	raw = strings.TrimSpace(raw)
	format = strings.TrimSpace(format)
	outFmt, isHls := decidePlayFormat(raw, format, headers, true)
	vod := mode == playModeVod
	play, direct, hdrs := a.preparePlayback(raw, headers, vod, isHls)
	return playPlan{Play: play, Direct: direct, Headers: hdrs, Format: outFmt}
}

// decidePlayFormat：DASH/HLS 声明 → URL 强信号 → 其它非空 format → 可选正文嗅探。
// allowSniff=false 仅给 PreparePlayback 等批量路径（不发网）。
func decidePlayFormat(raw, format string, headers map[string]string, allowSniff bool) (out string, isHls bool) {
	raw = strings.TrimSpace(raw)
	format = strings.TrimSpace(format)
	if model.FormatIsDash(format) {
		return model.FormatDASH, false
	}
	if model.FormatIsHls(format) {
		return model.FormatHLS, true
	}
	if model.URLIsHls(raw) {
		return model.FormatHLS, true
	}
	if format != "" {
		return format, false
	}
	if !allowSniff {
		return "", false
	}
	sniffed := sniffPlayFormatFn(raw, headers)
	if model.FormatIsHls(sniffed) {
		return model.FormatHLS, true
	}
	if sniffed != "" {
		return sniffed, false
	}
	return "", false
}

func sniffPlayFormatHTTP(raw string, headers map[string]string) string {
	ctx, cancel := context.WithTimeout(context.Background(), playSniffTimeout)
	defer cancel()
	return sniffPlayFormat(ctx, raw, headers)
}

func sniffPlayFormat(ctx context.Context, raw string, headers map[string]string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return ""
	}
	head, ct, err := fetchPlayPrefix(ctx, raw, headers, true)
	if err != nil || len(head) == 0 {
		head, ct, err = fetchPlayPrefix(ctx, raw, headers, false)
		if err != nil {
			return ""
		}
	}
	return model.ClassifyPlayPrefix(head, ct)
}

func fetchPlayPrefix(ctx context.Context, raw string, headers map[string]string, withRange bool) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	util.ApplyHeaders(req, headers)
	if withRange {
		req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", playSniffBytes-1))
	}
	client := &http.Client{
		Timeout:   playSniffTimeout,
		Transport: util.GetClient().Transport,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("play sniff http %d", resp.StatusCode)
	}
	buf, err := io.ReadAll(io.LimitReader(resp.Body, playSniffBytes))
	if err != nil && len(buf) == 0 {
		return nil, "", err
	}
	return buf, resp.Header.Get("Content-Type"), nil
}

// shouldRegisterVodHls 点播 HLS 预登记透传（首开仍可走 Direct；失败再切 url）。
// filter 仅随去广告开关；登记本身不拉流。
func shouldRegisterVodHls(isHls bool) (register, filter bool) {
	if !isHls {
		return false, false
	}
	return true, settings.IsAdFilterEnabled()
}
