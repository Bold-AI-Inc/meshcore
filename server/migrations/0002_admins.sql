CREATE TABLE admins (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email                 TEXT UNIQUE NOT NULL,
    password_hash         TEXT NOT NULL,
    status                TEXT NOT NULL DEFAULT 'active',
    created_by            TEXT NOT NULL,
    failed_login_attempts INT NOT NULL DEFAULT 0,
    locked_until          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
