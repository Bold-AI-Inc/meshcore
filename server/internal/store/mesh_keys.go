package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MeshKeyStore struct {
	pool *pgxpool.Pool
}

func NewMeshKeyStore(pool *pgxpool.Pool) *MeshKeyStore {
	return &MeshKeyStore{pool: pool}
}

func (s *MeshKeyStore) Create(ctx context.Context, userID, keyHash, keyPrefix string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO mesh_api_keys (user_id, key_hash, key_prefix) VALUES ($1, $2, $3)`,
		userID, keyHash, keyPrefix,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAlreadyExists
	}
	return err
}

func (s *MeshKeyStore) RevokeByUserID(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE mesh_api_keys SET status = 'revoked', revoked_at = now() WHERE user_id = $1 AND status = 'active'`,
		userID,
	)
	return err
}

func (s *MeshKeyStore) Rotate(ctx context.Context, userID, keyHash, keyPrefix string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE mesh_api_keys SET status = 'revoked', revoked_at = now() WHERE user_id = $1 AND status = 'active'`,
		userID,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO mesh_api_keys (user_id, key_hash, key_prefix) VALUES ($1, $2, $3)`,
		userID, keyHash, keyPrefix,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrAlreadyExists
		}
		return err
	}

	return tx.Commit(ctx)
}
