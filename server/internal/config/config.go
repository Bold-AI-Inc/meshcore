package config

import (
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	FrontendOrigin     string
	DatabaseURL        string
	GatewayDatabaseURL string
	MasterKey          string
	MeshKeyPepper      string
	CookieSecure       bool

	QueueCapacity         int64
	QueueAdmitTimeoutMs   int
	QueueGoroutineCeiling int
	QueueHeapCeilingMB    int64

	MaxRequestBodyBytes int64

	GenerationTimeout time.Duration

	EmbeddingsTimeout          time.Duration
	MaxGenerationResponseBytes int64

	DBExecMode string

	AdminPoolMaxConns   int32
	GatewayPoolMaxConns int32

	LogRetentionDays int

	SnapshotReconcileInterval time.Duration

	SuperAdminEmail string

	MetricsToken string

	TrustProxyHeader bool
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		FrontendOrigin:     getEnv("FRONTEND_ORIGIN", "http://localhost:3000"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		GatewayDatabaseURL: os.Getenv("GATEWAY_DATABASE_URL"),
		MasterKey:          os.Getenv("MESH_MASTER_KEY"),
		MeshKeyPepper:      os.Getenv("MESH_KEY_PEPPER"),
	}

	if cfg.DatabaseURL == "" {
		return nil, errors.New("missing required environment variable: DATABASE_URL")
	}
	if cfg.GatewayDatabaseURL == "" {
		return nil, errors.New("missing required environment variable: GATEWAY_DATABASE_URL")
	}
	if cfg.MeshKeyPepper == "" {
		return nil, errors.New("missing required environment variable: MESH_KEY_PEPPER")
	}
	if key, err := hex.DecodeString(cfg.MasterKey); err != nil || len(key) != 32 {
		return nil, errors.New("MESH_MASTER_KEY must be 64 hex characters (32 bytes) — generate one with: openssl rand -hex 32")
	}

	secure, err := strconv.ParseBool(getEnv("COOKIE_SECURE", "false"))
	if err != nil {
		return nil, err
	}
	cfg.CookieSecure = secure

	queueCapacity, err := strconv.ParseInt(getEnv("QUEUE_CAPACITY", "200"), 10, 64)
	if err != nil {
		return nil, errors.New("QUEUE_CAPACITY must be an integer")
	}
	cfg.QueueCapacity = queueCapacity

	admitTimeoutMs, err := strconv.Atoi(getEnv("QUEUE_ADMIT_TIMEOUT_MS", "4000"))
	if err != nil {
		return nil, errors.New("QUEUE_ADMIT_TIMEOUT_MS must be an integer")
	}
	cfg.QueueAdmitTimeoutMs = admitTimeoutMs

	goroutineCeiling, err := strconv.Atoi(getEnv("QUEUE_GOROUTINE_CEILING", "5000"))
	if err != nil {
		return nil, errors.New("QUEUE_GOROUTINE_CEILING must be an integer")
	}
	cfg.QueueGoroutineCeiling = goroutineCeiling

	heapCeilingMB, err := strconv.ParseInt(getEnv("QUEUE_HEAP_CEILING_MB", "1024"), 10, 64)
	if err != nil {
		return nil, errors.New("QUEUE_HEAP_CEILING_MB must be an integer")
	}
	cfg.QueueHeapCeilingMB = heapCeilingMB

	maxBodyMB, err := strconv.ParseInt(getEnv("MAX_REQUEST_BODY_MB", "300"), 10, 64)
	if err != nil {
		return nil, errors.New("MAX_REQUEST_BODY_MB must be an integer")
	}
	cfg.MaxRequestBodyBytes = maxBodyMB * 1024 * 1024

	generationTimeoutSec, err := strconv.Atoi(getEnv("GENERATION_TIMEOUT_SECONDS", "120"))
	if err != nil {
		return nil, errors.New("GENERATION_TIMEOUT_SECONDS must be an integer")
	}
	cfg.GenerationTimeout = time.Duration(generationTimeoutSec) * time.Second

	embeddingsTimeoutSec, err := strconv.Atoi(getEnv("EMBEDDINGS_TIMEOUT_SECONDS", "60"))
	if err != nil || embeddingsTimeoutSec < 1 {
		return nil, errors.New("EMBEDDINGS_TIMEOUT_SECONDS must be a positive integer")
	}
	cfg.EmbeddingsTimeout = time.Duration(embeddingsTimeoutSec) * time.Second

	maxGenerationResponseMB, err := strconv.ParseInt(getEnv("MAX_GENERATION_RESPONSE_MB", "300"), 10, 64)
	if err != nil {
		return nil, errors.New("MAX_GENERATION_RESPONSE_MB must be an integer")
	}
	cfg.MaxGenerationResponseBytes = maxGenerationResponseMB * 1024 * 1024

	cfg.DBExecMode = getEnv("DB_EXEC_MODE", "cache_describe")
	switch cfg.DBExecMode {
	case "cache_statement", "cache_describe", "describe_exec", "simple":
	default:
		return nil, errors.New("DB_EXEC_MODE must be one of: cache_statement, cache_describe, describe_exec, simple")
	}

	adminMaxConns, err := strconv.Atoi(getEnv("ADMIN_POOL_MAX_CONNS", "8"))
	if err != nil || adminMaxConns < 1 {
		return nil, errors.New("ADMIN_POOL_MAX_CONNS must be a positive integer")
	}
	cfg.AdminPoolMaxConns = int32(adminMaxConns)

	gatewayMaxConns, err := strconv.Atoi(getEnv("GATEWAY_POOL_MAX_CONNS", "16"))
	if err != nil || gatewayMaxConns < 1 {
		return nil, errors.New("GATEWAY_POOL_MAX_CONNS must be a positive integer")
	}
	cfg.GatewayPoolMaxConns = int32(gatewayMaxConns)

	retentionDays, err := strconv.Atoi(getEnv("LOG_RETENTION_DAYS", "30"))
	if err != nil || retentionDays < 0 {
		return nil, errors.New("LOG_RETENTION_DAYS must be a non-negative integer")
	}
	cfg.LogRetentionDays = retentionDays

	reconcileSec, err := strconv.Atoi(getEnv("SNAPSHOT_RECONCILE_SECONDS", "45"))
	if err != nil || reconcileSec < 1 {
		return nil, errors.New("SNAPSHOT_RECONCILE_SECONDS must be a positive integer")
	}
	cfg.SnapshotReconcileInterval = time.Duration(reconcileSec) * time.Second

	cfg.SuperAdminEmail = strings.ToLower(getEnv("SUPERADMIN_EMAIL", "alan@experiencebold.ai"))

	cfg.MetricsToken = os.Getenv("METRICS_TOKEN")

	trustProxy, err := strconv.ParseBool(getEnv("TRUST_PROXY_HEADER", "false"))
	if err != nil {
		return nil, errors.New("TRUST_PROXY_HEADER must be a boolean")
	}
	cfg.TrustProxyHeader = trustProxy

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
