-- Per-(user, model) hourly call cap tracking, keyed like provider_hourly_usage
-- but by (user_id, model_id) instead of provider_id.
CREATE TABLE user_model_hourly_usage (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    model_id   UUID NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    hour_start TIMESTAMPTZ NOT NULL,
    call_count BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, model_id, hour_start)
);
