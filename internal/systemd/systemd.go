// Package systemd 以结构化方式调用 systemctl。
//
// 安全设计：
//   - 服务名必须通过白名单正则校验，禁止注入任意 Shell 命令；
//   - 命令始终以 executable + args 方式执行，不使用 shell；
//   - 所有调用都有超时。
package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/tanglx02/apphub/internal/executor"
	"github.com/tanglx02/apphub/internal/models"
)

// unitPattern 允许的服务名格式：仅允许安全字符，且必须以受支持的类型结尾。
var unitPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@:\-]{0,200}\.(service|socket|timer|mount|target)$`)

// ErrInvalidUnit 服务名非法。
var ErrInvalidUnit = errors.New("systemd 服务名非法：仅允许字母、数字、点、下划线、@、-、:，且必须以 .service/.socket/.timer/.mount/.target 结尾")

// ValidateUnit 校验服务名，返回标准化后的名称。
func ValidateUnit(unit string) (string, error) {
	u := strings.TrimSpace(unit)
	if u == "" {
		return "", ErrInvalidUnit
	}
	if strings.ContainsAny(u, " \t\n\r\"'`$&|;<>(){}[]!*?~#\\") {
		return "", ErrInvalidUnit
	}
	if !unitPattern.MatchString(u) {
		return "", ErrInvalidUnit
	}
	return u, nil
}

// Available 判断 systemctl 是否存在。
func Available() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}

// Client 封装 systemctl 调用。
type Client struct {
	exec    *executor.Executor
	useSudo bool
	timeout time.Duration
}

// NewClient 创建 systemd 客户端。非 root 运行时自动使用 sudo（需配合 /etc/sudoers.d/apphub）。
func NewClient(ex *executor.Executor, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	useSudo := false
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		useSudo = true
	}
	return &Client{exec: ex, useSudo: useSudo, timeout: timeout}
}

// UseSudo 返回是否使用 sudo。
func (c *Client) UseSudo() bool { return c.useSudo }

// SetUseSudo 覆盖 sudo 策略（设置项）。
func (c *Client) SetUseSudo(v bool) { c.useSudo = v }

func (c *Client) cmd(args []string) (executable string, argv []string) {
	if c.useSudo {
		return "sudo", append([]string{"systemctl"}, args...)
	}
	return "systemctl", args
}

func (c *Client) run(ctx context.Context, action string, args []string, timeout time.Duration) (*executor.Result, error) {
	exe, argv := c.cmd(args)
	spec := executor.Spec{
		Name:       "systemd:" + action,
		Executable: exe,
		Args:       argv,
		Timeout:    timeout,
	}
	return c.exec.Run(ctx, spec)
}

// Start 启动服务。
func (c *Client) Start(ctx context.Context, unit string) error {
	u, err := ValidateUnit(unit)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "start", []string{"start", u}, c.timeout)
	return err
}

// Stop 停止服务。
func (c *Client) Stop(ctx context.Context, unit string) error {
	u, err := ValidateUnit(unit)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "stop", []string{"stop", u}, c.timeout)
	return err
}

// Restart 重启服务。
func (c *Client) Restart(ctx context.Context, unit string) error {
	u, err := ValidateUnit(unit)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "restart", []string{"restart", u}, c.timeout)
	return err
}

// Enable 设置开机自启。
func (c *Client) Enable(ctx context.Context, unit string) error {
	u, err := ValidateUnit(unit)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "enable", []string{"enable", u}, c.timeout)
	return err
}

// Disable 取消开机自启。
func (c *Client) Disable(ctx context.Context, unit string) error {
	u, err := ValidateUnit(unit)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "disable", []string{"disable", u}, c.timeout)
	return err
}

// ActiveState 返回 systemctl is-active 的原始输出。
func (c *Client) ActiveState(ctx context.Context, unit string) (string, error) {
	u, err := ValidateUnit(unit)
	if err != nil {
		return "", err
	}
	res, err := c.run(ctx, "is-active", []string{"is-active", u}, 5*time.Second)
	if err != nil && res == nil {
		return "", err
	}
	if res == nil {
		return "unknown", nil
	}
	return strings.TrimSpace(res.Stdout), nil
}

// UnitStatus 服务详细状态。
type UnitStatus struct {
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	LoadState   string `json:"load_state"`
	MainPID     int    `json:"main_pid"`
	Description string `json:"description"`
	Enabled     string `json:"enabled"`
}

// Show 读取服务详细属性。
func (c *Client) Show(ctx context.Context, unit string) (*UnitStatus, error) {
	u, err := ValidateUnit(unit)
	if err != nil {
		return nil, err
	}
	res, err := c.run(ctx, "show", []string{"show", u,
		"--property=ActiveState,SubState,LoadState,MainPID,Description,UnitFileState", "--no-pager"}, 5*time.Second)
	if err != nil || res == nil {
		return nil, err
	}
	st := &UnitStatus{ActiveState: "unknown"}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := line[:idx]
		val := line[idx+1:]
		switch key {
		case "ActiveState":
			st.ActiveState = val
		case "SubState":
			st.SubState = val
		case "LoadState":
			st.LoadState = val
		case "MainPID":
			fmt.Sscanf(val, "%d", &st.MainPID)
		case "Description":
			st.Description = val
		case "UnitFileState":
			st.Enabled = val
		}
	}
	return st, nil
}

// Journal 读取服务最近 N 行日志。
func (c *Client) Journal(ctx context.Context, unit string, lines int) (string, error) {
	u, err := ValidateUnit(unit)
	if err != nil {
		return "", err
	}
	if lines <= 0 {
		lines = 100
	}
	if lines > 2000 {
		lines = 2000
	}
	exe := "journalctl"
	args := []string{"-u", u, "-n", fmt.Sprintf("%d", lines), "--no-pager", "-o", "cat"}
	if c.useSudo {
		exe = "sudo"
		args = append([]string{"journalctl"}, args...)
	}
	res, err := c.exec.Run(ctx, executor.Spec{
		Name:       "systemd:journal",
		Executable: exe,
		Args:       args,
		Timeout:    10 * time.Second,
	})
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", nil
	}
	return res.Stdout + res.Stderr, nil
}

// ParseActiveState 把 systemctl 输出映射为应用状态。
func ParseActiveState(state string) models.AppStatus {
	switch strings.TrimSpace(state) {
	case "active":
		return models.StatusOnline
	case "inactive":
		return models.StatusOffline
	case "failed":
		return models.StatusError
	case "activating", "reloading":
		return models.StatusStarting
	case "deactivating":
		return models.StatusStopping
	default:
		return models.StatusUnknown
	}
}
