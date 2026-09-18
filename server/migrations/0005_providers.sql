CREATE TABLE providers (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                    TEXT UNIQUE NOT NULL,
    encrypted_api_key       BYTEA NOT NULL,
    key_nonce               BYTEA NOT NULL,
    endpoint_url            TEXT NOT NULL,
    http_method             TEXT NOT NULL DEFAULT 'POST',
    header_template         JSONB NOT NULL,
    request_body_template   JSONB NOT NULL,
    response_delta_path     TEXT NOT NULL,
    response_done_signal    TEXT,
    max_outbound_rps        INT NOT NULL DEFAULT 10,
    max_concurrent_upstream INT NOT NULL DEFAULT 50,
    retry_enabled           BOOLEAN NOT NULL DEFAULT true,
    max_retries             INT NOT NULL DEFAULT 2,
    retry_backoff_ms        INT NOT NULL DEFAULT 100,
    max_calls_per_hour      INT,
    log_requests            BOOLEAN NOT NULL DEFAULT true,

    -- selects the built-in content-block shape the gateway renders multimodal
    -- blocks into; 'generic' means text-only, {{query}} substitution only.
    provider_family TEXT NOT NULL DEFAULT 'generic'
        CHECK (provider_family IN ('generic', 'anthropic', 'openai', 'openai_responses', 'gemini', 'ollama')),

    -- dot-paths into the provider's SSE stream for token usage capture.
    -- NULL means the provider doesn't report it, which bills as 0.
    usage_input_tokens_path  TEXT,
    usage_output_tokens_path TEXT,

    -- embeddings: own endpoint/body/response shape, sharing the provider's
    -- auth/header template. All nullable; populated only when supports_embeddings.
    supports_embeddings             BOOLEAN NOT NULL DEFAULT false,
    embedding_endpoint_url          TEXT,
    embedding_request_body_template JSONB,
    embedding_response_vector_path  TEXT,
    embedding_usage_tokens_path     TEXT,

    -- image/video generation: own endpoint/body/response shape. generation_mode
    -- distinguishes a single-POST provider ('sync', e.g. image endpoints) from a
    -- job-queue provider ('async_poll', e.g. Sora): POST starts a job, poll the
    -- status URL until terminal, then fetch content. The async_poll columns are
    -- only read when generation_mode = 'async_poll'; their templates take
    -- {{job_id}} substitution.
    supports_generation              BOOLEAN NOT NULL DEFAULT false,
    generation_endpoint_url          TEXT,
    generation_request_body_template JSONB,
    generation_response_media_path   TEXT,
    generation_mode TEXT NOT NULL DEFAULT 'sync'
        CHECK (generation_mode IN ('sync', 'async_poll')),
    generation_job_id_path            TEXT,
    generation_status_url_template    TEXT,
    generation_status_path            TEXT,
    generation_content_url_template   TEXT,
    generation_poll_interval_ms       INT NOT NULL DEFAULT 3000,
    generation_max_wait_seconds       INT NOT NULL DEFAULT 600,
    generation_status_success_values  TEXT[] NOT NULL DEFAULT '{completed}',
    generation_status_failure_values  TEXT[] NOT NULL DEFAULT '{failed}',
    generation_usage_input_tokens_path  TEXT,
    generation_usage_output_tokens_path TEXT,
    generation_duration_seconds_path    TEXT,

    -- provider-native web search ("grounding"); billed per search, separate
    -- from tokens. 0 means don't bill for searches (folded into token price).
    supports_web_search              BOOLEAN NOT NULL DEFAULT false,
    web_search_request_body_template JSONB,
    web_search_price_per_call         NUMERIC(16, 9) NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
