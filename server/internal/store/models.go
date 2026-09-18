package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mesh-server/internal/domain"
)

type ModelStore struct {
	pool *pgxpool.Pool
}

func NewModelStore(pool *pgxpool.Pool) *ModelStore {
	return &ModelStore{pool: pool}
}

const modelColumns = `m.id, m.provider_id, p.name, m.name, m.resolved_model, m.status, m.blocked,
	m.input_price_per_1k_tokens, m.output_price_per_1k_tokens, m.kind,
	m.supports_image_in, m.supports_document_in, m.supports_audio_in, m.supports_video_in,
	m.supports_web_search, m.price_per_second, m.output_media_type, m.created_at`

const modelFromJoin = `FROM models m JOIN providers p ON p.id = m.provider_id`

func scanModel(row interface {
	Scan(dest ...any) error
}) (*domain.Model, error) {
	m := &domain.Model{}
	err := row.Scan(&m.ID, &m.ProviderID, &m.ProviderName, &m.Name, &m.ResolvedModel,
		&m.Status, &m.Blocked, &m.InputPricePer1kTokens, &m.OutputPricePer1kTokens, &m.Kind,
		&m.SupportsImageIn, &m.SupportsDocumentIn, &m.SupportsAudioIn, &m.SupportsVideoIn,
		&m.SupportsWebSearch, &m.PricePerSecond, &m.OutputMediaType, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

type ModelCreateParams struct {
	Kind               domain.ModelKind
	SupportsImageIn    bool
	SupportsDocumentIn bool
	SupportsAudioIn    bool
	SupportsVideoIn    bool
	SupportsWebSearch  bool
	PricePerSecond     float64
	OutputMediaType    *string
}

func (s *ModelStore) Create(ctx context.Context, providerID, name, resolvedModel string, inputPrice, outputPrice float64, extra ModelCreateParams) (*domain.Model, error) {
	kind := extra.Kind
	if kind == "" {
		kind = domain.ModelKindChat
	}
	m := &domain.Model{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO models (provider_id, name, resolved_model, input_price_per_1k_tokens, output_price_per_1k_tokens,
			kind, supports_image_in, supports_document_in, supports_audio_in, supports_video_in, supports_web_search,
			output_media_type, price_per_second)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 RETURNING id, provider_id, name, resolved_model, status, blocked, input_price_per_1k_tokens, output_price_per_1k_tokens,
			kind, supports_image_in, supports_document_in, supports_audio_in, supports_video_in, supports_web_search,
			price_per_second, output_media_type, created_at`,
		providerID, name, resolvedModel, inputPrice, outputPrice,
		kind, extra.SupportsImageIn, extra.SupportsDocumentIn, extra.SupportsAudioIn, extra.SupportsVideoIn, extra.SupportsWebSearch,
		extra.OutputMediaType, extra.PricePerSecond,
	).Scan(&m.ID, &m.ProviderID, &m.Name, &m.ResolvedModel, &m.Status, &m.Blocked,
		&m.InputPricePer1kTokens, &m.OutputPricePer1kTokens,
		&m.Kind, &m.SupportsImageIn, &m.SupportsDocumentIn, &m.SupportsAudioIn, &m.SupportsVideoIn,
		&m.SupportsWebSearch, &m.PricePerSecond, &m.OutputMediaType, &m.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAlreadyExists
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

func (s *ModelStore) list(ctx context.Context, where string, args ...any) ([]domain.Model, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+modelColumns+` `+modelFromJoin+` `+where+` ORDER BY m.created_at DESC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := []domain.Model{}
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, *m)
	}
	return models, rows.Err()
}

func (s *ModelStore) List(ctx context.Context) ([]domain.Model, error) {
	return s.list(ctx, "")
}

func (s *ModelStore) ListByProvider(ctx context.Context, providerID string) ([]domain.Model, error) {
	return s.list(ctx, "WHERE m.provider_id = $1", providerID)
}

func (s *ModelStore) Get(ctx context.Context, id string) (*domain.Model, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+modelColumns+` `+modelFromJoin+` WHERE m.id = $1`,
		id,
	)
	m, err := scanModel(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *ModelStore) Update(ctx context.Context, id, name, resolvedModel string, inputPrice, outputPrice float64, blocked bool, extra ModelCreateParams) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE models SET name=$2, resolved_model=$3, input_price_per_1k_tokens=$4,
		 output_price_per_1k_tokens=$5, blocked=$6,
		 supports_image_in=$7, supports_document_in=$8, supports_audio_in=$9, supports_video_in=$10,
		 output_media_type=$11, supports_web_search=$12, price_per_second=$13
		 WHERE id=$1`,
		id, name, resolvedModel, inputPrice, outputPrice, blocked,
		extra.SupportsImageIn, extra.SupportsDocumentIn, extra.SupportsAudioIn, extra.SupportsVideoIn,
		extra.OutputMediaType, extra.SupportsWebSearch, extra.PricePerSecond,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAlreadyExists
	}
	return err
}

func (s *ModelStore) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM models WHERE id = $1`, id)
	return err
}
