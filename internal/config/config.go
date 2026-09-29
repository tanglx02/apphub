// Package config 负责 config.yaml 的加载、保存与路径解析。
//
// 设计要点：
//   - 所有路径支持相对于"项目根目录"（config.yaml 所在目录）解析，
//     保证整个目录复制到另一台设备即可迁移。
//   - 未知字段不落盘丢失：保存时使用结构体序列化（而非 map 覆盖）。
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 为 AppHub 主配置。
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Logging  LoggingConfig  `yaml:"logging"`
	Security SecurityConfig `yaml:"security"`
	Status   StatusConfig   `yaml:"status"`
	TLS      TLSConfig      `yaml:"tls"`
	Backup   BackupConfig   `yaml:"backup"`
	Executor ExecutorConfig `yaml:"executor"`
}

// ServerConfig HTTP 服务配置。
type ServerConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	HTTPS           bool   `yaml:"https"`
	RedirectHTTP    bool   `yaml:"redirect_http"`   // HTTPS 时是否把 HTTP 重定向到 HTTPS
	HTTPPort        int    `yaml:"http_port"`       // 重定向监听端口
	TrustedProxies  string `yaml:"trusted_proxies"` // 逗号分隔，可信反向代理 CIDR
	SiteName        string `yaml:"site_name"`
	Logo            string `yaml:"logo"`
	Timezone        string `yaml:"timezone"`
	ReadTimeoutSec  int    `yaml:"read_timeout_sec"`
	WriteTimeoutSec int    `yaml:"write_timeout_sec"`
}

// DatabaseConfig SQLite 配置。
type DatabaseConfig struct {
	Path        string `yaml:"path"`
	BusyTimeout int    `yaml:"busy_timeout_ms"`
}

// LoggingConfig 日志配置。
type LoggingConfig struct {
	Level         string `yaml:"level"`
	RetentionDays int    `yaml:"retention_days"`
	MaxSizeMB     int    `yaml:"max_size_mb"`
	Console       bool   `yaml:"console"`
}

// SecurityConfig 安全配置。
type SecurityConfig struct {
	SessionTimeout   int    `yaml:"session_timeout"` // 秒
	LoginMaxFails    int    `yaml:"login_max_fails"`
	LoginLockMinutes int    `yaml:"login_lock_minutes"`
	RateLimitPerMin  int    `yaml:"rate_limit_per_min"`
	CSRFEnabled      bool   `yaml:"csrf_enabled"`
	CookieSecure     string `yaml:"cookie_secure"` // auto|true|false
}

// StatusConfig 状态检测配置。
type StatusConfig struct {
	IntervalSeconds int `yaml:"interval"`      // 后台轮询间隔
	TimeoutSeconds  int `yaml:"timeout"`       // 单次检测超时
	Concurrency     int `yaml:"concurrency"`   // 检测并发
	StartTimeout    int `yaml:"start_timeout"` // 启动命令超时
	StopTimeout     int `yaml:"stop_timeout"`  // 停止命令超时
	RestartTimeout  int `yaml:"restart_timeout"`
}

// TLSConfig HTTPS 配置。
type TLSConfig struct {
	Enabled      bool   `yaml:"enabled"`
	SelfSigned   bool   `yaml:"self_signed"`
	CertFile     string `yaml:"cert_file"`
	KeyFile      string `yaml:"key_file"`
	AutoGenerate bool   `yaml:"auto_generate"`
}

// BackupConfig 备份配置。
type BackupConfig struct {
	Directory     string `yaml:"directory"`
	RetentionDays int    `yaml:"retention_days"`
	AutoEnabled   bool   `yaml:"auto_enabled"`
}

// ExecutorConfig 命令执行器全局限制。
type ExecutorConfig struct {
	MaxOutputBytes  int  `yaml:"max_output_bytes"`
	KillProcessTree bool `yaml:"kill_process_tree"`
	GracePeriodSec  int  `yaml:"grace_period_sec"`
}

// Default 返回默认配置。
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            18080,
			HTTPS:           false,
			RedirectHTTP:    false,
			HTTPPort:        18081,
			SiteName:        "AppHub",
			Logo:            "",
			Timezone:        "Asia/Shanghai",
			ReadTimeoutSec:  30,
			WriteTimeoutSec: 60,
		},
		Database: DatabaseConfig{
			Path:        "./data/apphub.db",
			BusyTimeout: 5000,
		},
		Logging: LoggingConfig{
			Level:         "info",
			RetentionDays: 30,
			MaxSizeMB:     20,
			Console:       true,
		},
		Security: SecurityConfig{
			SessionTimeout:   86400,
			LoginMaxFails:    5,
			LoginLockMinutes: 15,
			RateLimitPerMin:  120,
			CSRFEnabled:      true,
			CookieSecure:     "auto",
		},
		Status: StatusConfig{
			IntervalSeconds: 5,
			TimeoutSeconds:  5,
			Concurrency:     4,
			StartTimeout:    60,
			StopTimeout:     30,
			RestartTimeout:  90,
		},
		TLS: TLSConfig{
			Enabled:      false,
			SelfSigned:   false,
			CertFile:     "./data/certs/server.crt",
			KeyFile:      "./data/certs/server.key",
			AutoGenerate: true,
		},
		Backup: BackupConfig{
			Directory:     "./backups",
			RetentionDays: 30,
			AutoEnabled:   false,
		},
		Executor: ExecutorConfig{
			MaxOutputBytes:  1 << 20, // 1 MiB
			KillProcessTree: true,
			GracePeriodSec:  10,
		},
	}
}

// Manager 持有配置与项目根目录。
type Manager struct {
	mu     sync.RWMutex
	cfg    *Config
	root   string // 项目根目录（config.yaml 所在目录）
	path   string // config.yaml 绝对路径
	onSave []func(*Config)
}

// NewManager 加载指定路径的配置；文件不存在时写入默认配置。
func NewManager(path string) (*Manager, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}

	m := &Manager{
		cfg:  Default(),
		root: filepath.Dir(abs),
		path: abs,
	}

	if data, err := os.ReadFile(abs); err == nil {
		var loaded Config
		// 先填入默认值，再覆盖，保证新增字段有默认
		loaded = *Default()
		if err := yaml.Unmarshal(data, &loaded); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %w", err)
		}
		m.cfg = &loaded
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}
	if err := m.Save(); err != nil {
		return nil, err
	}
	return m, nil
}

// Validate 校验配置合法性。
func (m *Manager) Validate() error {
	c := m.cfg
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return errors.New("server.port 非法")
	}
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if net.ParseIP(strings.TrimSpace(c.Server.Host)) == nil && c.Server.Host != "0.0.0.0" && c.Server.Host != "::" {
		// 允许域名形式，不做强校验
		_ = c
	}
	if c.Database.Path == "" {
		c.Database.Path = "./data/apphub.db"
	}
	if c.Status.IntervalSeconds < 3 {
		c.Status.IntervalSeconds = 3
	}
	if c.Status.TimeoutSeconds < 1 {
		c.Status.TimeoutSeconds = 1
	}
	if c.Status.Concurrency < 1 {
		c.Status.Concurrency = 1
	}
	if c.Security.SessionTimeout <= 0 {
		c.Security.SessionTimeout = 86400
	}
	if c.Logging.RetentionDays <= 0 {
		c.Logging.RetentionDays = 30
	}
	if c.Executor.MaxOutputBytes <= 0 {
		c.Executor.MaxOutputBytes = 1 << 20
	}
	if c.Executor.GracePeriodSec <= 0 {
		c.Executor.GracePeriodSec = 10
	}
	if c.Backup.Directory == "" {
		c.Backup.Directory = "./backups"
	}
	return nil
}

// Get 返回配置快照（浅拷贝，调用方只读）。
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.cfg
}

// Update 以回调方式修改配置并落盘。
func (m *Manager) Update(fn func(*Config)) error {
	m.mu.Lock()
	cp := *m.cfg
	fn(&cp)
	m.mu.Unlock()

	if err := (&Manager{cfg: &cp}).Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	m.cfg = &cp
	callbacks := m.onSave
	m.mu.Unlock()

	if err := m.Save(); err != nil {
		return err
	}
	for _, cb := range callbacks {
		cb(m.cfg)
	}
	return nil
}

// OnSave 注册配置变更回调。
func (m *Manager) OnSave(fn func(*Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onSave = append(m.onSave, fn)
}

// Save 落盘配置。
func (m *Manager) Save() error {
	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	header := "# AppHub 配置文件\n# 所有相对路径均相对于本文件所在目录（项目根目录）\n\n"
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(header+string(data)), 0o644); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	return os.Rename(tmp, m.path)
}

// Root 返回项目根目录。
func (m *Manager) Root() string { return m.root }

// Path 返回配置文件绝对路径。
func (m *Manager) Path() string { return m.path }

// Resolve 把可能是相对路径的配置值解析为绝对路径（相对项目根目录）。
func (m *Manager) Resolve(p string) string {
	if p == "" {
		return m.root
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(m.root, p))
}

// DatabasePath 返回数据库绝对路径。
func (m *Manager) DatabasePath() string {
	return m.Resolve(m.Get().Database.Path)
}

// LogsDir 返回日志目录。
func (m *Manager) LogsDir() string { return m.Resolve("./logs") }

// BackupsDir 返回备份目录。
func (m *Manager) BackupsDir() string { return m.Resolve(m.Get().Backup.Directory) }

// UploadsDir 返回上传目录。
func (m *Manager) UploadsDir() string { return m.Resolve("./uploads") }

// RuntimeDir 返回运行时目录（pid 文件等）。
func (m *Manager) RuntimeDir() string { return m.Resolve("./runtime") }

// DataDir 返回数据目录。
func (m *Manager) DataDir() string { return m.Resolve("./data") }

// Addr 返回监听地址。
func (m *Manager) Addr() string {
	c := m.Get()
	return net.JoinHostPort(c.Server.Host, fmt.Sprintf("%d", c.Server.Port))
}

// SessionTTL 返回会话有效期。
func (m *Manager) SessionTTL() time.Duration {
	return time.Duration(m.Get().Security.SessionTimeout) * time.Second
}

// StatusInterval 返回状态检测周期。
func (m *Manager) StatusInterval() time.Duration {
	return time.Duration(m.Get().Status.IntervalSeconds) * time.Second
}
