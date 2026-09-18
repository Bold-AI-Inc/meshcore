package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"mesh-server/internal/domain"
)

type AccessStore struct {
	pool *pgxpool.Pool
}

func NewAccessStore(pool *pgxpool.Pool) *AccessStore {
	return &AccessStore{pool: pool}
}

func (s *AccessStore) Grant(ctx context.Context, userID, modelID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_model_access (user_id, model_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, modelID,
	)
	return err
}

func (s *AccessStore) Revoke(ctx context.Context, userID, modelID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM user_model_access WHERE user_id = $1 AND model_id = $2`,
		userID, modelID,
	)
	return err
}

func (s *AccessStore) ListForUser(ctx context.Context, userID string) ([]domain.AccessGrant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.id, m.name, p.name
		 FROM user_model_access a
		 JOIN models m ON m.id = a.model_id
		 JOIN providers p ON p.id = m.provider_id
		 WHERE a.user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grants := []domain.AccessGrant{}
	for rows.Next() {
		var g domain.AccessGrant
		if err := rows.Scan(&g.ModelID, &g.ModelName, &g.ProviderName); err != nil {
			return nil, err
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}
