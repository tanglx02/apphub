package apps

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tanglx02/apphub/internal/executor"
	"github.com/tanglx02/apphub/internal/health"
	"github.com/tanglx02/apphub/internal/logging"
	"github.com/tanglx02/apphub/internal/models"
	"github.com/tanglx02/apphub/internal/systemd"
)

// Actor 记录操作者上下文，用于审计日志。
type Actor struct {
	Username string
	IP       string
}

// AuditWriter 审计写入接口（由 api 层实现，含操作者信息）。
type AuditWriter func(action models.AuditAction, target, result, detail string, actor Actor)

// Service 应用控制服务：状态机 + systemd/command 控制 + 批量并发限制。
type Service struct {
	repo     *Repository
	cats     *CategoryRepository
	exec     *executor.Executor
	sysd     *systemd.Client
	status   *health.Manager
	audit    AuditWriter
	settings SettingsLookup

	mu       sync.Mutex
	opLocks  map[int64]*sync.Mutex
	actors   map[int64]Actor
	batchMax int
	allowAll bool
}

// SettingsLookup 读取运行期设置的回调。
type SettingsLookup func(key string, def bool) bool

// NewService 创建应用控制服务。
func NewService(repo *Repository, cats *CategoryRepository, ex *executor.Executor,
	sysd *systemd.Client, status *health.Manager, audit AuditWriter, settings SettingsLookup) *Service {
	s := &Service{
		repo:     repo,
		cats:     cats,
		exec:     ex,
		sysd:     sysd,
		status:   status,
		audit:    audit,
		settings: settings,
		opLocks:  make(map[int64]*sync.Mutex),
		actors:   make(map[int64]Actor),
		batchMax: 3,
		allowAll: false,
	}
	// 命令执行器的审计回调：写入审计日志，操作者取当前操作上下文
	ex.SetAudit(func(appID int64, action, result, detail string) {
		s.mu.Lock()
		actor, ok := s.actors[appID]
		s.mu.Unlock()
		if !ok {
			actor = Actor{Username: "system"}
		}
		name := ""
		if app, err := repo.Get(appID); err == nil && app != nil {
			name = app.Name
		}
		if s.audit != nil {
			s.audit(models.AuditAppStart, name, result, fmt.Sprintf("%s %s", action, detail), actor)
		}
	})
	return s
}

// SetBatchLimit 设置批量操作并发上限。
func (s *Service) SetBatchLimit(n int) {
	if n > 0 {
		s.mu.Lock()
		s.batchMax = n
		s.mu.Unlock()
	}
}

func (s *Service) lockFor(id int64) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.opLocks[id]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.opLocks[id] = m
	return m
}

func (s *Service) setActor(id int64, a Actor) {
	s.mu.Lock()
	s.actors[id] = a
	s.mu.Unlock()
}

func (s *Service) clearActor(id int64) {
	s.mu.Lock()
	delete(s.actors, id)
	s.mu.Unlock()
}

// ErrInTransition 应用处于过渡态，拒绝重复操作。
var ErrInTransition = errors.New("应用正在操作中，请稍候")

// ErrNoCommand 未配置对应命令。
var ErrNoCommand = errors.New("未配置该操作所需的命令")

// ErrExternalControl 非本地应用不支持服务控制。
var ErrExternalControl = errors.New("非本地应用不支持服务控制")

// ensureLocal 服务控制仅允许本地应用；EXTERNAL 只做导航与在线检测。
func ensureLocal(app *models.App) error {
	if app != nil && app.Scope == models.ScopeExternal {
		return ErrExternalControl
	}
	return nil
}

// Start 启动应用。
func (s *Service) Start(ctx context.Context, id int64, actor Actor) error {
	app, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if err := ensureLocal(app); err != nil {
		return err
	}
	lock := s.lockFor(id)
	if !tryLock(lock, ctx) {
		return ErrInTransition
	}
	defer lock.Unlock()

	s.setActor(id, actor)
	defer s.clearActor(id)

	rt := s.status.Get(id)
	if rt.Status == models.StatusStarting {
		s.audit(models.AuditAppStart, app.Name, "failed", "应用正在启动，请稍候", actor)
		return errors.New("应用正在启动，请稍候")
	}

	hold := startHold(app)
	s.status.MarkTransition(id, models.StatusStarting, hold)

	var opErr error
	if app.AppType == models.AppTypeSystemd {
		opErr = s.startSystemd(ctx, app)
	} else {
		opErr = s.startCommand(ctx, app)
	}

	if opErr != nil {
		s.status.MarkTransition(id, models.StatusError, 15*time.Second)
		s.audit(models.AuditAppStart, app.Name, "failed", truncate(opErr.Error(), 500), actor)
		logging.Error("启动应用失败 [%s]: %v", app.Name, opErr)
		return opErr
	}
	s.audit(models.AuditAppStart, app.Name, "success", fmt.Sprintf("类型=%s", app.AppType), actor)
	// 启动后按应用类型延迟刷新，避免刚启动就被判定为离线
	s.status.RefreshSoon(id, refreshDelay(app))
	logging.Info("已启动应用 [%s]", app.Name)
	return nil
}

// Stop 停止应用。
func (s *Service) Stop(ctx context.Context, id int64, actor Actor) error {
	app, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if err := ensureLocal(app); err != nil {
		return err
	}
	lock := s.lockFor(id)
	if !tryLock(lock, ctx) {
		return ErrInTransition
	}
	defer lock.Unlock()

	s.setActor(id, actor)
	defer s.clearActor(id)

	rt := s.status.Get(id)
	if rt.Status == models.StatusStopping {
		return errors.New("应用正在停止，请稍候")
	}

	s.status.MarkTransition(id, models.StatusStopping, 30*time.Second)

	var opErr error
	if app.AppType == models.AppTypeSystemd {
		opErr = s.stopSystemd(ctx, app)
	} else {
		opErr = s.stopCommand(ctx, app)
	}

	if opErr != nil {
		s.status.MarkTransition(id, models.StatusError, 15*time.Second)
		s.audit(models.AuditAppStop, app.Name, "failed", truncate(opErr.Error(), 500), actor)
		logging.Error("停止应用失败 [%s]: %v", app.Name, opErr)
		return opErr
	}
	s.audit(models.AuditAppStop, app.Name, "success", fmt.Sprintf("类型=%s", app.AppType), actor)
	s.status.RefreshSoon(id, 1200*time.Millisecond)
	logging.Info("已停止应用 [%s]", app.Name)
	return nil
}

// Restart 重启应用。
func (s *Service) Restart(ctx context.Context, id int64, actor Actor) error {
	app, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if err := ensureLocal(app); err != nil {
		return err
	}
	lock := s.lockFor(id)
	if !tryLock(lock, ctx) {
		return ErrInTransition
	}
	defer lock.Unlock()

	s.setActor(id, actor)
	defer s.clearActor(id)

	s.status.MarkTransition(id, models.StatusStarting, 90*time.Second)

	var opErr error
	switch {
	case app.AppType == models.AppTypeSystemd:
		opErr = s.sysd.Restart(ctx, app.SystemdUnit)
	case strings.TrimSpace(app.RestartCommand) != "" || strings.TrimSpace(app.RestartArgs) != "":
		opErr = s.runCommand(ctx, app, "restart", restartTimeout(app))
	default:
		if err := s.stopCommand(ctx, app); err != nil {
			opErr = err
			break
		}
		time.Sleep(800 * time.Millisecond)
		opErr = s.startCommand(ctx, app)
	}

	if opErr != nil {
		s.status.MarkTransition(id, models.StatusError, 15*time.Second)
		s.audit(models.AuditAppRestart, app.Name, "failed", truncate(opErr.Error(), 500), actor)
		return opErr
	}
	s.audit(models.AuditAppRestart, app.Name, "success", "", actor)
	s.status.RefreshSoon(id, refreshDelay(app))
	return nil
}

// BatchResult 批量操作结果。
type BatchResult struct {
	Total   int               `json:"total"`
	Success int               `json:"success"`
	Failed  int               `json:"failed"`
	Items   []BatchResultItem `json:"items"`
}

// BatchResultItem 单项结果。
type BatchResultItem struct {
	AppID   int64  `json:"app_id"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// Batch 批量操作，限制并发避免瞬间拉高系统负载。
func (s *Service) Batch(ctx context.Context, ids []int64, action string, actor Actor) BatchResult {
	result := BatchResult{Total: len(ids), Items: make([]BatchResultItem, 0, len(ids))}
	if len(ids) == 0 {
		return result
	}
	s.mu.Lock()
	limit := s.batchMax
	s.mu.Unlock()
	if limit <= 0 {
		limit = 3
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(appID int64) {
			defer wg.Done()
			defer func() { <-sem }()
			var err error
			switch action {
			case "start":
				err = s.Start(ctx, appID, actor)
			case "stop":
				err = s.Stop(ctx, appID, actor)
			case "restart":
				err = s.Restart(ctx, appID, actor)
			default:
				err = errors.New("未知操作")
			}
			name := ""
			if app, gerr := s.repo.Get(appID); gerr == nil {
				name = app.Name
			}
			mu.Lock()
			if err != nil {
				result.Failed++
				result.Items = append(result.Items, BatchResultItem{AppID: appID, Name: name, OK: false, Message: err.Error()})
			} else {
				result.Success++
				result.Items = append(result.Items, BatchResultItem{AppID: appID, Name: name, OK: true})
			}
			mu.Unlock()
		}(id)
	}
	wg.Wait()

	auditAction := models.AuditBatchStart
	switch action {
	case "stop":
		auditAction = models.AuditBatchStop
	case "restart":
		auditAction = models.AuditBatchRestart
	}
	result_ := "success"
	if result.Failed > 0 {
		result_ = "failed"
	}
	s.audit(auditAction, fmt.Sprintf("%d 个应用", len(ids)), result_,
		fmt.Sprintf("成功 %d 失败 %d", result.Success, result.Failed), actor)
	return result
}

// StartAll 启动全部应用（需在设置中开启"允许批量启动"）。
func (s *Service) StartAll(ctx context.Context, actor Actor) (BatchResult, error) {
	s.mu.Lock()
	allowed := s.allowAll
	s.mu.Unlock()
	permitted := allowed
	if s.settings != nil {
		permitted = permitted || s.settings("allow_batch_start", false)
	}
	if !permitted {
		return BatchResult{}, errors.New("未开启「允许批量启动」，请先在设置中启用")
	}
	list, err := s.repo.List()
	if err != nil {
		return BatchResult{}, err
	}
	ids := make([]int64, 0, len(list))
	for _, a := range list {
		if a.Enabled {
			ids = append(ids, a.ID)
		}
	}
	return s.Batch(ctx, ids, "start", actor), nil
}

// TestCommand 测试启动/停止命令（管理员主动触发，保存时不自动执行）。
func (s *Service) TestCommand(ctx context.Context, id int64, kind string, actor Actor) (*executor.Result, error) {
	app, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if err := ensureLocal(app); err != nil {
		return nil, err
	}
	s.setActor(id, actor)
	defer s.clearActor(id)
	s.audit(models.AuditTestCommand, app.Name, "success", "测试"+kind+"命令", actor)
	switch kind {
	case "start":
		spec := s.buildSpec(app, "start")
		if strings.TrimSpace(spec.Executable) == "" && !spec.Shell {
			return nil, ErrNoCommand
		}
		spec.Timeout = 15 * time.Second
		return s.exec.Run(ctx, spec)
	case "stop":
		spec := s.buildSpec(app, "stop")
		if strings.TrimSpace(spec.Executable) == "" && !spec.Shell {
			return nil, ErrNoCommand
		}
		spec.Timeout = 15 * time.Second
		return s.exec.Run(ctx, spec)
	case "status":
		if app.AppType == models.AppTypeSystemd && app.SystemdUnit != "" {
			state, err := s.sysd.ActiveState(ctx, app.SystemdUnit)
			if err != nil {
				return nil, err
			}
			return &executor.Result{Stdout: state}, nil
		}
		timeout := time.Duration(app.CheckTimeoutSec) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		pid := s.currentPID(app.ID)
		checker := &health.Checker{SystemdActive: func(u string) string {
			st, err := s.sysd.ActiveState(ctx, u)
			if err != nil {
				return "unknown"
			}
			return st
		}}
		status, detail := checker.Check(ctx, app, pid, timeout)
		return &executor.Result{Stdout: fmt.Sprintf("%s (%s)", status, detail)}, nil
	}
	return nil, errors.New("未知测试类型")
}

// AutoStart 在服务启动时启动标记为开机自启的应用（默认全部关闭）。
func (s *Service) AutoStart(ctx context.Context) {
	list, err := s.repo.List()
	if err != nil {
		return
	}
	for _, a := range list {
		if !a.Enabled || !a.AutoStart {
			continue
		}
		if a.Scope == models.ScopeExternal {
			continue // 非本地应用不由 AppHub 启动
		}
		logging.Info("自动启动应用 [%s]", a.Name)
		if err := s.Start(ctx, a.ID, Actor{Username: "system"}); err != nil {
			logging.Warn("自动启动失败 [%s]: %v", a.Name, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ---------- 内部实现 ----------

func (s *Service) startSystemd(ctx context.Context, app *models.App) error {
	if strings.TrimSpace(app.SystemdUnit) == "" {
		return errors.New("未配置 systemd 服务名")
	}
	if !systemd.Available() {
		return errors.New("当前系统不支持 systemctl")
	}
	return s.sysd.Start(ctx, app.SystemdUnit)
}

func (s *Service) stopSystemd(ctx context.Context, app *models.App) error {
	if strings.TrimSpace(app.SystemdUnit) == "" {
		return errors.New("未配置 systemd 服务名")
	}
	if !systemd.Available() {
		return errors.New("当前系统不支持 systemctl")
	}
	return s.sysd.Stop(ctx, app.SystemdUnit)
}

func (s *Service) startCommand(ctx context.Context, app *models.App) error {
	spec := s.buildSpec(app, "start")
	if err := executor.Validate(spec); err != nil {
		return ErrNoCommand
	}
	// 已运行则不再启动，防止重复启动
	if p := executor.Registry().Get(app.ID); p != nil && p.Alive() {
		return errors.New("应用已在运行中")
	}
	spec.Timeout = 5 * time.Second
	_, err := s.exec.StartBackground(ctx, spec)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) stopCommand(ctx context.Context, app *models.App) error {
	// 1) 优先使用停止命令
	if spec := s.buildSpec(app, "stop"); executor.Validate(spec) == nil {
		spec.Timeout = stopTimeout(app)
		res, err := s.exec.Run(ctx, spec)
		if err != nil {
			return err
		}
		if res != nil && res.ExitCode != 0 && res.ExitCode != -1 {
			msg := strings.TrimSpace(res.Stderr)
			if msg == "" {
				msg = strings.TrimSpace(res.Stdout)
			}
			if msg != "" {
				return fmt.Errorf("停止命令退出码 %d: %s", res.ExitCode, truncate(msg, 200))
			}
			return fmt.Errorf("停止命令退出码 %d", res.ExitCode)
		}
	}

	// 2) 其次清理由 AppHub 启动的进程
	if p := executor.Registry().Get(app.ID); p != nil {
		return s.exec.StopProcess(p, stopTimeout(app))
	}

	// 3) 最后按状态目标中的 PID 清理
	if app.StatusType == models.CheckProcess && strings.TrimSpace(app.StatusTarget) != "" {
		var pid int
		if _, err := fmt.Sscanf(app.StatusTarget, "%d", &pid); err == nil && pid > 0 {
			p := &executor.Process{AppID: app.ID, PID: pid, StartedAt: time.Now()}
			return s.exec.StopProcess(p, stopTimeout(app))
		}
	}
	if spec := s.buildSpec(app, "stop"); executor.Validate(spec) != nil {
		return ErrNoCommand
	}
	return nil
}

func (s *Service) runCommand(ctx context.Context, app *models.App, kind string, timeout time.Duration) error {
	spec := s.buildSpec(app, kind)
	if err := executor.Validate(spec); err != nil {
		return ErrNoCommand
	}
	spec.Timeout = timeout
	res, err := s.exec.Run(ctx, spec)
	if err != nil {
		return err
	}
	if res != nil && res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		return fmt.Errorf("命令退出码 %d: %s", res.ExitCode, truncate(msg, 200))
	}
	return nil
}

// buildSpec 按应用配置构造执行描述。
func (s *Service) buildSpec(app *models.App, kind string) executor.Spec {
	spec := executor.Spec{
		Name:    fmt.Sprintf("%s:%s", app.Name, kind),
		WorkDir: strings.TrimSpace(app.WorkDir),
		Env:     app.EnvMap(),
		AppID:   app.ID,
		Shell:   app.ShellMode,
	}
	switch kind {
	case "start":
		spec.Executable = strings.TrimSpace(app.StartCommand)
		spec.Args = app.Args("start")
		if app.ShellMode {
			spec.Command = app.StartCommand
		}
	case "stop":
		spec.Executable = strings.TrimSpace(app.StopCommand)
		spec.Args = app.Args("stop")
		if app.ShellMode {
			spec.Command = app.StopCommand
		}
	case "restart":
		spec.Executable = strings.TrimSpace(app.RestartCommand)
		spec.Args = app.Args("restart")
		if app.ShellMode {
			spec.Command = app.RestartCommand
		}
	}
	return spec
}

func (s *Service) currentPID(appID int64) int {
	if p := executor.Registry().Get(appID); p != nil && p.Alive() {
		return p.PID
	}
	return 0
}

func startHold(app *models.App) time.Duration {
	if app.AppType == models.AppTypeSystemd {
		return 30 * time.Second
	}
	return 45 * time.Second
}

func refreshDelay(app *models.App) time.Duration {
	if app.AppType == models.AppTypeSystemd {
		return 2 * time.Second
	}
	return 3 * time.Second
}

func stopTimeout(app *models.App) time.Duration {
	if app.TimeoutSeconds > 0 {
		return time.Duration(app.TimeoutSeconds) * time.Second
	}
	return 30 * time.Second
}

func restartTimeout(app *models.App) time.Duration {
	if app.TimeoutSeconds > 0 {
		return time.Duration(app.TimeoutSeconds+30) * time.Second
	}
	return 90 * time.Second
}

func tryLock(m *sync.Mutex, ctx context.Context) bool {
	// 使用 goroutine 尝试加锁，避免长时间阻塞请求线程
	acquired := make(chan struct{})
	go func() {
		m.Lock()
		close(acquired)
	}()
	select {
	case <-acquired:
		return true
	case <-ctx.Done():
		return false
	case <-time.After(90 * time.Second):
		return false
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
