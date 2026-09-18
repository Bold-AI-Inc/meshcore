CREATE TABLE admin_audit_log (
    id          BIGSERIAL PRIMARY KEY,
    admin_email TEXT NOT NULL,
    action      TEXT NOT NULL,
    target      TEXT NOT NULL,
    detail      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
