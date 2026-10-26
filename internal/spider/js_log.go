package spider

import "github.com/bobo/KOTV/internal/util"

var jsLogs = util.AppendLog{Name: "js-spider.log", Opened: "js-spider log opened"}

// jsLog 输出 JS 爬虫诊断日志：stderr（Flutter 可收）+ 落盘 LogDir/js-spider.log。
func jsLog(format string, args ...interface{}) {
	jsLogs.Printf(format, args...)
}

func jsPreview(s string, max int) string {
	return util.Preview(s, max)
}
