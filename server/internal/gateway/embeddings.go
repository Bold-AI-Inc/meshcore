package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"mesh-server/internal/httputil"
	"mesh-server/internal/reqqueue"
	"mesh-server/internal/templating"
)

const errEmbeddingsNotSupported = "embeddings_not_supported_by_provider"

type embeddingsRequest struct {
	Input string `json:"input"`
}

func (h *Handler) Embeddings(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestedModel := r.URL.Query().Get("model")
	requestID := newRequestID()
	w.Header().Set("X-Request-Id", requestID)

	r.Body = http.MaxBytesReader(w, r.Body, h.maxRequestBodyBytes)

	resolved, ok := h.authenticate(r)
	if !ok {
		h.deny(w, r, start, nil, "", "", requestedModel, http.StatusUnauthorized, errInvalidOrRevokedKey, nil)
		return
	}

	now := time.Now()
	if code, ok := checkUserStatus(resolved, now); !ok {
		h.deny(w, r, start, &resolved, "", "", requestedModel, http.StatusUnauthorized, code, nil)
		return
	}

	access, ok := resolved.EmbeddingAccess[requestedModel]
	if !ok {
		h.deny(w, r, start, &resolved, "", "", requestedModel, http.StatusForbidden, errModelNotPermitted, nil)
		return
	}
	if access.ModelBlocked || access.ModelStatus == "retired" {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusForbidden, errModelBlocked, nil)
		return
	}
	if !access.Provider.SupportsEmbeddings || access.Provider.EmbeddingEndpointURL == nil ||
		len(access.Provider.EmbeddingRequestBodyTemplate) == 0 || access.Provider.EmbeddingResponseVectorPath == nil {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusBadRequest, errEmbeddingsNotSupported, nil)
		return
	}

	if status, code, extra := h.policyDenial(now, resolved.UserID, access); code != "" {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, status, code, extra)
		return
	}

	var req embeddingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Input == "" {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusBadRequest, errInvalidRequest, nil)
		return
	}

	if h.queue != nil {
		admitCtx, cancel := context.WithTimeout(r.Context(), h.queueAdmitTimeout)
		reservation, admitted := h.queue.Acquire(admitCtx, reqqueue.CostText)
		cancel()
		if !admitted {
			h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusServiceUnavailable, errServerBusy, nil)
			return
		}
		defer reservation.Release()
	}

	if limiter := access.Provider.Limiter; limiter != nil {
		limitCtx, cancelLimit := context.WithTimeout(r.Context(), h.queueAdmitTimeout)
		releaseProvider, err := limiter.Acquire(limitCtx)
		cancelLimit()
		if err != nil {
			h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusServiceUnavailable, errProviderBusy, nil)
			return
		}
		defer releaseProvider()
	}

	h.mem.IncrProviderHourly(access.Provider.ID)
	h.mem.IncrUserModelHourly(resolved.UserID, access.ModelID)

	authVars := map[string]string{"api_key": access.Provider.APIKey, "model": access.ResolvedModel}
	body, err := templating.Render(access.Provider.EmbeddingRequestBodyTemplate, map[string]string{"model": access.ResolvedModel})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}
	inputJSON, err := json.Marshal(req.Input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}
	body = templating.RenderRaw(body, map[string]json.RawMessage{"input_json": inputJSON})

	headers, err := templating.RenderHeaders(access.Provider.HeaderTemplate, authVars)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	upstreamCtx, cancelUpstream := context.WithTimeout(r.Context(), h.embeddingsTimeout)
	defer cancelUpstream()

	httpReq, err := http.NewRequestWithContext(upstreamCtx, access.Provider.HTTPMethod, *access.Provider.EmbeddingEndpointURL, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	statusCode := http.StatusBadGateway
	outcome := "success"
	var vector []float64
	var tokensIn int

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		slog.Warn("embeddings upstream unreachable",
			"request_id", requestID, "provider", access.Provider.ID,
			"model", requestedModel, "err", err)
		writeError(w, http.StatusBadGateway, "upstream_unavailable", nil)
		outcome = "upstream_error"
	} else {
		defer resp.Body.Close()
		statusCode = resp.StatusCode

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 25<<20))
		var parsed any
		if readErr == nil {
			_ = json.Unmarshal(respBody, &parsed)
		}
		if n, ok := resolveNumberPath(parsed, access.Provider.EmbeddingUsageSegs); ok {
			tokensIn = n
		}

		vector, ok = resolveVectorPath(parsed, *access.Provider.EmbeddingResponseVectorPath)
		if resp.StatusCode >= 400 || !ok {
			outcome = "upstream_error"
			slog.Warn("embeddings upstream error",
				"request_id", requestID, "provider", access.Provider.ID,
				"model", requestedModel, "provider_status", resp.StatusCode, "vector_resolved", ok)
			writeError(w, http.StatusBadGateway, "upstream_unavailable", map[string]any{
				"provider_status":  resp.StatusCode,
				"provider_message": extractProviderErrorMessage(respBody),
			})
		} else {
			writeJSON(w, http.StatusOK, map[string]any{
				"id":        requestID,
				"model":     requestedModel,
				"embedding": vector,
				"usage":     map[string]any{"input_tokens": tokensIn},
			})
		}
	}

	inputCost := (float64(tokensIn) / 1000.0) * access.InputPricePer1kTokens
	h.mem.AddDailySpendUSD(resolved.UserID, access.ModelID, inputCost)

	if !h.logsEnabled(access.Provider.ID) {
		return
	}

	logID, err := uuid.NewV7()
	if err != nil {
		return
	}
	var sourceIP *string
	if ip := httputil.ClientIP(r, h.trustProxyHeader); ip != "" {
		sourceIP = &ip
	}
	var userAgent *string
	if ua := r.UserAgent(); ua != "" {
		userAgent = &ua
	}
	keyID, userID, providerID, modelID, reqModel := resolved.KeyID, resolved.UserID, access.Provider.ID, access.ModelID, requestedModel

	h.telemetry.Enqueue(logEvent{
		ID:             logID,
		MeshKeyID:      &keyID,
		UserID:         &userID,
		ProviderID:     &providerID,
		ModelID:        &modelID,
		RequestedModel: &reqModel,
		Outcome:        outcome,
		SourceIP:       sourceIP,
		UserAgent:      userAgent,
		StatusCode:     statusCode,
		LatencyMs:      int(time.Since(start).Milliseconds()),
		TokensIn:       tokensIn,
		InputCost:      inputCost,
	})
}

func resolveVectorPath(value any, path string) ([]float64, bool) {
	current, ok := navigatePath(value, strings.Split(path, "."))
	if !ok {
		return nil, false
	}
	arr, ok := current.([]any)
	if !ok {
		return nil, false
	}
	vector := make([]float64, 0, len(arr))
	for _, v := range arr {
		n, ok := v.(float64)
		if !ok {
			return nil, false
		}
		vector = append(vector, n)
	}
	return vector, true
}
