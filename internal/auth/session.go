package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/tanglx02/apphub/internal/models"
)

// CookieName 为会话 Cookie 名称。
const CookieName = "apphub_session"

// SessionStore 会话存储：DB 持久化 + 内存缓存。
type SessionStore struct {
	db    *sql.DB
	mu    sync.RWMutex
	ttl   time.Duration
	cache map[string]*models.Session
}

// NewSessionStore 创建会话存储并启动清理协程。
func NewSessionStore(db *sql.DB, ttl time.Duration) *SessionStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	s := &SessionStore{db: db, ttl: ttl, cache: make(map[string]*models.Session)}
	go s.cleanupLoop()
	return s
}

// SetTTL 更新会话有效期。
func (s *SessionStore) SetTTL(ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ttl = ttl
}

// Create 创建新会话。
func (s *SessionStore) Create(u *models.User, ip, ua string) (*models.Session, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(b)
	now := time.Now()
	sess := &models.Session{
		ID:        id,
		UserID:    u.ID,
		Username:  u.Username,
		Role:      u.Role,
		IP:        ip,
		UserAgent: truncate(ua, 200),
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}
	if _, err := s.db.Exec(
		`INSERT INTO sessions(id, user_id, username, role, ip, user_agent, created_at, expires_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.Username, string(sess.Role), sess.IP, sess.UserAgent, sess.CreatedAt, sess.ExpiresAt,
	); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[id] = sess
	s.mu.Unlock()
	return sess, nil
}

// Get 读取会话，过期返回 nil。
func (s *SessionStore) Get(id string) *models.Session {
	if id == "" {
		return nil
	}
	s.mu.RLock()
	cached, ok := s.cache[id]
	s.mu.RUnlock()
	if ok {
		if cached.Expired() {
			_ = s.Delete(id)
			return nil
		}
		return cached
	}

	var sess models.Session
	var role string
	err := s.db.QueryRow(
		`SELECT id, user_id, username, role, ip, user_agent, created_at, expires_at FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &sess.Username, &role, &sess.IP, &sess.UserAgent, &sess.CreatedAt, &sess.ExpiresAt)
	if err != nil {
		return nil
	}
	sess.Role = models.Role(role)
	if sess.Expired() {
		_ = s.Delete(id)
		return nil
	}
	s.mu.Lock()
	s.cache[id] = &sess
	s.mu.Unlock()
	return &sess
}

// Delete 删除单个会话。
func (s *SessionStore) Delete(id string) error {
	s.mu.Lock()
	delete(s.cache, id)
	s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteByUser 使用户的全部会话失效（改密、重置密码时使用）。
func (s *SessionStore) DeleteByUser(userID int64) error {
	s.mu.Lock()
	for id, sess := range s.cache {
		if sess.UserID == userID {
			delete(s.cache, id)
		}
	}
	s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// DeleteAll 使全部会话失效。
func (s *SessionStore) DeleteAll() error {
	s.mu.Lock()
	s.cache = make(map[string]*models.Session)
	s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM sessions`)
	return err
}

// List 列出活跃会话。
func (s *SessionStore) List() ([]models.Session, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, username, role, ip, user_agent, created_at, expires_at
		 FROM sessions WHERE expires_at > ? ORDER BY created_at DESC`, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Session, 0)
	for rows.Next() {
		var s models.Session
		var role string
		if err := rows.Scan(&s.ID, &s.UserID, &s.Username, &role, &s.IP, &s.UserAgent, &s.CreatedAt, &s.ExpiresAt); err != nil {
			continue
		}
		s.Role = models.Role(role)
		out = append(out, s)
	}
	return out, rows.Err()
}

// ErrNoSession 表示请求未携带有效会话。
var ErrNoSession = errors.New("未登录或会话已过期")

func (s *SessionStore) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Add(-24*time.Hour))
		s.mu.Lock()
		for id, sess := range s.cache {
			if sess.Expired() {
				delete(s.cache, id)
			}
		}
		s.mu.Unlock()
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
