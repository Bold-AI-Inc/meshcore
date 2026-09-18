CREATE TABLE user_model_access (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    model_id   UUID NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, model_id)
);

CREATE INDEX idx_user_model_access_user ON user_model_access (user_id);
