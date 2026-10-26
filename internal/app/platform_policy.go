package app

import (
	"fmt"
	"runtime"
)

// iOS 端不提供本地爬虫能力（JS/PY/站点抓取）；仅保留前端展示与播放链路。
func localCrawlerDisabled() bool {
	return runtime.GOOS == "ios"
}

func requireLocalCrawler() error {
	if localCrawlerDisabled() {
		return fmt.Errorf("请先连接可用后端服务")
	}
	return nil
}
