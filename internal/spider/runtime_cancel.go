package spider

// SoftCancelForClient 软打断当前 Scope 的 Java / Python·JS / Node 进行中调用。
// 协作取消或回收被放弃的运行时单元（Node 只 Stop 该 Scope 所属用户桶内的包）。
// clientID 为空时不做任何事（避免误伤其它窗口）。
func SoftCancelForClient(clientID string) {
	if clientID == "" {
		return
	}
	InterruptJavaBridgeForClient(clientID)
	InterruptScriptSpidersForClient(clientID)
	InterruptNodeForClient(clientID)
}

// HardRestartSharedRuntimes 硬重启本机共享爬虫运行时：JVM、Py/JS 池、共享 Node 桶。
// 不动远端各用户独立运行时（含每用户 Node 桶），不删脚本磁盘缓存。
func HardRestartSharedRuntimes() {
	RestartSharedRuntime()
}
