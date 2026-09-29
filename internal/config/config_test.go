package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigAndPathResolution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("创建配置管理器失败: %v", err)
	}
	if m.Root() != filepath.Clean(dir) {
		t.Fatalf("项目根目录错误: %s", m.Root())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("默认配置应被写入磁盘")
	}

	cfg := m.Get()
	if cfg.Server.Port != 18080 {
		t.Fatalf("默认端口错误: %d", cfg.Server.Port)
	}
	if !filepath.IsAbs(m.DatabasePath()) {
		t.Fatalf("数据库路径应为绝对路径: %s", m.DatabasePath())
	}
	if !stringsHasPrefix(m.DatabasePath(), dir) {
		t.Fatalf("相对路径应基于项目根目录解析: %s", m.DatabasePath())
	}
}

func TestUpdateAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := m.Update(func(c *Config) {
		c.Server.Port = 19999
		c.Server.SiteName = "测试中心"
	}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	reloaded, err := NewManager(path)
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if reloaded.Get().Server.Port != 19999 {
		t.Fatalf("配置未持久化: %d", reloaded.Get().Server.Port)
	}
	if reloaded.Get().Server.SiteName != "测试中心" {
		t.Fatalf("站点名称未持久化: %s", reloaded.Get().Server.SiteName)
	}
}

func TestValidationClamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	m, _ := NewManager(path)
	if err := m.Update(func(c *Config) {
		c.Status.IntervalSeconds = 0
		c.Security.SessionTimeout = -1
	}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	cfg := m.Get()
	if cfg.Status.IntervalSeconds < 3 {
		t.Fatalf("检测间隔应被限制为 >= 3，实际 %d", cfg.Status.IntervalSeconds)
	}
	if cfg.Security.SessionTimeout <= 0 {
		t.Fatalf("会话时长应被修正，实际 %d", cfg.Security.SessionTimeout)
	}
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
