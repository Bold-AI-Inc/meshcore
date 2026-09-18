package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SessionStore struct {
	pool *pgxpool.Pool
}

func NewSessionStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{pool: pool}
}

func (s *SessionStore) Create(ctx context.Context, adminID, tokenHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO admin_sessions (admin_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		adminID, tokenHash, expiresAt,
	)
	return err
}

type AuthedAdmin struct {
	ID     string
	Email  string
	Status string
}

func (s *SessionStore) FindActiveAdmin(ctx context.Context, tokenHash string) (*AuthedAdmin, error) {
	var admin AuthedAdmin
	err := s.pool.QueryRow(ctx,
		`SELECT a.id, a.email, a.status
		 FROM admin_sessions s
		 JOIN admins a ON a.id = s.admin_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`,
		tokenHash,
	).Scan(&admin.ID, &admin.Email, &admin.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

func (s *SessionStore) Delete(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *SessionStore) DeleteForAdminExcept(ctx context.Context, adminID, keepTokenHash string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM admin_sessions WHERE admin_id = $1 AND token_hash <> $2`,
		adminID, keepTokenHash,
	)
	return err
}

func (s *SessionStore) DeleteForAdmin(ctx context.Context, adminID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_sessions WHERE admin_id = $1`, adminID)
	return err
}

func (s *SessionStore) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM admin_sessions WHERE expires_at < now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
