package parse

import "github.com/bobo/KOTV/internal/util"

var parseLogs = util.AppendLog{Name: "parse.log", Opened: "parse log opened"}

// parseLog 二次解析诊断：stderr + LogDir/parse.log。
func parseLog(format string, args ...interface{}) {
	parseLogs.Printf(format, args...)
}

func parsePreview(s string, max int) string {
	return util.Preview(s, max)
}
