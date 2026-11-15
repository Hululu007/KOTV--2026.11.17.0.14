package spider

// SoftCancelForClient 取消当前 Scope 的 Java / Python·JS / Node 进行中调用。
// 立刻结束 Go 侧等待；被放弃的运行时单元随后回收（JAR callCtx stopLocked、
// Py 忙 slot Kill、Node 对应用户桶内 pkg Stop）。clientID 为空时 no-op。
func SoftCancelForClient(clientID string) {
	if clientID == "" {
		return
	}
	InterruptJavaBridgeForClient(clientID)
	InterruptScriptSpidersForClient(clientID)
	InterruptNodeForClient(clientID)
}

// HardRestartSharedRuntimes 硬重启本机共享爬虫运行时：JVM、Py/JS 池、共享 Node 桶。
// 不动远端各用户独立运行时，不删脚本磁盘缓存。
func HardRestartSharedRuntimes() {
	restartSharedRuntime()
}
