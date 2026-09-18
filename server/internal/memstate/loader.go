package memstate

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"mesh-server/internal/security"
)

func (s *Store) load(ctx context.Context) (*Snapshot, *liveKeys, error) {
	snap := &Snapshot{KeyIndex: map[[32]byte]ResolvedKey{}, SilentProviders: map[string]bool{}}

	providers, err := s.loadProviders(ctx, snap)
	if err != nil {
		return nil, nil, err
	}
	models, err := s.loadModels(ctx, providers)
	if err != nil {
		return nil, nil, err
	}
	userIDs, err := s.loadKeys(ctx, snap)
	if err != nil {
		return nil, nil, err
	}
	live := &liveKeys{spend: map[string]bool{}, providers: map[string]bool{}}
	for id := range providers {
		live.providers[id] = true
	}
	if err := s.loadGrants(ctx, snap, models, userIDs, live); err != nil {
		return nil, nil, err
	}
	return snap, live, nil
}

type liveKeys struct {
	spend     map[string]bool
	providers map[string]bool
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}

func splitPathPtr(path *string) []string {
	if path == nil {
		return nil
	}
	return splitPath(*path)
}

func (s *Store) loadProviders(ctx context.Context, snap *Snapshot) (map[string]*ProviderConfig, error) {
	gcm, err := security.NewMasterCipher(s.masterKey)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			p.id, p.name, p.endpoint_url, p.http_method, p.header_template, p.request_body_template,
			p.response_delta_path, p.response_done_signal,
			p.usage_input_tokens_path, p.usage_output_tokens_path,
			p.encrypted_api_key, p.key_nonce,
			p.max_outbound_rps, p.max_concurrent_upstream, p.retry_enabled, p.max_retries,
			p.retry_backoff_ms, p.max_calls_per_hour, p.provider_family,
			p.supports_embeddings, p.embedding_endpoint_url, p.embedding_request_body_template, p.embedding_response_vector_path,
			p.supports_generation, p.generation_endpoint_url, p.generation_request_body_template, p.generation_response_media_path,
			p.supports_web_search, p.web_search_request_body_template, p.web_search_price_per_call,
			p.generation_mode, p.generation_job_id_path, p.generation_status_url_template,
			p.generation_status_path, p.generation_content_url_template,
			p.generation_status_success_values, p.generation_status_failure_values,
			p.generation_poll_interval_ms, p.generation_max_wait_seconds,
			p.embedding_usage_tokens_path, p.generation_usage_input_tokens_path,
			p.generation_usage_output_tokens_path, p.generation_duration_seconds_path,
			p.log_requests
		FROM providers p
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	providers := map[string]*ProviderConfig{}
	for rows.Next() {
		var (
			p                             ProviderConfig
			headerTemplate, bodyTemplate  json.RawMessage
			embeddingRequestBodyTemplate  json.RawMessage
			generationRequestBodyTemplate json.RawMessage
			webSearchRequestBodyTemplate  json.RawMessage
			encryptedAPIKey, keyNonce     []byte
			logRequests                   bool
		)
		if err := rows.Scan(
			&p.ID, &p.Name, &p.EndpointURL, &p.HTTPMethod, &headerTemplate, &bodyTemplate,
			&p.ResponseDeltaPath, &p.ResponseDoneSignal,
			&p.UsageInputTokensPath, &p.UsageOutputTokensPath,
			&encryptedAPIKey, &keyNonce,
			&p.MaxOutboundRPS, &p.MaxConcurrentUpstream, &p.RetryEnabled, &p.MaxRetries,
			&p.RetryBackoffMs, &p.MaxCallsPerHour, &p.ProviderFamily,
			&p.SupportsEmbeddings, &p.EmbeddingEndpointURL, &embeddingRequestBodyTemplate, &p.EmbeddingResponseVectorPath,
			&p.SupportsGeneration, &p.GenerationEndpointURL, &generationRequestBodyTemplate, &p.GenerationResponseMediaPath,
			&p.SupportsWebSearch, &webSearchRequestBodyTemplate, &p.WebSearchPricePerCall,
			&p.GenerationMode, &p.GenerationJobIDPath, &p.GenerationStatusURLTemplate,
			&p.GenerationStatusPath, &p.GenerationContentURLTemplate,
			&p.GenerationStatusSuccess, &p.GenerationStatusFailure,
			&p.GenerationPollIntervalMs, &p.GenerationMaxWaitSeconds,
			&p.EmbeddingUsageTokensPath, &p.GenerationUsageInputTokensPath,
			&p.GenerationUsageOutputTokensPath, &p.GenerationDurationSecondsPath,
			&logRequests,
		); err != nil {
			return nil, err
		}

		apiKey, err := security.DecryptWithCipher(gcm, encryptedAPIKey, keyNonce)
		if err != nil {
			slog.Error("memstate: skipping provider, API key failed to decrypt", "provider", p.ID, "err", err)
			continue
		}
		p.APIKey = apiKey
		p.HeaderTemplate = headerTemplate
		p.RequestBodyTemplate = bodyTemplate
		p.EmbeddingRequestBodyTemplate = embeddingRequestBodyTemplate
		p.GenerationRequestBodyTemplate = generationRequestBodyTemplate
		p.WebSearchRequestBodyTemplate = webSearchRequestBodyTemplate

		p.GenerationJobIDSegs = splitPathPtr(p.GenerationJobIDPath)
		p.GenerationStatusSegs = splitPathPtr(p.GenerationStatusPath)
		p.GenerationDurationSegs = splitPathPtr(p.GenerationDurationSecondsPath)
		p.EmbeddingUsageSegs = splitPathPtr(p.EmbeddingUsageTokensPath)
		p.GenerationUsageInSegs = splitPathPtr(p.GenerationUsageInputTokensPath)
		p.GenerationUsageOutSegs = splitPathPtr(p.GenerationUsageOutputTokensPath)

		p.DeltaPathSegs = splitPath(p.ResponseDeltaPath)
		p.UsageInPathSegs = splitPathPtr(p.UsageInputTokensPath)
		p.UsageOutPathSegs = splitPathPtr(p.UsageOutputTokensPath)

		p.Limiter = s.providerLimiter(p.ID, p.MaxOutboundRPS, p.MaxConcurrentUpstream)

		if !logRequests {
			snap.SilentProviders[p.ID] = true
		}

		s.EnsureProviderCounter(p.ID)

		cfg := p
		providers[p.ID] = &cfg
	}
	return providers, rows.Err()
}

type modelConfig struct {
	access ModelAccess
	kind   string
}

func (s *Store) loadModels(ctx context.Context, providers map[string]*ProviderConfig) (map[string]*modelConfig, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			m.id, m.name, m.resolved_model, m.blocked, m.status, m.kind,
			m.input_price_per_1k_tokens, m.output_price_per_1k_tokens,
			m.supports_image_in, m.supports_document_in, m.supports_audio_in, m.supports_video_in,
			m.supports_web_search, m.price_per_second, m.output_media_type, m.provider_id
		FROM models m
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := map[string]*modelConfig{}
	for rows.Next() {
		var (
			a               ModelAccess
			modelName       string
			kind            string
			outputMediaType *string
			providerID      string
		)
		if err := rows.Scan(
			&a.ModelID, &modelName, &a.ResolvedModel, &a.ModelBlocked, &a.ModelStatus, &kind,
			&a.InputPricePer1kTokens, &a.OutputPricePer1kTokens,
			&a.SupportsImageIn, &a.SupportsDocumentIn, &a.SupportsAudioIn, &a.SupportsVideoIn,
			&a.SupportsWebSearch, &a.PricePerSecond, &outputMediaType, &providerID,
		); err != nil {
			return nil, err
		}
		provider, ok := providers[providerID]
		if !ok {
			continue
		}
		if outputMediaType != nil {
			a.OutputMediaType = *outputMediaType
		}
		a.Provider = provider
		a.ModelName = modelName
		models[a.ModelID] = &modelConfig{access: a, kind: kind}
	}
	return models, rows.Err()
}

func (s *Store) loadKeys(ctx context.Context, snap *Snapshot) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT k.id, k.key_hash, k.expires_at, u.id, u.status, u.expires_at
		FROM mesh_api_keys k
		JOIN users u ON u.id = k.user_id
		WHERE k.status = 'active'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	userIDs := map[string]bool{}
	for rows.Next() {
		var (
			keyID, keyHash              string
			keyExpiresAt, userExpiresAt *time.Time
			userID, userStatus          string
		)
		if err := rows.Scan(&keyID, &keyHash, &keyExpiresAt, &userID, &userStatus, &userExpiresAt); err != nil {
			return nil, err
		}
		rawHash, err := hex.DecodeString(keyHash)
		if err != nil || len(rawHash) != 32 {
			continue
		}
		var hashKey [32]byte
		copy(hashKey[:], rawHash)
		snap.KeyIndex[hashKey] = ResolvedKey{
			KeyID:         keyID,
			UserID:        userID,
			UserStatus:    userStatus,
			UserExpiresAt: userExpiresAt,
			KeyExpiresAt:  keyExpiresAt,
		}
		userIDs[userID] = true
	}
	return userIDs, rows.Err()
}

type userAccess struct {
	chat, embedding, generation map[string]ModelAccess
}

func (s *Store) loadGrants(ctx context.Context, snap *Snapshot, models map[string]*modelConfig, userIDs map[string]bool, live *liveKeys) error {
	rows, err := s.pool.Query(ctx, `
		SELECT
			a.user_id, a.model_id,
			COALESCE(pol.daily_cap_usd, 20.00),
			to_char(pol.allowed_from, 'HH24:MI'), to_char(pol.allowed_to, 'HH24:MI'),
			COALESCE(pol.timezone, 'UTC'), COALESCE(pol.active_days, '{1,2,3,4,5}'),
			COALESCE(pol.always_open, false), pol.max_calls_per_hour
		FROM user_model_access a
		LEFT JOIN user_model_policies pol ON pol.user_id = a.user_id AND pol.model_id = a.model_id
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	byUser := map[string]*userAccess{}
	locations := map[string]*time.Location{}
	for rows.Next() {
		var (
			userID, modelID        string
			policy                 PolicySnapshot
			allowedFrom, allowedTo *string
		)
		if err := rows.Scan(
			&userID, &modelID,
			&policy.DailyCapUSD, &allowedFrom, &allowedTo,
			&policy.Timezone, &policy.ActiveDays, &policy.AlwaysOpen, &policy.MaxCallsPerHour,
		); err != nil {
			return err
		}
		if _, exists := models[modelID]; exists {
			live.spend[spendKey(userID, modelID)] = true
		}
		if !userIDs[userID] {
			continue
		}
		model, ok := models[modelID]
		if !ok {
			continue
		}

		policy.AllowedFrom, policy.AllowedTo = allowedFrom, allowedTo
		loc, cached := locations[policy.Timezone]
		if !cached {
			var err error
			if loc, err = time.LoadLocation(policy.Timezone); err != nil {
				loc = time.UTC
			}
			locations[policy.Timezone] = loc
		}
		policy.Location = loc
		if allowedFrom != nil && allowedTo != nil {
			from, okFrom := parseHHMM(*allowedFrom)
			to, okTo := parseHHMM(*allowedTo)
			if okFrom && okTo {
				policy.FromMinutes, policy.ToMinutes, policy.HasWindow = from, to, true
			}
		}

		access := model.access
		access.Policy = policy

		ua, ok := byUser[userID]
		if !ok {
			ua = &userAccess{
				chat:       map[string]ModelAccess{},
				embedding:  map[string]ModelAccess{},
				generation: map[string]ModelAccess{},
			}
			byUser[userID] = ua
		}
		switch model.kind {
		case "embedding":
			ua.embedding[access.ModelName] = access
		case "image_generation", "video_generation":
			ua.generation[access.ModelName] = access
		default:
			ua.chat[access.ModelName] = access
		}

		s.EnsureSpendCounter(userID, modelID)
		s.EnsureUserModelHourlyCounter(userID, modelID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	empty := &userAccess{
		chat:       map[string]ModelAccess{},
		embedding:  map[string]ModelAccess{},
		generation: map[string]ModelAccess{},
	}
	for hash, resolved := range snap.KeyIndex {
		ua, ok := byUser[resolved.UserID]
		if !ok {
			ua = empty
		}
		resolved.Access = ua.chat
		resolved.EmbeddingAccess = ua.embedding
		resolved.GenerationAccess = ua.generation
		snap.KeyIndex[hash] = resolved
	}
	return nil
}

func parseHHMM(s string) (minutes int, ok bool) {
	hh, mm, found := strings.Cut(s, ":")
	if !found {
		return 0, false
	}
	h, err1 := strconv.Atoi(hh)
	m, err2 := strconv.Atoi(mm)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return h*60 + m, true
}
