// Package models 定义 AppHub 的核心数据模型与枚举。
package models

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// AppStatus 应用运行状态。
type AppStatus string

const (
	StatusOnline   AppStatus = "ONLINE"
	StatusOffline  AppStatus = "OFFLINE"
	StatusStarting AppStatus = "STARTING"
	StatusStopping AppStatus = "STOPPING"
	StatusError    AppStatus = "ERROR"
	StatusUnknown  AppStatus = "UNKNOWN"
)

// IsTransitioning 判断状态是否处于过渡态（此时禁止重复操作）。
func (s AppStatus) IsTransitioning() bool {
	return s == StatusStarting || s == StatusStopping
}

// IsRunning 判断应用是否处于运行态。
func (s AppStatus) IsRunning() bool { return s == StatusOnline }

// AppType 应用控制类型（仅 LOCAL 应用使用；systemd / 自定义命令）。
type AppType string

const (
	AppTypeSystemd AppType = "systemd"
	AppTypeCommand AppType = "command"
)

// AppScope 应用范围类型：本地应用可管理，非本地应用仅导航 + 在线检测。
type AppScope string

const (
	ScopeLocal    AppScope = "LOCAL"
	ScopeExternal AppScope = "EXTERNAL"
)

// Text 返回范围的中文文案（前台展示用）。
func (s AppScope) Text() string {
	if s == ScopeExternal {
		return "非本地应用"
	}
	return "本地应用"
}

// Endpoint 应用访问入口（附加导航链接；本地应用的 内网/公网 快捷入口仍由
// internal_url / external_url 承载，此处用于扩展更多入口）。
type Endpoint struct {
	ID          int64  `json:"id"`
	AppID       int64  `json:"app_id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Type        string `json:"type"`
	Enabled     bool   `json:"enabled"`
	OpenNewTab  bool   `json:"open_new_tab"`
	SortOrder   int    `json:"sort_order"`
	Description string `json:"description,omitempty"`
}

// StatusCheckType 健康检查方式。
type StatusCheckType string

const (
	CheckHTTP    StatusCheckType = "http"
	CheckTCP     StatusCheckType = "tcp"
	CheckProcess StatusCheckType = "process"
	CheckSystemd StatusCheckType = "systemd"
	CheckNone    StatusCheckType = "none"
)

// App 为被管理的应用定义。
type App struct {
	ID          int64  `json:"id" db:"id"`
	Name        string `json:"name" db:"name"`
	Slug        string `json:"slug" db:"slug"`
	Description string `json:"description" db:"description"`
	Icon        string `json:"icon" db:"icon"`
	CategoryID  int64  `json:"category_id" db:"category_id"`
	Tags        string `json:"tags" db:"tags"`
	InternalURL string `json:"internal_url" db:"internal_url"`
	ExternalURL string `json:"external_url" db:"external_url"`

	AppType     AppType `json:"app_type" db:"app_type"`
	SystemdUnit string  `json:"systemd_unit" db:"systemd_unit"`

	StartCommand   string `json:"start_command" db:"start_command"`
	StopCommand    string `json:"stop_command" db:"stop_command"`
	RestartCommand string `json:"restart_command" db:"restart_command"`

	// 结构化参数：命令以 JSON 数组存储，避免 shell 注入
	ShellMode   bool   `json:"shell_mode" db:"shell_mode"`
	StartArgs   string `json:"start_args" db:"start_args"`
	StopArgs    string `json:"stop_args" db:"stop_args"`
	RestartArgs string `json:"restart_args" db:"restart_args"`

	StatusType    StatusCheckType `json:"status_type" db:"status_type"`
	StatusTarget  string          `json:"status_target" db:"status_target"`
	ExpectedCodes string          `json:"expected_codes" db:"expected_codes"`
	WorkDir       string          `json:"work_dir" db:"work_dir"`
	Environment   string          `json:"environment" db:"environment"`

	Enabled         bool `json:"enabled" db:"enabled"`
	AutoStart       bool `json:"auto_start" db:"auto_start"`
	Favorite        bool `json:"favorite" db:"favorite"`
	SortOrder       int  `json:"sort_order" db:"sort_order"`
	TimeoutSeconds  int  `json:"timeout_seconds" db:"timeout_seconds"`
	CheckTimeoutSec int  `json:"check_timeout_sec" db:"check_timeout_sec"`

	// PublicVisible 控制是否在前台只读导航中展示（不影响后台管理）。
	PublicVisible bool `json:"public_visible" db:"public_visible"`

	// Scope 应用范围：LOCAL（本机可管理）/ EXTERNAL（仅导航 + 在线检测）。
	// EXTERNAL 应用忽略所有管理字段（systemd/命令/工作目录等）。
	Scope AppScope `json:"type" db:"scope"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// TagList 返回标签切片。
func (a *App) TagList() []string {
	if strings.TrimSpace(a.Tags) == "" {
		return nil
	}
	parts := strings.FieldsFunc(a.Tags, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Args 解析结构化参数（JSON 数组）。
func (a *App) Args(field string) []string {
	raw := ""
	switch field {
	case "start":
		raw = a.StartArgs
	case "stop":
		raw = a.StopArgs
	case "restart":
		raw = a.RestartArgs
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		// 兼容空格分隔的旧格式
		return strings.Fields(raw)
	}
	return out
}

// ExpectedStatusCodes 返回期望的 HTTP 状态码集合，空表示 2xx/3xx 均可。
func (a *App) ExpectedStatusCodes() []int {
	if strings.TrimSpace(a.ExpectedCodes) == "" {
		return nil
	}
	var out []int
	if err := json.Unmarshal([]byte(a.ExpectedCodes), &out); err == nil {
		return out
	}
	for _, p := range strings.FieldsFunc(a.ExpectedCodes, func(r rune) bool { return r == ',' }) {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// EnvMap 解析环境变量配置（每行 KEY=VALUE）。
func (a *App) EnvMap() []string {
	if strings.TrimSpace(a.Environment) == "" {
		return nil
	}
	out := make([]string, 0)
	for _, line := range strings.Split(a.Environment, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// Category 应用分类。
type Category struct {
	ID        int64     `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Slug      string    `json:"slug" db:"slug"`
	Icon      string    `json:"icon" db:"icon"`
	Color     string    `json:"color" db:"color"`
	SortOrder int       `json:"sort_order" db:"sort_order"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// AppRuntime 为运行时状态（内存态，来自 StatusManager）。
type AppRuntime struct {
	AppID       int64      `json:"app_id"`
	Status      AppStatus  `json:"status"`
	PID         int        `json:"pid,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	UptimeSec   int64      `json:"uptime_seconds,omitempty"`
	LastChecked time.Time  `json:"last_checked"`
	LastError   string     `json:"last_error,omitempty"`
	Detail      string     `json:"detail,omitempty"`
	CPU         float64    `json:"cpu_percent,omitempty"`
	MemoryMB    float64    `json:"memory_mb,omitempty"`
}

// AppWithStatus 为前端使用的应用 + 实时状态组合。
type AppWithStatus struct {
	App
	Runtime       AppRuntime `json:"runtime"`
	CategoryName  string     `json:"category_name,omitempty"`
	CategoryIcon  string     `json:"category_icon,omitempty"`
	CategoryColor string     `json:"category_color,omitempty"`
}

// MarshalJSON 展平 App 字段，避免前端嵌套取值。
func (a AppWithStatus) MarshalJSON() ([]byte, error) {
	type alias App
	m := make(map[string]any)
	b, err := json.Marshal(alias(a.App))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	m["runtime"] = a.Runtime
	if a.CategoryName != "" {
		m["category_name"] = a.CategoryName
	}
	if a.CategoryIcon != "" {
		m["category_icon"] = a.CategoryIcon
	}
	if a.CategoryColor != "" {
		m["category_color"] = a.CategoryColor
	}
	return json.Marshal(m)
}

// PublicApp 为前台只读导航的公开视图（PublicAppDTO）。
//
// 安全约束：仅允许展示普通用户需要的字段，
// 严禁包含 systemd 服务名、启停命令、工作目录、环境变量、PID 等管理信息。
type PublicApp struct {
	ID            int64            `json:"id"`
	Name          string           `json:"name"`
	Type          AppScope         `json:"type"`
	Description   string           `json:"description,omitempty"`
	Icon          string           `json:"icon,omitempty"`
	Category      string           `json:"category,omitempty"`
	CategoryIcon  string           `json:"category_icon,omitempty"`
	CategoryColor string           `json:"category_color,omitempty"`
	Status        AppStatus        `json:"status"`
	Endpoints     []PublicEndpoint `json:"endpoints"`
	Tags          string           `json:"tags,omitempty"`
	SortOrder     int              `json:"sort_order"`
}

// PublicEndpoint 前台展示的访问入口（不含任何管理信息）。
type PublicEndpoint struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	OpenNewTab bool   `json:"open_new_tab"`
}

// PublicConfig 为前台导航的全局公开配置。
type PublicConfig struct {
	Enabled        bool   `json:"enabled"`
	Title          string `json:"title"`
	Subtitle       string `json:"subtitle"`
	ShowResources  bool   `json:"show_resources"`
	ShowCategories bool   `json:"show_categories"`
	ShowSearch     bool   `json:"show_search"`
	AllowFavorite  bool   `json:"allow_favorite"`
	DefaultTheme   string `json:"default_theme"`
}

// PublicSummary 为前台概览统计（不泄露主机名/路径等管理信息）。
type PublicSummary struct {
	Total        int       `json:"total"`
	LocalApps    int       `json:"local_apps"`
	ExternalApps int       `json:"external_apps"`
	Online       int       `json:"online"`
	Offline      int       `json:"offline"`
	Error        int       `json:"error"`
	Starting     int       `json:"starting"`
	OnlineRate   int       `json:"online_rate"`
	ServerUp     bool      `json:"server_up"`
	CPU          float64   `json:"cpu_percent,omitempty"`
	MemTotal     uint64    `json:"mem_total_mb,omitempty"`
	MemUsed      uint64    `json:"mem_used_mb,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}
