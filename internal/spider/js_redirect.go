package spider

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/bobo/KOTV/internal/util"
)

// jsRedirects：302 Location（绝对 URL）→ 发起请求的原始 URL（JS 爬虫 insecure 客户端）。
var jsRedirects = util.NewRedirectOrigins(512)

// ClearJSRedirectMap 清空 JS 侧 302 回源映射（清理缓存工具）。
func ClearJSRedirectMap() {
	jsRedirects.Clear()
}

func applyJSRedirectDance(reqURL string, resp *http.Response, body []byte) (code int, hdrs http.Header, out []byte) {
	code = resp.StatusCode
	hdrs = resp.Header.Clone()
	out = body
	if util.IsHTTPRedirect(code) {
		if loc := resp.Header.Get("Location"); loc != "" {
			jsRedirects.Remember(reqURL, loc)
		}
		return code, hdrs, out
	}
	if code == 406 {
		if origin, ok := jsRedirects.Lookup(reqURL); ok {
			h := make(http.Header)
			h.Set("Location", origin)
			return http.StatusFound, h, nil
		}
	}
	return code, hdrs, out
}

func drainBody(resp *http.Response) []byte {
	if resp == nil || resp.Body == nil {
		return nil
	}
	b, _ := io.ReadAll(resp.Body)
	return b
}

// jsRequestTransport 关闭自动解压；TLS 不校验；hosts 表改写 dial 主机名。
func jsRequestTransport() http.RoundTripper {
	base := util.GetClient().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	var cl *http.Transport
	if t, ok := base.(*http.Transport); ok {
		cl = t.Clone()
	} else {
		cl = &http.Transport{}
	}
	cl.DisableCompression = true
	if cl.TLSClientConfig == nil {
		cl.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	} else {
		cl.TLSClientConfig = cl.TLSClientConfig.Clone()
		cl.TLSClientConfig.InsecureSkipVerify = true
	}
	baseDial := cl.DialContext
	if baseDial == nil {
		d := &net.Dialer{Timeout: 30 * time.Second}
		baseDial = d.DialContext
	}
	cl.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
			port = ""
		}
		mapped := ResolveVodHost(host)
		if mapped != host && mapped != "" {
			if port != "" {
				addr = net.JoinHostPort(mapped, port)
			} else {
				addr = mapped
			}
		}
		return baseDial(ctx, network, addr)
	}
	return cl
}

func wrapDialTimeout(base func(context.Context, string, string) (net.Conn, error), d time.Duration) func(context.Context, string, string) (net.Conn, error) {
	if d <= 0 {
		return base
	}
	if base == nil {
		dialer := &net.Dialer{Timeout: d}
		base = dialer.DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		cctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return base(cctx, network, addr)
	}
}

// decodeJSContentEncoding：gzip，以及 nowrap deflate。
func decodeJSContentEncoding(encoding string, body []byte) []byte {
	return util.DecodeContentEncoding(encoding, body)
}
