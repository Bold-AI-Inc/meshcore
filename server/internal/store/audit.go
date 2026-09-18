package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditStore struct {
	pool *pgxpool.Pool
}

func NewAuditStore(pool *pgxpool.Pool) *AuditStore {
	return &AuditStore{pool: pool}
}

func (s *AuditStore) Log(ctx context.Context, adminEmail, action, target string, detail *string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO admin_audit_log (admin_email, action, target, detail) VALUES ($1, $2, $3, $4)`,
		adminEmail, action, target, detail,
	)
	return err
}
