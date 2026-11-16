package localproxy

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// BridgeClient 共用 Transport，供 Go→桥（jar/py/health）复用连接；超时只靠调用方 context。
func BridgeClient() *http.Client {
	bridgeHTTPOnce.Do(func() {
		bridgeHTTP = &http.Client{
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConns:        64,
				MaxIdleConnsPerHost: 16,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	})
	return bridgeHTTP
}

// BridgeHealthy 探测 base+"/health"；超时用 ctx，Client 复用 BridgeClient。
func BridgeHealthy(base string, timeout time.Duration) bool {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return false
	}
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := BridgeClient().Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode < 400
}

var (
	bridgeHTTPOnce sync.Once
	bridgeHTTP     *http.Client
)
