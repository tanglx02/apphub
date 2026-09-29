package auth

import (
	"database/sql"
	"errors"
	"time"

	"github.com/tanglx02/apphub/internal/models"
)

// UserStore 管理员账户存储。
type UserStore struct {
	db interface {
		Exec(query string, args ...any) (sql.Result, error)
		QueryRow(query string, args ...any) *sql.Row
		Query(query string, args ...any) (*sql.Rows, error)
	}
}

// NewUserStore 创建用户存储。
func NewUserStore(db *sql.DB) *UserStore { return &UserStore{db: db} }

// Count 返回用户数量，用于判断是否需要初始化。
func (s *UserStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// Create 创建用户（密码需已哈希）。
func (s *UserStore) Create(username, passwordHash string, role models.Role) (*models.User, error) {
	res, err := s.db.Exec(`INSERT INTO users(username, password_hash, role, created_at) VALUES(?, ?, ?, ?)`,
		username, passwordHash, string(role), time.Now())
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &models.User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		Role:         role,
		CreatedAt:    time.Now(),
	}, nil
}

// GetByUsername 按用户名查询。
func (s *UserStore) GetByUsername(username string) (*models.User, error) {
	u := &models.User{}
	var role string
	var lastLoginAt sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, username, password_hash, role, created_at, last_login_at, last_login_ip
		 FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &role, &u.CreatedAt, &lastLoginAt, &u.LastLoginIP)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	u.Role = models.Role(role)
	if lastLoginAt.Valid {
		t := lastLoginAt.Time
		u.LastLoginAt = &t
	}
	return u, nil
}

// GetByID 按 ID 查询。
func (s *UserStore) GetByID(id int64) (*models.User, error) {
	u := &models.User{}
	var role string
	var lastLoginAt sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, username, password_hash, role, created_at, last_login_at, last_login_ip
		 FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &role, &u.CreatedAt, &lastLoginAt, &u.LastLoginIP)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	u.Role = models.Role(role)
	if lastLoginAt.Valid {
		t := lastLoginAt.Time
		u.LastLoginAt = &t
	}
	return u, nil
}

// UpdatePassword 更新密码哈希。
func (s *UserStore) UpdatePassword(id int64, passwordHash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	return err
}

// TouchLogin 更新最后登录时间与 IP。
func (s *UserStore) TouchLogin(id int64, ip string) error {
	_, err := s.db.Exec(`UPDATE users SET last_login_at = ?, last_login_ip = ? WHERE id = ?`, time.Now(), ip, id)
	return err
}

// List 列出全部用户。
func (s *UserStore) List() ([]models.User, error) {
	rows, err := s.db.Query(`SELECT id, username, role, created_at, last_login_at, last_login_ip FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.User, 0)
	for rows.Next() {
		var u models.User
		var role string
		var last sql.NullTime
		if err := rows.Scan(&u.ID, &u.Username, &role, &u.CreatedAt, &last, &u.LastLoginIP); err != nil {
			continue
		}
		u.Role = models.Role(role)
		if last.Valid {
			t := last.Time
			u.LastLoginAt = &t
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
