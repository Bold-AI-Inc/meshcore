package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mesh-server/internal/memstate"
	"mesh-server/internal/providerfamily"
	"mesh-server/internal/templating"
)

func buildRequest(ctx context.Context, p *memstate.ProviderConfig, bodyTemplate []byte, resolvedModel string, blocks []providerfamily.Block, maxTokens *int) (*http.Request, error) {
	contentJSON, err := providerfamily.BuildContentJSON(p.ProviderFamily, blocks)
	if err != nil {
		return nil, err
	}
	authVars := map[string]string{
		"api_key": p.APIKey,
		"model":   resolvedModel,
	}
	bodyVars := map[string]string{
		"model": resolvedModel,
		"query": providerfamily.PlainText(blocks),
	}

	endpointURL := templating.RenderString(p.EndpointURL, authVars)

	headers, err := templating.RenderHeaders(p.HeaderTemplate, authVars)
	if err != nil {
		return nil, err
	}
	body, err := templating.Render(bodyTemplate, bodyVars)
	if err != nil {
		return nil, err
	}

	resolvedMaxTokens := defaultMaxTokens
	if maxTokens != nil {
		resolvedMaxTokens = *maxTokens
	}
	maxTokensJSON, err := json.Marshal(resolvedMaxTokens)
	if err != nil {
		return nil, err
	}

	body = templating.RenderRaw(body, map[string]json.RawMessage{"content_json": contentJSON, "max_tokens_json": maxTokensJSON})

	req, err := http.NewRequestWithContext(ctx, p.HTTPMethod, endpointURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

type buildRequestError struct{ err error }

func (e *buildRequestError) Error() string { return e.err.Error() }
func (e *buildRequestError) Unwrap() error { return e.err }

func doUpstreamRequestWithRetry(ctx context.Context, client *http.Client, p *memstate.ProviderConfig, bodyTemplate []byte, resolvedModel string, blocks []providerfamily.Block, maxTokens *int) (*http.Response, error) {
	maxAttempts := 1
	if p.RetryEnabled && p.MaxRetries > 0 {
		maxAttempts = p.MaxRetries + 1
	}
	backoff := time.Duration(p.RetryBackoffMs) * time.Millisecond
	if backoff <= 0 {
		backoff = 100 * time.Millisecond
	}

	var resp *http.Response
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			backoff *= 2
		}

		req, buildErr := buildRequest(ctx, p, bodyTemplate, resolvedModel, blocks, maxTokens)
		if buildErr != nil {
			return nil, &buildRequestError{buildErr}
		}
		resp, err = client.Do(req)
		if err == nil {
			return resp, nil
		}
	}
	return nil, err
}

type streamSpec struct {
	deltaPathSegs []string
	doneSignal    *string
	usageInSegs   []string
	usageOutSegs  []string

	searchFamily string
}

type streamCallbacks struct {
	onDelta    func(text string)
	onUsageIn  func(tokens int)
	onUsageOut func(tokens int)
	onSearch   func(providerfamily.SearchEvent)
}

func streamDeltas(body *bufio.Reader, spec streamSpec, cb streamCallbacks) error {
	for {
		line, err := body.ReadBytes('\n')
		trimmedLine := bytes.TrimSpace(line)

		if payload, ok := bytes.CutPrefix(trimmedLine, dataPrefix); ok {
			payload = bytes.TrimSpace(payload)
			switch {
			case len(payload) == 0:
			case bytes.Equal(payload, doneSentinel):
				return nil
			case spec.doneSignal != nil && string(payload) == *spec.doneSignal:
				return nil
			default:
				var parsed any
				if jsonErr := json.Unmarshal(payload, &parsed); jsonErr == nil {
					if delta, ok := resolveDeltaPath(parsed, spec.deltaPathSegs); ok && delta != "" {
						cb.onDelta(delta)
					}
					if spec.usageInSegs != nil {
						if n, ok := resolveNumberPath(parsed, spec.usageInSegs); ok {
							cb.onUsageIn(n)
						}
					}
					if spec.usageOutSegs != nil {
						if n, ok := resolveNumberPath(parsed, spec.usageOutSegs); ok {
							cb.onUsageOut(n)
						}
					}
					if spec.searchFamily != "" {
						if ev := providerfamily.ExtractSearchEvent(spec.searchFamily, parsed); !ev.Empty() {
							cb.onSearch(ev)
						}
					}
				}
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

var (
	dataPrefix   = []byte("data:")
	doneSentinel = []byte("[DONE]")
)

func navigatePath(value any, segments []string) (any, bool) {
	if len(segments) == 0 {
		return nil, false
	}
	current := value
	for _, seg := range segments {
		if idx, err := strconv.Atoi(seg); err == nil {
			arr, ok := current.([]any)
			if !ok || idx < 0 || idx >= len(arr) {
				return nil, false
			}
			current = arr[idx]
			continue
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = obj[seg]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func resolveDeltaPath(value any, segments []string) (string, bool) {
	current, ok := navigatePath(value, segments)
	if !ok {
		return "", false
	}
	text, ok := current.(string)
	return text, ok
}

func resolveNumberPath(value any, segments []string) (int, bool) {
	current, ok := navigatePath(value, segments)
	if !ok {
		return 0, false
	}
	n, ok := current.(float64)
	return int(n), ok
}

var errorMessagePath = []string{"error", "message"}

func extractProviderErrorMessage(body []byte) string {
	var parsed any
	if err := json.Unmarshal(body, &parsed); err == nil {
		if msg, ok := resolveDeltaPath(parsed, errorMessagePath); ok && msg != "" {
			return msg
		}
	}
	return strings.TrimSpace(string(body))
}
