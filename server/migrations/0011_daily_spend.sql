CREATE TABLE daily_spend (
    user_id        UUID NOT NULL REFERENCES users(id),
    model_id       UUID NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    date           DATE NOT NULL,
    total_cost_usd NUMERIC(20,9) NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, model_id, date)
);
