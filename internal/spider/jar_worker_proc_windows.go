//go:build windows

package spider

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// CREATE_NO_WINDOW：不分配控制台；仅 HideWindow 时 console 子系统（node.exe 等）仍会先闪黑框。
const createNoWindow = 0x08000000

// setChildProcAttrs 隐藏 Windows 控制台黑框（node / python / java / taskkill）。
func setChildProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// killProcessTree 用 taskkill /T 杀掉进程树（JVM 可能再拉子进程）。
func killProcessTree(proc *os.Process) {
	if proc == nil {
		return
	}
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(proc.Pid))
	setChildProcAttrs(cmd)
	_ = cmd.Run()
	_ = proc.Kill()
}
