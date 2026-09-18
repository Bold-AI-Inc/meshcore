package adminapi

import (
	"net/http"

	"mesh-server/internal/httputil"
	"mesh-server/internal/middleware"
	"mesh-server/internal/security"
)

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *authHandlers) changePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if len(req.NewPassword) < 8 {
		httputil.WriteError(w, http.StatusBadRequest, "weak_password")
		return
	}

	adminID, _ := r.Context().Value(middleware.AdminIDKey).(string)
	admin, err := h.deps.Admins.FindByID(r.Context(), adminID)
	if err != nil {
		httputil.WriteError(w, http.StatusUnauthorized, "invalid_session")
		return
	}

	if !security.VerifyPassword(admin.PasswordHash, req.OldPassword) {
		logAudit(r.Context(), h.deps.Audit, admin.Email, "change_password_failed", admin.Email, map[string]string{"ip": h.deps.clientIP(r), "reason": "bad_old_password"})
		httputil.WriteError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	newHash, err := security.HashPassword(req.NewPassword)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	if err := h.deps.Admins.UpdatePasswordHash(r.Context(), admin.ID, newHash); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	h.deps.SessionCache.InvalidateAll()

	var currentHash string
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		currentHash = security.HashToken(cookie.Value)
	}
	if err := h.deps.Sessions.DeleteForAdminExcept(r.Context(), admin.ID, currentHash); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	logAudit(r.Context(), h.deps.Audit, admin.Email, "change_password", admin.Email, map[string]string{"ip": h.deps.clientIP(r)})
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
