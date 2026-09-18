CREATE TABLE provider_hourly_usage (
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    hour_start  TIMESTAMPTZ NOT NULL,
    call_count  INT NOT NULL DEFAULT 0,
    PRIMARY KEY (provider_id, hour_start)
);
