package domain

import (
	"encoding/json"
	"time"
)

type ProviderFamily string

const (
	ProviderFamilyGeneric         ProviderFamily = "generic"
	ProviderFamilyAnthropic       ProviderFamily = "anthropic"
	ProviderFamilyOpenAI          ProviderFamily = "openai"
	ProviderFamilyGemini          ProviderFamily = "gemini"
	ProviderFamilyOllama          ProviderFamily = "ollama"
	ProviderFamilyOpenAIResponses ProviderFamily = "openai_responses"
)

type Provider struct {
	ID                    string          `json:"id"`
	Name                  string          `json:"name"`
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
	ProviderFamily        ProviderFamily  `json:"provider_family"`

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

	LogRequests bool `json:"log_requests"`

	CreatedAt time.Time `json:"created_at"`
}

type ModelKind string

const (
	ModelKindChat            ModelKind = "chat"
	ModelKindEmbedding       ModelKind = "embedding"
	ModelKindImageGeneration ModelKind = "image_generation"
	ModelKindVideoGeneration ModelKind = "video_generation"
)

type Model struct {
	ID                     string    `json:"id"`
	ProviderID             string    `json:"provider_id"`
	ProviderName           string    `json:"provider_name"`
	Name                   string    `json:"name"`
	ResolvedModel          string    `json:"resolved_model"`
	Status                 string    `json:"status"`
	Blocked                bool      `json:"blocked"`
	InputPricePer1kTokens  float64   `json:"input_price_per_1k_tokens"`
	OutputPricePer1kTokens float64   `json:"output_price_per_1k_tokens"`
	Kind                   ModelKind `json:"kind"`
	SupportsImageIn        bool      `json:"supports_image_in"`
	SupportsDocumentIn     bool      `json:"supports_document_in"`
	SupportsAudioIn        bool      `json:"supports_audio_in"`
	SupportsVideoIn        bool      `json:"supports_video_in"`
	SupportsWebSearch      bool      `json:"supports_web_search"`
	PricePerSecond         float64   `json:"price_per_second"`
	OutputMediaType        *string   `json:"output_media_type"`
	CreatedAt              time.Time `json:"created_at"`
}

type AccessGrant struct {
	ModelID      string `json:"model_id"`
	ModelName    string `json:"model_name"`
	ProviderName string `json:"provider_name"`
}
