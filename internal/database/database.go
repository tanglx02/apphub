// Package database 封装 SQLite 连接、PRAGMA 设置与版本化迁移。
//
// 安全约束：所有查询必须参数化，禁止字符串拼接 SQL。
package database

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动，避免 CGO

	"github.com/tanglx02/apphub/internal/logging"
)

// DB 为数据库句柄封装。
type DB struct {
	*sql.DB
	mu   sync.RWMutex
	path string
}

// Open 打开（或创建）SQLite 数据库，并应用必要的 PRAGMA 与迁移。
func Open(path string, busyTimeoutMs int) (*DB, error) {
	if path == "" {
		return nil, errors.New("数据库路径为空")
	}
	if err := ensureDir(path); err != nil {
		return nil, err
	}
	if busyTimeoutMs <= 0 {
		busyTimeoutMs = 5000
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)",
		path, busyTimeoutMs)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite 为单写者模型，限制连接池避免锁竞争
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	db := &DB{DB: sqlDB, path: path}
	if err := db.ping(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := db.Migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	logging.Info("数据库已就绪: %s", path)
	return db, nil
}

func (db *DB) ping() error {
	for i := 0; i < 3; i++ {
		if err := db.DB.Ping(); err == nil {
			return nil
		} else if i == 2 {
			return fmt.Errorf("数据库连接失败: %w", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

// Path 返回数据库文件路径。
func (db *DB) Path() string { return db.path }

// Close 关闭数据库。
func (db *DB) Close() error { return db.DB.Close() }

// Healthy 用于 /ready 探针。
func (db *DB) Healthy() error {
	var n int
	return db.QueryRow(`SELECT 1`).Scan(&n)
}

// Version 读取当前 schema 版本。
func (db *DB) Version() (int, error) {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func (db *DB) setVersion(v int) error {
	// PRAGMA 不支持参数化，但 v 为内部整数，安全
	_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v))
	return err
}

// Migrate 按序应用迁移，禁止升级时破坏既有数据。
func (db *DB) Migrate() error {
	current, err := db.Version()
	if err != nil {
		return err
	}
	for i, m := range migrations {
		version := i + 1
		if version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		for _, stmt := range splitStatements(m.SQL) {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("迁移 %s 失败: %w", m.Name, err)
			}
		}
		if err := setVersionTx(tx, version); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交迁移 %s 失败: %w", m.Name, err)
		}
		logging.Info("已应用数据库迁移 %d: %s", version, m.Name)
	}
	return nil
}

func setVersionTx(tx *sql.Tx, v int) error {
	_, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v))
	return err
}

// Backup 生成一致性数据库快照（VACUUM INTO），无需停机。
func (db *DB) Backup(destPath string) error {
	if err := ensureDir(destPath); err != nil {
		return err
	}
	dest, err := filepath.Abs(destPath)
	if err != nil {
		return err
	}
	// VACUUM INTO 不支持绑定参数，路径由内部生成并做引号转义
	lit := "'" + strings.ReplaceAll(dest, "'", "''") + "'"
	if _, err := db.Exec("VACUUM INTO " + lit); err != nil {
		return fmt.Errorf("导出数据库快照失败: %w", err)
	}
	return nil
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
