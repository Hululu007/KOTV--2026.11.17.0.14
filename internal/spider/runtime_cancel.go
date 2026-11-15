package spider

import (
	"strings"

	"github.com/bobo/KOTV/internal/hostclient"
)

// 爬虫运行时取消 / 重启门面（JVM · Python · QuickJS · Node · Chromium）。
// 包外请只走本文件导出符号；底层 interrupt* / sniff 钩子不导出。

// SoftCancelForClient 取消指定 Scope 下全部引擎进行中的调用。
// 立刻结束 Go 侧等待；被放弃的单元随后回收
//（JAR callCtx → stopLocked、Py/JS 忙 slot Kill、Node 对应用户桶 pkg Stop、Chromium tab）。
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
	interruptJavaBridgeForClient(clientID)
	interruptScriptSpidersForClient(clientID) // Python + QuickJS
	interruptNodeForClient(clientID)
	if clientID == "" {
		runExtraSoftAll()
	} else {
		runExtraSoftScope(clientID)
	}
}

// HardRestartSharedRuntimes 硬重启本机共享爬虫运行时：JVM、Python、QuickJS、共享 Node、共享 Chromium。
// 不动远端各用户独立运行时，不删脚本磁盘缓存。
func HardRestartSharedRuntimes() {
	restartSharedRuntime()
	runExtraHardShared()
}

// KillUserRuntime 杀掉指定用户的 JVM、Python、QuickJS、Node、Chromium（不动本机共享与其它用户）。
func KillUserRuntime(userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	forgetJarRecent(userID)
	jarPathByUser.Delete(userID)
	clearJSRedirectsForUser(userID)
	DestroyUserScripts(userID)
	clearJarForUser(userID)
	userBridgesMu.Lock()
	b := userBridges[userID]
	delete(userBridges, userID)
	userBridgesMu.Unlock()
	if b != nil {
		b.epoch.Add(1)
		b.mu.Lock()
		b.stopLocked()
		b.mu.Unlock()
	}
	destroyUserNodeHost(userID)
	runExtraKillUser(userID)
}

// RestartCallerRuntime 卡死恢复时硬重启当前请求所属运行时。切仓不走这里。
func RestartCallerRuntime() {
	if uid := hostclient.RuntimeUserID(); uid != "" {
		KillUserRuntime(uid)
		return
	}
	HardRestartSharedRuntimes()
}

// ShutdownAllRuntimes 进程退出：硬清共享 + 销毁全部脚本/Node（含用户桶）+ 关 JVM + Chromium。
func ShutdownAllRuntimes() {
	HardRestartSharedRuntimes()
	ResetScriptSpiders()
	ShutdownJavaBridge()
	runExtraShutdown()
}
