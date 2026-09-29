package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testSpec(t *testing.T, name string) Spec {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS != "windows" {
		return Spec{Name: name, Executable: "/bin/sh", Args: []string{"-c", "echo hello"}, WorkDir: dir, Timeout: 5 * time.Second}
	}
	return Spec{Name: name, Shell: true, Command: "echo hello", WorkDir: dir, Timeout: 5 * time.Second}
}

func TestRunCaptureOutput(t *testing.T) {
	e := New(Config{MaxOutputBytes: 1 << 16, GracePeriodSec: 1, DefaultTimeout: 5 * time.Second}, nil)
	res, err := e.Run(context.Background(), testSpec(t, "echo"))
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !strings.Contains(res.Stdout, "hello") {
		t.Fatalf("未捕获到输出: %q", res.Stdout)
	}
	if res.ExitCode != 0 {
		t.Fatalf("退出码应为 0，实际 %d (%s)", res.ExitCode, res.Stderr)
	}
}

func TestRunTimeout(t *testing.T) {
	e := New(Config{MaxOutputBytes: 1 << 16, GracePeriodSec: 1, DefaultTimeout: 2 * time.Second}, nil)

	var spec Spec
	if runtime.GOOS != "windows" {
		spec = Spec{Name: "sleep", Executable: "/bin/sh", Args: []string{"-c", "sleep 30"}, Timeout: 500 * time.Millisecond}
	} else {
		spec = Spec{Name: "sleep", Shell: true, Command: "ping -n 30 127.0.0.1 >nul", Timeout: 500 * time.Millisecond}
	}

	start := time.Now()
	res, err := e.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("超时执行不应返回 error: %v", err)
	}
	if !res.TimedOut {
		t.Fatal("应标记为超时")
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("超时未及时终止进程")
	}
}

func TestOutputLimit(t *testing.T) {
	e := New(Config{MaxOutputBytes: 1024, GracePeriodSec: 1, DefaultTimeout: 5 * time.Second}, nil)

	var spec Spec
	if runtime.GOOS != "windows" {
		spec = Spec{Name: "big", Executable: "/bin/sh", Args: []string{"-c", "yes abcdefghij | head -c 200000"}, Timeout: 5 * time.Second}
	} else {
		spec = Spec{Name: "big", Shell: true, Command: "for /L %i in (1,1,20000) do @echo abcdefghij", Timeout: 5 * time.Second}
	}
	res, err := e.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if len(res.Stdout) > 2048 {
		t.Fatalf("输出未被限制: %d 字节", len(res.Stdout))
	}
}

func TestValidateSpec(t *testing.T) {
	if err := Validate(Spec{}); err == nil {
		t.Fatal("空定义应被拒绝")
	}
	if err := Validate(Spec{Executable: "/bin/true"}); err != nil {
		t.Fatalf("合法定义被拒绝: %v", err)
	}
	if err := Validate(Spec{Shell: true, Command: "echo hi"}); err != nil {
		t.Fatalf("合法 shell 定义被拒绝: %v", err)
	}
	if err := Validate(Spec{Shell: true, Command: "  "}); err == nil {
		t.Fatal("空 shell 命令应被拒绝")
	}
}

func TestStartBackgroundAndStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("进程组管理能力仅在 Linux 下完整支持")
	}
	SetProcessLogDir(t.TempDir())

	e := New(Config{MaxOutputBytes: 1 << 16, GracePeriodSec: 2, KillProcessTree: true, DefaultTimeout: 5 * time.Second}, nil)
	spec := Spec{
		Name:    "sleeper",
		Shell:   true,
		Command: "sleep 300",
		AppID:   99,
		Timeout: 3 * time.Second,
		WorkDir: t.TempDir(),
	}

	p, err := e.StartBackground(context.Background(), spec)
	if err != nil {
		t.Fatalf("后台启动失败: %v", err)
	}
	if p.PID <= 0 {
		t.Fatal("PID 无效")
	}
	if !alive(p.PID) {
		t.Fatal("进程应处于运行中")
	}
	if Registry().Get(99) == nil {
		t.Fatal("进程未登记到注册表")
	}

	if err := e.StopProcess(p, 3*time.Second); err != nil {
		t.Fatalf("停止进程失败: %v", err)
	}
	if alive(p.PID) {
		t.Fatal("进程应已退出")
	}
}

func TestRegistryPersistence(t *testing.T) {
	dir := t.TempDir()
	Registry().SetDir(dir)
	Registry().Set(&Process{AppID: 1234, PID: os.Getpid(), StartedAt: time.Now()})
	if Registry().Get(1234) == nil {
		t.Fatal("登记失败")
	}
	Registry().Remove(1234)
	if Registry().Get(1234) != nil {
		t.Fatal("移除失败")
	}
	if _, err := os.Stat(filepath.Join(dir, "app-1234.json")); !os.IsNotExist(err) {
		t.Fatal("PID 文件未清理")
	}
}

func TestLimitedBuffer(t *testing.T) {
	var b limitedBuffer
	b.limit = 8
	n, _ := b.Write([]byte("1234567890"))
	if n != 10 {
		t.Fatalf("应报告写入全部字节，实际 %d", n)
	}
	if b.String() != "12345678" {
		t.Fatalf("超出部分应被丢弃: %q", b.String())
	}
	if !b.truncated {
		t.Fatal("应标记为截断")
	}
}

var _ = exec.Command
