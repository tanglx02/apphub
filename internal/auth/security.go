package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

// CSRFManager 生成并校验与会话绑定的 CSRF Token。
//
// 前端从 /api/v1/auth/me 获取 token，之后所有写请求携带 X-CSRF-Token。
// 配合 SameSite=Lax Cookie 形成双重防护。
type CSRFManager struct {
	secret []byte
}

// NewCSRFManager 创建 CSRF 管理器（进程内随机密钥）。
func NewCSRFManager() *CSRFManager {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		b = []byte("apphub-csrf-fallback-secret")
	}
	return &CSRFManager{secret: b}
}

// Token 生成会话绑定的令牌。
func (c *CSRFManager) Token(sessionID string) string {
	mac := hmac.New(sha256.New, c.secret)
	_, _ = mac.Write([]byte("csrf:" + sessionID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Validate 校验令牌。
func (c *CSRFManager) Validate(sessionID, token string) bool {
	if sessionID == "" || token == "" {
		return false
	}
	want := c.Token(sessionID)
	return subtle.ConstantTimeCompare([]byte(want), []byte(token)) == 1
}

// RateLimiter 基于滑动窗口的简单限流器。
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	records map[string][]time.Time
}

// NewRateLimiter 创建限流器（每窗口 limit 次）。
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 120
	}
	return &RateLimiter{limit: limit, window: window, records: make(map[string][]time.Time)}
}

// Allow 判断 key 是否允许访问。
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-r.window)
	items := r.records[key]
	kept := items[:0]
	for _, t := range items {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= r.limit {
		r.records[key] = kept
		return false
	}
	r.records[key] = append(kept, now)
	return true
}

// Reset 清除 key 的记录（登录成功后调用）。
func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, key)
}

func (r *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		cutoff := time.Now().Add(-r.window)
		for k, items := range r.records {
			kept := items[:0]
			for _, t := range items {
				if t.After(cutoff) {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				delete(r.records, k)
			} else {
				r.records[k] = kept
			}
		}
		r.mu.Unlock()
	}
}

// LoginGuard 登录失败锁定。
type LoginGuard struct {
	mu       sync.Mutex
	maxFails int
	lockFor  time.Duration
	fails    map[string]int
	locked   map[string]time.Time
}

// NewLoginGuard 创建登录守卫。
func NewLoginGuard(maxFails int, lockMinutes int) *LoginGuard {
	if maxFails <= 0 {
		maxFails = 5
	}
	if lockMinutes <= 0 {
		lockMinutes = 15
	}
	g := &LoginGuard{
		maxFails: maxFails,
		lockFor:  time.Duration(lockMinutes) * time.Minute,
		fails:    make(map[string]int),
		locked:   make(map[string]time.Time),
	}
	go g.cleanupLoop()
	return g
}

// ErrLocked 账户被临时锁定。
var ErrLocked = errors.New("登录失败次数过多，请稍后再试")

// Check 返回剩余锁定时间；0 表示未锁定。
func (g *LoginGuard) Check(key string) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	if until, ok := g.locked[key]; ok {
		if remain := time.Until(until); remain > 0 {
			return remain
		}
		delete(g.locked, key)
		delete(g.fails, key)
	}
	return 0
}

// Fail 记录一次失败，返回是否已触发锁定。
func (g *LoginGuard) Fail(key string) (locked bool, remain time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fails[key]++
	if g.fails[key] >= g.maxFails {
		until := time.Now().Add(g.lockFor)
		g.locked[key] = until
		delete(g.fails, key)
		return true, g.lockFor
	}
	return false, 0
}

// Success 清除失败计数。
func (g *LoginGuard) Success(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fails, key)
	delete(g.locked, key)
}

func (g *LoginGuard) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		g.mu.Lock()
		now := time.Now()
		for k, until := range g.locked {
			if now.After(until) {
				delete(g.locked, k)
			}
		}
		g.mu.Unlock()
	}
}

// ClientIP 解析客户端真实 IP。仅信任配置中的可信代理。
func ClientIP(r *http.Request, trusted []string) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote := net.ParseIP(host)
	if remote == nil {
		return host
	}
	for _, cidr := range trusted {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			if net.ParseIP(cidr).Equal(remote) {
				return forwardedIP(r, remote)
			}
			continue
		}
		if network.Contains(remote) {
			return forwardedIP(r, remote)
		}
	}
	return host
}

func forwardedIP(r *http.Request, fallback net.IP) string {
	for _, h := range []string{"X-Forwarded-For", "X-Real-IP"} {
		v := r.Header.Get(h)
		if v == "" {
			continue
		}
		parts := splitComma(v)
		if len(parts) == 0 {
			continue
		}
		if ip := net.ParseIP(parts[0]); ip != nil {
			return ip.String()
		}
	}
	return fallback.String()
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == ',' {
			if t := trimSpace(cur); t != "" {
				out = append(out, t)
			}
			cur = ""
			continue
		}
		cur += string(c)
	}
	if t := trimSpace(cur); t != "" {
		out = append(out, t)
	}
	return out
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
