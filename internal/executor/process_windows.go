//go:build windows

package executor

import (
	"os"
	"os/exec"
	"time"
)

// osSignal 为平台相关的信号类型（Windows 下仅作数值占位）。
type osSignal = int

func osFindProcess(pid int) (*os.Process, error) { return os.FindProcess(pid) }

// setProcGroup Windows 无 POSIX 进程组，退化为普通进程（开发环境使用）。
func setProcGroup(cmd *exec.Cmd) {}

func pidOf(cmd *exec.Cmd) int {
	if cmd.Process != nil {
		return cmd.Process.Pid
	}
	return 0
}

func pgidOf(cmd *exec.Cmd) int { return 0 }

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := osFindProcess(pid)
	if err != nil {
		return false
	}
	return p != nil
}

func killTree(cmd *exec.Cmd, graceSec int) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

func termSignal() osSignal { return 15 }

func killSignal() osSignal { return 9 }

func signalGroup(p *Process, sig osSignal) error {
	proc, err := osFindProcess(p.PID)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func signalPid(pid int, sig osSignal) error {
	proc, err := osFindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func shellCommand() (string, string) { return "cmd", "/C" }

// 占位：Windows 下不实现真实存活探测的时间等待
var _ = time.Second
