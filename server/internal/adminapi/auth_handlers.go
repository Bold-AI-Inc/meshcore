package adminapi

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"mesh-server/internal/httputil"
	"mesh-server/internal/middleware"
	"mesh-server/internal/security"
)

type authHandlers struct {
	deps Deps
}

var dummyPasswordHash = func() string {
	filler, err := security.GenerateToken()
	if err != nil {
		filler = "mesh-login-timing-filler"
	}
	hash, err := security.HashPassword(filler)
	if err != nil {
		return ""
	}
	return hash
}()

var unknownAccountAudit = newAuditThrottle(time.Minute)

type auditThrottle struct {
	mu     sync.Mutex
	window time.Duration
	seen   map[string]time.Time
}

func newAuditThrottle(window time.Duration) *auditThrottle {
	return &auditThrottle{window: window, seen: make(map[string]time.Time)}
}

func (t *auditThrottle) allow(key string) bool {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.seen) > 1024 {
		for k, at := range t.seen {
			if now.Sub(at) > t.window {
				delete(t.seen, k)
			}
		}
	}
	if at, ok := t.seen[key]; ok && now.Sub(at) < t.window {
		return false
	}
	t.seen[key] = now
	return true
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *authHandlers) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	ip := h.deps.clientIP(r)

	admin, err := h.deps.Admins.FindByEmail(r.Context(), req.Email)
	if err != nil || admin.Status != "active" {
		_ = security.VerifyPassword(dummyPasswordHash, req.Password)
		if unknownAccountAudit.allow(ip) {
			logAudit(r.Context(), h.deps.Audit, req.Email, "login_failed", req.Email, map[string]string{"ip": ip, "reason": "unknown_account"})
		}
		httputil.WriteError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	if admin.LockedUntil != nil && admin.LockedUntil.After(time.Now()) {
		logAudit(r.Context(), h.deps.Audit, admin.Email, "login_failed", admin.Email, map[string]string{"ip": ip, "reason": "account_locked"})
		httputil.WriteError(w, http.StatusUnauthorized, "account_locked")
		return
	}

	if !security.VerifyPassword(admin.PasswordHash, req.Password) {
		locked, lockErr := h.deps.Admins.RecordFailedLogin(r.Context(), admin.ID)
		if lockErr != nil {
			slog.Error("record failed login failed", "admin", admin.Email, "err", lockErr)
		}
		reason := "bad_password"
		if locked {
			reason = "bad_password_now_locked"
		}
		logAudit(r.Context(), h.deps.Audit, admin.Email, "login_failed", admin.Email, map[string]string{"ip": ip, "reason": reason})
		httputil.WriteError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	token, err := security.GenerateToken()
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if err := h.deps.Sessions.Create(r.Context(), admin.ID, security.HashToken(token), expiresAt); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	if err := h.deps.Admins.RecordSuccessfulLogin(r.Context(), admin.ID); err != nil {
		slog.Error("record successful login failed", "admin", admin.Email, "err", err)
	}

	csrfToken, err := security.GenerateToken()
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	setSessionCookie(w, h.deps.CookieSecure, token, expiresAt)
	setCSRFCookie(w, h.deps.CookieSecure, csrfToken, expiresAt)
	logAudit(r.Context(), h.deps.Audit, admin.Email, "login", admin.Email, map[string]string{"ip": ip})
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "csrf_token": csrfToken})
}

func (h *authHandlers) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		tokenHash := security.HashToken(cookie.Value)
		_ = h.deps.Sessions.Delete(r.Context(), tokenHash)
		h.deps.SessionCache.Invalidate(tokenHash)
	}
	if email, ok := r.Context().Value(middleware.AdminEmailKey).(string); ok && email != "" {
		logAudit(r.Context(), h.deps.Audit, email, "logout", email, map[string]string{"ip": h.deps.clientIP(r)})
	}
	clearSessionCookie(w, h.deps.CookieSecure)
	clearCSRFCookie(w, h.deps.CookieSecure)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *authHandlers) me(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	var csrfToken string
	if cookie, err := r.Cookie(csrfCookieName); err == nil {
		csrfToken = cookie.Value
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"email":          email,
		"csrf_token":     csrfToken,
		"is_super_admin": h.deps.isSuperAdmin(email),
	})
}
