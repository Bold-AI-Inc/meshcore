CREATE TABLE mesh_api_keys (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id),
    key_hash   TEXT UNIQUE NOT NULL,
    key_prefix TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_one_active_key_per_user
    ON mesh_api_keys (user_id) WHERE status = 'active';
