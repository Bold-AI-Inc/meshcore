package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mesh-server/internal/domain"
)

const maxFailedLoginAttempts = 5
const lockoutDuration = 15 * time.Minute

type AdminStore struct {
	pool *pgxpool.Pool
}

func NewAdminStore(pool *pgxpool.Pool) *AdminStore {
	return &AdminStore{pool: pool}
}

func (s *AdminStore) Count(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM admins").Scan(&count)
	return count, err
}

func (s *AdminStore) Create(ctx context.Context, email, passwordHash, createdBy string) (*domain.Admin, error) {
	admin := &domain.Admin{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO admins (email, password_hash, created_by)
		 VALUES ($1, $2, $3)
		 RETURNING id, email, password_hash, status, created_by, created_at, failed_login_attempts, locked_until`,
		strings.ToLower(strings.TrimSpace(email)), passwordHash, createdBy,
	).Scan(&admin.ID, &admin.Email, &admin.PasswordHash, &admin.Status, &admin.CreatedBy, &admin.CreatedAt,
		&admin.FailedLoginAttempts, &admin.LockedUntil)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	return admin, nil
}

func (s *AdminStore) FindByEmail(ctx context.Context, email string) (*domain.Admin, error) {
	admin := &domain.Admin{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, status, created_by, created_at, failed_login_attempts, locked_until
		 FROM admins WHERE lower(email) = $1`,
		strings.ToLower(strings.TrimSpace(email)),
	).Scan(&admin.ID, &admin.Email, &admin.PasswordHash, &admin.Status, &admin.CreatedBy, &admin.CreatedAt,
		&admin.FailedLoginAttempts, &admin.LockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return admin, nil
}

func (s *AdminStore) FindByID(ctx context.Context, id string) (*domain.Admin, error) {
	admin := &domain.Admin{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, status, created_by, created_at, failed_login_attempts, locked_until
		 FROM admins WHERE id = $1`,
		id,
	).Scan(&admin.ID, &admin.Email, &admin.PasswordHash, &admin.Status, &admin.CreatedBy, &admin.CreatedAt,
		&admin.FailedLoginAttempts, &admin.LockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return admin, nil
}

func (s *AdminStore) UpdatePasswordHash(ctx context.Context, id, passwordHash string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE admins SET password_hash = $1 WHERE id = $2`,
		passwordHash, id,
	)
	return err
}

func (s *AdminStore) List(ctx context.Context) ([]domain.Admin, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, email, password_hash, status, created_by, created_at, failed_login_attempts, locked_until
		 FROM admins ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	admins := []domain.Admin{}
	for rows.Next() {
		var admin domain.Admin
		if err := rows.Scan(&admin.ID, &admin.Email, &admin.PasswordHash, &admin.Status, &admin.CreatedBy, &admin.CreatedAt,
			&admin.FailedLoginAttempts, &admin.LockedUntil); err != nil {
			return nil, err
		}
		admins = append(admins, admin)
	}
	return admins, rows.Err()
}

func (s *AdminStore) RecordFailedLogin(ctx context.Context, id string) (bool, error) {
	var attempts int
	err := s.pool.QueryRow(ctx,
		`UPDATE admins SET failed_login_attempts = failed_login_attempts + 1 WHERE id = $1
		 RETURNING failed_login_attempts`,
		id,
	).Scan(&attempts)
	if err != nil {
		return false, err
	}
	if attempts < maxFailedLoginAttempts {
		return false, nil
	}
	lockedUntil := time.Now().Add(lockoutDuration)
	if _, err := s.pool.Exec(ctx,
		`UPDATE admins SET locked_until = $2 WHERE id = $1`,
		id, lockedUntil,
	); err != nil {
		return false, err
	}
	return true, nil
}

func (s *AdminStore) RecordSuccessfulLogin(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE admins SET failed_login_attempts = 0, locked_until = NULL WHERE id = $1`,
		id,
	)
	return err
}

func (s *AdminStore) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE admins SET status = $1 WHERE id = $2`, status, id)
	return err
}

func (s *AdminStore) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admins WHERE id = $1`, id)
	return err
}
