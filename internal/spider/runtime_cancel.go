package spider

// 爬虫运行时取消 / 重启门面（JVM · Python · QuickJS · Node）。
// 包外请只走本文件与 KillUserRuntime / RestartCallerRuntime / ResetScriptSpiders /
// ClearJarDisk / ShutdownJavaBridge；勿再散调 Interrupt*。

// SoftCancelForClient 取消指定 Scope 下全部引擎进行中的调用。
// 立刻结束 Go 侧等待；被放弃的单元随后回收
//（JAR callCtx → stopLocked、Py/JS 忙 slot Kill、Node 对应用户桶 pkg Stop）。
// clientID 为空时 no-op（避免误伤全部会话）。
func SoftCancelForClient(clientID string) {
	if clientID == "" {
		return
	}
	softCancelEngines(clientID)
}

// SoftCancelAll 打断全部 Scope 的进行中调用（清缓存等全局操作）。
// 与 SoftCancelForClient("") 的 no-op 不同。
func SoftCancelAll() {
	softCancelEngines("")
}

func softCancelEngines(clientID string) {
	InterruptJavaBridgeForClient(clientID)
	InterruptScriptSpidersForClient(clientID) // Python + QuickJS
	InterruptNodeForClient(clientID)
}

// HardRestartSharedRuntimes 硬重启本机共享爬虫运行时：JVM、Python、QuickJS、共享 Node。
// 不动远端各用户独立运行时，不删脚本磁盘缓存。
func HardRestartSharedRuntimes() {
	restartSharedRuntime()
}

// ShutdownAllRuntimes 进程退出：硬清共享 + 销毁全部脚本/Node（含用户桶）+ 关 JVM。
func ShutdownAllRuntimes() {
	HardRestartSharedRuntimes()
	ResetScriptSpiders()
	ShutdownJavaBridge()
}
