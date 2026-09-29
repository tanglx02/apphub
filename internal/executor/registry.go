package executor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Process 记录由 AppHub 启动的常驻进程。
type Process struct {
	AppID     int64     `json:"app_id"`
	PID       int       `json:"pid"`
	PGID      int       `json:"pgid"`
	Cmd       string    `json:"cmd"`
	WorkDir   string    `json:"work_dir"`
	StartedAt time.Time `json:"started_at"`

	cmd     *exec.Cmd
	logFile *os.File
	exited  bool
	mu      sync.RWMutex
}

func (p *Process) markExited() {
	p.mu.Lock()
	p.exited = true
	p.mu.Unlock()
}

// Exited 判断进程是否已退出。
func (p *Process) Exited() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exited
}

// Alive 判断进程是否仍在运行。
func (p *Process) Alive() bool { return !p.Exited() && alive(p.PID) }

// Uptime 返回运行时长。
func (p *Process) Uptime() time.Duration { return time.Since(p.StartedAt) }

// ProcessRegistry 管理全部由 AppHub 启动的进程，并持久化 PID 以便重启后恢复感知。
type ProcessRegistry struct {
	mu    sync.RWMutex
	dir   string
	procs map[int64]*Process
}

var (
	registryOnce sync.Once
	registry     *ProcessRegistry
)

// Registry 返回全局进程注册表。
func Registry() *ProcessRegistry {
	registryOnce.Do(func() {
		registry = &ProcessRegistry{procs: make(map[int64]*Process)}
	})
	return registry
}

// SetDir 设置 PID 持久化目录。
func (r *ProcessRegistry) SetDir(dir string) {
	r.mu.Lock()
	r.dir = dir
	r.mu.Unlock()
	_ = os.MkdirAll(dir, 0o755)
}

// Set 登记进程并写入 PID 文件。
func (r *ProcessRegistry) Set(p *Process) {
	r.mu.Lock()
	r.procs[p.AppID] = p
	dir := r.dir
	r.mu.Unlock()
	if dir != "" {
		_ = r.save(p)
	}
}

// Get 获取应用关联进程。
func (r *ProcessRegistry) Get(appID int64) *Process {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.procs[appID]
}

// Remove 移除进程并清理 PID 文件。
func (r *ProcessRegistry) Remove(appID int64) {
	r.mu.Lock()
	delete(r.procs, appID)
	dir := r.dir
	r.mu.Unlock()
	if dir != "" {
		_ = os.Remove(filepath.Join(dir, pidFileName(appID)))
	}
}

// All 返回全部登记进程。
func (r *ProcessRegistry) All() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Process, 0, len(r.procs))
	for _, p := range r.procs {
		out = append(out, p)
	}
	return out
}

// Load 从磁盘恢复 PID 记录（服务重启后仍可感知此前启动的进程）。
func (r *ProcessRegistry) Load() error {
	r.mu.RLock()
	dir := r.dir
	r.mu.RUnlock()
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var p Process
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		if !alive(p.PID) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
			continue
		}
		r.mu.Lock()
		r.procs[p.AppID] = &p
		r.mu.Unlock()
	}
	return nil
}

func (r *ProcessRegistry) save(p *Process) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.dir, pidFileName(p.AppID)), data, 0o644)
}

func pidFileName(appID int64) string {
	return "app-" + itoa(appID) + ".json"
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [24]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
