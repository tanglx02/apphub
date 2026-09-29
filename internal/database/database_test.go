package database

import (
	"path/filepath"
	"testing"

	"github.com/tanglx02/apphub/internal/models"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"), 5000)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpenAndMigrate(t *testing.T) {
	db := openTestDB(t)

	v, err := db.Version()
	if err != nil {
		t.Fatalf("读取版本失败: %v", err)
	}
	if v != len(migrations) {
		t.Fatalf("迁移版本不匹配: 期望 %d, 实际 %d", len(migrations), v)
	}
	if err := db.Healthy(); err != nil {
		t.Fatalf("健康检查失败: %v", err)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	db := openTestDB(t)
	first, _ := db.Version()
	if err := db.Migrate(); err != nil {
		t.Fatalf("重复迁移失败: %v", err)
	}
	second, _ := db.Version()
	if first != second {
		t.Fatalf("重复迁移改变了版本号: %d -> %d", first, second)
	}
}

func TestSplitStatements(t *testing.T) {
	block := "CREATE TABLE a(id INTEGER); INSERT INTO a VALUES('a;b'); SELECT 1;"
	stmts := splitStatements(block)
	if len(stmts) != 3 {
		t.Fatalf("期望 3 条语句，实际 %d: %#v", len(stmts), stmts)
	}
	if stmts[1] != "INSERT INTO a VALUES('a;b')" {
		t.Fatalf("字符串内的分号应被保留，实际: %s", stmts[1])
	}
}

func TestSettingsStore(t *testing.T) {
	db := openTestDB(t)
	store := NewSettingsStore(db)

	if v := store.Get("missing", "fallback"); v != "fallback" {
		t.Fatalf("默认值错误: %s", v)
	}
	if err := store.Set("site_name", "测试中心"); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if v := store.Get("site_name", ""); v != "测试中心" {
		t.Fatalf("读取值错误: %s", v)
	}
	if err := store.SetBool("allow_batch_start", true); err != nil {
		t.Fatalf("写入布尔失败: %v", err)
	}
	if !store.GetBool("allow_batch_start", false) {
		t.Fatal("布尔值读取错误")
	}
	if err := store.SetInt("refresh_interval", 10); err != nil {
		t.Fatalf("写入整数失败: %v", err)
	}
	if store.GetInt("refresh_interval", 5) != 10 {
		t.Fatal("整数值读取错误")
	}
}

func TestAuditStore(t *testing.T) {
	db := openTestDB(t)
	store := NewAuditStore(db)

	if err := store.Insert(1, "admin", "192.168.1.1", models.AuditAppStart, "Vaultwarden", "success", "detail"); err != nil {
		t.Fatalf("写入审计失败: %v", err)
	}
	list, total, err := store.List(50, 0, "", "")
	if err != nil {
		t.Fatalf("读取审计失败: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("期望 1 条记录，实际 total=%d len=%d", total, len(list))
	}
	if list[0].ActionCN != "启动应用" {
		t.Fatalf("动作中文名错误: %s", list[0].ActionCN)
	}

	if _, total, _ = store.List(50, 0, string(models.AuditAppStop), ""); total != 0 {
		t.Fatalf("按动作过滤错误，期望 0 条")
	}

	if err := store.Clear(); err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	if _, total, _ = store.List(50, 0, "", ""); total != 0 {
		t.Fatalf("清空后仍存在记录")
	}
}

func TestBackupSnapshot(t *testing.T) {
	db := openTestDB(t)
	dest := filepath.Join(t.TempDir(), "snapshot.db")
	if err := db.Backup(dest); err != nil {
		t.Fatalf("生成快照失败: %v", err)
	}
	snap, err := Open(dest, 5000)
	if err != nil {
		t.Fatalf("快照不可打开: %v", err)
	}
	if v, _ := snap.Version(); v == 0 {
		t.Fatal("快照缺少 schema 版本，数据可能不完整")
	}
	_ = snap.Close()
}
