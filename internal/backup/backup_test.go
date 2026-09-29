package backup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tanglx02/apphub/internal/database"
	"github.com/tanglx02/apphub/internal/logging"
)

func dbPathOf(root string) string { return filepath.Join(root, "data", "apphub.db") }

func setup(t *testing.T) (*Manager, *database.DB, string) {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(root, "data", "apphub.db")
	db, err := database.Open(dbPath, 5000)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfgPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("server:\n  port: 18080\n"), 0o644); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}
	_ = os.MkdirAll(filepath.Join(root, "uploads"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "uploads", "icon.png"), []byte("fake-png"), 0o644)

	logger, _ := logging.New("", logging.LevelError, 1, 1, false)
	m := New(db, root, filepath.Join(root, "backups"), cfgPath, dbPath, logger)
	return m, db, root
}

func TestCreateAndListBackup(t *testing.T) {
	m, _, _ := setup(t)

	rec, err := m.Create("单元测试备份", "")
	if err != nil {
		t.Fatalf("创建备份失败: %v", err)
	}
	if rec.SizeBytes == 0 {
		t.Fatal("备份文件大小不应为 0")
	}
	if _, err := os.Stat(rec.Path); err != nil {
		t.Fatalf("备份文件不存在: %v", err)
	}

	list, err := m.List()
	if err != nil || len(list) == 0 {
		t.Fatalf("备份列表为空: %v", err)
	}
	if list[0].Name != rec.Name {
		t.Fatalf("备份名称不匹配: %s != %s", list[0].Name, rec.Name)
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	m, db, root := setup(t)

	if _, err := db.Exec(`INSERT INTO apps(name, slug, app_type, status_type) VALUES('待恢复应用','restore-me','command','none')`); err != nil {
		t.Fatalf("插入测试数据失败: %v", err)
	}
	rec, err := m.Create("恢复测试", "")
	if err != nil {
		t.Fatalf("创建备份失败: %v", err)
	}

	// 删除后再恢复
	if _, err := db.Exec(`DELETE FROM apps`); err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	// 恢复流程要求服务已停止：释放当前连接，再以"独立 CLI 进程"的方式重开连接
	if err := db.Close(); err != nil {
		t.Fatalf("关闭数据库失败: %v", err)
	}
	// 修改配置与上传目录，验证恢复能覆盖
	_ = os.WriteFile(filepath.Join(root, "config.yaml"), []byte("server:\n  port: 19999\n"), 0o644)
	_ = os.Remove(filepath.Join(root, "uploads", "icon.png"))

	// 模拟 apphub --restore：以新的连接执行恢复
	cliDB, err := database.Open(dbPathOf(root), 5000)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	logger, _ := logging.New("", logging.LevelError, 1, 1, false)
	cli := New(cliDB, root, filepath.Join(root, "backups"), filepath.Join(root, "config.yaml"), dbPathOf(root), logger)
	cli.SetDBCloser(cliDB.Close)
	pre, err := cli.Restore(rec.Name, DefaultOptions())
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if pre == nil {
		t.Fatal("恢复前应自动生成备份")
	}
	_ = cliDB.Close()

	data, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("读取恢复后的配置失败: %v", err)
	}
	if string(data) == "server:\n  port: 19999\n" {
		t.Fatal("配置文件未被恢复")
	}
	if _, err := os.Stat(filepath.Join(root, "uploads", "icon.png")); err != nil {
		t.Fatal("上传图标未被恢复")
	}

	// 数据库需要重开连接后校验
	db2, err := database.Open(dbPathOf(root), 5000)
	if err != nil {
		t.Fatalf("重开数据库失败: %v", err)
	}
	defer db2.Close()
	var name string
	if err := db2.QueryRow(`SELECT name FROM apps WHERE slug = 'restore-me'`).Scan(&name); err != nil {
		t.Fatalf("应用数据未恢复: %v", err)
	}
	if name != "待恢复应用" {
		t.Fatalf("恢复内容错误: %s", name)
	}
}

func TestRestoreRejectsMissing(t *testing.T) {
	m, _, _ := setup(t)
	if _, err := m.Restore("not-exists.tar.gz", DefaultOptions()); err != ErrNotFound {
		t.Fatalf("不存在的备份应返回 ErrNotFound，实际: %v", err)
	}
}

func TestDeleteBackup(t *testing.T) {
	m, _, _ := setup(t)
	rec, _ := m.Create("待删除", "")
	if err := m.Delete(rec.Name); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := m.Delete(rec.Name); err != ErrNotFound {
		t.Fatalf("重复删除应返回 ErrNotFound，实际: %v", err)
	}
}
