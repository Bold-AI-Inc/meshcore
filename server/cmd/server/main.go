package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"mesh-server/internal/adminapi"
	"mesh-server/internal/config"
	"mesh-server/internal/gateway"
	"mesh-server/internal/memstate"
	"mesh-server/internal/middleware"
	"mesh-server/internal/reqqueue"
	"mesh-server/internal/security"
	"mesh-server/internal/store"
	"mesh-server/internal/supervise"
)

func main() {
	switch {
	case len(os.Args) > 1 && os.Args[1] == "bootstrap-admin":
		runBootstrap(os.Args[2:])
	default:
		runServer()
	}
}

func runServer() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()

	adminPool, err := store.NewPool(ctx, cfg.DatabaseURL, store.PoolSizing{
		MinConns: 0, MaxConns: cfg.AdminPoolMaxConns, IdleTimeout: 30 * time.Second,
		ExecMode: cfg.DBExecMode,
	})
	if err != nil {
		slog.Error("admin pool init failed", "err", err)
		os.Exit(1)
	}
	defer adminPool.Close()

	gatewayPool, err := store.NewPool(ctx, cfg.GatewayDatabaseURL, store.PoolSizing{
		MinConns: 2, MaxConns: cfg.GatewayPoolMaxConns,
		ExecMode: cfg.DBExecMode,
	})
	if err != nil {
		slog.Error("gateway pool init failed", "err", err)
		os.Exit(1)
	}
	defer gatewayPool.Close()

	admins := store.NewAdminStore(adminPool)
	sessions := store.NewSessionStore(adminPool)
	users := store.NewUserStore(adminPool)
	meshKeys := store.NewMeshKeyStore(adminPool)
	providers := store.NewProviderStore(adminPool)
	models := store.NewModelStore(adminPool)
	access := store.NewAccessStore(adminPool)
	policies := store.NewPolicyStore(adminPool)
	requestLogs := store.NewRequestLogStore(adminPool)
	audit := store.NewAuditStore(adminPool)
	sessionCache := middleware.NewSessionCache()

	memState := memstate.New(gatewayPool, cfg.MasterKey)
	now := time.Now()
	if err := memState.SeedFromPostgres(ctx, now); err != nil {
		slog.Error("memstate: failed to seed counters from Postgres", "err", err)
		os.Exit(1)
	}
	if err := memState.Rebuild(ctx); err != nil {
		slog.Error("memstate: failed to build initial snapshot", "err", err)
		os.Exit(1)
	}

	telemetry := gateway.NewTelemetry(gatewayPool)
	telemetry.StartWorkers()
	stopTickers, tickersDone := runCheckpointTickers(memState)

	supervise.Go("snapshot-reconcile-ticker", func() {
		ticker := time.NewTicker(cfg.SnapshotReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopTickers:
				select {}
			case <-ticker.C:
				rebuildCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := memState.Rebuild(rebuildCtx); err != nil {
					slog.Error("snapshot reconcile failed", "err", err)
				}
				cancel()
			}
		}
	})

	{
		supervise.Go("log-retention-job", func() {
			runRetention := func() {
				retentionCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()

				if cfg.LogRetentionDays > 0 {
					cutoff := time.Now().UTC().AddDate(0, 0, -cfg.LogRetentionDays)
					deleted, err := requestLogs.DeleteOlderThan(retentionCtx, cutoff, 5000, 200)
					if err != nil {
						slog.Error("log retention failed", "err", err)
					}
					if deleted > 0 {
						slog.Info("log retention swept", "deleted", deleted, "older_than", cutoff.Format(time.RFC3339))
					}
				}

				sessionsDeleted, err := sessions.DeleteExpired(retentionCtx)
				if err != nil {
					slog.Error("admin session sweep failed", "err", err)
				}
				if sessionsDeleted > 0 {
					slog.Info("admin sessions swept", "deleted", sessionsDeleted)
				}
			}
			timer := time.NewTimer(5 * time.Minute)
			defer timer.Stop()
			select {
			case <-stopTickers:
				select {}
			case <-timer.C:
				runRetention()
			}
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-stopTickers:
					select {}
				case <-ticker.C:
					runRetention()
				}
			}
		})
	}

	queue := reqqueue.New(reqqueue.Config{
		Capacity:         cfg.QueueCapacity,
		AdmitTimeout:     time.Duration(cfg.QueueAdmitTimeoutMs) * time.Millisecond,
		PollInterval:     3 * time.Second,
		GoroutineCeiling: cfg.QueueGoroutineCeiling,
		HeapCeilingBytes: uint64(cfg.QueueHeapCeilingMB) * 1024 * 1024,
	})

	adminRouter := adminapi.NewRouter(adminapi.Deps{
		Admins:        admins,
		Sessions:      sessions,
		Users:         users,
		MeshKeys:      meshKeys,
		Providers:     providers,
		Models:        models,
		Access:        access,
		Policies:      policies,
		RequestLogs:   requestLogs,
		Audit:         audit,
		MemState:      memState,
		SessionCache:  sessionCache,
		CookieSecure:  cfg.CookieSecure,
		MeshKeyPepper: cfg.MeshKeyPepper,
		MasterKey:     cfg.MasterKey,

		SuperAdminEmail: cfg.SuperAdminEmail,
		MetricsToken:    cfg.MetricsToken,

		TrustProxyHeader: cfg.TrustProxyHeader,

		Telemetry:   telemetry,
		Queue:       queue,
		AdminPool:   adminPool,
		GatewayPool: gatewayPool,
	})
	gatewayHandler := gateway.NewHandler(memState, telemetry, cfg.MeshKeyPepper, gateway.Options{
		Queue:                      queue,
		MaxRequestBodyBytes:        cfg.MaxRequestBodyBytes,
		QueueAdmitTimeout:          time.Duration(cfg.QueueAdmitTimeoutMs) * time.Millisecond,
		GenerationTimeout:          cfg.GenerationTimeout,
		EmbeddingsTimeout:          cfg.EmbeddingsTimeout,
		MaxGenerationResponseBytes: cfg.MaxGenerationResponseBytes,
		TrustProxyHeader:           cfg.TrustProxyHeader,
	})
	gatewayRouter := gateway.NewRouter(gatewayHandler)

	topMux := http.NewServeMux()
	topMux.Handle("/v1/", gatewayRouter)
	topMux.Handle("/admin/", adminRouter)
	topMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	topMux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		if err := gatewayPool.Ping(pingCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"degraded","reason":"database_unreachable"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	handler := middleware.Recover(middleware.SecurityHeaders(middleware.CORS(cfg.FrontendOrigin)(topMux)))

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: handler,

		ReadHeaderTimeout: 10 * time.Second,

		IdleTimeout: 120 * time.Second,
	}

	go func() {
		slog.Info("mesh-server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)

	close(stopTickers)
	tickersDone.Wait()

	telemetryCtx, telemetryCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer telemetryCancel()
	telemetry.Stop(telemetryCtx)

	queue.Stop()
	sessionCache.Close()
}

func runCheckpointTickers(mem *memstate.Store) (stop chan struct{}, done *sync.WaitGroup) {
	stop = make(chan struct{})
	done = &sync.WaitGroup{}
	done.Add(2)

	supervise.Go("budget-checkpoint-ticker", func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		checkpoint := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := mem.CheckpointDailySpend(ctx, time.Now()); err != nil {
				slog.Error("budget checkpoint failed", "err", err)
			}
			cancel()
		}
		for {
			select {
			case <-ticker.C:
				checkpoint()
			case <-stop:
				checkpoint()
				done.Done()
				select {}
			}
		}
	})

	supervise.Go("boundary-checkpoint-ticker", func() {
		currentHour := time.Now().UTC().Truncate(time.Hour)
		currentDay := time.Now().UTC().Truncate(24 * time.Hour)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		checkpointHourly := func(hourStart time.Time, finalLabel string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := mem.CheckpointAndResetHourly(ctx, hourStart); err != nil {
				slog.Error("provider hourly checkpoint failed", "err", err, "final", finalLabel != "")
			}
			if err := mem.CheckpointAndResetUserModelHourly(ctx, hourStart); err != nil {
				slog.Error("user-model hourly checkpoint failed", "err", err, "final", finalLabel != "")
			}
		}
		checkpointDay := func(dayStart time.Time, finalLabel string) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := mem.CheckpointAndResetDailySpend(ctx, dayStart); err != nil {
				slog.Error("daily spend checkpoint failed", "err", err, "final", finalLabel != "")
			}
		}

		for {
			select {
			case <-ticker.C:
			case <-stop:
				checkpointHourly(currentHour, " (final)")
				done.Done()
				select {}
			}

			now := time.Now().UTC()

			if dayNow := now.Truncate(24 * time.Hour); dayNow.After(currentDay) {
				checkpointDay(currentDay, "")
				currentDay = dayNow
			}

			hourNow := now.Truncate(time.Hour)
			if !hourNow.After(currentHour) {
				continue
			}
			checkpointHourly(currentHour, "")
			currentHour = hourNow
		}
	})

	return stop, done
}

func runBootstrap(args []string) {
	fs := flag.NewFlagSet("bootstrap-admin", flag.ExitOnError)
	email := fs.String("email", "", "")
	password := fs.String("password", "", "")
	_ = fs.Parse(args)

	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "email and password are required")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := store.NewPool(ctx, cfg.DatabaseURL, store.PoolSizing{MinConns: 0, MaxConns: 2})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer pool.Close()

	admins := store.NewAdminStore(pool)
	count, err := admins.Count(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if count > 0 {
		fmt.Fprintln(os.Stderr, "an admin already exists; use the authenticated dashboard to add more")
		os.Exit(1)
	}

	hash, err := security.HashPassword(*password)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if _, err := admins.Create(ctx, *email, hash, "bootstrap_cli"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("admin created:", *email)
}
