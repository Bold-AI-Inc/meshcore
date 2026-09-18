package adminapi

import (
	"errors"
	"net/http"
	"strings"

	"mesh-server/internal/httputil"
	"mesh-server/internal/middleware"
	"mesh-server/internal/security"
	"mesh-server/internal/store"
)

func (d Deps) isSuperAdmin(email string) bool {
	return d.SuperAdminEmail != "" && strings.EqualFold(email, d.SuperAdminEmail)
}

// requireSuperAdmin gates the actions that can lock other operators out or
// halt all traffic. It lives on Deps rather than authHandlers so the status
// handlers can reuse it for the kill switch.
func (d Deps) requireSuperAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	if !d.isSuperAdmin(email) {
		httputil.WriteError(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	return email, true
}

type createAdminRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type createAdminResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (h *authHandlers) createAdmin(w http.ResponseWriter, r *http.Request) {
	var req createAdminRequest
	if err := httputil.DecodeJSON(r, &req); err != nil || req.Email == "" || len(req.Password) < 8 {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	creatorEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	if h.deps.isSuperAdmin(req.Email) && !h.deps.isSuperAdmin(creatorEmail) {
		httputil.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	admin, err := h.deps.Admins.Create(r.Context(), req.Email, hash, creatorEmail)
	if errors.Is(err, store.ErrAlreadyExists) {
		httputil.WriteError(w, http.StatusConflict, "email_already_exists")
		return
	}
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	logAudit(r.Context(), h.deps.Audit, creatorEmail, "create_admin", admin.Email, nil)

	httputil.WriteJSON(w, http.StatusCreated, createAdminResponse{ID: admin.ID, Email: admin.Email})
}

type adminListItem struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	CreatedBy string `json:"created_by"`
}

func (h *authHandlers) setAdminStatus(w http.ResponseWriter, r *http.Request, status string) {
	actor, ok := h.deps.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	targetID := r.PathValue("id")
	target, err := h.deps.Admins.FindByID(r.Context(), targetID)
	if errors.Is(err, store.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "admin_not_found")
		return
	}
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	if strings.EqualFold(target.Email, actor) {
		httputil.WriteError(w, http.StatusBadRequest, "cannot_modify_own_account")
		return
	}

	if err := h.deps.Admins.UpdateStatus(r.Context(), targetID, status); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	if status != "active" {
		if err := h.deps.Sessions.DeleteForAdmin(r.Context(), targetID); err != nil {
			httputil.WriteInternalError(w, err)
			return
		}
		h.deps.SessionCache.InvalidateAll()
	}

	action := "pause_admin"
	if status == "active" {
		action = "resume_admin"
	}
	logAudit(r.Context(), h.deps.Audit, actor, action, target.Email, map[string]string{"ip": h.deps.clientIP(r)})
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *authHandlers) pauseAdmin(w http.ResponseWriter, r *http.Request) {
	h.setAdminStatus(w, r, "paused")
}

func (h *authHandlers) resumeAdmin(w http.ResponseWriter, r *http.Request) {
	h.setAdminStatus(w, r, "active")
}

func (h *authHandlers) deleteAdmin(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.deps.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	targetID := r.PathValue("id")
	target, err := h.deps.Admins.FindByID(r.Context(), targetID)
	if errors.Is(err, store.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "admin_not_found")
		return
	}
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	if strings.EqualFold(target.Email, actor) {
		httputil.WriteError(w, http.StatusBadRequest, "cannot_modify_own_account")
		return
	}

	if err := h.deps.Admins.Delete(r.Context(), targetID); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	h.deps.SessionCache.InvalidateAll()

	logAudit(r.Context(), h.deps.Audit, actor, "delete_admin", target.Email, map[string]string{"ip": h.deps.clientIP(r)})
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *authHandlers) listAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := h.deps.Admins.List(r.Context())
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	items := make([]adminListItem, 0, len(admins))
	for _, admin := range admins {
		items = append(items, adminListItem{
			ID:        admin.ID,
			Email:     admin.Email,
			Status:    admin.Status,
			CreatedBy: admin.CreatedBy,
		})
	}

	httputil.WriteJSON(w, http.StatusOK, items)
}
