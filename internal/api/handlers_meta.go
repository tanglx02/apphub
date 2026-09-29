package api

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tanglx02/apphub/internal/apps"
	"github.com/tanglx02/apphub/internal/backup"
	"github.com/tanglx02/apphub/internal/buildinfo"
	"github.com/tanglx02/apphub/internal/config"
	"github.com/tanglx02/apphub/internal/models"
	"github.com/tanglx02/apphub/internal/system"
	"github.com/tanglx02/apphub/internal/systemd"
)

// backupOptionsType 为恢复选项的本地别名。
type backupOptionsType = backup.Options

// ---------- 分类 ----------

func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Cats.List()
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "读取分类失败", err))
		return
	}
	OK(w, r, list)
}

func (s *Server) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	var c models.Category
	if err := decodeJSON(r, &c); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "分类名称不能为空")
		return
	}
	if c.Slug == "" {
		c.Slug = apps.Slugify(c.Name)
	}
	if err := s.deps.Cats.Create(&c); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "创建分类失败", err))
		return
	}
	s.audit(models.AuditCategoryChange, c.Name, "success", "新增分类", actorFrom(r))
	Created(w, r, c)
}

func (s *Server) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "分类 ID 非法")
		return
	}
	var c models.Category
	if err := decodeJSON(r, &c); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	c.ID = id
	if strings.TrimSpace(c.Name) == "" {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "分类名称不能为空")
		return
	}
	if err := s.deps.Cats.Update(&c); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "更新分类失败", err))
		return
	}
	s.audit(models.AuditCategoryChange, c.Name, "success", "修改分类", actorFrom(r))
	OK(w, r, c)
}

func (s *Server) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "分类 ID 非法")
		return
	}
	if err := s.deps.Cats.Delete(id); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "删除分类失败", err))
		return
	}
	s.audit(models.AuditCategoryChange, strconv.FormatInt(id, 10), "success", "删除分类（应用保留，仅置空分类）", actorFrom(r))
	OK(w, r, map[string]any{"ok": true})
}

// ---------- 系统 ----------

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	cfg := s.deps.Config.Get()
	totalMB, usedMB := system.Memory()
	diskTotal, diskUsed := system.Disk(s.deps.Config.Root())
	info := models.SystemInfo{
		Version:     buildinfo.Version,
		Commit:      buildinfo.GitCommit,
		BuildTime:   buildinfo.BuildTime,
		Hostname:    system.Hostname(),
		OS:          system.OSName(),
		Arch:        system.Arch(),
		Kernel:      system.Kernel(),
		Uptime:      system.UptimeSeconds(),
		CPUPercent:  system.CPUPercent(),
		CPUCores:    system.CPUCores(),
		CPUModel:    system.CPUModel(),
		MemTotalMB:  totalMB,
		MemUsedMB:   usedMB,
		DiskTotalGB: round2(diskTotal),
		DiskUsedGB:  round2(diskUsed),
		LoadAvg:     system.LoadAvg(),
		IP:          system.LocalIP(),
		Timezone:    cfg.Server.Timezone,
		DBVersion:   s.dbVersion(),
		APIVersion:  buildinfo.APIVersion,
	}
	OK(w, r, info)
}

func (s *Server) handleSystemResources(w http.ResponseWriter, r *http.Request) {
	totalMB, usedMB := system.Memory()
	diskTotal, diskUsed := system.Disk(s.deps.Config.Root())
	OK(w, r, map[string]any{
		"cpu_percent":   round2(system.CPUPercent()),
		"mem_total_mb":  totalMB,
		"mem_used_mb":   usedMB,
		"disk_total_gb": round2(diskTotal),
		"disk_used_gb":  round2(diskUsed),
		"load_avg":      system.LoadAvg(),
		"uptime":        system.UptimeSeconds(),
		"ip":            system.LocalIP(),
		"updated_at":    time.Now(),
	})
}

// handleEventStream SSE 推送应用状态快照，避免前端高频轮询。
func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "当前连接不支持流式响应")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	interval := s.deps.Status.Options().Interval
	if interval < 2*time.Second {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			snapshot := s.deps.Status.Snapshot()
			payload := make(map[string]any, len(snapshot))
			for id, rt := range snapshot {
				payload[strconv.FormatInt(id, 10)] = rt
			}
			data, err := jsonMarshal(payload)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: status\ndata: %s\n\n", string(data))
			flusher.Flush()
		}
	}
}

// handleServiceStatus 查看 AppHub 自身 systemd 服务状态。
func (s *Server) handleServiceStatus(w http.ResponseWriter, r *http.Request) {
	if !systemd.Available() {
		OK(w, r, map[string]any{"available": false, "message": "当前系统不支持 systemd"})
		return
	}
	st, err := s.deps.Sysd.Show(context.Background(), "apphub.service")
	if err != nil {
		OK(w, r, map[string]any{"available": true, "installed": false, "active": "unknown"})
		return
	}
	OK(w, r, map[string]any{
		"available": true,
		"installed": true,
		"active":    st.ActiveState,
		"sub":       st.SubState,
		"enabled":   st.Enabled,
		"pid":       st.MainPID,
	})
}

type serviceActionRequest struct {
	Action string `json:"action"` // start | stop | restart | enable | disable
}

// handleServiceAction 管理 AppHub 自身服务（危险操作，需管理员确认）。
func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	if !systemd.Available() {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "当前系统不支持 systemd")
		return
	}
	var req serviceActionRequest
	if err := decodeJSON(r, &req); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	var err error
	switch req.Action {
	case "start":
		err = s.deps.Sysd.Start(ctx, "apphub.service")
	case "stop":
		err = s.deps.Sysd.Stop(ctx, "apphub.service")
	case "restart":
		err = s.deps.Sysd.Restart(ctx, "apphub.service")
	case "enable":
		err = s.deps.Sysd.Enable(ctx, "apphub.service")
	case "disable":
		err = s.deps.Sysd.Disable(ctx, "apphub.service")
	default:
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "操作类型非法")
		return
	}
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "操作失败："+err.Error())
		return
	}
	s.audit(models.AuditSettingsUpdate, "apphub.service", "success", "服务操作: "+req.Action, actorFrom(r))
	OK(w, r, map[string]any{"ok": true, "action": req.Action})
}

// ---------- 设置 ----------

// handleGetSettings 返回配置文件 + 运行时设置。
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.deps.Config.Get()
	certExists := false
	if cfg.TLS.CertFile != "" {
		if _, err := os.Stat(s.deps.Config.Resolve(cfg.TLS.CertFile)); err == nil {
			certExists = true
		}
	}
	OK(w, r, map[string]any{
		"config": cfg,
		"runtime": map[string]any{
			"site_name":         s.siteName(),
			"logo":              s.deps.Settings.Get("logo", cfg.Server.Logo),
			"refresh_interval":  s.deps.Settings.GetInt("refresh_interval", cfg.Status.IntervalSeconds),
			"allow_batch_start": s.deps.Settings.GetBool("allow_batch_start", false),
			"default_category":  s.deps.Settings.Get("default_category", ""),
		},
		"paths": map[string]any{
			"root":        s.deps.Config.Root(),
			"data":        s.deps.Config.DataDir(),
			"logs":        s.deps.Config.LogsDir(),
			"backups":     s.deps.Config.BackupsDir(),
			"uploads":     s.deps.Config.UploadsDir(),
			"database":    s.deps.Config.DatabasePath(),
			"cert_file":   s.deps.Config.Resolve(cfg.TLS.CertFile),
			"cert_exists": certExists,
		},
		"version":    buildinfo.Version,
		"db_version": s.dbVersion(),
	})
}

// handleUpdateSettings 更新设置（监听地址等需重启生效的项会提示）。
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Config  *config.Config    `json:"config"`
		Runtime map[string]string `json:"runtime"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	needRestart := false
	if payload.Config != nil {
		newCfg := payload.Config
		old := s.deps.Config.Get()
		if newCfg.Server.Host != old.Server.Host || newCfg.Server.Port != old.Server.Port ||
			newCfg.TLS.Enabled != old.TLS.Enabled {
			needRestart = true
		}
		if newCfg.Server.Port <= 0 || newCfg.Server.Port > 65535 {
			Fail(w, r, http.StatusBadRequest, CodeBadRequest, "监听端口非法")
			return
		}
		if newCfg.Status.IntervalSeconds < 3 {
			newCfg.Status.IntervalSeconds = 3
		}
		if err := s.deps.Config.Update(func(c *config.Config) {
			c.Server = newCfg.Server
			c.Logging = newCfg.Logging
			c.Security = newCfg.Security
			c.Status = newCfg.Status
			c.TLS = newCfg.TLS
			c.Backup = newCfg.Backup
			c.Executor = newCfg.Executor
			c.Database = newCfg.Database
		}); err != nil {
			FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "保存设置失败", err))
			return
		}
		// 热更新
		s.applyRuntimeConfig()
	}
	for k, v := range payload.Runtime {
		_ = s.deps.Settings.Set(k, v)
	}
	s.audit(models.AuditSettingsUpdate, "系统设置", "success", "", actorFrom(r))
	OK(w, r, map[string]any{"ok": true, "need_restart": needRestart})
}

// applyRuntimeConfig 将配置变更应用到运行中的组件。
func (s *Server) applyRuntimeConfig() {
	cfg := s.deps.Config.Get()
	s.deps.Status.UpdateOptions(healthOptionsFrom(cfg))
	s.deps.Sessions.SetTTL(s.deps.Config.SessionTTL())
	if s.deps.Exec != nil {
		s.deps.Exec.UpdateConfig(executorConfigFrom(cfg))
	}
	s.deps.Logger.SetLevel(levelFrom(cfg.Logging.Level))
}

func levelFrom(s string) loggingLevel { return parseLevel(s) }

// ---------- 审计 ----------

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	action := r.URL.Query().Get("action")
	keyword := r.URL.Query().Get("keyword")
	list, total, err := s.deps.Audits.List(limit, offset, action, keyword)
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "读取审计日志失败", err))
		return
	}
	OK(w, r, map[string]any{"items": list, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) handleClearAudit(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Audits.Clear(); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "清空审计日志失败", err))
		return
	}
	s.audit(models.AuditSettingsUpdate, "审计日志", "success", "清空全部审计记录", actorFrom(r))
	OK(w, r, map[string]any{"ok": true})
}

// ---------- 备份 ----------

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Backups.List()
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "读取备份列表失败", err))
		return
	}
	OK(w, r, map[string]any{"items": list, "dir": s.deps.Backups.Dir()})
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Note string `json:"note"`
	}
	_ = decodeJSON(r, &payload)
	rec, err := s.deps.Backups.Create(payload.Note, "")
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "创建备份失败", err))
		return
	}
	s.audit(models.AuditBackup, rec.Name, "success", fmt.Sprintf("%d 字节", rec.SizeBytes), actorFrom(r))
	Created(w, r, rec)
}

type restoreRequest struct {
	Name           string `json:"name"`
	RestoreConfig  bool   `json:"restore_config"`
	RestoreDB      bool   `json:"restore_db"`
	RestoreUploads bool   `json:"restore_uploads"`
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	var req restoreRequest
	if err := decodeJSON(r, &req); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	if req.Name == "" {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请选择要恢复的备份")
		return
	}
	pre, err := s.deps.Backups.Restore(req.Name, backupOptions(req))
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "恢复失败："+err.Error())
		return
	}
	msg := "已恢复，请重启 AppHub 服务后生效"
	if pre != nil {
		msg = fmt.Sprintf("已在恢复前自动备份为 %s；%s", pre.Name, msg)
	}
	s.audit(models.AuditRestore, req.Name, "success", msg, actorFrom(r))
	OK(w, r, map[string]any{"ok": true, "need_restart": true, "message": msg})
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "备份名称为空")
		return
	}
	if err := s.deps.Backups.Delete(name); err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "备份不存在或已删除")
		return
	}
	OK(w, r, map[string]any{"ok": true})
}

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	path := filepath.Join(s.deps.Backups.Dir(), name)
	f, err := os.Open(path)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "备份文件不存在")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	_, _ = io.Copy(w, f)
}

// ---------- 上传 ----------

var svgDangerPattern = regexp.MustCompile(`(?is)<\s*(script|iframe|object|embed|foreignObject)|on[a-z]+\s*=|javascript:`)

// handleUpload 上传应用图标（PNG / SVG / 其他图片），SVG 会做安全过滤。
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil { // 8 MiB
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "上传内容过大或格式有误")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "未选择文件")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".png": true, ".svg": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".ico": true}
	if !allowed[ext] {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "不支持的文件类型，仅允许 PNG / JPG / SVG / WEBP / GIF / ICO")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, 4<<20))
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "读取文件失败")
		return
	}
	if ext == ".svg" {
		if svgDangerPattern.Match(data) {
			Fail(w, r, http.StatusBadRequest, CodeBadRequest, "SVG 文件包含不安全的脚本内容，已拒绝上传")
			return
		}
	}

	dir := s.deps.Config.UploadsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "创建上传目录失败", err))
		return
	}
	name := fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), sanitizeName(header.Filename), ext)
	dest := filepath.Join(dir, name)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "保存文件失败", err))
		return
	}
	OK(w, r, map[string]any{"url": "/uploads/" + name, "name": name})
}

// handleServeUpload 提供上传的图标文件（需登录，避免匿名抓取）。
func (s *Server) handleServeUpload(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	path := filepath.Join(s.deps.Config.UploadsDir(), name)
	f, err := os.Open(path)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "文件不存在")
		return
	}
	defer f.Close()
	ctype := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	// SVG 以安全方式下发，禁止内联脚本执行
	if strings.EqualFold(filepath.Ext(name), ".svg") {
		ctype = "image/svg+xml"
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:")
		w.Header().Set("X-Content-Type-Options", "nosniff")
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, f)
}

func sanitizeName(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	var b strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 24 {
		out = out[:24]
	}
	if out == "" {
		out = "icon"
	}
	return out
}

func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }

// backupOptions 转换恢复选项。
func backupOptions(req restoreRequest) backupOptionsType {
	return backupOptionsType{
		RestoreConfig:  req.RestoreConfig,
		RestoreDB:      req.RestoreDB,
		RestoreUploads: req.RestoreUploads,
		PreBackup:      true,
	}
}
