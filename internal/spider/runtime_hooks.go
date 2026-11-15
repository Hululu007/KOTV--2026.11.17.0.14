package spider

import "sync"

// 嗅探等跨包运行时挂到统一取消/Kill 门面，避免 spider↔parse 循环依赖。
var (
	runtimeExtraMu sync.Mutex
	extraSoftScope func(scopeID string)
	extraSoftAll   func()
	extraKillUser  func(userID string)
	extraHardShared func()
	extraShutdown  func()
)

// RuntimeExtraHooks SoftCancel / Kill / HardRestart / Shutdown 的附加钩子（如 Chromium）。
type RuntimeExtraHooks struct {
	SoftCancelScope   func(scopeID string)
	SoftCancelAll     func()
	KillUser          func(userID string)
	HardRestartShared func()
	Shutdown          func()
}

// SetRuntimeExtraHooks 由 app 启动时注册（通常接 parse 嗅探）。
func SetRuntimeExtraHooks(h RuntimeExtraHooks) {
	runtimeExtraMu.Lock()
	defer runtimeExtraMu.Unlock()
	extraSoftScope = h.SoftCancelScope
	extraSoftAll = h.SoftCancelAll
	extraKillUser = h.KillUser
	extraHardShared = h.HardRestartShared
	extraShutdown = h.Shutdown
}

func runExtraSoftScope(scopeID string) {
	runtimeExtraMu.Lock()
	fn := extraSoftScope
	runtimeExtraMu.Unlock()
	if fn != nil {
		fn(scopeID)
	}
}

func runExtraSoftAll() {
	runtimeExtraMu.Lock()
	fn := extraSoftAll
	runtimeExtraMu.Unlock()
	if fn != nil {
		fn()
	}
}

func runExtraKillUser(userID string) {
	runtimeExtraMu.Lock()
	fn := extraKillUser
	runtimeExtraMu.Unlock()
	if fn != nil {
		fn(userID)
	}
}

func runExtraHardShared() {
	runtimeExtraMu.Lock()
	fn := extraHardShared
	runtimeExtraMu.Unlock()
	if fn != nil {
		fn()
	}
}

func runExtraShutdown() {
	runtimeExtraMu.Lock()
	fn := extraShutdown
	runtimeExtraMu.Unlock()
	if fn != nil {
		fn()
	}
}
