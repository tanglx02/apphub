package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tanglx02/apphub/internal/apps"
	"github.com/tanglx02/apphub/internal/buildinfo"
	"github.com/tanglx02/apphub/internal/executor"
	"github.com/tanglx02/apphub/internal/models"
	"github.com/tanglx02/apphub/internal/systemd"

	"gopkg.in/yaml.v3"
)

// handleListApps 返回应用列表 + 实时状态。
func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Repo.List()
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "读取应用列表失败", err))
		return
	}
	cats, _ := s.deps.Cats.List()
	catMap := make(map[int64]models.Category, len(cats))
	for _, c := range cats {
		catMap[c.ID] = c
	}

	out := make([]models.AppWithStatus, 0, len(list))
	for i := range list {
		a := list[i]
		item := models.AppWithStatus{App: a, Runtime: s.runtimeFor(&a)}
		if c, ok := catMap[a.CategoryID]; ok {
			item.CategoryName = c.Name
			item.CategoryIcon = c.Icon
			item.CategoryColor = c.Color
		}
		out = append(out, item)
	}

	total := len(out)
	online, offline, errCnt := 0, 0, 0
	for _, item := range out {
		switch item.Runtime.Status {
		case models.StatusOnline:
			online++
		case models.StatusOffline:
			offline++
		case models.StatusError:
			errCnt++
		}
	}
	rate := 0
	if total > 0 {
		rate = online * 100 / total
	}

	OK(w, r, map[string]any{
		"apps": out,
		"summary": map[string]any{
			"total":      total,
			"online":     online,
			"offline":    offline,
			"error":      errCnt,
			"rate":       rate,
			"updated_at": time.Now(),
		},
	})
}

// runtimeFor 组装运行时状态（含 PID、启动时间、资源占用）。
func (s *Server) runtimeFor(app *models.App) models.AppRuntime {
	rt := s.deps.Status.Get(app.ID)
	if p := executor.Registry().Get(app.ID); p != nil && p.Alive() {
		rt.PID = p.PID
		started := p.StartedAt
		rt.StartedAt = &started
		rt.UptimeSec = int64(time.Since(p.StartedAt).Seconds())
		rt.CPU, rt.MemoryMB = processStats(p.PID)
	}
	if app.AppType == models.AppTypeSystemd && rt.PID == 0 && systemd.Available() {
		if st, err := s.deps.Sysd.Show(context.Background(), app.SystemdUnit); err == nil && st != nil && st.MainPID > 0 {
			rt.PID = st.MainPID
			rt.CPU, rt.MemoryMB = processStats(st.MainPID)
		}
	}
	return rt
}

// handleGetApp 应用详情。
func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	app, err := s.deps.Repo.Get(id)
	if err != nil {
		if errors.Is(err, apps.ErrNotFound) {
			Fail(w, r, http.StatusNotFound, CodeNotFound, "应用不存在")
			return
		}
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "读取应用失败", err))
		return
	}
	item := models.AppWithStatus{App: *app, Runtime: s.runtimeFor(app)}
	if c, err := s.deps.Cats.Get(app.CategoryID); err == nil && c != nil {
		item.CategoryName = c.Name
		item.CategoryIcon = c.Icon
		item.CategoryColor = c.Color
	}
	OK(w, r, item)
}

type appPayload struct {
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	Description     string `json:"description"`
	Icon            string `json:"icon"`
	CategoryID      int64  `json:"category_id"`
	Tags            string `json:"tags"`
	InternalURL     string `json:"internal_url"`
	ExternalURL     string `json:"external_url"`
	AppType         string `json:"app_type"`
	SystemdUnit     string `json:"systemd_unit"`
	StartCommand    string `json:"start_command"`
	StopCommand     string `json:"stop_command"`
	RestartCommand  string `json:"restart_command"`
	ShellMode       bool   `json:"shell_mode"`
	StartArgs       string `json:"start_args"`
	StopArgs        string `json:"stop_args"`
	RestartArgs     string `json:"restart_args"`
	StatusType      string `json:"status_type"`
	StatusTarget    string `json:"status_target"`
	ExpectedCodes   string `json:"expected_codes"`
	WorkDir         string `json:"work_dir"`
	Environment     string `json:"environment"`
	Enabled         bool   `json:"enabled"`
	AutoStart       bool   `json:"auto_start"`
	Favorite        bool   `json:"favorite"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	CheckTimeoutSec int    `json:"check_timeout_sec"`
	// PublicVisible 用指针区分"未传"与"显式关闭"：未传时默认在前台显示
	PublicVisible *bool `json:"public_visible"`
	// Type 应用范围：LOCAL（默认）/ EXTERNAL
	Type      string            `json:"type"`
	Endpoints []models.Endpoint `json:"endpoints"`
}

func (p appPayload) toApp() *models.App {
	return &models.App{
		Name:            strings.TrimSpace(p.Name),
		Slug:            strings.TrimSpace(p.Slug),
		Description:     strings.TrimSpace(p.Description),
		Icon:            strings.TrimSpace(p.Icon),
		CategoryID:      p.CategoryID,
		Tags:            strings.TrimSpace(p.Tags),
		InternalURL:     strings.TrimSpace(p.InternalURL),
		ExternalURL:     strings.TrimSpace(p.ExternalURL),
		AppType:         models.AppType(p.AppType),
		SystemdUnit:     strings.TrimSpace(p.SystemdUnit),
		StartCommand:    strings.TrimSpace(p.StartCommand),
		StopCommand:     strings.TrimSpace(p.StopCommand),
		RestartCommand:  strings.TrimSpace(p.RestartCommand),
		ShellMode:       p.ShellMode,
		StartArgs:       strings.TrimSpace(p.StartArgs),
		StopArgs:        strings.TrimSpace(p.StopArgs),
		RestartArgs:     strings.TrimSpace(p.RestartArgs),
		StatusType:      models.StatusCheckType(p.StatusType),
		StatusTarget:    strings.TrimSpace(p.StatusTarget),
		ExpectedCodes:   strings.TrimSpace(p.ExpectedCodes),
		WorkDir:         strings.TrimSpace(p.WorkDir),
		Environment:     p.Environment,
		Enabled:         p.Enabled,
		AutoStart:       p.AutoStart,
		Favorite:        p.Favorite,
		TimeoutSeconds:  p.TimeoutSeconds,
		CheckTimeoutSec: p.CheckTimeoutSec,
		PublicVisible:   p.PublicVisible == nil || *p.PublicVisible,
		Scope:           normalizeScope(p.Type),
	}
}

// normalizeScope 解析应用范围类型，空值默认 LOCAL。
func normalizeScope(t string) models.AppScope {
	if models.AppScope(strings.ToUpper(strings.TrimSpace(t))) == models.ScopeExternal {
		return models.ScopeExternal
	}
	return models.ScopeLocal
}

// saveEndpoints 用 payload 中的入口列表整体替换应用的附加入口。
func (s *Server) saveEndpoints(appID int64, eps []models.Endpoint) {
	if s.deps.Endpoints == nil || eps == nil {
		return
	}
	if err := s.deps.Endpoints.DeleteByApp(appID); err != nil {
		logError(nil, "清理应用入口失败 app=%d: %v", appID, err)
	}
	for i := range eps {
		e := eps[i]
		e.ID = 0
		e.AppID = appID
		e.URL = strings.TrimSpace(e.URL)
		if e.URL == "" {
			continue
		}
		if !strings.HasPrefix(e.URL, "http://") && !strings.HasPrefix(e.URL, "https://") {
			continue
		}
		if strings.TrimSpace(e.Name) == "" {
			e.Name = "入口"
		}
		if e.Type == "" {
			e.Type = "WEB"
		}
		e.OpenNewTab = true
		if err := s.deps.Endpoints.Create(&e); err != nil {
			logError(nil, "保存应用入口失败 app=%d: %v", appID, err)
		}
	}
}

// validateApp 校验应用配置（保存时不执行任何命令）。
func validateApp(a *models.App) *ErrHTTP {
	if a.Scope == "" {
		a.Scope = models.ScopeLocal
	}
	if a.Name == "" {
		return NewErr(http.StatusBadRequest, CodeBadRequest, "应用名称不能为空")
	}
	if len(a.Name) > 64 {
		return NewErr(http.StatusBadRequest, CodeBadRequest, "应用名称过长（最多 64 个字符）")
	}
	if a.Scope == models.ScopeExternal {
		// 非本地应用：只需要访问地址 + 在线检测，所有管理配置一律忽略
		a.SystemdUnit = ""
		a.StartCommand = ""
		a.StopCommand = ""
		a.RestartCommand = ""
		a.StartArgs = ""
		a.StopArgs = ""
		a.RestartArgs = ""
		a.WorkDir = ""
		a.Environment = ""
		a.ShellMode = false
		a.AppType = ""
		a.AutoStart = false
		if a.ExternalURL == "" && a.InternalURL == "" {
			return NewErr(http.StatusBadRequest, CodeBadRequest, "非本地应用必须配置访问地址")
		}
		switch a.StatusType {
		case models.CheckHTTP, models.CheckTCP, models.CheckNone:
		case "":
			a.StatusType = models.CheckHTTP
		default:
			return NewErr(http.StatusBadRequest, CodeBadRequest, "非本地应用仅支持 http / tcp / none 检测方式")
		}
		if a.StatusTarget == "" && a.StatusType != models.CheckNone {
			a.StatusTarget = a.ExternalURL
		}
		if a.Slug == "" {
			a.Slug = apps.Slugify(a.Name)
		}
		return nil
	}
	if a.AppType != models.AppTypeSystemd && a.AppType != models.AppTypeCommand {
		return NewErr(http.StatusBadRequest, CodeBadRequest, "应用类型必须为 systemd 或 command")
	}
	if a.Slug == "" {
		a.Slug = apps.Slugify(a.Name)
	}
	if a.InternalURL != "" && !strings.HasPrefix(a.InternalURL, "http://") && !strings.HasPrefix(a.InternalURL, "https://") {
		return NewErr(http.StatusBadRequest, CodeBadRequest, "内网地址必须以 http:// 或 https:// 开头")
	}
	if a.ExternalURL != "" && !strings.HasPrefix(a.ExternalURL, "http://") && !strings.HasPrefix(a.ExternalURL, "https://") {
		return NewErr(http.StatusBadRequest, CodeBadRequest, "公网地址必须以 http:// 或 https:// 开头")
	}
	if a.AppType == models.AppTypeSystemd {
		if _, err := systemd.ValidateUnit(a.SystemdUnit); err != nil {
			return NewErr(http.StatusBadRequest, CodeBadRequest, err.Error())
		}
	} else {
		if strings.TrimSpace(a.StartCommand) == "" {
			return NewErr(http.StatusBadRequest, CodeBadRequest, "Command 应用必须配置启动命令")
		}
	}
	switch a.StatusType {
	case models.CheckHTTP, models.CheckTCP, models.CheckProcess, models.CheckSystemd, models.CheckNone:
	default:
		return NewErr(http.StatusBadRequest, CodeBadRequest, "状态检测方式非法")
	}
	if a.TimeoutSeconds < 0 {
		a.TimeoutSeconds = 0
	}
	if a.TimeoutSeconds > 600 {
		return NewErr(http.StatusBadRequest, CodeBadRequest, "超时时间不能超过 600 秒")
	}
	if a.CheckTimeoutSec <= 0 {
		a.CheckTimeoutSec = 5
	}
	if a.WorkDir != "" {
		if !filepath.IsAbs(a.WorkDir) {
			return NewErr(http.StatusBadRequest, CodeBadRequest, "工作目录必须使用绝对路径")
		}
	}
	return nil
}

// handleCreateApp 添加应用。
func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var p appPayload
	if err := decodeJSON(r, &p); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	a := p.toApp()
	if err := validateApp(a); err != nil {
		Fail(w, r, err.Status, err.Code, err.Message)
		return
	}
	if existing, _ := s.deps.Repo.GetBySlug(a.Slug); existing != nil {
		a.Slug = fmt.Sprintf("%s-%d", a.Slug, time.Now().Unix()%10000)
	}
	if err := s.deps.Repo.Create(a); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "添加应用失败", err))
		return
	}
	if a.Scope == models.ScopeExternal {
		a.AppType = ""
	}
	s.saveEndpoints(a.ID, p.Endpoints)
	s.deps.Status.Refresh(a.ID)
	s.audit(models.AuditAppCreate, a.Name, "success", fmt.Sprintf("类型=%s", a.Scope.Text()), actorFrom(r))
	Created(w, r, a)
}

// handleUpdateApp 编辑应用。
func (s *Server) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	existing, err := s.deps.Repo.Get(id)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "应用不存在")
		return
	}
	var p appPayload
	if err := decodeJSON(r, &p); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	a := p.toApp()
	a.ID = id
	a.SortOrder = existing.SortOrder
	a.CreatedAt = existing.CreatedAt
	if err := validateApp(a); err != nil {
		Fail(w, r, err.Status, err.Code, err.Message)
		return
	}
	if a.Scope == models.ScopeExternal {
		a.AppType = ""
	}
	if err := s.deps.Repo.Update(a); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "更新应用失败", err))
		return
	}
	s.saveEndpoints(id, p.Endpoints)
	s.deps.Status.Refresh(id)
	s.audit(models.AuditAppUpdate, a.Name, "success", "", actorFrom(r))
	OK(w, r, a)
}

// handleDeleteApp 删除应用定义（绝不删除用户实际项目目录）。
func (s *Server) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	app, err := s.deps.Repo.Get(id)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "应用不存在")
		return
	}
	if err := s.deps.Repo.Delete(id); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "删除应用失败", err))
		return
	}
	if s.deps.Endpoints != nil {
		_ = s.deps.Endpoints.DeleteByApp(id)
	}
	s.audit(models.AuditAppDelete, app.Name, "success", "仅删除 AppHub 中的应用定义", actorFrom(r))
	OK(w, r, map[string]any{"ok": true})
}

func (s *Server) controlApp(w http.ResponseWriter, r *http.Request, action string) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	app, err := s.deps.Repo.Get(id)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "应用不存在")
		return
	}
	actor := actorFrom(r)
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	var opErr error
	switch action {
	case "start":
		opErr = s.deps.Apps.Start(ctx, id, actor)
	case "stop":
		opErr = s.deps.Apps.Stop(ctx, id, actor)
	case "restart":
		opErr = s.deps.Apps.Restart(ctx, id, actor)
	}
	if opErr != nil {
		// 用户可见信息：命令层已给出友好描述，此处仅做统一收敛
		msg := opErr.Error()
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, msg)
		return
	}
	OK(w, r, map[string]any{
		"ok":      true,
		"app_id":  id,
		"name":    app.Name,
		"status":  s.deps.Status.Get(id).Status,
		"message": fmt.Sprintf("%s 已%s", app.Name, actionText(action)),
	})
}

func actionText(action string) string {
	switch action {
	case "start":
		return "启动"
	case "stop":
		return "停止"
	case "restart":
		return "重启"
	}
	return "操作"
}

func (s *Server) handleStartApp(w http.ResponseWriter, r *http.Request) { s.controlApp(w, r, "start") }
func (s *Server) handleStopApp(w http.ResponseWriter, r *http.Request)  { s.controlApp(w, r, "stop") }
func (s *Server) handleRestartApp(w http.ResponseWriter, r *http.Request) {
	s.controlApp(w, r, "restart")
}

// handleAppStatus 单个应用实时状态（读取缓存，不触发额外系统调用）。
func (s *Server) handleAppStatus(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	rt := s.deps.Status.Get(id)
	OK(w, r, rt)
}

// handleFavorite 切换收藏。
func (s *Server) handleFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	app, err := s.deps.Repo.Get(id)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "应用不存在")
		return
	}
	fav := !app.Favorite
	if err := s.deps.Repo.SetFavorite(id, fav); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "操作失败", err))
		return
	}
	OK(w, r, map[string]any{"ok": true, "favorite": fav})
}

type batchRequest struct {
	IDs    []int64 `json:"ids"`
	Action string  `json:"action"`
}

// handleBatch 批量启停（服务端限制并发）。
func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := decodeJSON(r, &req); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	if len(req.IDs) == 0 {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请至少选择一个应用")
		return
	}
	if len(req.IDs) > 50 {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "单次批量操作最多支持 50 个应用")
		return
	}
	switch req.Action {
	case "start", "stop", "restart":
	default:
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "操作类型非法")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	res := s.deps.Apps.Batch(ctx, req.IDs, req.Action, actorFrom(r))
	OK(w, r, res)
}

// handleStartAll 一键启动全部（需设置开启）。
func (s *Server) handleStartAll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	res, err := s.deps.Apps.StartAll(ctx, actorFrom(r))
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	OK(w, r, res)
}

type reorderRequest struct {
	IDs []int64 `json:"ids"`
}

// handleReorderApps 拖拽排序保存。
func (s *Server) handleReorderApps(w http.ResponseWriter, r *http.Request) {
	var req reorderRequest
	if err := decodeJSON(r, &req); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	if err := s.deps.Repo.Reorder(req.IDs); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "保存排序失败", err))
		return
	}
	OK(w, r, map[string]any{"ok": true})
}

type testRequest struct {
	Kind string `json:"kind"` // start | stop | status
}

// handleTestCommand 测试命令（管理员主动触发，保存时不会自动执行）。
func (s *Server) handleTestCommand(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	var req testRequest
	if err := decodeJSON(r, &req); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.deps.Apps.TestCommand(ctx, id, req.Kind, actorFrom(r))
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	OK(w, r, res)
}

// handleAppLogs 应用日志：支持最近 N 行与 SSE 实时日志。
func (s *Server) handleAppLogs(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	app, err := s.deps.Repo.Get(id)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "应用不存在")
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines <= 0 {
		lines = 100
	}
	follow := r.URL.Query().Get("follow") == "1" || r.URL.Query().Get("follow") == "true"

	if !follow {
		content, err := s.readLogs(app, lines)
		if err != nil {
			Fail(w, r, http.StatusBadRequest, CodeBadRequest, err.Error())
			return
		}
		OK(w, r, map[string]any{"lines": splitLines(content), "source": logSource(app)})
		return
	}
	s.streamLogs(w, r, app)
}

func logSource(app *models.App) string {
	if app.AppType == models.AppTypeSystemd {
		return "journalctl"
	}
	return "file"
}

func (s *Server) readLogs(app *models.App, lines int) (string, error) {
	if app.AppType == models.AppTypeSystemd {
		if !systemd.Available() {
			return "", errors.New("当前系统不支持 journalctl")
		}
		return s.deps.Sysd.Journal(context.Background(), app.SystemdUnit, lines)
	}
	path := s.processLogPath(app.ID)
	data, err := os.ReadFile(path)
	if err != nil {
		return "(暂无日志。启动应用后，其 stdout/stderr 会写入 " + path + ")", nil
	}
	return tailLines(string(data), lines), nil
}

func (s *Server) processLogPath(id int64) string {
	return filepath.Join(s.deps.Config.RuntimeDir(), "logs", fmt.Sprintf("app-%d.log", id))
}

// streamLogs 以 SSE 方式推送日志增量。
func (s *Server) streamLogs(w http.ResponseWriter, r *http.Request, app *models.App) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "当前连接不支持流式响应")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	last := ""
	send := func(event, data string) {
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, escapeSSE(data))
		flusher.Flush()
	}

	send("log", "已连接日志流")

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			content, err := s.readLogs(app, 100)
			if err != nil {
				send("error", err.Error())
				continue
			}
			if content != last {
				last = content
				send("log", content)
			}
		}
	}
}

func escapeSSE(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

func tailLines(s string, n int) string {
	lines := splitLines(s)
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return []string{}
	}
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	out := make([]string, 0)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}

// handleExportApps 导出应用配置为 JSON。
func (s *Server) handleExportApps(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Repo.List()
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "导出失败", err))
		return
	}
	cats, _ := s.deps.Cats.List()
	payload := map[string]any{
		"version":     buildinfo.Version,
		"exported_at": time.Now(),
		"categories":  cats,
		"apps":        list,
	}
	s.audit(models.AuditExport, "应用配置", "success", fmt.Sprintf("%d 个应用", len(list)), actorFrom(r))
	OK(w, r, payload)
}

type importRequest struct {
	Mode       string            `json:"mode"`    // merge | replace
	Format     string            `json:"format"`  // json | yaml
	Content    string            `json:"content"` // 原始文本（非空时优先解析）
	Categories []models.Category `json:"categories"`
	Apps       []models.App      `json:"apps"`
}

type importPayload struct {
	Apps       []models.App      `json:"apps" yaml:"apps"`
	Categories []models.Category `json:"categories" yaml:"categories"`
}

// handleImportApps 导入应用配置（支持 JSON 与 YAML）。
func (s *Server) handleImportApps(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := decodeJSON(r, &req); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}

	if strings.TrimSpace(req.Content) != "" {
		var payload importPayload
		switch strings.ToLower(strings.TrimSpace(req.Format)) {
		case "yaml", "yml":
			if err := yaml.Unmarshal([]byte(req.Content), &payload); err != nil {
				Fail(w, r, http.StatusBadRequest, CodeBadRequest, "YAML 解析失败："+err.Error())
				return
			}
		default:
			if err := json.Unmarshal([]byte(req.Content), &payload); err != nil {
				Fail(w, r, http.StatusBadRequest, CodeBadRequest, "JSON 解析失败："+err.Error())
				return
			}
		}
		req.Apps = append(req.Apps, payload.Apps...)
		req.Categories = append(req.Categories, payload.Categories...)
	}

	if len(req.Apps) == 0 {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "导入内容为空")
		return
	}
	imported := 0
	for i := range req.Apps {
		a := req.Apps[i]
		a.ID = 0
		if a.Name == "" {
			continue
		}
		if a.Slug == "" {
			a.Slug = apps.Slugify(a.Name)
		}
		if err := validateApp(&a); err != nil {
			continue
		}
		if existing, _ := s.deps.Repo.GetBySlug(a.Slug); existing != nil {
			a.Slug = fmt.Sprintf("%s-%d", a.Slug, time.Now().UnixNano()%100000)
		}
		if err := s.deps.Repo.Create(&a); err != nil {
			continue
		}
		imported++
	}
	s.audit(models.AuditImport, "应用配置", "success", fmt.Sprintf("导入 %d 个应用", imported), actorFrom(r))
	OK(w, r, map[string]any{"imported": imported})
}

// pathID 解析路径参数中的 ID。
func pathID(r *http.Request) (int64, error) {
	raw := r.PathValue("id")
	if raw == "" {
		return 0, errors.New("missing id")
	}
	return strconv.ParseInt(raw, 10, 64)
}
