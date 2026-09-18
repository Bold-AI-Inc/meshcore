package adminapi

import (
	"errors"
	"net/http"

	"mesh-server/internal/domain"
	"mesh-server/internal/httputil"
	"mesh-server/internal/middleware"
	"mesh-server/internal/security"
	"mesh-server/internal/store"
)

type modelHandlers struct {
	deps Deps
}

type createModelRequest struct {
	ProviderID             string           `json:"provider_id"`
	Name                   string           `json:"name"`
	ResolvedModel          string           `json:"resolved_model"`
	InputPricePer1kTokens  float64          `json:"input_price_per_1k_tokens"`
	OutputPricePer1kTokens float64          `json:"output_price_per_1k_tokens"`
	Kind                   domain.ModelKind `json:"kind"`
	SupportsImageIn        bool             `json:"supports_image_in"`
	SupportsDocumentIn     bool             `json:"supports_document_in"`
	SupportsAudioIn        bool             `json:"supports_audio_in"`
	SupportsVideoIn        bool             `json:"supports_video_in"`
	SupportsWebSearch      bool             `json:"supports_web_search"`
	PricePerSecond         float64          `json:"price_per_second"`
	OutputMediaType        *string          `json:"output_media_type"`
}

func (h *modelHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req createModelRequest
	if err := httputil.DecodeJSON(r, &req); err != nil ||
		req.ProviderID == "" || req.Name == "" || req.ResolvedModel == "" ||
		req.InputPricePer1kTokens < 0 || req.OutputPricePer1kTokens < 0 {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	conn, err := h.deps.Providers.GetConnection(r.Context(), req.ProviderID)
	if errors.Is(err, store.ErrNotFound) {
		httputil.WriteError(w, http.StatusBadRequest, "provider_not_found")
		return
	}
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	apiKey, err := security.DecryptWithMasterKey(h.deps.MasterKey, conn.EncryptedAPIKey, conn.KeyNonce)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	if err := verifyModelByKind(r.Context(), req.Kind, conn, apiKey, req.ResolvedModel); err != nil {
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":    "model_verification_failed",
			"failures": []modelFailure{{Model: req.Name, Reason: err.Error()}},
		})
		return
	}

	model, err := h.deps.Models.Create(r.Context(), req.ProviderID, req.Name, req.ResolvedModel,
		req.InputPricePer1kTokens, req.OutputPricePer1kTokens, store.ModelCreateParams{
			Kind:               req.Kind,
			SupportsImageIn:    req.SupportsImageIn,
			SupportsDocumentIn: req.SupportsDocumentIn,
			SupportsAudioIn:    req.SupportsAudioIn,
			SupportsVideoIn:    req.SupportsVideoIn,
			SupportsWebSearch:  req.SupportsWebSearch,
			PricePerSecond:     req.PricePerSecond,
			OutputMediaType:    req.OutputMediaType,
		})
	if errors.Is(err, store.ErrAlreadyExists) {
		httputil.WriteError(w, http.StatusConflict, "model_already_exists")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		httputil.WriteError(w, http.StatusBadRequest, "provider_not_found")
		return
	}
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "create_model", model.Name, nil)
	rebuildMemState(r.Context(), h.deps.MemState)

	httputil.WriteJSON(w, http.StatusCreated, model)
}

func (h *modelHandlers) list(w http.ResponseWriter, r *http.Request) {
	models, err := h.deps.Models.List(r.Context())
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, models)
}

type updateModelRequest struct {
	Name                   string  `json:"name"`
	ResolvedModel          string  `json:"resolved_model"`
	InputPricePer1kTokens  float64 `json:"input_price_per_1k_tokens"`
	OutputPricePer1kTokens float64 `json:"output_price_per_1k_tokens"`
	Blocked                bool    `json:"blocked"`
	SupportsImageIn        bool    `json:"supports_image_in"`
	SupportsDocumentIn     bool    `json:"supports_document_in"`
	SupportsAudioIn        bool    `json:"supports_audio_in"`
	SupportsVideoIn        bool    `json:"supports_video_in"`
	SupportsWebSearch      bool    `json:"supports_web_search"`
	PricePerSecond         float64 `json:"price_per_second"`
	OutputMediaType        *string `json:"output_media_type"`
}

func (h *modelHandlers) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateModelRequest
	if err := httputil.DecodeJSON(r, &req); err != nil || req.Name == "" || req.ResolvedModel == "" ||
		req.InputPricePer1kTokens < 0 || req.OutputPricePer1kTokens < 0 {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	existing, err := h.deps.Models.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "model_not_found")
		return
	}
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	conn, err := h.deps.Providers.GetConnection(r.Context(), existing.ProviderID)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	apiKey, err := security.DecryptWithMasterKey(h.deps.MasterKey, conn.EncryptedAPIKey, conn.KeyNonce)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	if err := verifyModelByKind(r.Context(), existing.Kind, conn, apiKey, req.ResolvedModel); err != nil {
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":    "model_verification_failed",
			"failures": []modelFailure{{Model: req.Name, Reason: err.Error()}},
		})
		return
	}

	if err := h.deps.Models.Update(r.Context(), id, req.Name, req.ResolvedModel,
		req.InputPricePer1kTokens, req.OutputPricePer1kTokens, req.Blocked, store.ModelCreateParams{
			SupportsImageIn:    req.SupportsImageIn,
			SupportsDocumentIn: req.SupportsDocumentIn,
			SupportsAudioIn:    req.SupportsAudioIn,
			SupportsVideoIn:    req.SupportsVideoIn,
			SupportsWebSearch:  req.SupportsWebSearch,
			PricePerSecond:     req.PricePerSecond,
			OutputMediaType:    req.OutputMediaType,
		}); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			httputil.WriteError(w, http.StatusConflict, "model_already_exists")
			return
		}
		httputil.WriteInternalError(w, err)
		return
	}

	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "update_model", req.Name, nil)
	rebuildMemState(r.Context(), h.deps.MemState)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *modelHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.deps.Models.Delete(r.Context(), id); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "delete_model", id, nil)
	rebuildMemState(r.Context(), h.deps.MemState)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
