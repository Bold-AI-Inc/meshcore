package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mesh-server/internal/domain"
)

type PolicyStore struct {
	pool *pgxpool.Pool
}

func NewPolicyStore(pool *pgxpool.Pool) *PolicyStore {
	return &PolicyStore{pool: pool}
}

func (s *PolicyStore) ListForUser(ctx context.Context, userID string) ([]domain.ModelPolicy, error) {
	today := time.Now().Format("2006-01-02")
	rows, err := s.pool.Query(ctx,
		`SELECT a.model_id,
		        COALESCE(p.daily_cap_usd, 20.00),
		        to_char(p.allowed_from, 'HH24:MI'),
		        to_char(p.allowed_to, 'HH24:MI'),
		        COALESCE(p.timezone, 'UTC'),
		        COALESCE(p.active_days, '{1,2,3,4,5}'),
		        COALESCE(p.always_open, false),
		        p.max_calls_per_hour,
		        COALESCE(d.total_cost_usd, 0)
		 FROM user_model_access a
		 LEFT JOIN user_model_policies p ON p.user_id = a.user_id AND p.model_id = a.model_id
		 LEFT JOIN daily_spend d ON d.user_id = a.user_id AND d.model_id = a.model_id AND d.date = $2::date
		 WHERE a.user_id = $1`,
		userID, today,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	policies := []domain.ModelPolicy{}
	for rows.Next() {
		p := domain.ModelPolicy{UserID: userID}
		var allowedFrom, allowedTo *string
		if err := rows.Scan(&p.ModelID, &p.DailyCapUSD, &allowedFrom, &allowedTo,
			&p.Timezone, &p.ActiveDays, &p.AlwaysOpen, &p.MaxCallsPerHour, &p.SpentTodayUSD); err != nil {
			return nil, err
		}
		p.AllowedFrom = allowedFrom
		p.AllowedTo = allowedTo
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *PolicyStore) Upsert(ctx context.Context, p domain.ModelPolicy) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_model_policies (user_id, model_id, daily_cap_usd, allowed_from, allowed_to, timezone, active_days, always_open, max_calls_per_hour)
		 VALUES ($1, $2, $3, $4::time, $5::time, $6, $7, $8, $9)
		 ON CONFLICT (user_id, model_id) DO UPDATE SET
		   daily_cap_usd = $3, allowed_from = $4::time, allowed_to = $5::time,
		   timezone = $6, active_days = $7, always_open = $8, max_calls_per_hour = $9`,
		p.UserID, p.ModelID, p.DailyCapUSD, p.AllowedFrom, p.AllowedTo, p.Timezone, p.ActiveDays, p.AlwaysOpen, p.MaxCallsPerHour,
	)
	return err
}
