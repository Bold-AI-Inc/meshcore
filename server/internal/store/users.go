package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mesh-server/internal/domain"
)

type UserStore struct {
	pool *pgxpool.Pool
}

func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

func (s *UserStore) Create(ctx context.Context, email, displayName string) (*domain.User, error) {
	user := &domain.User{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, display_name)
		 VALUES ($1, $2)
		 RETURNING id, email, display_name, status, expires_at, created_at`,
		email, displayName,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Status, &user.ExpiresAt, &user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	return user, nil
}

func (s *UserStore) ListWithKeyInfo(ctx context.Context) ([]domain.UserWithKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT u.id, u.email, u.display_name, u.status, u.expires_at, u.created_at,
		        k.key_prefix, k.status
		 FROM users u
		 LEFT JOIN mesh_api_keys k ON k.user_id = u.id AND k.status = 'active'
		 ORDER BY u.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []domain.UserWithKey{}
	for rows.Next() {
		var row domain.UserWithKey
		if err := rows.Scan(
			&row.User.ID, &row.User.Email, &row.User.DisplayName, &row.User.Status,
			&row.User.ExpiresAt, &row.User.CreatedAt, &row.KeyPrefix, &row.KeyStatus,
		); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (s *UserStore) UpdateExpiry(ctx context.Context, userID string, expiresAt *time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET expires_at = $1 WHERE id = $2`, expiresAt, userID)
	return err
}

func (s *UserStore) UpdateStatus(ctx context.Context, userID, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET status = $1 WHERE id = $2`, status, userID)
	return err
}

func (s *UserStore) Delete(ctx context.Context, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM mesh_api_keys WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM daily_spend WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
