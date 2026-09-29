package database

import (
	"database/sql"
	"strconv"
	"time"
)

// SettingsStore 提供 key/value 形式的运行时设置读写。
type SettingsStore struct {
	db *DB
}

// NewSettingsStore 创建设置存储。
func NewSettingsStore(db *DB) *SettingsStore { return &SettingsStore{db: db} }

// Get 读取设置，不存在返回默认值。
func (s *SettingsStore) Get(key, def string) string {
	var val string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if err != nil {
		if err == sql.ErrNoRows {
			return def
		}
		return def
	}
	return val
}

// Set 写入设置（UPSERT）。
func (s *SettingsStore) Set(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key, value, updated_at) VALUES(?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now())
	return err
}

// GetBool 读取布尔设置。
func (s *SettingsStore) GetBool(key string, def bool) bool {
	v := s.Get(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// GetInt 读取整数设置。
func (s *SettingsStore) GetInt(key string, def int) int {
	v := s.Get(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// SetBool 写入布尔设置。
func (s *SettingsStore) SetBool(key string, v bool) error {
	return s.Set(key, strconv.FormatBool(v))
}

// SetInt 写入整数设置。
func (s *SettingsStore) SetInt(key string, v int) error {
	return s.Set(key, strconv.Itoa(v))
}

// All 返回全部设置。
func (s *SettingsStore) All() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		out[k] = v
	}
	return out, rows.Err()
}

// Delete 删除设置。
func (s *SettingsStore) Delete(key string) error {
	_, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, key)
	return err
}
