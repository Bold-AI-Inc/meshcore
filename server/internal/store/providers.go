package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mesh-server/internal/domain"
)

type ProviderStore struct {
	pool *pgxpool.Pool
}

func NewProviderStore(pool *pgxpool.Pool) *ProviderStore {
	return &ProviderStore{pool: pool}
}

type CreateProviderParams struct {
	Name                  string
	EncryptedAPIKey       []byte
	KeyNonce              []byte
	EndpointURL           string
	HTTPMethod            string
	HeaderTemplate        string
	RequestBodyTemplate   string
	ResponseDeltaPath     string
	ResponseDoneSignal    *string
	UsageInputTokensPath  *string
	UsageOutputTokensPath *string
	MaxOutboundRPS        int
	MaxConcurrentUpstream int
	RetryEnabled          bool
	MaxRetries            int
	RetryBackoffMs        int
	MaxCallsPerHour       *int
	ProviderFamily        domain.ProviderFamily

	SupportsEmbeddings           bool
	EmbeddingEndpointURL         *string
	EmbeddingRequestBodyTemplate *string
	EmbeddingResponseVectorPath  *string

	SupportsGeneration            bool
	GenerationEndpointURL         *string
	GenerationRequestBodyTemplate *string
	GenerationResponseMediaPath   *string

	SupportsWebSearch            bool
	WebSearchRequestBodyTemplate *string
	WebSearchPricePerCall        float64

	GenerationMode                string
	GenerationJobIDPath           *string
	GenerationStatusURLTemplate   *string
	GenerationStatusPath          *string
	GenerationContentURLTemplate  *string
	GenerationStatusSuccessValues []string
	GenerationStatusFailureValues []string
	GenerationPollIntervalMs      int
	GenerationMaxWaitSeconds      int

	EmbeddingUsageTokensPath        *string
	GenerationUsageInputTokensPath  *string
	GenerationUsageOutputTokensPath *string
	GenerationDurationSecondsPath   *string

	LogRequests bool
}

const providerColumns = `id, name, endpoint_url, http_method, header_template, request_body_template,
	response_delta_path, response_done_signal, usage_input_tokens_path, usage_output_tokens_path,
	max_outbound_rps, max_concurrent_upstream,
	retry_enabled, max_retries, retry_backoff_ms, max_calls_per_hour, provider_family,
	supports_embeddings, embedding_endpoint_url, embedding_request_body_template, embedding_response_vector_path,
	supports_generation, generation_endpoint_url, generation_request_body_template, generation_response_media_path,
	supports_web_search, web_search_request_body_template, web_search_price_per_call,
	generation_mode, generation_job_id_path, generation_status_url_template,
	generation_status_path, generation_content_url_template,
	generation_status_success_values, generation_status_failure_values,
	generation_poll_interval_ms, generation_max_wait_seconds,
	embedding_usage_tokens_path, generation_usage_input_tokens_path,
	generation_usage_output_tokens_path, generation_duration_seconds_path,
	log_requests,
	created_at`

func scanProvider(row interface {
	Scan(dest ...any) error
}) (*domain.Provider, error) {
	p := &domain.Provider{}
	err := row.Scan(&p.ID, &p.Name, &p.EndpointURL, &p.HTTPMethod, &p.HeaderTemplate,
		&p.RequestBodyTemplate, &p.ResponseDeltaPath, &p.ResponseDoneSignal,
		&p.UsageInputTokensPath, &p.UsageOutputTokensPath, &p.MaxOutboundRPS,
		&p.MaxConcurrentUpstream, &p.RetryEnabled, &p.MaxRetries, &p.RetryBackoffMs,
		&p.MaxCallsPerHour, &p.ProviderFamily,
		&p.SupportsEmbeddings, &p.EmbeddingEndpointURL, &p.EmbeddingRequestBodyTemplate, &p.EmbeddingResponseVectorPath,
		&p.SupportsGeneration, &p.GenerationEndpointURL, &p.GenerationRequestBodyTemplate, &p.GenerationResponseMediaPath,
		&p.SupportsWebSearch, &p.WebSearchRequestBodyTemplate, &p.WebSearchPricePerCall,
		&p.GenerationMode, &p.GenerationJobIDPath, &p.GenerationStatusURLTemplate,
		&p.GenerationStatusPath, &p.GenerationContentURLTemplate,
		&p.GenerationStatusSuccessValues, &p.GenerationStatusFailureValues,
		&p.GenerationPollIntervalMs, &p.GenerationMaxWaitSeconds,
		&p.EmbeddingUsageTokensPath, &p.GenerationUsageInputTokensPath,
		&p.GenerationUsageOutputTokensPath, &p.GenerationDurationSecondsPath,
		&p.LogRequests,
		&p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

type ModelInput struct {
	Name                   string
	ResolvedModel          string
	InputPricePer1kTokens  float64
	OutputPricePer1kTokens float64
	SupportsImageIn        bool
	SupportsDocumentIn     bool
	SupportsAudioIn        bool
	SupportsVideoIn        bool
	SupportsWebSearch      bool
	PricePerSecond         float64
	Kind                   domain.ModelKind
	OutputMediaType        *string
}

func (s *ProviderStore) CreateWithModels(ctx context.Context, params CreateProviderParams, modelInputs []ModelInput) (*domain.Provider, []domain.Model, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx,
		`INSERT INTO providers (name, encrypted_api_key, key_nonce, endpoint_url, http_method,
			header_template, request_body_template, response_delta_path, response_done_signal,
			usage_input_tokens_path, usage_output_tokens_path,
			max_outbound_rps, max_concurrent_upstream, retry_enabled, max_retries, retry_backoff_ms,
			max_calls_per_hour, provider_family,
			supports_embeddings, embedding_endpoint_url, embedding_request_body_template, embedding_response_vector_path,
			supports_generation, generation_endpoint_url, generation_request_body_template, generation_response_media_path,
			supports_web_search, web_search_request_body_template, web_search_price_per_call,
			generation_mode, generation_job_id_path, generation_status_url_template,
			generation_status_path, generation_content_url_template,
			generation_status_success_values, generation_status_failure_values,
			generation_poll_interval_ms, generation_max_wait_seconds,
			embedding_usage_tokens_path, generation_usage_input_tokens_path,
			generation_usage_output_tokens_path, generation_duration_seconds_path,
			log_requests)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41,$42,$43)
		 RETURNING `+providerColumns,
		params.Name, params.EncryptedAPIKey, params.KeyNonce, params.EndpointURL, params.HTTPMethod,
		params.HeaderTemplate, params.RequestBodyTemplate, params.ResponseDeltaPath,
		params.ResponseDoneSignal, params.UsageInputTokensPath, params.UsageOutputTokensPath,
		params.MaxOutboundRPS, params.MaxConcurrentUpstream,
		params.RetryEnabled, params.MaxRetries, params.RetryBackoffMs, params.MaxCallsPerHour, params.ProviderFamily,
		params.SupportsEmbeddings, params.EmbeddingEndpointURL, params.EmbeddingRequestBodyTemplate, params.EmbeddingResponseVectorPath,
		params.SupportsGeneration, params.GenerationEndpointURL, params.GenerationRequestBodyTemplate, params.GenerationResponseMediaPath,
		params.SupportsWebSearch, params.WebSearchRequestBodyTemplate, params.WebSearchPricePerCall,
		params.GenerationMode, params.GenerationJobIDPath, params.GenerationStatusURLTemplate,
		params.GenerationStatusPath, params.GenerationContentURLTemplate,
		params.GenerationStatusSuccessValues, params.GenerationStatusFailureValues,
		params.GenerationPollIntervalMs, params.GenerationMaxWaitSeconds,
		params.EmbeddingUsageTokensPath, params.GenerationUsageInputTokensPath,
		params.GenerationUsageOutputTokensPath, params.GenerationDurationSecondsPath,
		params.LogRequests,
	)

	provider, err := scanProvider(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, nil, ErrAlreadyExists
		}
		return nil, nil, err
	}

	models := make([]domain.Model, 0, len(modelInputs))
	for _, mi := range modelInputs {
		kind := mi.Kind
		if kind == "" {
			kind = domain.ModelKindChat
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO models (provider_id, name, resolved_model, input_price_per_1k_tokens, output_price_per_1k_tokens,
				supports_image_in, supports_document_in, supports_audio_in, supports_video_in, supports_web_search,
				kind, output_media_type, price_per_second)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			 RETURNING id, provider_id, name, resolved_model, status, blocked, input_price_per_1k_tokens, output_price_per_1k_tokens,
				kind, supports_image_in, supports_document_in, supports_audio_in, supports_video_in, supports_web_search,
				price_per_second, output_media_type, created_at`,
			provider.ID, mi.Name, mi.ResolvedModel, mi.InputPricePer1kTokens, mi.OutputPricePer1kTokens,
			mi.SupportsImageIn, mi.SupportsDocumentIn, mi.SupportsAudioIn, mi.SupportsVideoIn, mi.SupportsWebSearch, kind, mi.OutputMediaType, mi.PricePerSecond,
		)
		var m domain.Model
		err := row.Scan(&m.ID, &m.ProviderID, &m.Name, &m.ResolvedModel, &m.Status, &m.Blocked,
			&m.InputPricePer1kTokens, &m.OutputPricePer1kTokens,
			&m.Kind, &m.SupportsImageIn, &m.SupportsDocumentIn, &m.SupportsAudioIn, &m.SupportsVideoIn, &m.SupportsWebSearch,
			&m.PricePerSecond, &m.OutputMediaType, &m.CreatedAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return nil, nil, ErrAlreadyExists
			}
			return nil, nil, err
		}
		m.ProviderName = provider.Name
		models = append(models, m)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return provider, models, nil
}

type ProviderConnection struct {
	EndpointURL         string
	HTTPMethod          string
	HeaderTemplate      []byte
	RequestBodyTemplate []byte
	EncryptedAPIKey     []byte
	KeyNonce            []byte
	ProviderFamily      domain.ProviderFamily

	EmbeddingEndpointURL          *string
	EmbeddingRequestBodyTemplate  []byte
	GenerationEndpointURL         *string
	GenerationRequestBodyTemplate []byte
	WebSearchRequestBodyTemplate  []byte
}

func (s *ProviderStore) GetConnection(ctx context.Context, id string) (*ProviderConnection, error) {
	c := &ProviderConnection{}
	err := s.pool.QueryRow(ctx,
		`SELECT endpoint_url, http_method, header_template, request_body_template, encrypted_api_key, key_nonce,
			provider_family,
			embedding_endpoint_url, embedding_request_body_template,
			generation_endpoint_url, generation_request_body_template,
			web_search_request_body_template
		 FROM providers WHERE id = $1`,
		id,
	).Scan(&c.EndpointURL, &c.HTTPMethod, &c.HeaderTemplate, &c.RequestBodyTemplate, &c.EncryptedAPIKey, &c.KeyNonce,
		&c.ProviderFamily,
		&c.EmbeddingEndpointURL, &c.EmbeddingRequestBodyTemplate,
		&c.GenerationEndpointURL, &c.GenerationRequestBodyTemplate,
		&c.WebSearchRequestBodyTemplate)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *ProviderStore) Update(ctx context.Context, id string, params CreateProviderParams) (*domain.Provider, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE providers SET name=$2, encrypted_api_key=$3, key_nonce=$4, endpoint_url=$5, http_method=$6,
			header_template=$7, request_body_template=$8, response_delta_path=$9, response_done_signal=$10,
			usage_input_tokens_path=$11, usage_output_tokens_path=$12,
			max_outbound_rps=$13, max_concurrent_upstream=$14, retry_enabled=$15, max_retries=$16,
			retry_backoff_ms=$17, max_calls_per_hour=$18, provider_family=$19,
			supports_embeddings=$20, embedding_endpoint_url=$21, embedding_request_body_template=$22, embedding_response_vector_path=$23,
			supports_generation=$24, generation_endpoint_url=$25, generation_request_body_template=$26, generation_response_media_path=$27,
			supports_web_search=$29, web_search_request_body_template=$30, web_search_price_per_call=$31,
			generation_mode=$32, generation_job_id_path=$33, generation_status_url_template=$34,
			generation_status_path=$35, generation_content_url_template=$36,
			generation_status_success_values=$37, generation_status_failure_values=$38,
			generation_poll_interval_ms=$39, generation_max_wait_seconds=$40,
			embedding_usage_tokens_path=$41, generation_usage_input_tokens_path=$42,
			generation_usage_output_tokens_path=$43, generation_duration_seconds_path=$44,
			log_requests=$28
		 WHERE id=$1
		 RETURNING `+providerColumns,
		id, params.Name, params.EncryptedAPIKey, params.KeyNonce, params.EndpointURL, params.HTTPMethod,
		params.HeaderTemplate, params.RequestBodyTemplate, params.ResponseDeltaPath,
		params.ResponseDoneSignal, params.UsageInputTokensPath, params.UsageOutputTokensPath,
		params.MaxOutboundRPS, params.MaxConcurrentUpstream,
		params.RetryEnabled, params.MaxRetries, params.RetryBackoffMs, params.MaxCallsPerHour, params.ProviderFamily,
		params.SupportsEmbeddings, params.EmbeddingEndpointURL, params.EmbeddingRequestBodyTemplate, params.EmbeddingResponseVectorPath,
		params.SupportsGeneration, params.GenerationEndpointURL, params.GenerationRequestBodyTemplate, params.GenerationResponseMediaPath,
		params.LogRequests,
		params.SupportsWebSearch, params.WebSearchRequestBodyTemplate, params.WebSearchPricePerCall,
		params.GenerationMode, params.GenerationJobIDPath, params.GenerationStatusURLTemplate,
		params.GenerationStatusPath, params.GenerationContentURLTemplate,
		params.GenerationStatusSuccessValues, params.GenerationStatusFailureValues,
		params.GenerationPollIntervalMs, params.GenerationMaxWaitSeconds,
		params.EmbeddingUsageTokensPath, params.GenerationUsageInputTokensPath,
		params.GenerationUsageOutputTokensPath, params.GenerationDurationSecondsPath,
	)

	provider, err := scanProvider(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAlreadyExists
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return provider, nil
}

func (s *ProviderStore) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM providers WHERE id = $1`, id)
	return err
}

func (s *ProviderStore) List(ctx context.Context) ([]domain.Provider, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+providerColumns+` FROM providers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	providers := []domain.Provider{}
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, *p)
	}
	return providers, rows.Err()
}
