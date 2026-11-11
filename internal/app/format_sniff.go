package app

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bobo/KOTV/internal/model"
	"github.com/bobo/KOTV/internal/util"
)

const playSniffBytes = 512
const playSniffTimeout = 3 * time.Second

// sniffPlayFormatFn 可在单测里替换，避免真实网络。
var sniffPlayFormatFn = sniffPlayFormatHTTP

// resolvePlayFormat 合并站源 mime、URL 强信号与正文嗅探，得到回包 format。
// isHls 为真时点播可据此强制登记 /proxy/play（伪装地址无 m3u8 字样时）。
func resolvePlayFormat(raw, format string, headers map[string]string) (out string, isHls bool) {
	raw = strings.TrimSpace(raw)
	format = strings.TrimSpace(format)
	if model.FormatIsDash(format) {
		if strings.EqualFold(format, "mpd") || strings.EqualFold(format, "dash") {
			return model.FormatDASH, false
		}
		return format, false
	}
	if model.FormatIsHls(format) {
		return model.FormatHLS, true
	}
	if model.URLIsHls(raw) {
		return model.FormatHLS, true
	}
	sniffed := sniffPlayFormatFn(raw, headers)
	if model.FormatIsHls(sniffed) {
		return model.FormatHLS, true
	}
	if sniffed != "" {
		return sniffed, false
	}
	return format, false
}

func sniffPlayFormatHTTP(raw string, headers map[string]string) string {
	ctx, cancel := context.WithTimeout(context.Background(), playSniffTimeout)
	defer cancel()
	return SniffPlayFormat(ctx, raw, headers)
}

// SniffPlayFormat 对 http(s) 直链短读前缀，判定 HLS/DASH mime；失败返回空。
func SniffPlayFormat(ctx context.Context, raw string, headers map[string]string) string {
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
	applyPlaySniffHeaders(req, headers)
	if withRange {
		req.Header.Set("Range", "bytes=0-"+strconv.Itoa(playSniffBytes-1))
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
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusPartialContent {
		return nil, "", errPlaySniffStatus(resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, playSniffBytes)
	buf, err := io.ReadAll(limited)
	if err != nil && len(buf) == 0 {
		return nil, "", err
	}
	return buf, resp.Header.Get("Content-Type"), nil
}

type playSniffStatusError int

func (e playSniffStatusError) Error() string { return "play sniff http " + strconv.Itoa(int(e)) }

func errPlaySniffStatus(code int) error { return playSniffStatusError(code) }

func applyPlaySniffHeaders(req *http.Request, headers map[string]string) {
	if headers == nil || strings.TrimSpace(headers["User-Agent"]) == "" {
		req.Header.Set("User-Agent", util.OkHttpUA)
	}
	for k, v := range headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
}
