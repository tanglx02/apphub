package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tanglx02/apphub/internal/apps"
	"github.com/tanglx02/apphub/internal/auth"
	"github.com/tanglx02/apphub/internal/backup"
	"github.com/tanglx02/apphub/internal/buildinfo"
	"github.com/tanglx02/apphub/internal/config"
	"github.com/tanglx02/apphub/internal/database"
	"github.com/tanglx02/apphub/internal/executor"
	"github.com/tanglx02/apphub/internal/health"
	"github.com/tanglx02/apphub/internal/logging"
	"github.com/tanglx02/apphub/internal/models"
	"github.com/tanglx02/apphub/internal/systemd"
	"github.com/tanglx02/apphub/web"
)

type contextKey string

const (
	ctxKeyRequestID contextKey = "request_id"
	ctxKeySession   contextKey = "session"
	ctxKeyUserID    contextKey = "user_id"
)

var requestCounter uint64

// Deps 为 API 层依赖集合。
type Deps struct {
	Config     *config.Manager
	DB         *database.DB
	Logger     *logging.Logger
	Users      *auth.UserStore
	Sessions   *auth.SessionStore
	CSRF       *auth.CSRFManager
	RateLimit  *auth.RateLimiter
	LoginGuard *auth.LoginGuard
	Apps       *apps.Service
	Repo       *apps.Repository
	Cats       *apps.CategoryRepository
	Endpoints  *apps.EndpointRepository
	Status     *health.Manager
	Sysd       *systemd.Client
	Backups    *backup.Manager
	Settings   *database.SettingsStore
	Audits     *database.AuditStore
	Exec       *executor.Executor
	TLS        bool
}

// Server HTTP 服务。
type Server struct {
	deps Deps
	mux  *http.ServeMux
}

// New 创建 API 服务并注册路由。
func New(d Deps) *Server {
	s := &Server{deps: d, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler 返回带全局中间件的处理器。
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = s.securityHeaders(h)
	h = s.recoverer(h)
	h = s.requestIDMiddleware(h)
	return h
}

func logError(r *http.Request, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if r != nil {
		logging.Error("[%s %s] %s", r.Method, r.URL.Path, msg)
		return
	}
	logging.Error("%s", msg)
}

// ---------- 中间件 ----------

func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("%d-%s", atomic.AddUint64(&requestCounter, 1), shortID())
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func shortID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// securityHeaders 设置安全响应头。
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Robots-Tag", "noindex, nofollow")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; "+
				"script-src 'self' 'unsafe-inline'; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'")
		if s.deps.TLS {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// recoverer 捕获 panic，避免单个请求异常导致进程退出，也避免泄漏堆栈。
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logging.Error("请求处理异常 %s %s: %v", r.Method, r.URL.Path, rec)
				if !isAPI(r.URL.Path) {
					http.Error(w, "服务器内部错误", http.StatusInternalServerError)
					return
				}
				Fail(w, r, http.StatusInternalServerError, CodeInternal, "服务器内部错误，请查看日志了解详情")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func isAPI(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/health" || path == "/ready"
}

// requireAuth 校验会话；未登录返回 401。
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil || cookie.Value == "" {
			Fail(w, r, http.StatusUnauthorized, CodeUnauthorized, "未登录或会话已过期")
			return
		}
		sess := s.deps.Sessions.Get(cookie.Value)
		if sess == nil {
			Fail(w, r, http.StatusUnauthorized, CodeUnauthorized, "未登录或会话已过期")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeySession, sess)
		ctx = context.WithValue(ctx, ctxKeyUserID, sess.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// csrfGuard 校验写请求的 CSRF Token。
func (s *Server) csrfGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := s.deps.Config.Get()
		if !cfg.Security.CSRFEnabled {
			next.ServeHTTP(w, r)
			return
		}
		sess := sessionFrom(r)
		if sess == nil {
			Fail(w, r, http.StatusUnauthorized, CodeUnauthorized, "未登录或会话已过期")
			return
		}
		token := r.Header.Get("X-CSRF-Token")
		if token == "" {
			token = r.FormValue("csrf_token")
		}
		if !s.deps.CSRF.Validate(sess.ID, token) {
			Fail(w, r, http.StatusForbidden, CodeCSRF, "CSRF 校验失败，请刷新页面后重试")
			return
		}
		next.ServeHTTP(w, r)
	}
}

// rateLimitGuard 基础限流。
func (s *Server) rateLimitGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.deps.RateLimit == nil {
			next.ServeHTTP(w, r)
			return
		}
		cfg := s.deps.Config.Get()
		trusted := parseList(cfg.Server.TrustedProxies)
		ip := auth.ClientIP(r, trusted)
		if !s.deps.RateLimit.Allow(ip) {
			Fail(w, r, http.StatusTooManyRequests, CodeRateLimited, "请求过于频繁，请稍后再试")
			return
		}
		next.ServeHTTP(w, r)
	}
}

// withGuards 组合鉴权 + CSRF + 限流。
func (s *Server) withGuards(h http.HandlerFunc) http.HandlerFunc {
	return s.rateLimitGuard(s.requireAuth(s.csrfGuard(h)))
}

func sessionFrom(r *http.Request) *models.Session {
	if v, ok := r.Context().Value(ctxKeySession).(*models.Session); ok {
		return v
	}
	return nil
}

func actorFrom(r *http.Request) apps.Actor {
	sess := sessionFrom(r)
	if sess == nil {
		return apps.Actor{Username: "system"}
	}
	return apps.Actor{Username: sess.Username, IP: clientIPOf(r)}
}

func clientIPOf(r *http.Request) string {
	cfg := currentConfig()
	return auth.ClientIP(r, parseList(cfg.Server.TrustedProxies))
}

var configProvider func() config.Config

// SetConfigProvider 注入配置读取函数（供 actorFrom 使用）。
func SetConfigProvider(fn func() config.Config) { configProvider = fn }

func currentConfig() config.Config {
	if configProvider != nil {
		return configProvider()
	}
	return *config.Default()
}

func parseList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ---------- 审计 ----------

func (s *Server) audit(action models.AuditAction, target, result, detail string, actor apps.Actor) {
	if s.deps.Audits == nil {
		return
	}
	var userID int64
	if u, err := s.deps.Users.GetByUsername(actor.Username); err == nil && u != nil {
		userID = u.ID
	}
	_ = s.deps.Audits.Insert(userID, actor.Username, actor.IP, action, target, result, detail)
}

// ---------- 路由 ----------

func (s *Server) routes() {
	mux := s.mux

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, map[string]any{
			"status":  "ok",
			"version": buildinfo.Version,
			"time":    time.Now().Format(time.RFC3339),
		}, "")
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := s.deps.DB.Healthy(); err != nil {
			writeJSON(w, r, http.StatusServiceUnavailable, map[string]any{"status": "db_unavailable"}, "数据库不可用")
			return
		}
		writeJSON(w, r, http.StatusOK, map[string]any{"status": "ready"}, "")
	})

	// 认证
	mux.HandleFunc("POST /api/v1/auth/login", s.rateLimitGuard(s.handleLogin))
	mux.HandleFunc("POST /api/v1/auth/setup", s.rateLimitGuard(s.handleSetup))
	mux.HandleFunc("POST /api/v1/auth/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /api/v1/auth/me", s.requireAuth(s.handleMe))
	mux.HandleFunc("POST /api/v1/auth/password", s.withGuards(s.handleChangePassword))
	mux.HandleFunc("GET /api/v1/auth/sessions", s.requireAuth(s.handleListSessions))
	mux.HandleFunc("DELETE /api/v1/auth/sessions", s.withGuards(s.handleRevokeAllSessions))

	// 前台只读导航（Public API：仅 GET、匿名可访问、字段过滤，禁止任何写操作）
	mux.HandleFunc("GET /api/v1/public/config", s.rateLimitGuard(s.handlePublicConfig))
	mux.HandleFunc("GET /api/v1/public/apps", s.rateLimitGuard(s.handlePublicApps))
	mux.HandleFunc("GET /api/v1/public/categories", s.rateLimitGuard(s.handlePublicCategories))
	mux.HandleFunc("GET /api/v1/public/summary", s.rateLimitGuard(s.handlePublicSummary))
	mux.HandleFunc("GET /api/v1/public/overview", s.rateLimitGuard(s.handlePublicOverview))

	// 应用
	mux.HandleFunc("GET /api/v1/apps", s.requireAuth(s.handleListApps))
	mux.HandleFunc("POST /api/v1/apps", s.withGuards(s.handleCreateApp))
	mux.HandleFunc("PUT /api/v1/apps/reorder", s.withGuards(s.handleReorderApps))
	mux.HandleFunc("POST /api/v1/apps/batch", s.withGuards(s.handleBatch))
	mux.HandleFunc("POST /api/v1/apps/start-all", s.withGuards(s.handleStartAll))
	mux.HandleFunc("GET /api/v1/apps/export", s.requireAuth(s.handleExportApps))
	mux.HandleFunc("POST /api/v1/apps/import", s.withGuards(s.handleImportApps))
	mux.HandleFunc("GET /api/v1/apps/{id}", s.requireAuth(s.handleGetApp))
	mux.HandleFunc("PUT /api/v1/apps/{id}", s.withGuards(s.handleUpdateApp))
	mux.HandleFunc("DELETE /api/v1/apps/{id}", s.withGuards(s.handleDeleteApp))
	mux.HandleFunc("POST /api/v1/apps/{id}/start", s.withGuards(s.handleStartApp))
	mux.HandleFunc("POST /api/v1/apps/{id}/stop", s.withGuards(s.handleStopApp))
	mux.HandleFunc("POST /api/v1/apps/{id}/restart", s.withGuards(s.handleRestartApp))
	mux.HandleFunc("GET /api/v1/apps/{id}/status", s.requireAuth(s.handleAppStatus))
	mux.HandleFunc("GET /api/v1/apps/{id}/logs", s.requireAuth(s.handleAppLogs))
	mux.HandleFunc("POST /api/v1/apps/{id}/favorite", s.withGuards(s.handleFavorite))
	mux.HandleFunc("POST /api/v1/apps/{id}/test", s.withGuards(s.handleTestCommand))

	// 附加入口（EXTRA 导航入口；本地/非本地应用均可配置）
	mux.HandleFunc("GET /api/v1/apps/{id}/endpoints", s.requireAuth(s.handleListEndpoints))
	mux.HandleFunc("POST /api/v1/apps/{id}/endpoints", s.withGuards(s.handleCreateEndpoint))
	mux.HandleFunc("PUT /api/v1/apps/{id}/endpoints/{eid}", s.withGuards(s.handleUpdateEndpoint))
	mux.HandleFunc("DELETE /api/v1/apps/{id}/endpoints/{eid}", s.withGuards(s.handleDeleteEndpoint))

	// 分类
	mux.HandleFunc("GET /api/v1/categories", s.requireAuth(s.handleListCategories))
	mux.HandleFunc("POST /api/v1/categories", s.withGuards(s.handleCreateCategory))
	mux.HandleFunc("PUT /api/v1/categories/{id}", s.withGuards(s.handleUpdateCategory))
	mux.HandleFunc("DELETE /api/v1/categories/{id}", s.withGuards(s.handleDeleteCategory))

	// 系统
	mux.HandleFunc("GET /api/v1/system/info", s.requireAuth(s.handleSystemInfo))
	mux.HandleFunc("GET /api/v1/system/resources", s.requireAuth(s.handleSystemResources))
	mux.HandleFunc("GET /api/v1/system/events", s.requireAuth(s.handleEventStream))
	mux.HandleFunc("GET /api/v1/system/service", s.requireAuth(s.handleServiceStatus))
	mux.HandleFunc("POST /api/v1/system/service", s.withGuards(s.handleServiceAction))

	// 设置
	mux.HandleFunc("GET /api/v1/settings", s.requireAuth(s.handleGetSettings))
	mux.HandleFunc("PUT /api/v1/settings", s.withGuards(s.handleUpdateSettings))

	// 审计
	mux.HandleFunc("GET /api/v1/audit", s.requireAuth(s.handleListAudit))
	mux.HandleFunc("DELETE /api/v1/audit", s.withGuards(s.handleClearAudit))

	// 备份
	mux.HandleFunc("GET /api/v1/backups", s.requireAuth(s.handleListBackups))
	mux.HandleFunc("POST /api/v1/backups", s.withGuards(s.handleCreateBackup))
	mux.HandleFunc("POST /api/v1/backups/restore", s.withGuards(s.handleRestoreBackup))
	mux.HandleFunc("DELETE /api/v1/backups/{name}", s.withGuards(s.handleDeleteBackup))
	mux.HandleFunc("GET /api/v1/backups/{name}/download", s.requireAuth(s.handleDownloadBackup))

	// 上传与静态资源
	mux.HandleFunc("POST /api/v1/upload", s.withGuards(s.handleUpload))
	// 图标为公开静态资源（前台导航无需登录即可显示）
	mux.HandleFunc("GET /uploads/{name}", s.rateLimitGuard(s.handleServeUpload))

	// 前端静态资源（SPA fallback）
	mux.HandleFunc("/", s.handleSPA)
}

// handleSPA 服务嵌入的前端资源。
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && (strings.HasPrefix(r.URL.Path, "/api/")) {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "接口不存在")
		return
	}
	web.ServeSPA(w, r)
}

// decodeJSON 解析请求体，限制大小避免恶意超大 payload。
func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("请求体为空")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20)) // 4 MiB
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("请求参数解析失败: %w", err)
	}
	return nil
}
