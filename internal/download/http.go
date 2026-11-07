package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func downloadHTTP(ctx context.Context, raw string, headers map[string]string, dest string, threads int, onProg func(bytes, total int64)) (int64, int64, error) {
	n, total, err := downloadHTTPOnce(ctx, raw, headers, dest, threads, onProg)
	if err == nil {
		return n, total, nil
	}
	// 网盘 CDN 并行 Range / 长连接常中途掐流；清残片后强制单线程再试一次。
	if !isUnexpectedEOF(err) {
		return n, total, err
	}
	cleanupDownloadParts(dest)
	return downloadHTTPOnce(ctx, raw, headers, dest, 1, onProg)
}

func downloadHTTPOnce(ctx context.Context, raw string, headers map[string]string, dest string, threads int, onProg func(bytes, total int64)) (int64, int64, error) {
	threads = clampThreads(threads)
	total, acceptRange, err := probeSize(ctx, raw, headers)
	if err != nil {
		return 0, 0, err
	}
	if total <= 0 || !acceptRange || threads == 1 {
		return downloadHTTPSingle(ctx, raw, headers, dest, onProg)
	}
	partSize := total / int64(threads)
	if partSize < 256*1024 {
		return downloadHTTPSingle(ctx, raw, headers, dest, onProg)
	}

	tmpParts := make([]string, threads)
	var written atomic.Int64
	var firstErr error
	var errMu sync.Mutex
	var wg sync.WaitGroup
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	for i := 0; i < threads; i++ {
		start := int64(i) * partSize
		end := start + partSize - 1
		if i == threads-1 {
			end = total - 1
		}
		part := fmt.Sprintf("%s.part%d", dest, i)
		tmpParts[i] = part
		wg.Add(1)
		go func(i int, start, end int64, part string) {
			defer wg.Done()
			n, err := downloadRange(ctx2, raw, headers, part, start, end, func(delta int64) {
				cur := written.Add(delta)
				if onProg != nil {
					onProg(cur, total)
				}
			})
			_ = n
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				errMu.Unlock()
			}
		}(i, start, end, part)
	}
	wg.Wait()
	if firstErr != nil {
		for _, p := range tmpParts {
			_ = os.Remove(p)
		}
		return written.Load(), total, firstErr
	}
	out, err := os.Create(dest + ".part")
	if err != nil {
		return written.Load(), total, err
	}
	for _, p := range tmpParts {
		f, err := os.Open(p)
		if err != nil {
			_ = out.Close()
			_ = os.Remove(dest + ".part")
			return written.Load(), total, err
		}
		_, copyErr := io.Copy(out, f)
		_ = f.Close()
		_ = os.Remove(p)
		if copyErr != nil {
			_ = out.Close()
			_ = os.Remove(dest + ".part")
			return written.Load(), total, copyErr
		}
	}
	_ = out.Close()
	_ = os.Remove(dest)
	if err := os.Rename(dest+".part", dest); err != nil {
		return written.Load(), total, err
	}
	return total, total, nil
}

func isUnexpectedEOF(err error) bool {
	if err == nil {
		return false
	}
	if err == io.ErrUnexpectedEOF {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "unexpected eof")
}

func cleanupDownloadParts(dest string) {
	_ = os.Remove(dest)
	_ = os.Remove(dest + ".part")
	for i := 0; i < 16; i++ {
		_ = os.Remove(fmt.Sprintf("%s.part%d", dest, i))
	}
}

func probeSize(ctx context.Context, raw string, headers map[string]string) (int64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, raw, nil)
	if err != nil {
		return 0, false, err
	}
	applyHeaders(req, headers)
	resp, err := newUpstreamClient(headers, 30*time.Second).Do(req)
	if err != nil {
		return 0, false, nil // 探测失败则退回单线程 GET
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, false, nil
	}
	total := resp.ContentLength
	ar := resp.Header.Get("Accept-Ranges")
	accept := ar != "" && ar != "none"
	if total <= 0 {
		if cl := resp.Header.Get("Content-Length"); cl != "" {
			total, _ = strconv.ParseInt(cl, 10, 64)
		}
	}
	return total, accept, nil
}

func downloadRange(ctx context.Context, raw string, headers map[string]string, dest string, start, end int64, onDelta func(int64)) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return 0, err
	}
	applyHeaders(req, headers)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := newUpstreamClient(headers, 0).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	buf := make([]byte, 256*1024)
	var written int64
	for {
		if ctx.Err() != nil {
			_ = os.Remove(dest)
			return written, ctx.Err()
		}
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			nw, ew := f.Write(buf[:nr])
			written += int64(nw)
			if onDelta != nil {
				onDelta(int64(nw))
			}
			if ew != nil {
				_ = os.Remove(dest)
				return written, ew
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			_ = os.Remove(dest)
			return written, er
		}
	}
	return written, nil
}

func downloadHTTPSingle(ctx context.Context, raw string, headers map[string]string, dest string, onProg func(bytes, total int64)) (int64, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return 0, 0, err
	}
	applyHeaders(req, headers)
	resp, err := newUpstreamClient(headers, 0).Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	total := resp.ContentLength
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, 0, err
	}
	var written int64
	buf := make([]byte, 256*1024)
	for {
		if ctx.Err() != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return written, total, ctx.Err()
		}
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			nw, ew := f.Write(buf[:nr])
			written += int64(nw)
			if onProg != nil {
				onProg(written, total)
			}
			if ew != nil {
				_ = f.Close()
				_ = os.Remove(tmp)
				return written, total, ew
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return written, total, er
		}
	}
	_ = f.Close()
	_ = os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return written, total, err
	}
	return written, total, nil
}

func applyHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 KOTV")
	}
}
