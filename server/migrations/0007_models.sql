CREATE TABLE models (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id               UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    name                      TEXT NOT NULL,
    resolved_model            TEXT NOT NULL,
    status                    TEXT NOT NULL DEFAULT 'active',
    blocked                   BOOLEAN NOT NULL DEFAULT false,
    input_price_per_1k_tokens  NUMERIC(16, 9) NOT NULL,
    output_price_per_1k_tokens NUMERIC(16, 9) NOT NULL,
    price_per_second           NUMERIC(16, 9) NOT NULL DEFAULT 0,

    -- distinguishes a chat model from an embedding/generation model under the
    -- same provider; only 'chat' is resolvable via /v1/proxy.
    kind TEXT NOT NULL DEFAULT 'chat'
        CHECK (kind IN ('chat', 'embedding', 'image_generation', 'video_generation')),

    -- per-model content-type and web-search capability flags: a request
    -- containing a block the model doesn't support is rejected before any
    -- upstream call is attempted.
    supports_image_in    BOOLEAN NOT NULL DEFAULT false,
    supports_document_in BOOLEAN NOT NULL DEFAULT false,
    supports_audio_in    BOOLEAN NOT NULL DEFAULT false,
    supports_video_in    BOOLEAN NOT NULL DEFAULT false,
    supports_web_search  BOOLEAN NOT NULL DEFAULT false,

    -- fixed MIME type this model's generation endpoint produces; only
    -- meaningful for kind IN ('image_generation', 'video_generation').
    output_media_type TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider_id, name)
);

-- The gateway resolves a caller's "model" query parameter by name alone, so
-- two providers offering the same name must not be allowed to shadow each
-- other. If this fails, the error names an existing duplicate to rename.
CREATE UNIQUE INDEX idx_models_name_unique ON models (name);
