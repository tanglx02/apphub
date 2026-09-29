// Package health 实现应用在线检测（HTTP / TCP / Process / systemd）。
package health

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tanglx02/apphub/internal/models"
)

// httpScheme 仅允许 http/https，避免 file:// 等协议被滥用。
var httpScheme = regexp.MustCompile(`^https?://`)

// CheckHTTP 通过 HTTP 状态码判断应用是否在线。
// expected 为空时，2xx/3xx 均视为在线。
func CheckHTTP(ctx context.Context, rawURL string, timeout time.Duration, expected []int) (bool, string) {
	if !httpScheme.MatchString(strings.TrimSpace(rawURL)) {
		return false, "检查地址协议非法"
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, "构造请求失败"
	}
	req.Header.Set("User-Agent", "AppHub-HealthCheck/1.0")

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, // 本地自签名证书常见
			DisableKeepAlives: true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("重定向次数过多")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, "请求失败: " + err.Error()
	}
	defer resp.Body.Close()

	code := resp.StatusCode
	if len(expected) > 0 {
		for _, c := range expected {
			if code == c {
				return true, fmt.Sprintf("HTTP %d", code)
			}
		}
		return false, fmt.Sprintf("HTTP %d 不符合期望状态码", code)
	}
	if code >= 200 && code < 400 {
		return true, fmt.Sprintf("HTTP %d", code)
	}
	return false, fmt.Sprintf("HTTP %d", code)
}

// CheckTCP 通过 TCP 连接判断在线。
func CheckTCP(ctx context.Context, addr string, timeout time.Duration) (bool, string) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false, "地址为空"
	}
	if !strings.Contains(addr, ":") {
		// 纯端口形式，按本机处理
		if _, err := strconv.Atoi(addr); err == nil {
			addr = "127.0.0.1:" + addr
		} else {
			return false, "地址格式非法"
		}
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false, "连接失败: " + err.Error()
	}
	_ = conn.Close()
	return true, "TCP 已连接 " + addr
}

// ProcessAlive 由平台相关代码提供，判断 PID 是否存活。
var ProcessAlive = func(pid int) bool { return false }

// CheckProcess 通过进程 PID 判断在线。
func CheckProcess(pid int) (bool, string) {
	if pid <= 0 {
		return false, "PID 无效"
	}
	if ProcessAlive(pid) {
		return true, fmt.Sprintf("进程 %d 运行中", pid)
	}
	return false, fmt.Sprintf("进程 %d 不存在", pid)
}

// Checker 统一入口：按应用配置选择检测方式。
type Checker struct {
	SystemdActive func(unit string) string
}

// Check 执行检测，返回状态与详情。
func (c *Checker) Check(ctx context.Context, app *models.App, pid int, timeout time.Duration) (models.AppStatus, string) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	target := strings.TrimSpace(app.StatusTarget)

	switch app.StatusType {
	case models.CheckHTTP:
		if target == "" {
			target = app.InternalURL
		}
		if target == "" {
			return models.StatusUnknown, "未配置 HTTP 检查地址"
		}
		ok, detail := CheckHTTP(ctx, target, timeout, app.ExpectedStatusCodes())
		if ok {
			return models.StatusOnline, detail
		}
		return models.StatusOffline, detail

	case models.CheckTCP:
		if target == "" {
			target = hostPortOf(app.InternalURL)
		}
		ok, detail := CheckTCP(ctx, target, timeout)
		if ok {
			return models.StatusOnline, detail
		}
		return models.StatusOffline, detail

	case models.CheckProcess:
		if pid <= 0 {
			n, err := strconv.Atoi(target)
			if err != nil {
				return models.StatusOffline, "未记录进程 PID"
			}
			pid = n
		}
		ok, detail := CheckProcess(pid)
		if ok {
			return models.StatusOnline, detail
		}
		return models.StatusOffline, detail

	case models.CheckSystemd:
		unit := target
		if unit == "" {
			unit = app.SystemdUnit
		}
		if unit == "" {
			return models.StatusUnknown, "未配置 systemd 服务名"
		}
		if c.SystemdActive == nil {
			return models.StatusUnknown, "systemd 不可用"
		}
		return systemdStatus(c.SystemdActive(unit)), "systemctl is-active = " + c.SystemdActive(unit)

	case models.CheckNone:
		return models.StatusUnknown, "未启用健康检查"

	default:
		// 未配置时按应用类型推断
		if app.AppType == models.AppTypeSystemd && app.SystemdUnit != "" && c.SystemdActive != nil {
			state := c.SystemdActive(app.SystemdUnit)
			return systemdStatus(state), "systemctl is-active = " + state
		}
		if app.InternalURL != "" {
			ok, detail := CheckHTTP(ctx, app.InternalURL, timeout, app.ExpectedStatusCodes())
			if ok {
				return models.StatusOnline, detail
			}
			return models.StatusOffline, detail
		}
		return models.StatusUnknown, "未配置检查方式"
	}
}

func systemdStatus(state string) models.AppStatus {
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
	}
	return models.StatusUnknown
}

// hostPortOf 从 URL 提取 host:port。
func hostPortOf(rawURL string) string {
	u := strings.TrimSpace(rawURL)
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "https://")
	if idx := strings.IndexAny(u, "/?#"); idx >= 0 {
		u = u[:idx]
	}
	return u
}
