package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/sync/errgroup"

	"mesh-server/internal/domain"
	"mesh-server/internal/providerfamily"
	"mesh-server/internal/store"
	"mesh-server/internal/templating"
)

const verifyTimeout = 10 * time.Second

const verifyGenerationTimeout = 90 * time.Second

var testRawJSONValue = json.RawMessage(`"test"`)

func verifyProviderModel(ctx context.Context, endpointURLTemplate, httpMethod string, headerTemplate, bodyTemplate []byte, apiKey, resolvedModel string, family domain.ProviderFamily) error {
	return verifyEndpoint(ctx, endpointURLTemplate, httpMethod, headerTemplate, bodyTemplate, apiKey, resolvedModel, family, "", nil)
}

func verifyEmbeddingModel(ctx context.Context, endpointURLTemplate, httpMethod string, headerTemplate, bodyTemplate []byte, apiKey, resolvedModel string) error {
	return verifyEndpoint(ctx, endpointURLTemplate, httpMethod, headerTemplate, bodyTemplate, apiKey, resolvedModel, "", "input_json", testRawJSONValue)
}

func verifyGenerationModel(ctx context.Context, endpointURLTemplate, httpMethod string, headerTemplate, bodyTemplate []byte, apiKey, resolvedModel string) error {
	ctx = context.WithValue(ctx, verifyTimeoutKey{}, verifyGenerationTimeout)
	return verifyEndpoint(ctx, endpointURLTemplate, httpMethod, headerTemplate, bodyTemplate, apiKey, resolvedModel, "", "prompt_json", testRawJSONValue)
}

type verifyTimeoutKey struct{}

func timeoutFrom(ctx context.Context) time.Duration {
	if d, ok := ctx.Value(verifyTimeoutKey{}).(time.Duration); ok {
		return d
	}
	return verifyTimeout
}

type verifyTarget struct {
	name          string
	kind          domain.ModelKind
	resolvedModel string
}

func verifyModels(ctx context.Context, conn *store.ProviderConnection, apiKey string, targets []verifyTarget) []modelFailure {
	results := make([]modelFailure, len(targets))
	var g errgroup.Group
	g.SetLimit(8)
	for i, t := range targets {
		g.Go(func() error {
			if err := verifyModelByKind(ctx, t.kind, conn, apiKey, t.resolvedModel); err != nil {
				results[i] = modelFailure{Model: t.name, Reason: err.Error()}
			}
			return nil
		})
	}
	_ = g.Wait()

	var failures []modelFailure
	for _, f := range results {
		if f.Reason != "" {
			failures = append(failures, f)
		}
	}
	return failures
}

func verifyModelByKind(ctx context.Context, kind domain.ModelKind, conn *store.ProviderConnection, apiKey, resolvedModel string) error {
	switch kind {
	case domain.ModelKindEmbedding:
		if conn.EmbeddingEndpointURL == nil || len(conn.EmbeddingRequestBodyTemplate) == 0 {
			return errors.New("this provider has no embeddings endpoint/template configured — set that up on the provider before adding an embedding model")
		}
		return verifyEmbeddingModel(ctx, *conn.EmbeddingEndpointURL, conn.HTTPMethod, conn.HeaderTemplate, conn.EmbeddingRequestBodyTemplate, apiKey, resolvedModel)
	case domain.ModelKindImageGeneration, domain.ModelKindVideoGeneration:
		if conn.GenerationEndpointURL == nil || len(conn.GenerationRequestBodyTemplate) == 0 {
			return errors.New("this provider has no generation endpoint/template configured — set that up on the provider before adding a generation model")
		}
		return verifyGenerationModel(ctx, *conn.GenerationEndpointURL, conn.HTTPMethod, conn.HeaderTemplate, conn.GenerationRequestBodyTemplate, apiKey, resolvedModel)
	default:
		return verifyProviderModel(ctx, conn.EndpointURL, conn.HTTPMethod, conn.HeaderTemplate, conn.RequestBodyTemplate, apiKey, resolvedModel, conn.ProviderFamily)
	}
}

func verifyEndpoint(ctx context.Context, endpointURLTemplate, httpMethod string, headerTemplate, bodyTemplate []byte, apiKey, resolvedModel string, family domain.ProviderFamily, rawVarName string, rawVarValue json.RawMessage) error {
	authVars := map[string]string{"api_key": apiKey, "model": resolvedModel}
	bodyVars := map[string]string{"model": resolvedModel, "query": "test"}

	endpointURL := templating.RenderString(endpointURLTemplate, authVars)

	headers, err := templating.RenderHeaders(headerTemplate, authVars)
	if err != nil {
		return fmt.Errorf("invalid header template: %w", err)
	}
	body, err := templating.Render(bodyTemplate, bodyVars)
	if err != nil {
		return fmt.Errorf("invalid request body template: %w", err)
	}

	rawVars := map[string]json.RawMessage{}
	if rawVarName != "" {
		rawVars[rawVarName] = rawVarValue
	}
	contentJSON, err := providerfamily.BuildContentJSON(string(family), []providerfamily.Block{
		{Type: providerfamily.BlockText, Text: "test"},
	})
	if err != nil {
		return fmt.Errorf("invalid content template for family %q: %w", family, err)
	}
	rawVars["content_json"] = contentJSON
	rawVars["max_tokens_json"] = json.RawMessage("1024")
	body = templating.RenderRaw(body, rawVars)

	if parsed, err := url.Parse(endpointURL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("endpoint URL must be an absolute http:// or https:// URL")
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeoutFrom(ctx))
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, httpMethod, endpointURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("no response from provider: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("provider returned %d: %s", resp.StatusCode, string(snippet))
	}
	return nil
}
