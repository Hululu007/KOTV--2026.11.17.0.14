package server

import (
	"errors"
	"fmt"
	"log"
	"net"
	"syscall"
)

// ErrPortInUse 引擎端口已被别的程序占着。
var ErrPortInUse = errors.New("端口被其他程序占用")

// wsaeAddrInUse Windows 的 WSAEADDRINUSE；syscall.EADDRINUSE 在 Windows 上不是这个值。
const wsaeAddrInUse = syscall.Errno(10048)

func isAddrInUse(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && (errno == syscall.EADDRINUSE || errno == wsaeAddrInUse)
}

// listenEngine 先占 IPv4（前端走 127.0.0.1，被别的程序占着时各系统都会在这里失败），
// 再另开只收 IPv6 的监听给远端 IPv6；IPv6 不可用时只用 IPv4。
// 不用 "tcp" 双栈：macOS 允许双栈 IPv6 与别的程序的 IPv4 同时监听同一端口，引擎「起来了」却收不到本机请求。
func listenEngine(port int) ([]net.Listener, error) {
	ln4, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		if isAddrInUse(err) {
			return nil, fmt.Errorf("%d: %w", port, ErrPortInUse)
		}
		return nil, fmt.Errorf("无法启动 HTTP 服务: %w", err)
	}
	out := []net.Listener{ln4}
	if ln6, err := net.Listen("tcp6", fmt.Sprintf("[::]:%d", port)); err == nil {
		out = append(out, ln6)
	} else {
		log.Printf("IPv6 未监听，只用 IPv4: %v", err)
	}
	return out, nil
}
