CREATE TABLE user_model_policies (
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    model_id           UUID NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    daily_cap_usd      NUMERIC(10,2) NOT NULL DEFAULT 20.00,
    allowed_from       TIME,
    allowed_to         TIME,
    timezone           TEXT NOT NULL DEFAULT 'UTC',
    active_days        INT[] NOT NULL DEFAULT '{1,2,3,4,5}',
    always_open        BOOLEAN NOT NULL DEFAULT false,
    max_calls_per_hour INT,
    PRIMARY KEY (user_id, model_id)
);

CREATE INDEX idx_user_model_policies_user ON user_model_policies (user_id);
