package spider

// SoftCancelForClient 取消当前 Scope 下全部脚本引擎进行中的调用：
// JVM、Python、QuickJS、Node。立刻结束 Go 侧等待；被放弃的单元随后回收
//（JAR callCtx → stopLocked、Py/JS 忙 slot Kill、Node 对应用户桶 pkg Stop）。
// clientID 为空时 no-op。
func SoftCancelForClient(clientID string) {
	if clientID == "" {
		return
	}
	InterruptJavaBridgeForClient(clientID)
	InterruptScriptSpidersForClient(clientID) // Python + QuickJS
	InterruptNodeForClient(clientID)
}

// HardRestartSharedRuntimes 硬重启本机共享爬虫运行时：JVM、Python、QuickJS、共享 Node。
// 不动远端各用户独立运行时，不删脚本磁盘缓存。
func HardRestartSharedRuntimes() {
	restartSharedRuntime()
}
