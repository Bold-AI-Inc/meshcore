package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"mesh-server/internal/memstate"
	"mesh-server/internal/templating"
)

type jobPollResult struct {
	Seconds float64
	Status  string
}

var (
	errJobNoID       = errors.New("generation job response contained no job id at the configured path")
	errJobFailed     = errors.New("generation job reported failure")
	errJobTimedOut   = errors.New("generation job did not complete within the provider's configured max wait")
	errJobNoContent  = errors.New("generation job completed but the content download returned no data")
	errJobMisconfig  = errors.New("provider is set to async generation but is missing its job id / status / content configuration")
	errJobBadPollURL = errors.New("generation job status or content URL template did not render to a usable URL")
)

func asyncGenerationConfigured(p *memstate.ProviderConfig) bool {
	return len(p.GenerationJobIDSegs) > 0 &&
		p.GenerationStatusURLTemplate != nil && *p.GenerationStatusURLTemplate != "" &&
		len(p.GenerationStatusSegs) > 0 &&
		p.GenerationContentURLTemplate != nil && *p.GenerationContentURLTemplate != ""
}

func (h *Handler) runGenerationJob(
	ctx context.Context,
	p *memstate.ProviderConfig,
	headers map[string]string,
	jobEnvelope any,
	maxBytes int64,
) (outputs []mediaOutput, result jobPollResult, err error) {
	if !asyncGenerationConfigured(p) {
		return nil, result, errJobMisconfig
	}

	jobID, ok := resolveStringOrNumber(jobEnvelope, p.GenerationJobIDSegs)
	if !ok || jobID == "" {
		return nil, result, errJobNoID
	}

	if secs, ok := resolveNumberish(jobEnvelope, p.GenerationDurationSegs); ok {
		result.Seconds = secs
	}

	interval := time.Duration(p.GenerationPollIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 3 * time.Second
	}

	statusURL := templating.RenderString(*p.GenerationStatusURLTemplate, map[string]string{
		"api_key": p.APIKey, "model": "", "job_id": jobID,
	})
	if !looksLikeHTTPURL(statusURL) {
		return nil, result, errJobBadPollURL
	}

	for {
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, result, errJobTimedOut
		}

		status, secs, pollErr := h.pollGenerationJob(ctx, p, headers, statusURL)
		if pollErr != nil {
			if ctx.Err() != nil {
				return nil, result, errJobTimedOut
			}
			continue
		}
		if secs > 0 {
			result.Seconds = secs
		}
		result.Status = status

		if matchesStatus(status, p.GenerationStatusFailure) {
			return nil, result, fmt.Errorf("%w: status %q", errJobFailed, status)
		}
		if matchesStatus(status, p.GenerationStatusSuccess) {
			break
		}
	}

	contentURL := templating.RenderString(*p.GenerationContentURLTemplate, map[string]string{
		"api_key": p.APIKey, "model": "", "job_id": jobID,
	})
	if !looksLikeHTTPURL(contentURL) {
		return nil, result, errJobBadPollURL
	}

	data, err := h.downloadGeneratedContent(ctx, headers, contentURL, maxBytes)
	if err != nil {
		return nil, result, err
	}
	return []mediaOutput{{Type: "base64", Data: data}}, result, nil
}

func (h *Handler) pollGenerationJob(ctx context.Context, p *memstate.ProviderConfig, headers map[string]string, statusURL string) (status string, seconds float64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return "", 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return "", 0, err
	}
	if resp.StatusCode >= 400 {
		return "", 0, fmt.Errorf("status poll returned HTTP %d: %s", resp.StatusCode, extractProviderErrorMessage(body))
	}

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", 0, err
	}
	status, _ = resolveStringOrNumber(parsed, p.GenerationStatusSegs)
	seconds, _ = resolveNumberish(parsed, p.GenerationDurationSegs)
	return status, seconds, nil
}

func (h *Handler) downloadGeneratedContent(ctx context.Context, headers map[string]string, contentURL string, maxBytes int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		return "", fmt.Errorf("content download returned HTTP %d: %s", resp.StatusCode, extractProviderErrorMessage(body))
	}

	limited := io.LimitReader(resp.Body, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if int64(len(raw)) > maxBytes {
		return "", fmt.Errorf("generated media exceeds MAX_GENERATION_RESPONSE_MB (%d bytes)", maxBytes)
	}
	if len(raw) == 0 {
		return "", errJobNoContent
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func matchesStatus(status string, values []string) bool {
	for _, v := range values {
		if equalFold(status, v) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func looksLikeHTTPURL(s string) bool {
	return len(s) > 8 && (s[:7] == "http://" || s[:8] == "https://")
}

func resolveStringOrNumber(value any, segments []string) (string, bool) {
	current, ok := navigatePath(value, segments)
	if !ok {
		return "", false
	}
	switch v := current.(type) {
	case string:
		return v, true
	case float64:
		return fmt.Sprintf("%v", v), true
	default:
		return "", false
	}
}

func resolveNumberish(value any, segments []string) (float64, bool) {
	if len(segments) == 0 {
		return 0, false
	}
	current, ok := navigatePath(value, segments)
	if !ok {
		return 0, false
	}
	switch v := current.(type) {
	case float64:
		return v, true
	case string:
		var f float64
		if _, err := fmt.Sscanf(v, "%g", &f); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}
