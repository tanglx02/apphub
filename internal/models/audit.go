package models

import "time"

// AuditAction 审计动作类型。
type AuditAction string

const (
	AuditLogin          AuditAction = "login"
	AuditLogout         AuditAction = "logout"
	AuditLoginFailed    AuditAction = "login_failed"
	AuditAppCreate      AuditAction = "app_create"
	AuditAppUpdate      AuditAction = "app_update"
	AuditAppDelete      AuditAction = "app_delete"
	AuditAppStart       AuditAction = "app_start"
	AuditAppStop        AuditAction = "app_stop"
	AuditAppRestart     AuditAction = "app_restart"
	AuditBatchStart     AuditAction = "batch_start"
	AuditBatchStop      AuditAction = "batch_stop"
	AuditBatchRestart   AuditAction = "batch_restart"
	AuditSettingsUpdate AuditAction = "settings_update"
	AuditBackup         AuditAction = "backup"
	AuditRestore        AuditAction = "restore"
	AuditPasswordReset  AuditAction = "password_reset"
	AuditCategoryChange AuditAction = "category_change"
	AuditImport         AuditAction = "import"
	AuditExport         AuditAction = "export"
	AuditTestCommand    AuditAction = "test_command"
)

// Text 返回中文动作名，便于审计页面展示。
func (a AuditAction) Text() string {
	switch a {
	case AuditLogin:
		return "登录"
	case AuditLogout:
		return "退出登录"
	case AuditLoginFailed:
		return "登录失败"
	case AuditAppCreate:
		return "添加应用"
	case AuditAppUpdate:
		return "修改应用"
	case AuditAppDelete:
		return "删除应用"
	case AuditAppStart:
		return "启动应用"
	case AuditAppStop:
		return "停止应用"
	case AuditAppRestart:
		return "重启应用"
	case AuditBatchStart:
		return "批量启动"
	case AuditBatchStop:
		return "批量停止"
	case AuditBatchRestart:
		return "批量重启"
	case AuditSettingsUpdate:
		return "修改设置"
	case AuditBackup:
		return "备份"
	case AuditRestore:
		return "恢复"
	case AuditPasswordReset:
		return "重置密码"
	case AuditCategoryChange:
		return "修改分类"
	case AuditImport:
		return "导入配置"
	case AuditExport:
		return "导出配置"
	case AuditTestCommand:
		return "测试命令"
	}
	return string(a)
}

// AuditLog 审计日志条目。
type AuditLog struct {
	ID        int64       `json:"id"`
	UserID    int64       `json:"user_id"`
	Username  string      `json:"username"`
	IP        string      `json:"ip"`
	Action    AuditAction `json:"action"`
	ActionCN  string      `json:"action_cn"`
	Target    string      `json:"target"`
	Result    string      `json:"result"` // success | failed
	Detail    string      `json:"detail"`
	CreatedAt time.Time   `json:"created_at"`
}

// BackupRecord 备份记录。
type BackupRecord struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// SystemInfo 系统运行信息。
type SystemInfo struct {
	Version     string  `json:"version"`
	Commit      string  `json:"commit"`
	BuildTime   string  `json:"build_time"`
	Hostname    string  `json:"hostname"`
	OS          string  `json:"os"`
	Arch        string  `json:"arch"`
	Kernel      string  `json:"kernel"`
	Uptime      int64   `json:"uptime_seconds"`
	CPUPercent  float64 `json:"cpu_percent"`
	CPUCores    int     `json:"cpu_cores"`
	CPUModel    string  `json:"cpu_model"`
	MemTotalMB  uint64  `json:"mem_total_mb"`
	MemUsedMB   uint64  `json:"mem_used_mb"`
	DiskTotalGB float64 `json:"disk_total_gb"`
	DiskUsedGB  float64 `json:"disk_used_gb"`
	LoadAvg     string  `json:"load_avg"`
	IP          string  `json:"ip"`
	Timezone    string  `json:"timezone"`
	DBVersion   int     `json:"db_version"`
	APIVersion  string  `json:"api_version"`
}
