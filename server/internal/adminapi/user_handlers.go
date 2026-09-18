package adminapi

import (
	"errors"
	"net/http"
	"time"

	"mesh-server/internal/httputil"
	"mesh-server/internal/middleware"
	"mesh-server/internal/security"
	"mesh-server/internal/store"
)

type userHandlers struct {
	deps Deps
}

type createUserRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

type createUserResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Key         string `json:"key"`
	KeyPrefix   string `json:"key_prefix"`
}

func (h *userHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := httputil.DecodeJSON(r, &req); err != nil || req.Email == "" || req.DisplayName == "" {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	user, err := h.deps.Users.Create(r.Context(), req.Email, req.DisplayName)
	if errors.Is(err, store.ErrAlreadyExists) {
		httputil.WriteError(w, http.StatusConflict, "email_already_exists")
		return
	}
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	plaintext, hash, prefix, err := security.GenerateMeshKey(h.deps.MeshKeyPepper)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	if err := h.deps.MeshKeys.Create(r.Context(), user.ID, hash, prefix); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "create_user", user.Email, nil)
	rebuildMemState(r.Context(), h.deps.MemState)

	httputil.WriteJSON(w, http.StatusCreated, createUserResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Key:         plaintext,
		KeyPrefix:   prefix,
	})
}

type userListItem struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	ExpiresAt   *time.Time `json:"expires_at"`
	KeyPrefix   *string    `json:"key_prefix"`
	KeyStatus   *string    `json:"key_status"`
}

func (h *userHandlers) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.deps.Users.ListWithKeyInfo(r.Context())
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	items := make([]userListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, userListItem{
			ID:          row.User.ID,
			Email:       row.User.Email,
			DisplayName: row.User.DisplayName,
			Status:      row.User.Status,
			ExpiresAt:   row.User.ExpiresAt,
			KeyPrefix:   row.KeyPrefix,
			KeyStatus:   row.KeyStatus,
		})
	}

	httputil.WriteJSON(w, http.StatusOK, items)
}

type updateExpiryRequest struct {
	ExpiresAt *time.Time `json:"expires_at"`
}

func (h *userHandlers) updateExpiry(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	var req updateExpiryRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := h.deps.Users.UpdateExpiry(r.Context(), userID, req.ExpiresAt); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "update_user_expiry", userID, nil)
	rebuildMemState(r.Context(), h.deps.MemState)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type rotateKeyResponse struct {
	Key       string `json:"key"`
	KeyPrefix string `json:"key_prefix"`
}

func (h *userHandlers) rotateKey(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")

	plaintext, hash, prefix, err := security.GenerateMeshKey(h.deps.MeshKeyPepper)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	if err := h.deps.MeshKeys.Rotate(r.Context(), userID, hash, prefix); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	if err := h.deps.Users.UpdateStatus(r.Context(), userID, "active"); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "rotate_user_key", userID, nil)
	rebuildMemState(r.Context(), h.deps.MemState)

	httputil.WriteJSON(w, http.StatusOK, rotateKeyResponse{Key: plaintext, KeyPrefix: prefix})
}

func (h *userHandlers) revoke(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if err := h.deps.Users.UpdateStatus(r.Context(), userID, "revoked"); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	if err := h.deps.MeshKeys.RevokeByUserID(r.Context(), userID); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "revoke_user", userID, nil)
	rebuildMemState(r.Context(), h.deps.MemState)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *userHandlers) delete(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if err := h.deps.Users.Delete(r.Context(), userID); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "delete_user", userID, nil)
	rebuildMemState(r.Context(), h.deps.MemState)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
