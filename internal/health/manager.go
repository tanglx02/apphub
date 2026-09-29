package health

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/tanglx02/apphub/internal/logging"
	"github.com/tanglx02/apphub/internal/models"
)

// AppProvider 返回当前全部应用定义快照。
type AppProvider func() []models.App

// PIDProvider 返回指定应用最近一次启动的进程 PID（0 表示无）。
type PIDProvider func(appID int64) int

// Options 状态管理器参数。
type Options struct {
	Interval    time.Duration
	Timeout     time.Duration
	Concurrency int
}

// Manager 后台统一维护应用状态，避免每个浏览器请求都触发大量 shell 调用。
//
// 行为：
//   - 每 Interval 秒轮询一次全部应用，结果写入缓存；
//   - 检测带超时与并发上限，单个应用失败不影响其他应用；
//   - 支持手动置为 STARTING/STOPPING 过渡态并在超时后自动让位给真实检测结果。
type Manager struct {
	mu          sync.RWMutex
	cache       map[int64]*models.AppRuntime
	transitions map[int64]*transition
	apps        AppProvider
	pids        PIDProvider
	checker     *Checker
	opts        Options

	stopCh   chan struct{}
	stopOnce sync.Once
	refresh  chan int64
}

type transition struct {
	status   models.AppStatus
	deadline time.Time
}

// NewManager 创建状态管理器。
func NewManager(apps AppProvider, pids PIDProvider, checker *Checker, opts Options) *Manager {
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	m := &Manager{
		cache:       make(map[int64]*models.AppRuntime),
		transitions: make(map[int64]*transition),
		apps:        apps,
		pids:        pids,
		checker:     checker,
		opts:        opts,
		stopCh:      make(chan struct{}),
		refresh:     make(chan int64, 64),
	}
	return m
}

// Start 启动后台轮询协程。
func (m *Manager) Start() {
	go m.loop()
}

// Stop 停止后台轮询。
func (m *Manager) Stop() {
	m.stopOnce.Do(func() { close(m.stopCh) })
}

// UpdateOptions 更新轮询参数。
func (m *Manager) UpdateOptions(opts Options) {
	m.mu.Lock()
	if opts.Interval > 0 {
		m.opts.Interval = opts.Interval
	}
	if opts.Timeout > 0 {
		m.opts.Timeout = opts.Timeout
	}
	if opts.Concurrency > 0 {
		m.opts.Concurrency = opts.Concurrency
	}
	m.mu.Unlock()
}

// Options 返回当前参数。
func (m *Manager) Options() Options {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.opts
}

func (m *Manager) loop() {
	ticker := time.NewTicker(m.opts.Interval)
	defer ticker.Stop()
	m.checkAll()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAll()
			m.mu.RLock()
			interval := m.opts.Interval
			m.mu.RUnlock()
			ticker.Reset(interval)
		case id := <-m.refresh:
			m.checkOne(id)
		}
	}
}

// Refresh 请求立即刷新指定应用状态。
func (m *Manager) Refresh(appID int64) {
	select {
	case m.refresh <- appID:
	default:
	}
}

// RefreshSoon 在后台延迟刷新（用于启动/停止后等待应用就绪）。
func (m *Manager) RefreshSoon(appID int64, delay time.Duration) {
	go func() {
		select {
		case <-m.stopCh:
			return
		case <-time.After(delay):
			m.Refresh(appID)
		}
	}()
}

// MarkTransition 将应用置为过渡态（STARTING / STOPPING）。
func (m *Manager) MarkTransition(appID int64, status models.AppStatus, hold time.Duration) {
	if hold <= 0 {
		hold = 60 * time.Second
	}
	m.mu.Lock()
	m.transitions[appID] = &transition{status: status, deadline: time.Now().Add(hold)}
	rt, ok := m.cache[appID]
	if !ok {
		rt = &models.AppRuntime{AppID: appID, Status: models.StatusUnknown}
		m.cache[appID] = rt
	}
	rt.Status = status
	rt.LastChecked = time.Now()
	m.mu.Unlock()
}

// ClearTransition 清除过渡态。
func (m *Manager) ClearTransition(appID int64) {
	m.mu.Lock()
	delete(m.transitions, appID)
	m.mu.Unlock()
}

// Get 读取缓存状态。
func (m *Manager) Get(appID int64) models.AppRuntime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if rt, ok := m.cache[appID]; ok {
		return *rt
	}
	return models.AppRuntime{AppID: appID, Status: models.StatusUnknown}
}

// Snapshot 返回全部缓存状态副本。
func (m *Manager) Snapshot() map[int64]models.AppRuntime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[int64]models.AppRuntime, len(m.cache))
	for id, rt := range m.cache {
		out[id] = *rt
	}
	return out
}

// Summary 汇总统计。
func (m *Manager) Summary(total int) (online, offline, errorCnt int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, rt := range m.cache {
		switch rt.Status {
		case models.StatusOnline:
			online++
		case models.StatusError:
			errorCnt++
		case models.StatusOffline:
			offline++
		}
	}
	return
}

func (m *Manager) checkAll() {
	apps := m.apps()
	if len(apps) == 0 {
		return
	}
	m.mu.RLock()
	concurrency := m.opts.Concurrency
	timeout := m.opts.Timeout
	m.mu.RUnlock()

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range apps {
		app := apps[i]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					logging.Error("状态检测异常 app=%d: %v", app.ID, r)
				}
			}()
			m.runCheck(&app, timeout)
		}()
	}
	wg.Wait()
	m.prune(apps)
}

func (m *Manager) checkOne(appID int64) {
	apps := m.apps()
	for i := range apps {
		if apps[i].ID == appID {
			m.mu.RLock()
			timeout := m.opts.Timeout
			m.mu.RUnlock()
			m.runCheck(&apps[i], timeout)
			return
		}
	}
}

func (m *Manager) runCheck(app *models.App, timeout time.Duration) {
	if !app.Enabled {
		m.mu.Lock()
		m.cache[app.ID] = &models.AppRuntime{AppID: app.ID, Status: models.StatusOffline, LastChecked: time.Now(), Detail: "应用已禁用"}
		m.mu.Unlock()
		return
	}

	// 过渡态：启动/停止过程中保留过渡显示，避免状态闪烁；
	// 但一旦真实检测确认已到达目标状态（启动完成 → ONLINE、停止完成 → OFFLINE），
	// 立即结束过渡态，避免长时间停留在"启动中"。
	m.mu.RLock()
	tr, hasTr := m.transitions[app.ID]
	m.mu.RUnlock()
	inTransition := hasTr && time.Now().Before(tr.deadline)
	transitionStatus := models.StatusUnknown
	if inTransition {
		transitionStatus = tr.status
	}

	pid := 0
	if m.pids != nil {
		pid = m.pids(app.ID)
	}
	// 非本地应用：默认用访问地址做健康检查；状态只保留 ONLINE/OFFLINE/UNKNOWN，
	// 不出现 STARTING/STOPPING/ERROR 等管理状态。
	checkApp := app
	if app.Scope == models.ScopeExternal {
		c := *app
		c.AppType = ""
		c.SystemdUnit = ""
		if strings.TrimSpace(c.StatusTarget) == "" {
			c.StatusTarget = c.ExternalURL
		}
		if c.StatusType != models.CheckTCP {
			c.StatusType = models.CheckHTTP
		}
		checkApp = &c
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Second)
	defer cancel()
	status, detail := m.checker.Check(ctx, checkApp, pid, timeout)

	if app.Scope == models.ScopeExternal {
		if status == models.StatusError {
			status = models.StatusUnknown
			detail = "检测异常，无法确认远程服务状态"
		}
		if status == models.StatusOffline {
			detail = "远程服务当前无法访问"
		}
		if status == models.StatusOnline {
			detail = "远程服务可访问"
		}
	}

	// 过渡态到达目标后立刻结束过渡
	if inTransition {
		if transitionStatus == models.StatusStarting && status == models.StatusOnline {
			inTransition = false
		} else if transitionStatus == models.StatusStopping && status == models.StatusOffline {
			inTransition = false
		}
	}
	if !inTransition && hasTr {
		m.mu.Lock()
		delete(m.transitions, app.ID)
		m.mu.Unlock()
	}
	if inTransition {
		status = transitionStatus
		detail = transitionDetail(transitionStatus)
	}

	m.mu.Lock()
	rt := m.ensureLocked(app.ID)
	prev := rt.Status
	rt.Status = status
	rt.Detail = detail
	rt.LastChecked = time.Now()
	rt.PID = pid
	if status == models.StatusError || status == models.StatusOffline {
		rt.LastError = ""
	}
	if prev != status {
		logging.Debug("应用状态变化 [%s] %s -> %s", app.Name, prev, status)
	}
	if status == models.StatusOnline && rt.StartedAt == nil {
		now := time.Now()
		rt.StartedAt = &now
	}
	if status == models.StatusOffline {
		rt.StartedAt = nil
		rt.UptimeSec = 0
	}
	if rt.StartedAt != nil {
		rt.UptimeSec = int64(time.Since(*rt.StartedAt).Seconds())
	}
	m.mu.Unlock()
}

func (m *Manager) ensureLocked(appID int64) *models.AppRuntime {
	rt, ok := m.cache[appID]
	if !ok {
		rt = &models.AppRuntime{AppID: appID, Status: models.StatusUnknown}
		m.cache[appID] = rt
	}
	return rt
}

// prune 清理已删除应用的缓存。
func (m *Manager) prune(apps []models.App) {
	live := make(map[int64]struct{}, len(apps))
	for _, a := range apps {
		live[a.ID] = struct{}{}
	}
	m.mu.Lock()
	for id := range m.cache {
		if _, ok := live[id]; !ok {
			delete(m.cache, id)
			delete(m.transitions, id)
		}
	}
	m.mu.Unlock()
}

// transitionDetail 返回过渡态的展示文案。
func transitionDetail(status models.AppStatus) string {
	switch status {
	case models.StatusStarting:
		return "正在启动，等待就绪"
	case models.StatusStopping:
		return "正在停止，等待进程退出"
	}
	return ""
}
