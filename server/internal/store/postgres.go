package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PoolSizing struct {
	MinConns    int32
	MaxConns    int32
	IdleTimeout time.Duration

	ExecMode string
}

func execMode(name string) pgx.QueryExecMode {
	switch name {
	case "cache_statement":
		return pgx.QueryExecModeCacheStatement
	case "describe_exec":
		return pgx.QueryExecModeDescribeExec
	case "simple":
		return pgx.QueryExecModeSimpleProtocol
	default:
		return pgx.QueryExecModeCacheDescribe
	}
}

func NewPool(ctx context.Context, databaseURL string, sizing PoolSizing) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MinConns = sizing.MinConns
	cfg.MaxConns = sizing.MaxConns
	cfg.MaxConnLifetime = time.Hour
	if sizing.IdleTimeout > 0 {
		cfg.MaxConnIdleTime = sizing.IdleTimeout
	}
	cfg.ConnConfig.DefaultQueryExecMode = execMode(sizing.ExecMode)
	if cfg.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeCacheStatement {
		cfg.ConnConfig.StatementCacheCapacity = 0
	}
	cfg.HealthCheckPeriod = time.Minute

	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctxTimeout, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctxTimeout); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
