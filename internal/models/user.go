package models

import "time"

// Role 用户角色。第一版仅启用 admin，其余保留供扩展。
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// Valid 判断角色是否合法。
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleOperator, RoleViewer:
		return true
	}
	return false
}

// CanControl 是否允许执行启停等控制操作。
func (r Role) CanControl() bool { return r == RoleAdmin || r == RoleOperator }

// CanWrite 是否允许写入配置。
func (r Role) CanWrite() bool { return r != RoleViewer }

// User 管理员账户。
type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Role         Role       `json:"role"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP  string     `json:"last_login_ip,omitempty"`
}

// PublicUser 返回给前端的安全用户信息（不含 hash）。
type PublicUser struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Role        Role       `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP string     `json:"last_login_ip,omitempty"`
}

// Public 转换为安全视图。
func (u *User) Public() PublicUser {
	return PublicUser{
		ID:          u.ID,
		Username:    u.Username,
		Role:        u.Role,
		CreatedAt:   u.CreatedAt,
		LastLoginAt: u.LastLoginAt,
		LastLoginIP: u.LastLoginIP,
	}
}

// Session 会话记录。
type Session struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Expired 判断会话是否过期。
func (s *Session) Expired() bool { return time.Now().After(s.ExpiresAt) }
