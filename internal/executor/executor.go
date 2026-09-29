// Package executor 负责以受控方式执行外部命令。
//
// 安全设计：
//  1. 默认使用 executable + args，绝不拼接 shell 字符串；
//  2. Shell 模式必须由管理员显式开启，并在 UI 上明确警告；
//  3. 所有执行都有超时（context.WithTimeout）；
//  4. 输出大小有上限，防止无限输出撑爆内存；
//  5. 启动型命令使用独立进程组，停止时整组清理，避免子进程残留；
//  6. 所有执行都会回调审计函数记录日志。
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tanglx02/apphub/internal/logging"
)

// Spec 描述一次命令执行。
type Spec struct {
	Name       string        // 可读名称，用于日志
	Executable string        // 可执行文件（非 shell 模式）
	Args       []string      // 结构化参数
	Command    string        // 完整命令行（仅 shell 模式）
	Shell      bool          // 是否启用 shell 模式
	WorkDir    string        // 工作目录
	Env        []string      // 额外环境变量（KEY=VALUE）
	Timeout    time.Duration // 超时时间
	MaxOutput  int           // 单流最大输出字节
	AppID      int64         // 关联应用，用于审计
}

// Result 为命令执行结果。
type Result struct {
	ExitCode  int           `json:"exit_code"`
	Stdout    string        `json:"stdout"`
	Stderr    string        `json:"stderr"`
	Duration  time.Duration `json:"duration_ms"`
	TimedOut  bool          `json:"timed_out"`
	Truncated bool          `json:"truncated"`
	Error     string        `json:"error,omitempty"`
	PID       int           `json:"pid,omitempty"`
}

// AuditFunc 审计回调签名。
type AuditFunc func(appID int64, action string, result string, detail string)

// Config 执行器全局配置。
type Config struct {
	MaxOutputBytes  int
	GracePeriodSec  int
	KillProcessTree bool
	DefaultTimeout  time.Duration
}

// Executor 命令执行器。
type Executor struct {
	cfg    Config
	mu     sync.Mutex
	audit  AuditFunc
	logger *logging.Logger
}

// New 创建执行器。
func New(cfg Config, audit AuditFunc) *Executor {
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 1 << 20
	}
	if cfg.GracePeriodSec <= 0 {
		cfg.GracePeriodSec = 10
	}
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = 60 * time.Second
	}
	return &Executor{cfg: cfg, audit: audit}
}

// SetLogger 注入日志器。
func (e *Executor) SetLogger(l *logging.Logger) { e.logger = l }

// SetAudit 注入审计回调（所有命令执行都会回调）。
func (e *Executor) SetAudit(fn AuditFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.audit = fn
}

// Config 返回当前配置。
func (e *Executor) Config() Config { return e.cfg }

// UpdateConfig 更新配置（设置页保存后调用）。
func (e *Executor) UpdateConfig(cfg Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cfg.MaxOutputBytes > 0 {
		e.cfg.MaxOutputBytes = cfg.MaxOutputBytes
	}
	if cfg.GracePeriodSec > 0 {
		e.cfg.GracePeriodSec = cfg.GracePeriodSec
	}
	if cfg.DefaultTimeout > 0 {
		e.cfg.DefaultTimeout = cfg.DefaultTimeout
	}
	e.cfg.KillProcessTree = cfg.KillProcessTree
}

func (e *Executor) log(format string, args ...any) {
	if e.logger != nil {
		e.logger.Info(format, args...)
	}
}

func (e *Executor) auditf(appID int64, action, result, detail string) {
	if e.audit != nil {
		e.audit(appID, action, result, detail)
	}
}

// ErrInvalidSpec 命令定义非法。
var ErrInvalidSpec = errors.New("命令定义为空或非法")

// Validate 校验命令定义（不执行）。
func Validate(s Spec) error {
	if s.Shell {
		if strings.TrimSpace(s.Command) == "" {
			return ErrInvalidSpec
		}
		return nil
	}
	if strings.TrimSpace(s.Executable) == "" {
		return ErrInvalidSpec
	}
	return nil
}

// Run 同步执行命令并等待结束（用于停止/重启/测试）。
func (e *Executor) Run(ctx context.Context, spec Spec) (*Result, error) {
	if err := Validate(spec); err != nil {
		return nil, err
	}
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = e.cfg.DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, err := e.build(spec)
	if err != nil {
		return nil, err
	}

	maxOut := spec.MaxOutput
	if maxOut <= 0 {
		maxOut = e.cfg.MaxOutputBytes
	}
	var outBuf, errBuf limitedBuffer
	outBuf.limit = maxOut
	errBuf.limit = maxOut
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	start := time.Now()
	e.log("执行命令 [%s]: %s", spec.Name, describe(spec))
	if err := cmd.Start(); err != nil {
		res := &Result{ExitCode: -1, Stdout: outBuf.String(), Stderr: errBuf.String(), Error: err.Error()}
		e.auditf(spec.AppID, spec.Name, "failed", truncate(err.Error(), 500))
		return res, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var waitErr error
	timedOut := false
	select {
	case waitErr = <-done:
	case <-runCtx.Done():
		timedOut = true
		e.log("命令超时 [%s]，强制终止进程组", spec.Name)
		killTree(cmd, e.cfg.GracePeriodSec)
		select {
		case waitErr = <-done:
		case <-time.After(3 * time.Second):
			waitErr = errors.New("强制终止超时")
		}
	}

	res := &Result{
		ExitCode:  exitCode(waitErr, cmd),
		Stdout:    outBuf.String(),
		Stderr:    errBuf.String(),
		Duration:  time.Since(start),
		TimedOut:  timedOut,
		Truncated: outBuf.truncated || errBuf.truncated,
		PID:       pidOf(cmd),
	}
	if waitErr != nil {
		res.Error = waitErr.Error()
	}
	status := "success"
	if res.ExitCode != 0 || timedOut {
		status = "failed"
	}
	e.auditf(spec.AppID, spec.Name, status, fmt.Sprintf("exit=%d timeout=%v", res.ExitCode, timedOut))
	return res, nil
}

// StartBackground 启动常驻进程（不等待退出），并将其纳入进程组管理。
func (e *Executor) StartBackground(ctx context.Context, spec Spec) (*Process, error) {
	if err := Validate(spec); err != nil {
		return nil, err
	}
	cmd, err := e.build(spec)
	if err != nil {
		return nil, err
	}
	// 后台进程不继承父进程的输出管道，避免阻塞；输出写入日志目录
	logFile, err := openProcessLog(spec)
	if err != nil {
		logging.Warn("无法创建进程日志文件: %v", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		e.auditf(spec.AppID, spec.Name, "failed", truncate(err.Error(), 500))
		return nil, fmt.Errorf("启动命令执行失败: %w", err)
	}

	p := &Process{
		AppID:     spec.AppID,
		PID:       cmd.Process.Pid,
		PGID:      pgidOf(cmd),
		Cmd:       describe(spec),
		WorkDir:   spec.WorkDir,
		StartedAt: time.Now(),
		cmd:       cmd,
		logFile:   logFile,
	}

	// 异步回收，避免僵尸进程
	go func() {
		_ = cmd.Wait()
		p.markExited()
	}()

	e.log("已启动后台进程 [%s] pid=%d", spec.Name, p.PID)
	e.auditf(spec.AppID, spec.Name, "success", fmt.Sprintf("pid=%d", p.PID))
	Registry().Set(p)
	return p, nil
}

// StopProcess 优雅停止进程：SIGTERM -> 等待优雅期 -> SIGKILL。
func (e *Executor) StopProcess(p *Process, timeout time.Duration) error {
	if p == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = time.Duration(e.cfg.GracePeriodSec) * time.Second
	}
	if !alive(p.PID) {
		Registry().Remove(p.AppID)
		return nil
	}

	e.log("停止进程 pid=%d (优雅期 %v)", p.PID, timeout)
	if err := signalGroup(p, termSignal()); err != nil {
		logging.Warn("发送终止信号失败 pid=%d: %v", p.PID, err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !alive(p.PID) {
			Registry().Remove(p.AppID)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if e.cfg.KillProcessTree {
		_ = signalGroup(p, killSignal())
	} else {
		_ = signalPid(p.PID, killSignal())
	}
	time.Sleep(500 * time.Millisecond)
	Registry().Remove(p.AppID)
	return nil
}

// build 构造 exec.Cmd。
func (e *Executor) build(spec Spec) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if spec.Shell {
		sh, flag := shellCommand()
		cmd = exec.Command(sh, flag, spec.Command)
	} else {
		exe := spec.Executable
		// 相对路径按工作目录解析，避免 PATH 歧义
		if !filepath.IsAbs(exe) && strings.ContainsRune(exe, '/') && spec.WorkDir != "" {
			exe = filepath.Join(spec.WorkDir, exe)
		}
		cmd = exec.Command(exe, spec.Args...)
	}
	if spec.WorkDir != "" {
		cmd.Dir = spec.WorkDir
	}
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	// 独立进程组：停止时可整组清理
	setProcGroup(cmd)
	return cmd, nil
}

// describe 生成用于日志/展示的命令描述（注意：不用于执行）。
func describe(spec Spec) string {
	if spec.Shell {
		return "shell: " + spec.Command
	}
	if len(spec.Args) == 0 {
		return spec.Executable
	}
	return spec.Executable + " " + strings.Join(spec.Args, " ")
}

func exitCode(err error, cmd *exec.Cmd) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	return -1
}

// limitedBuffer 限制最大写入字节数，超出部分丢弃并标记截断。
type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
	mu        sync.Mutex
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil // 丢弃但仍报告成功，避免命令因 EPIPE 退出
	}
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var _ io.Writer = (*limitedBuffer)(nil)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func openProcessLog(spec Spec) (*os.File, error) {
	dir := processLogDir()
	_ = os.MkdirAll(dir, 0o755)
	name := fmt.Sprintf("app-%d.log", spec.AppID)
	if name == "app-0.log" {
		name = "command.log"
	}
	return os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

var processLogDirFunc = func() string { return filepath.Join(".", "runtime", "logs") }

// SetProcessLogDir 设置后台进程日志目录。
func SetProcessLogDir(dir string) {
	processLogDirFunc = func() string { return dir }
}

func processLogDir() string { return processLogDirFunc() }

// IsLinux 判断当前运行环境是否为 Linux（影响进程组能力）。
func IsLinux() bool { return runtime.GOOS == "linux" }
