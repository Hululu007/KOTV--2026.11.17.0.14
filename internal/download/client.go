package download

import (
	"fmt"
	"net/http"
	"time"
)

// newUpstreamClient 下载用 HTTP 客户端：跨域 302 时重挂 Cookie/鉴权头
//（默认 Client 会剥 Cookie，网盘 CDN 易 403 / unexpected EOF）。
// timeout<=0 表示不限制（整片长下载）。
func newUpstreamClient(headers map[string]string, timeout time.Duration) *http.Client {
	cli := &http.Client{
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			applyHeaders(next, headers)
			return nil
		},
	}
	if timeout > 0 {
		cli.Timeout = timeout
	}
	return cli
}
