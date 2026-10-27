package download

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	appruntime "github.com/bobo/KOTV/internal/runtime"
)

func isHLSURL(raw, format string) bool {
	f := strings.ToLower(strings.TrimSpace(format))
	if strings.Contains(f, "hls") || f == "m3u8" {
		return true
	}
	u := strings.ToLower(strings.Split(raw, "?")[0])
	return strings.Contains(u, ".m3u8") || strings.Contains(raw, "=m3u8")
}

func downloadHLS(ctx context.Context, src string, headers map[string]string, dest string, onProg func(float64, int64, int64, string)) error {
	tmp := dest + ".hls"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	playlist, err := httpGetString(ctx, src, headers)
	if err != nil {
		return err
	}
	if strings.Contains(playlist, "#EXT-X-STREAM-INF") {
		sub := firstPlaylistURI(playlist)
		if sub == "" {
			return fmt.Errorf("HLS 主列表无可用子流")
		}
		src = resolveURI(src, sub)
		playlist, err = httpGetString(ctx, src, headers)
		if err != nil {
			return err
		}
	}
	segs := segmentURIs(playlist)
	if len(segs) == 0 {
		return fmt.Errorf("HLS 无切片")
	}
	listPath := filepath.Join(tmp, "list.txt")
	var list strings.Builder
	var total int64
	for i, seg := range segs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		u := resolveURI(src, seg)
		part := filepath.Join(tmp, fmt.Sprintf("seg_%05d.ts", i))
		n, err := httpGetFile(ctx, u, headers, part)
		if err != nil {
			return fmt.Errorf("切片 %d: %w", i+1, err)
		}
		total += n
		list.WriteString("file '")
		list.WriteString(strings.ReplaceAll(part, `'`, `'\''`))
		list.WriteString("'\n")
		if onProg != nil {
			onProg(float64(i+1)/float64(len(segs))*0.9, total, 0, fmt.Sprintf("切片 %d/%d", i+1, len(segs)))
		}
	}
	if err := os.WriteFile(listPath, []byte(list.String()), 0o644); err != nil {
		return err
	}
	if onProg != nil {
		onProg(0.92, total, 0, "正在合并…")
	}
	if err := mergeSegments(ctx, listPath, segs, tmp, dest); err != nil {
		return err
	}
	if onProg != nil {
		if st, e := os.Stat(dest); e == nil {
			onProg(1, st.Size(), st.Size(), "完成")
		} else {
			onProg(1, total, total, "完成")
		}
	}
	return nil
}

func mergeSegments(ctx context.Context, listPath string, segs []string, tmp, dest string) error {
	ff := appruntime.FFmpeg()
	if ff != "" {
		cmd := exec.CommandContext(ctx, ff, "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", dest)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("ffmpeg 合并失败: %w (%s)", err, truncate(string(out), 200))
		}
		return nil
	}
	if runtime.GOOS == "ios" {
		return fmt.Errorf("本机无 ffmpeg，HLS 合并请连接远端引擎")
	}
	// 无 ffmpeg：MPEG-TS 二进制拼接（多数 HLS）；失败再报错。
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	for i := range segs {
		part := filepath.Join(tmp, fmt.Sprintf("seg_%05d.ts", i))
		f, err := os.Open(part)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, f)
		_ = f.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func httpGetString(ctx context.Context, raw string, headers map[string]string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 KOTV")
	}
	cli := &http.Client{Timeout: 60 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func httpGetFile(ctx context.Context, raw string, headers map[string]string, dest string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 KOTV")
	}
	cli := &http.Client{Timeout: 120 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return io.Copy(f, resp.Body)
}

func firstPlaylistURI(body string) string {
	sc := bufio.NewScanner(strings.NewReader(body))
	want := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			want = true
			continue
		}
		if want && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

func segmentURIs(body string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func resolveURI(base, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return base
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	u, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return u.ResolveReference(r).String()
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
