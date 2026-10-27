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
	return clampThreads(n)
}

// clampThreads 单任务并行上限（HTTP Range / HLS 切片共用）。
func clampThreads(n int) int {
	if n < 1 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}
