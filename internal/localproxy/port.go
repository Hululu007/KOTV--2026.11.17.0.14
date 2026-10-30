// Package localproxy 提供本地爬虫代理端口，避免 spider 与 server 循环依赖。
package localproxy

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/util"
)

const DefaultPort = 9978

// ExitPortInUse 引擎因 DefaultPort 被别的程序占着而退出时的退出码。
// 前端（Flutter 与安卓引擎服务）按这个值判定「端口被占」，不再自动重拉；两边写的是同一个数。
const ExitPortInUse = 98

// DefaultBridgePort 安卓 SpiderService / JAR·PY 侧车端口（与引擎 DefaultPort 分离）。
const DefaultBridgePort = 9979

var port atomic.Int32

func init() {
	port.Store(DefaultPort)
}

func SetPort(p int) {
	if p > 0 {
		port.Store(int32(p))
	}
}

func Port() int {
	p := int(port.Load())
	if p <= 0 {
		return DefaultPort
	}
	return p
}

// PortOrDefault 读取注入的端口；未设置或不是正数时用 DefaultPort。
func PortOrDefault(fn func() int) int {
	if fn != nil {
		if p := fn(); p > 0 {
			return p
		}
	}
	return DefaultPort
}

// LocalBaseURL 当前运行时本机引擎根（无 /proxy）。
func LocalBaseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", Port())
}

// DefaultBridgeBaseURL 本机 Spider 侧车根地址（无尾斜杠）。
func DefaultBridgeBaseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", DefaultBridgePort)
}

func BaseURL(local bool) string {
	if !local {
		// 远端请求：优先当前对外根（域名/反代），避免把局域网 IP 交给客户端。
		if b := strings.TrimRight(strings.TrimSpace(hostclient.PublicBase()), "/"); b != "" {
			return b + "/proxy"
		}
		if ip := util.LanIP(); ip != "" {
			return fmt.Sprintf("http://%s:%d/proxy", ip, Port())
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%d/proxy", Port())
}
