package download

import (
	"strconv"
	"strings"

	"github.com/bobo/KOTV/internal/settings"
)

func concurrencyLimit() int {
	n, _ := strconv.Atoi(strings.TrimSpace(settings.Get(settings.DownloadConcurrency)))
	if n < 1 {
		n = 2
	}
	if n > 5 {
		n = 5
	}
	return n
}

func threadLimit() int {
	n, _ := strconv.Atoi(strings.TrimSpace(settings.Get(settings.DownloadThreads)))
	if n < 1 {
		n = 4
	}
	return snapThreadOption(n)
}

// snapThreadOption 收成界面可选值 1/2/4/8（先钳到硬上限）。
func snapThreadOption(n int) int {
	n = clampThreads(n)
	switch n {
	case 1, 2, 4, 8:
		return n
	default:
		return 4
	}
}

// clampThreads 单任务并行硬上限（HTTP Range / HLS 切片共用）。
func clampThreads(n int) int {
	if n < 1 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}
