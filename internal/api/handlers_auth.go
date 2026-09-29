package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/tanglx02/apphub/internal/apps"
	"github.com/tanglx02/apphub/internal/auth"
	"github.com/tanglx02/apphub/internal/config"
	"github.com/tanglx02/apphub/internal/models"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	SiteName string `json:"site_name"`
}

type passwordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// handleLogin 管理员登录。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	count, err := s.deps.Users.Count()
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "服务暂不可用", err))
		return
	}
	if count == 0 {
		Fail(w, r, http.StatusConflict, CodeNotInitialized, "尚未创建管理员账户，请先完成初始化")
		return
	}

	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	ip := clientIPOf(r)

	if remain := s.deps.LoginGuard.Check(req.Username + "|" + ip); remain > 0 {
		Fail(w, r, http.StatusTooManyRequests, CodeRateLimited,
			"登录失败次数过多，请在 "+formatDuration(remain)+" 后重试")
		return
	}

	user, err := s.deps.Users.GetByUsername(req.Username)
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "服务暂不可用", err))
		return
	}
	ok := false
	if user != nil {
		ok, _ = auth.VerifyPassword(req.Password, user.PasswordHash)
	}
	if user == nil || !ok {
		locked, remain := s.deps.LoginGuard.Fail(req.Username + "|" + ip)
		s.audit(models.AuditLoginFailed, req.Username, "failed", "用户名或密码错误", apps.Actor{Username: req.Username, IP: ip})
		if locked {
			Fail(w, r, http.StatusTooManyRequests, CodeRateLimited,
				"登录失败次数过多，请在 "+formatDuration(remain)+" 后重试")
			return
		}
		Fail(w, r, http.StatusUnauthorized, CodeUnauthorized, "用户名或密码错误")
		return
	}

	sess, err := s.deps.Sessions.Create(user, ip, r.UserAgent())
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "创建会话失败", err))
		return
	}
	s.setSessionCookie(w, r, sess)
	_ = s.deps.Users.TouchLogin(user.ID, ip)
	s.deps.LoginGuard.Success(req.Username + "|" + ip)
	if s.deps.RateLimit != nil {
		s.deps.RateLimit.Reset(ip)
	}
	s.audit(models.AuditLogin, user.Username, "success", "", apps.Actor{Username: user.Username, IP: ip})

	OK(w, r, map[string]any{
		"user":       user.Public(),
		"csrf_token": s.deps.CSRF.Token(sess.ID),
		"expires_at": sess.ExpiresAt,
	})
}

// handleSetup 首次初始化，创建管理员账户。
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	count, err := s.deps.Users.Count()
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "服务暂不可用", err))
		return
	}
	if count > 0 {
		Fail(w, r, http.StatusConflict, CodeConflict, "管理员账户已存在，请直接登录")
		return
	}

	var req setupRequest
	if err := decodeJSON(r, &req); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if err := auth.ValidateUsername(req.Username); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "初始化失败", err))
		return
	}
	user, err := s.deps.Users.Create(req.Username, hash, models.RoleAdmin)
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "创建管理员失败", err))
		return
	}
	if strings.TrimSpace(req.SiteName) != "" {
		_ = s.deps.Settings.Set("site_name", strings.TrimSpace(req.SiteName))
		_ = s.deps.Config.Update(func(c *config.Config) { c.Server.SiteName = strings.TrimSpace(req.SiteName) })
	}
	s.audit(models.AuditPasswordReset, user.Username, "success", "初始化管理员账户",
		apps.Actor{Username: user.Username, IP: clientIPOf(r)})
	OK(w, r, map[string]any{"user": user.Public()})
}

// handleLogout 退出登录。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if sess := sessionFrom(r); sess != nil {
		_ = s.deps.Sessions.Delete(sess.ID)
		s.audit(models.AuditLogout, sess.Username, "success", "", apps.Actor{Username: sess.Username, IP: clientIPOf(r)})
	}
	s.clearSessionCookie(w)
	OK(w, r, map[string]any{"ok": true})
}

// handleMe 当前登录用户信息 + CSRF Token + 前端所需元信息。
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if sess == nil {
		Fail(w, r, http.StatusUnauthorized, CodeUnauthorized, "未登录或会话已过期")
		return
	}
	user, err := s.deps.Users.GetByID(sess.UserID)
	if err != nil || user == nil {
		Fail(w, r, http.StatusUnauthorized, CodeUnauthorized, "账户不存在或已被移除")
		return
	}
	cfg := s.deps.Config.Get()
	OK(w, r, map[string]any{
		"user":       user.Public(),
		"csrf_token": s.deps.CSRF.Token(sess.ID),
		"settings": map[string]any{
			"site_name":         s.siteName(),
			"logo":              s.deps.Settings.Get("logo", cfg.Server.Logo),
			"refresh_interval":  s.deps.Settings.GetInt("refresh_interval", cfg.Status.IntervalSeconds),
			"allow_batch_start": s.deps.Settings.GetBool("allow_batch_start", false),
			"timezone":          cfg.Server.Timezone,
			"https":             s.deps.TLS,
			"version":           versionString(),
			"db_version":        s.dbVersion(),
		},
	})
}

// handleChangePassword 修改密码，并使全部旧会话失效。
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if sess == nil {
		Fail(w, r, http.StatusUnauthorized, CodeUnauthorized, "未登录或会话已过期")
		return
	}
	var req passwordRequest
	if err := decodeJSON(r, &req); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	user, err := s.deps.Users.GetByID(sess.UserID)
	if err != nil || user == nil {
		Fail(w, r, http.StatusUnauthorized, CodeUnauthorized, "账户不存在")
		return
	}
	ok, _ := auth.VerifyPassword(req.OldPassword, user.PasswordHash)
	if !ok {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "当前密码不正确")
		return
	}
	if err := auth.ValidatePassword(req.NewPassword); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "修改密码失败", err))
		return
	}
	if err := s.deps.Users.UpdatePassword(user.ID, hash); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "修改密码失败", err))
		return
	}
	_ = s.deps.Sessions.DeleteByUser(user.ID)
	s.audit(models.AuditPasswordReset, user.Username, "success", "修改密码并使全部会话失效",
		apps.Actor{Username: user.Username, IP: clientIPOf(r)})
	OK(w, r, map[string]any{"ok": true})
}

// handleListSessions 活跃会话列表。
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Sessions.List()
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "读取会话失败", err))
		return
	}
	OK(w, r, list)
}

// handleRevokeAllSessions 使全部会话失效（强制重新登录）。
func (s *Server) handleRevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Sessions.DeleteAll(); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "操作失败", err))
		return
	}
	s.audit(models.AuditLogout, "全部会话", "success", "管理员主动使全部会话失效", actorFrom(r))
	OK(w, r, map[string]any{"ok": true})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, sess *models.Session) {
	secure := s.deps.TLS
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
		MaxAge:   int(time.Until(sess.ExpiresAt).Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func formatDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if minutes > 0 {
		return itoa(minutes) + " 分 " + itoa(seconds) + " 秒"
	}
	return itoa(seconds) + " 秒"
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [24]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
