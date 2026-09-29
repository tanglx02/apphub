package api

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/tanglx02/apphub/internal/buildinfo"
	"github.com/tanglx02/apphub/internal/config"
	"github.com/tanglx02/apphub/internal/executor"
	"github.com/tanglx02/apphub/internal/health"
	"github.com/tanglx02/apphub/internal/logging"
	"github.com/tanglx02/apphub/internal/system"
)

// siteName 返回系统名称（设置优先于配置文件）。
func (s *Server) siteName() string {
	if v := strings.TrimSpace(s.deps.Settings.Get("site_name", "")); v != "" {
		return v
	}
	return s.deps.Config.Get().Server.SiteName
}

// dbVersion 返回数据库 schema 版本。
func (s *Server) dbVersion() int {
	v, err := s.deps.DB.Version()
	if err != nil {
		return 0
	}
	return v
}

// versionString 返回版本号。
func versionString() string { return buildinfo.Short() }

// processStats 读取进程 CPU / 内存占用（非 Linux 返回 0）。
func processStats(pid int) (cpuPercent float64, memMB float64) {
	return system.ProcessStats(pid)
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// healthOptionsFrom 由配置派生状态检测参数。
func healthOptionsFrom(cfg config.Config) health.Options {
	return health.Options{
		Interval:    time.Duration(cfg.Status.IntervalSeconds) * time.Second,
		Timeout:     time.Duration(cfg.Status.TimeoutSeconds) * time.Second,
		Concurrency: cfg.Status.Concurrency,
	}
}

// executorConfigFrom 由配置派生执行器限制。
func executorConfigFrom(cfg config.Config) executor.Config {
	return executor.Config{
		MaxOutputBytes:  cfg.Executor.MaxOutputBytes,
		GracePeriodSec:  cfg.Executor.GracePeriodSec,
		KillProcessTree: cfg.Executor.KillProcessTree,
		DefaultTimeout:  time.Duration(cfg.Status.StartTimeout) * time.Second,
	}
}

// loggingLevel 日志级别类型别名。
type loggingLevel = logging.Level

func parseLevel(s string) loggingLevel { return logging.ParseLevel(s) }
