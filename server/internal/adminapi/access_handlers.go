package adminapi

import (
	"net/http"

	"mesh-server/internal/httputil"
	"mesh-server/internal/middleware"
)

type accessHandlers struct {
	deps Deps
}

type accessRequest struct {
	UserID  string `json:"user_id"`
	ModelID string `json:"model_id"`
}

func (h *accessHandlers) grant(w http.ResponseWriter, r *http.Request) {
	var req accessRequest
	if err := httputil.DecodeJSON(r, &req); err != nil || req.UserID == "" || req.ModelID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := h.deps.Access.Grant(r.Context(), req.UserID, req.ModelID); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "grant_access", req.UserID+":"+req.ModelID, nil)
	rebuildMemState(r.Context(), h.deps.MemState)

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *accessHandlers) revoke(w http.ResponseWriter, r *http.Request) {
	var req accessRequest
	if err := httputil.DecodeJSON(r, &req); err != nil || req.UserID == "" || req.ModelID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := h.deps.Access.Revoke(r.Context(), req.UserID, req.ModelID); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "revoke_access", req.UserID+":"+req.ModelID, nil)
	rebuildMemState(r.Context(), h.deps.MemState)

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *accessHandlers) list(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	grants, err := h.deps.Access.ListForUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, grants)
}
