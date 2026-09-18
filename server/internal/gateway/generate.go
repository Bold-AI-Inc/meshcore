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

const errGenerationNotSupported = "generation_not_supported_by_provider"

type generateRequest struct {
	Prompt string `json:"prompt"`
}

func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
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

	access, ok := resolved.GenerationAccess[requestedModel]
	if !ok {
		h.deny(w, r, start, &resolved, "", "", requestedModel, http.StatusForbidden, errModelNotPermitted, nil)
		return
	}
	if access.ModelBlocked || access.ModelStatus == "retired" {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusForbidden, errModelBlocked, nil)
		return
	}
	asyncMode := access.Provider.GenerationMode == "async_poll"
	mediaPathOK := asyncMode || access.Provider.GenerationResponseMediaPath != nil
	if !access.Provider.SupportsGeneration || access.Provider.GenerationEndpointURL == nil ||
		len(access.Provider.GenerationRequestBodyTemplate) == 0 || !mediaPathOK ||
		(asyncMode && !asyncGenerationConfigured(access.Provider)) {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusBadRequest, errGenerationNotSupported, nil)
		return
	}

	if status, code, extra := h.policyDenial(now, resolved.UserID, access); code != "" {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, status, code, extra)
		return
	}

	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusBadRequest, errInvalidRequest, nil)
		return
	}

	var reservation *reqqueue.Reservation
	if h.queue != nil {
		admitCtx, cancel := context.WithTimeout(r.Context(), h.queueAdmitTimeout)
		res, admitted := h.queue.Acquire(admitCtx, reqqueue.CostVideo)
		cancel()
		if !admitted {
			h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusServiceUnavailable, errServerBusy, nil)
			return
		}
		reservation = res
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
	body, err := templating.Render(access.Provider.GenerationRequestBodyTemplate, map[string]string{"model": access.ResolvedModel})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}
	promptJSON, err := json.Marshal(req.Prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}
	body = templating.RenderRaw(body, map[string]json.RawMessage{"prompt_json": promptJSON})

	headers, err := templating.RenderHeaders(access.Provider.HeaderTemplate, authVars)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", nil)
		return
	}

	upstreamBudget := h.generationTimeout
	if asyncMode {
		upstreamBudget = time.Duration(access.Provider.GenerationMaxWaitSeconds) * time.Second
		if upstreamBudget <= 0 {
			upstreamBudget = 10 * time.Minute
		}
	}
	upstreamCtx, cancelUpstream := context.WithTimeout(r.Context(), upstreamBudget)
	defer cancelUpstream()

	httpReq, err := http.NewRequestWithContext(upstreamCtx, access.Provider.HTTPMethod, *access.Provider.GenerationEndpointURL, bytes.NewReader(body))
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
	var outputs []mediaOutput
	var tokensIn, tokensOut int
	var generatedSeconds float64

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		slog.Warn("generation upstream unreachable",
			"request_id", requestID, "provider", access.Provider.ID,
			"model", requestedModel, "err", err)
		writeError(w, http.StatusBadGateway, "upstream_unavailable", nil)
		outcome = "upstream_error"
	} else {
		defer resp.Body.Close()
		statusCode = resp.StatusCode

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, h.maxGenerationResponseBytes))
		var parsed any
		if readErr == nil {
			_ = json.Unmarshal(respBody, &parsed)
		}

		if n, ok := resolveNumberPath(parsed, access.Provider.GenerationUsageInSegs); ok {
			tokensIn = n
		}
		if n, ok := resolveNumberPath(parsed, access.Provider.GenerationUsageOutSegs); ok {
			tokensOut = n
		}

		var genErr error
		if resp.StatusCode < 400 && asyncMode {
			reservation.Release()
			var jobRes jobPollResult
			outputs, jobRes, genErr = h.runGenerationJob(upstreamCtx, access.Provider, headers, parsed, h.maxGenerationResponseBytes)
			generatedSeconds = jobRes.Seconds
			ok = genErr == nil && len(outputs) > 0
		} else {
			outputs, ok = resolveMediaPath(parsed, *access.Provider.GenerationResponseMediaPath)
			if secs, found := resolveNumberish(parsed, access.Provider.GenerationDurationSegs); found {
				generatedSeconds = secs
			}
		}

		if resp.StatusCode >= 400 || !ok || len(outputs) == 0 {
			outcome = "upstream_error"
			slog.Warn("generation upstream error",
				"request_id", requestID, "provider", access.Provider.ID,
				"model", requestedModel, "provider_status", resp.StatusCode,
				"async", asyncMode, "job_err", genErr)
			extra := map[string]any{}
			if resp.StatusCode >= 400 {
				extra["provider_status"] = resp.StatusCode
				extra["provider_message"] = extractProviderErrorMessage(respBody)
			} else if genErr != nil {
				extra["detail"] = genErr.Error()
			}
			if len(extra) == 0 {
				extra = nil
			}
			writeError(w, http.StatusBadGateway, "upstream_unavailable", extra)
		} else {
			mediaType := access.OutputMediaType
			if mediaType == "" {
				mediaType = "application/octet-stream"
			}
			body := map[string]any{
				"id":         requestID,
				"model":      requestedModel,
				"media_type": mediaType,
				"output":     outputs[0],
				"outputs":    outputs,
				"usage": map[string]any{
					"input_tokens":  tokensIn,
					"output_tokens": tokensOut,
				},
			}
			if generatedSeconds > 0 {
				body["usage"].(map[string]any)["seconds"] = generatedSeconds
			}
			writeJSON(w, http.StatusOK, body)
		}
	}

	inputCost := (float64(tokensIn) / 1000.0) * access.InputPricePer1kTokens
	outputCost := (float64(tokensOut) / 1000.0) * access.OutputPricePer1kTokens
	durationCost := generatedSeconds * access.PricePerSecond
	outputCost += durationCost

	h.mem.AddDailySpendUSD(resolved.UserID, access.ModelID, inputCost+outputCost)

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
		ID:               logID,
		MeshKeyID:        &keyID,
		UserID:           &userID,
		ProviderID:       &providerID,
		ModelID:          &modelID,
		RequestedModel:   &reqModel,
		Outcome:          outcome,
		SourceIP:         sourceIP,
		UserAgent:        userAgent,
		StatusCode:       statusCode,
		LatencyMs:        int(time.Since(start).Milliseconds()),
		TokensIn:         tokensIn,
		TokensOut:        tokensOut,
		InputCost:        inputCost,
		OutputCost:       outputCost,
		GeneratedSeconds: generatedSeconds,
	})
}

type mediaOutput struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

func resolveMediaPath(value any, path string) ([]mediaOutput, bool) {
	current, ok := navigatePath(value, strings.Split(path, "."))
	if !ok {
		return nil, false
	}
	switch v := current.(type) {
	case string:
		return []mediaOutput{classifyMedia(v)}, v != ""
	case []any:
		outputs := make([]mediaOutput, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil, false
			}
			outputs = append(outputs, classifyMedia(s))
		}
		return outputs, len(outputs) > 0
	default:
		return nil, false
	}
}

func classifyMedia(s string) mediaOutput {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return mediaOutput{Type: "url", Data: s}
	}
	return mediaOutput{Type: "base64", Data: s}
}
