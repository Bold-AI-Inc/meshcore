CREATE TABLE request_logs (
    id             UUID PRIMARY KEY,

    -- nullable: a denial can occur before a key resolves to a user at all
    -- (invalid/unknown bearer token). Populated for every real, post-auth call.
    mesh_key_id    UUID,
    user_id        UUID,
    provider_id    UUID,
    model_id       UUID,

    source_ip      INET,
    user_agent     TEXT,
    browser        TEXT,
    browser_version TEXT,
    os             TEXT,
    os_version     TEXT,
    device_type    TEXT,

    status_code    INT,
    latency_ms     INT,
    tokens_in      INT,
    tokens_out     INT,
    input_cost     NUMERIC(16,9) NOT NULL DEFAULT 0,
    output_cost    NUMERIC(16,9) NOT NULL DEFAULT 0,
    web_searches   INT NOT NULL DEFAULT 0,
    generated_seconds NUMERIC(10,2) NOT NULL DEFAULT 0,

    -- 'success' reached the provider with an expected status; 'upstream_error'
    -- reached the provider but it/the connection failed; 'denied' was rejected
    -- before contacting a provider (bad key, policy/rate/budget limit, etc).
    -- deny_reason mirrors the error code from gateway/errors.go and is only
    -- ever set alongside outcome = 'denied'.
    outcome        TEXT NOT NULL DEFAULT 'success'
        CHECK (outcome IN ('success', 'upstream_error', 'denied')),
    deny_reason    TEXT,

    -- requested model name, captured verbatim even when it never resolves to
    -- a real model_id (e.g. a name the caller has no grant for at all).
    requested_model TEXT,

    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_logs_provider_created     ON request_logs (provider_id, created_at DESC);
CREATE INDEX idx_logs_user_created         ON request_logs (user_id, created_at DESC);
CREATE INDEX idx_logs_key_created          ON request_logs (mesh_key_id, created_at DESC);
CREATE INDEX idx_logs_outcome_created      ON request_logs (outcome, created_at DESC);
CREATE INDEX idx_logs_user_model_created   ON request_logs (user_id, model_id, created_at DESC);

-- drives the retention job (cmd/server), which deletes rows past
-- LOG_RETENTION_DAYS in bounded chunks.
CREATE INDEX idx_logs_created ON request_logs (created_at);

-- partial: "which calls actually used search" is the first question asked of
-- this column, and the overwhelming majority of rows never searched at all.
CREATE INDEX idx_logs_web_searches ON request_logs (created_at DESC)
    WHERE web_searches > 0;
