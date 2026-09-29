//go:build !windows

package executor

import (
	"os/exec"
	"syscall"
	"time"
)

// setProcGroup 让子进程拥有独立进程组，便于停止时整组清理。
func setProcGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func pidOf(cmd *exec.Cmd) int {
	if cmd.Process != nil {
		return cmd.Process.Pid
	}
	return 0
}

// pgidOf 返回子进程所在进程组 ID（Setpgid 成功后等于 PID）。
func pgidOf(cmd *exec.Cmd) int {
	if cmd.Process != nil {
		return cmd.Process.Pid
	}
	return 0
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// killTree 终止进程组：先 SIGTERM，等待优雅期后 SIGKILL。
func killTree(cmd *exec.Cmd, graceSec int) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if graceSec <= 0 {
		graceSec = 10
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	deadline := time.Now().Add(time.Duration(graceSec) * time.Second)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

func termSignal() osSignal { return syscall.SIGTERM }

func killSignal() osSignal { return syscall.SIGKILL }

func signalGroup(p *Process, sig osSignal) error {
	target := p.PID
	if p.PGID > 0 {
		target = -p.PGID
	}
	return syscall.Kill(target, sig)
}

func signalPid(pid int, sig osSignal) error { return syscall.Kill(pid, sig) }

func shellCommand() (string, string) { return "/bin/sh", "-c" }

// osSignal 为平台相关的信号类型。
type osSignal = syscall.Signal
