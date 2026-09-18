package adminapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"mesh-server/internal/domain"
	"mesh-server/internal/httputil"
	"mesh-server/internal/middleware"
	"mesh-server/internal/providerfamily"
	"mesh-server/internal/security"
	"mesh-server/internal/store"
	"mesh-server/internal/templating"
)

type providerHandlers struct {
	deps Deps
}

type modelInputRequest struct {
	Name                   string           `json:"name"`
	ResolvedModel          string           `json:"resolved_model"`
	InputPricePer1kTokens  float64          `json:"input_price_per_1k_tokens"`
	OutputPricePer1kTokens float64          `json:"output_price_per_1k_tokens"`
	SupportsImageIn        bool             `json:"supports_image_in"`
	SupportsDocumentIn     bool             `json:"supports_document_in"`
	SupportsAudioIn        bool             `json:"supports_audio_in"`
	SupportsVideoIn        bool             `json:"supports_video_in"`
	SupportsWebSearch      bool             `json:"supports_web_search"`
	PricePerSecond         float64          `json:"price_per_second"`
	Kind                   domain.ModelKind `json:"kind"`
	OutputMediaType        *string          `json:"output_media_type"`
}

type createProviderRequest struct {
	Name                  string                `json:"name"`
	APIKey                string                `json:"api_key"`
	APIKeyFromProviderID  *string               `json:"api_key_from_provider_id"`
	EndpointURL           string                `json:"endpoint_url"`
	HTTPMethod            string                `json:"http_method"`
	HeaderTemplate        json.RawMessage       `json:"header_template"`
	RequestBodyTemplate   json.RawMessage       `json:"request_body_template"`
	ResponseDeltaPath     string                `json:"response_delta_path"`
	ResponseDoneSignal    *string               `json:"response_done_signal"`
	UsageInputTokensPath  *string               `json:"usage_input_tokens_path"`
	UsageOutputTokensPath *string               `json:"usage_output_tokens_path"`
	MaxOutboundRPS        int                   `json:"max_outbound_rps"`
	MaxConcurrentUpstream int                   `json:"max_concurrent_upstream"`
	RetryEnabled          bool                  `json:"retry_enabled"`
	MaxRetries            int                   `json:"max_retries"`
	RetryBackoffMs        int                   `json:"retry_backoff_ms"`
	MaxCallsPerHour       *int                  `json:"max_calls_per_hour"`
	ProviderFamily        domain.ProviderFamily `json:"provider_family"`
	Models                []modelInputRequest   `json:"models"`

	SupportsEmbeddings           bool            `json:"supports_embeddings"`
	EmbeddingEndpointURL         *string         `json:"embedding_endpoint_url"`
	EmbeddingRequestBodyTemplate json.RawMessage `json:"embedding_request_body_template"`
	EmbeddingResponseVectorPath  *string         `json:"embedding_response_vector_path"`

	SupportsGeneration            bool            `json:"supports_generation"`
	GenerationEndpointURL         *string         `json:"generation_endpoint_url"`
	GenerationRequestBodyTemplate json.RawMessage `json:"generation_request_body_template"`
	GenerationResponseMediaPath   *string         `json:"generation_response_media_path"`

	SupportsWebSearch            bool            `json:"supports_web_search"`
	WebSearchRequestBodyTemplate json.RawMessage `json:"web_search_request_body_template"`
	WebSearchPricePerCall        float64         `json:"web_search_price_per_call"`

	GenerationMode                string   `json:"generation_mode"`
	GenerationJobIDPath           *string  `json:"generation_job_id_path"`
	GenerationStatusURLTemplate   *string  `json:"generation_status_url_template"`
	GenerationStatusPath          *string  `json:"generation_status_path"`
	GenerationContentURLTemplate  *string  `json:"generation_content_url_template"`
	GenerationStatusSuccessValues []string `json:"generation_status_success_values"`
	GenerationStatusFailureValues []string `json:"generation_status_failure_values"`
	GenerationPollIntervalMs      int      `json:"generation_poll_interval_ms"`
	GenerationMaxWaitSeconds      int      `json:"generation_max_wait_seconds"`

	EmbeddingUsageTokensPath        *string `json:"embedding_usage_tokens_path"`
	GenerationUsageInputTokensPath  *string `json:"generation_usage_input_tokens_path"`
	GenerationUsageOutputTokensPath *string `json:"generation_usage_output_tokens_path"`
	GenerationDurationSecondsPath   *string `json:"generation_duration_seconds_path"`

	LogRequests *bool `json:"log_requests"`
}

func resolvedLogRequests(v *bool) bool {
	return v == nil || *v
}

func (req createProviderRequest) resolvedFamily() domain.ProviderFamily {
	if req.ProviderFamily == "" {
		return domain.ProviderFamilyGeneric
	}
	return req.ProviderFamily
}

type modelFailure struct {
	Model  string `json:"model"`
	Reason string `json:"reason"`
}

func nullableJSONString(raw json.RawMessage) *string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	s := string(raw)
	return &s
}

func webSearchValid(supports bool, template json.RawMessage, pricePerCall float64, family domain.ProviderFamily) bool {
	if pricePerCall < 0 || !providerfamily.Known(string(family)) {
		return false
	}
	if !supports {
		return true
	}
	if len(template) == 0 || !json.Valid(template) {
		return false
	}
	resolved := family
	if resolved == "" {
		resolved = domain.ProviderFamilyGeneric
	}
	return providerfamily.SupportsWebSearch(string(resolved))
}

func (req createProviderRequest) webSearchValid() bool {
	return webSearchValid(req.SupportsWebSearch, req.WebSearchRequestBodyTemplate, req.WebSearchPricePerCall, req.resolvedFamily())
}

func (req updateProviderRequest) webSearchValid() bool {
	return webSearchValid(req.SupportsWebSearch, req.WebSearchRequestBodyTemplate, req.WebSearchPricePerCall, req.resolvedFamily())
}

func (req createProviderRequest) resolvedGenerationMode() string {
	if req.GenerationMode == "" {
		return "sync"
	}
	return req.GenerationMode
}

func (req updateProviderRequest) resolvedGenerationMode() string {
	if req.GenerationMode == "" {
		return "sync"
	}
	return req.GenerationMode
}

func defaultStatuses(v []string, fallback string) []string {
	if len(v) == 0 {
		return []string{fallback}
	}
	return v
}

func defaultInt(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func generationValid(mode string, jobIDPath, statusURL, statusPath, contentURL *string) bool {
	if mode == "" || mode == "sync" {
		return true
	}
	if mode != "async_poll" {
		return false
	}
	nonEmpty := func(p *string) bool { return p != nil && *p != "" }
	return nonEmpty(jobIDPath) && nonEmpty(statusURL) && nonEmpty(statusPath) && nonEmpty(contentURL)
}

func (req createProviderRequest) generationValid() bool {
	return generationValid(req.GenerationMode, req.GenerationJobIDPath, req.GenerationStatusURLTemplate,
		req.GenerationStatusPath, req.GenerationContentURLTemplate)
}

func (req updateProviderRequest) generationValid() bool {
	return generationValid(req.GenerationMode, req.GenerationJobIDPath, req.GenerationStatusURLTemplate,
		req.GenerationStatusPath, req.GenerationContentURLTemplate)
}

func bodyTemplatesReferenceKey(templates ...json.RawMessage) bool {
	for _, t := range templates {
		if templating.References(t, "api_key") {
			return true
		}
	}
	return false
}

func (req createProviderRequest) valid() bool {
	hasKey := req.APIKey != "" || (req.APIKeyFromProviderID != nil && *req.APIKeyFromProviderID != "")
	return req.Name != "" && hasKey && req.EndpointURL != "" && req.HTTPMethod != "" &&
		len(req.HeaderTemplate) > 0 && len(req.RequestBodyTemplate) > 0 && req.ResponseDeltaPath != "" &&
		len(req.Models) > 0 && json.Valid(req.HeaderTemplate) && json.Valid(req.RequestBodyTemplate) &&
		!bodyTemplatesReferenceKey(req.RequestBodyTemplate, req.EmbeddingRequestBodyTemplate, req.GenerationRequestBodyTemplate, req.WebSearchRequestBodyTemplate) &&
		req.webSearchValid() && req.generationValid()
}

func (h *providerHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req createProviderRequest
	if err := httputil.DecodeJSON(r, &req); err != nil || !req.valid() {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	apiKeyPlaintext := req.APIKey
	if apiKeyPlaintext == "" && req.APIKeyFromProviderID != nil {
		conn, err := h.deps.Providers.GetConnection(r.Context(), *req.APIKeyFromProviderID)
		if errors.Is(err, store.ErrNotFound) {
			httputil.WriteError(w, http.StatusBadRequest, "source_provider_not_found")
			return
		}
		if err != nil {
			httputil.WriteInternalError(w, err)
			return
		}
		decrypted, err := security.DecryptWithMasterKey(h.deps.MasterKey, conn.EncryptedAPIKey, conn.KeyNonce)
		if err != nil {
			httputil.WriteInternalError(w, err)
			return
		}
		apiKeyPlaintext = decrypted
	}

	newConn := &store.ProviderConnection{
		EndpointURL:                   req.EndpointURL,
		HTTPMethod:                    req.HTTPMethod,
		HeaderTemplate:                req.HeaderTemplate,
		RequestBodyTemplate:           req.RequestBodyTemplate,
		ProviderFamily:                req.resolvedFamily(),
		EmbeddingEndpointURL:          req.EmbeddingEndpointURL,
		EmbeddingRequestBodyTemplate:  req.EmbeddingRequestBodyTemplate,
		GenerationEndpointURL:         req.GenerationEndpointURL,
		GenerationRequestBodyTemplate: req.GenerationRequestBodyTemplate,
	}

	targets := make([]verifyTarget, 0, len(req.Models))
	for _, m := range req.Models {
		if m.Name == "" || m.ResolvedModel == "" {
			httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		targets = append(targets, verifyTarget{name: m.Name, kind: m.Kind, resolvedModel: m.ResolvedModel})
	}
	if failures := verifyModels(r.Context(), newConn, apiKeyPlaintext, targets); len(failures) > 0 {
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":    "model_verification_failed",
			"failures": failures,
		})
		return
	}

	encryptedKey, nonce, err := security.EncryptWithMasterKey(h.deps.MasterKey, apiKeyPlaintext)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	modelInputs := make([]store.ModelInput, len(req.Models))
	for i, m := range req.Models {
		modelInputs[i] = store.ModelInput{
			Name:                   m.Name,
			ResolvedModel:          m.ResolvedModel,
			InputPricePer1kTokens:  m.InputPricePer1kTokens,
			OutputPricePer1kTokens: m.OutputPricePer1kTokens,
			SupportsImageIn:        m.SupportsImageIn,
			SupportsDocumentIn:     m.SupportsDocumentIn,
			SupportsAudioIn:        m.SupportsAudioIn,
			SupportsVideoIn:        m.SupportsVideoIn,
			SupportsWebSearch:      m.SupportsWebSearch,
			PricePerSecond:         m.PricePerSecond,
			Kind:                   m.Kind,
			OutputMediaType:        m.OutputMediaType,
		}
	}

	provider, models, err := h.deps.Providers.CreateWithModels(r.Context(), store.CreateProviderParams{
		Name:                  req.Name,
		EncryptedAPIKey:       encryptedKey,
		KeyNonce:              nonce,
		EndpointURL:           req.EndpointURL,
		HTTPMethod:            req.HTTPMethod,
		HeaderTemplate:        string(req.HeaderTemplate),
		RequestBodyTemplate:   string(req.RequestBodyTemplate),
		ResponseDeltaPath:     req.ResponseDeltaPath,
		ResponseDoneSignal:    req.ResponseDoneSignal,
		UsageInputTokensPath:  req.UsageInputTokensPath,
		UsageOutputTokensPath: req.UsageOutputTokensPath,
		MaxOutboundRPS:        req.MaxOutboundRPS,
		MaxConcurrentUpstream: req.MaxConcurrentUpstream,
		RetryEnabled:          req.RetryEnabled,
		MaxRetries:            req.MaxRetries,
		RetryBackoffMs:        req.RetryBackoffMs,
		MaxCallsPerHour:       req.MaxCallsPerHour,
		ProviderFamily:        req.resolvedFamily(),

		SupportsEmbeddings:           req.SupportsEmbeddings,
		EmbeddingEndpointURL:         req.EmbeddingEndpointURL,
		EmbeddingRequestBodyTemplate: nullableJSONString(req.EmbeddingRequestBodyTemplate),
		EmbeddingResponseVectorPath:  req.EmbeddingResponseVectorPath,

		SupportsGeneration:            req.SupportsGeneration,
		GenerationEndpointURL:         req.GenerationEndpointURL,
		GenerationRequestBodyTemplate: nullableJSONString(req.GenerationRequestBodyTemplate),
		GenerationResponseMediaPath:   req.GenerationResponseMediaPath,

		SupportsWebSearch:            req.SupportsWebSearch,
		WebSearchRequestBodyTemplate: nullableJSONString(req.WebSearchRequestBodyTemplate),
		WebSearchPricePerCall:        req.WebSearchPricePerCall,

		GenerationMode:                req.resolvedGenerationMode(),
		GenerationJobIDPath:           req.GenerationJobIDPath,
		GenerationStatusURLTemplate:   req.GenerationStatusURLTemplate,
		GenerationStatusPath:          req.GenerationStatusPath,
		GenerationContentURLTemplate:  req.GenerationContentURLTemplate,
		GenerationStatusSuccessValues: defaultStatuses(req.GenerationStatusSuccessValues, "completed"),
		GenerationStatusFailureValues: defaultStatuses(req.GenerationStatusFailureValues, "failed"),
		GenerationPollIntervalMs:      defaultInt(req.GenerationPollIntervalMs, 3000),
		GenerationMaxWaitSeconds:      defaultInt(req.GenerationMaxWaitSeconds, 600),

		EmbeddingUsageTokensPath:        req.EmbeddingUsageTokensPath,
		GenerationUsageInputTokensPath:  req.GenerationUsageInputTokensPath,
		GenerationUsageOutputTokensPath: req.GenerationUsageOutputTokensPath,
		GenerationDurationSecondsPath:   req.GenerationDurationSecondsPath,

		LogRequests: resolvedLogRequests(req.LogRequests),
	}, modelInputs)
	if errors.Is(err, store.ErrAlreadyExists) {
		httputil.WriteError(w, http.StatusConflict, "provider_or_model_already_exists")
		return
	}
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "create_provider", provider.Name, nil)
	rebuildMemState(r.Context(), h.deps.MemState)

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"provider": provider, "models": models})
}

func (h *providerHandlers) list(w http.ResponseWriter, r *http.Request) {
	providers, err := h.deps.Providers.List(r.Context())
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, providers)
}

type updateProviderRequest struct {
	Name                  string          `json:"name"`
	APIKey                string          `json:"api_key"`
	EndpointURL           string          `json:"endpoint_url"`
	HTTPMethod            string          `json:"http_method"`
	HeaderTemplate        json.RawMessage `json:"header_template"`
	RequestBodyTemplate   json.RawMessage `json:"request_body_template"`
	ResponseDeltaPath     string          `json:"response_delta_path"`
	ResponseDoneSignal    *string         `json:"response_done_signal"`
	UsageInputTokensPath  *string         `json:"usage_input_tokens_path"`
	UsageOutputTokensPath *string         `json:"usage_output_tokens_path"`
	MaxOutboundRPS        int             `json:"max_outbound_rps"`
	MaxConcurrentUpstream int             `json:"max_concurrent_upstream"`
	RetryEnabled          bool            `json:"retry_enabled"`
	MaxRetries            int             `json:"max_retries"`
	RetryBackoffMs        int             `json:"retry_backoff_ms"`
	MaxCallsPerHour       *int            `json:"max_calls_per_hour"`

	ProviderFamily domain.ProviderFamily `json:"provider_family"`

	SupportsEmbeddings           bool            `json:"supports_embeddings"`
	EmbeddingEndpointURL         *string         `json:"embedding_endpoint_url"`
	EmbeddingRequestBodyTemplate json.RawMessage `json:"embedding_request_body_template"`
	EmbeddingResponseVectorPath  *string         `json:"embedding_response_vector_path"`

	SupportsGeneration            bool            `json:"supports_generation"`
	GenerationEndpointURL         *string         `json:"generation_endpoint_url"`
	GenerationRequestBodyTemplate json.RawMessage `json:"generation_request_body_template"`
	GenerationResponseMediaPath   *string         `json:"generation_response_media_path"`

	SupportsWebSearch            bool            `json:"supports_web_search"`
	WebSearchRequestBodyTemplate json.RawMessage `json:"web_search_request_body_template"`
	WebSearchPricePerCall        float64         `json:"web_search_price_per_call"`

	GenerationMode                string   `json:"generation_mode"`
	GenerationJobIDPath           *string  `json:"generation_job_id_path"`
	GenerationStatusURLTemplate   *string  `json:"generation_status_url_template"`
	GenerationStatusPath          *string  `json:"generation_status_path"`
	GenerationContentURLTemplate  *string  `json:"generation_content_url_template"`
	GenerationStatusSuccessValues []string `json:"generation_status_success_values"`
	GenerationStatusFailureValues []string `json:"generation_status_failure_values"`
	GenerationPollIntervalMs      int      `json:"generation_poll_interval_ms"`
	GenerationMaxWaitSeconds      int      `json:"generation_max_wait_seconds"`

	EmbeddingUsageTokensPath        *string `json:"embedding_usage_tokens_path"`
	GenerationUsageInputTokensPath  *string `json:"generation_usage_input_tokens_path"`
	GenerationUsageOutputTokensPath *string `json:"generation_usage_output_tokens_path"`
	GenerationDurationSecondsPath   *string `json:"generation_duration_seconds_path"`

	LogRequests *bool `json:"log_requests"`
}

func (req updateProviderRequest) valid() bool {
	return req.Name != "" && req.EndpointURL != "" && req.HTTPMethod != "" &&
		len(req.HeaderTemplate) > 0 && len(req.RequestBodyTemplate) > 0 && req.ResponseDeltaPath != "" &&
		json.Valid(req.HeaderTemplate) && json.Valid(req.RequestBodyTemplate) &&
		!bodyTemplatesReferenceKey(req.RequestBodyTemplate, req.EmbeddingRequestBodyTemplate, req.GenerationRequestBodyTemplate, req.WebSearchRequestBodyTemplate) &&
		req.webSearchValid() && req.generationValid()
}

func (req updateProviderRequest) resolvedFamily() domain.ProviderFamily {
	if req.ProviderFamily == "" {
		return domain.ProviderFamilyGeneric
	}
	return req.ProviderFamily
}

func (h *providerHandlers) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateProviderRequest
	if err := httputil.DecodeJSON(r, &req); err != nil || !req.valid() {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	apiKeyPlaintext := req.APIKey
	if apiKeyPlaintext == "" {
		conn, err := h.deps.Providers.GetConnection(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "provider_not_found")
			return
		}
		if err != nil {
			httputil.WriteInternalError(w, err)
			return
		}
		decrypted, err := security.DecryptWithMasterKey(h.deps.MasterKey, conn.EncryptedAPIKey, conn.KeyNonce)
		if err != nil {
			httputil.WriteInternalError(w, err)
			return
		}
		apiKeyPlaintext = decrypted
	}

	models, err := h.deps.Models.ListByProvider(r.Context(), id)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	conn := &store.ProviderConnection{
		EndpointURL:                   req.EndpointURL,
		HTTPMethod:                    req.HTTPMethod,
		HeaderTemplate:                req.HeaderTemplate,
		RequestBodyTemplate:           req.RequestBodyTemplate,
		ProviderFamily:                req.resolvedFamily(),
		EmbeddingEndpointURL:          req.EmbeddingEndpointURL,
		EmbeddingRequestBodyTemplate:  req.EmbeddingRequestBodyTemplate,
		GenerationEndpointURL:         req.GenerationEndpointURL,
		GenerationRequestBodyTemplate: req.GenerationRequestBodyTemplate,
	}

	targets := make([]verifyTarget, 0, len(models))
	for _, m := range models {
		targets = append(targets, verifyTarget{name: m.Name, kind: m.Kind, resolvedModel: m.ResolvedModel})
	}
	if failures := verifyModels(r.Context(), conn, apiKeyPlaintext, targets); len(failures) > 0 {
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":    "model_verification_failed",
			"failures": failures,
		})
		return
	}

	encryptedKey, nonce, err := security.EncryptWithMasterKey(h.deps.MasterKey, apiKeyPlaintext)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	provider, err := h.deps.Providers.Update(r.Context(), id, store.CreateProviderParams{
		Name:                  req.Name,
		EncryptedAPIKey:       encryptedKey,
		KeyNonce:              nonce,
		EndpointURL:           req.EndpointURL,
		HTTPMethod:            req.HTTPMethod,
		HeaderTemplate:        string(req.HeaderTemplate),
		RequestBodyTemplate:   string(req.RequestBodyTemplate),
		ResponseDeltaPath:     req.ResponseDeltaPath,
		ResponseDoneSignal:    req.ResponseDoneSignal,
		UsageInputTokensPath:  req.UsageInputTokensPath,
		UsageOutputTokensPath: req.UsageOutputTokensPath,
		MaxOutboundRPS:        req.MaxOutboundRPS,
		MaxConcurrentUpstream: req.MaxConcurrentUpstream,
		RetryEnabled:          req.RetryEnabled,
		MaxRetries:            req.MaxRetries,
		RetryBackoffMs:        req.RetryBackoffMs,
		MaxCallsPerHour:       req.MaxCallsPerHour,
		ProviderFamily:        req.resolvedFamily(),

		SupportsEmbeddings:           req.SupportsEmbeddings,
		EmbeddingEndpointURL:         req.EmbeddingEndpointURL,
		EmbeddingRequestBodyTemplate: nullableJSONString(req.EmbeddingRequestBodyTemplate),
		EmbeddingResponseVectorPath:  req.EmbeddingResponseVectorPath,

		SupportsGeneration:            req.SupportsGeneration,
		GenerationEndpointURL:         req.GenerationEndpointURL,
		GenerationRequestBodyTemplate: nullableJSONString(req.GenerationRequestBodyTemplate),
		GenerationResponseMediaPath:   req.GenerationResponseMediaPath,

		SupportsWebSearch:            req.SupportsWebSearch,
		WebSearchRequestBodyTemplate: nullableJSONString(req.WebSearchRequestBodyTemplate),
		WebSearchPricePerCall:        req.WebSearchPricePerCall,

		GenerationMode:                req.resolvedGenerationMode(),
		GenerationJobIDPath:           req.GenerationJobIDPath,
		GenerationStatusURLTemplate:   req.GenerationStatusURLTemplate,
		GenerationStatusPath:          req.GenerationStatusPath,
		GenerationContentURLTemplate:  req.GenerationContentURLTemplate,
		GenerationStatusSuccessValues: defaultStatuses(req.GenerationStatusSuccessValues, "completed"),
		GenerationStatusFailureValues: defaultStatuses(req.GenerationStatusFailureValues, "failed"),
		GenerationPollIntervalMs:      defaultInt(req.GenerationPollIntervalMs, 3000),
		GenerationMaxWaitSeconds:      defaultInt(req.GenerationMaxWaitSeconds, 600),

		EmbeddingUsageTokensPath:        req.EmbeddingUsageTokensPath,
		GenerationUsageInputTokensPath:  req.GenerationUsageInputTokensPath,
		GenerationUsageOutputTokensPath: req.GenerationUsageOutputTokensPath,
		GenerationDurationSecondsPath:   req.GenerationDurationSecondsPath,

		LogRequests: resolvedLogRequests(req.LogRequests),
	})
	if errors.Is(err, store.ErrAlreadyExists) {
		httputil.WriteError(w, http.StatusConflict, "provider_already_exists")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "provider_not_found")
		return
	}
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "update_provider", provider.Name, nil)
	rebuildMemState(r.Context(), h.deps.MemState)
	httputil.WriteJSON(w, http.StatusOK, provider)
}

func (h *providerHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.deps.Providers.Delete(r.Context(), id); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "delete_provider", id, nil)
	rebuildMemState(r.Context(), h.deps.MemState)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
