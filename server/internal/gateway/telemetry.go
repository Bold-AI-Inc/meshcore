package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mesh-server/internal/supervise"
	"mesh-server/internal/uadevice"
)

const (
	telemetryChannelCapacity = 4096
	telemetryWorkerCount     = 2
	telemetryFlushInterval   = time.Second
	telemetryFlushBatchSize  = 500
)

type logEvent struct {
	ID               uuid.UUID
	MeshKeyID        *string
	UserID           *string
	ProviderID       *string
	ModelID          *string
	RequestedModel   *string
	Outcome          string
	DenyReason       *string
	SourceIP         *string
	UserAgent        *string
	StatusCode       int
	LatencyMs        int
	TokensIn         int
	TokensOut        int
	InputCost        float64
	OutputCost       float64
	WebSearches      int
	GeneratedSeconds float64

	Browser        *string
	BrowserVersion *string
	OS             *string
	OSVersion      *string
	DeviceType     *string
}

type Telemetry struct {
	pool *pgxpool.Pool

	events  chan logEvent
	denials chan logEvent

	droppedSuccess atomic.Int64
	droppedDenied  atomic.Int64
	insertFailures atomic.Int64
	written        atomic.Int64

	stop    chan struct{}
	stopped sync.WaitGroup
}

func NewTelemetry(pool *pgxpool.Pool) *Telemetry {
	return &Telemetry{
		pool:    pool,
		events:  make(chan logEvent, telemetryChannelCapacity),
		denials: make(chan logEvent, telemetryChannelCapacity),
		stop:    make(chan struct{}),
	}
}

func (t *Telemetry) Enqueue(ev logEvent) {
	if ev.Outcome == "denied" {
		select {
		case t.denials <- ev:
		default:
			t.droppedDenied.Add(1)
		}
		return
	}
	select {
	case t.events <- ev:
	default:
		t.droppedSuccess.Add(1)
	}
}

type Stats struct {
	QueuedEvents   int   `json:"queued_events"`
	QueuedDenials  int   `json:"queued_denials"`
	Written        int64 `json:"written_total"`
	DroppedSuccess int64 `json:"dropped_success_total"`
	DroppedDenied  int64 `json:"dropped_denied_total"`
	InsertFailures int64 `json:"insert_failures_total"`
}

func (t *Telemetry) Stats() Stats {
	return Stats{
		QueuedEvents:   len(t.events),
		QueuedDenials:  len(t.denials),
		Written:        t.written.Load(),
		DroppedSuccess: t.droppedSuccess.Load(),
		DroppedDenied:  t.droppedDenied.Load(),
		InsertFailures: t.insertFailures.Load(),
	}
}

func (t *Telemetry) StartWorkers() {
	t.stopped.Add(telemetryWorkerCount)
	for i := 0; i < telemetryWorkerCount; i++ {
		workerName := fmt.Sprintf("telemetry-worker-%d", i)
		supervise.Go(workerName, t.worker)
	}
	supervise.Go("telemetry-drop-reporter", t.reportDrops)
}

func (t *Telemetry) reportDrops() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var lastSuccess, lastDenied, lastFailures int64
	for {
		select {
		case <-t.stop:
			select {}
		case <-ticker.C:
			s, d, f := t.droppedSuccess.Load(), t.droppedDenied.Load(), t.insertFailures.Load()
			if s != lastSuccess || d != lastDenied || f != lastFailures {
				slog.Warn("request_logs is incomplete",
					"dropped_success", s, "dropped_success_delta", s-lastSuccess,
					"dropped_denied", d, "dropped_denied_delta", d-lastDenied,
					"insert_failures", f, "insert_failures_delta", f-lastFailures)
				lastSuccess, lastDenied, lastFailures = s, d, f
			}
		}
	}
}

func (t *Telemetry) Stop(ctx context.Context) {
	close(t.stop)
	done := make(chan struct{})
	go func() {
		t.stopped.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		slog.Warn("telemetry shutdown drain timed out, some request_logs events may be lost")
	}
}

func (t *Telemetry) worker() {
	ticker := time.NewTicker(telemetryFlushInterval)
	defer ticker.Stop()

	batch := make([]logEvent, 0, telemetryFlushBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx := context.Background()
		enrichBatch(batch)
		if err := t.insertBatch(ctx, batch); err != nil {
			t.insertFailures.Add(int64(len(batch)))
			slog.Error("telemetry batch insert failed", "err", err, "batch_size", len(batch))
		} else {
			t.written.Add(int64(len(batch)))
		}
		batch = batch[:0]
	}

	add := func(ev logEvent) {
		batch = append(batch, ev)
		if len(batch) >= telemetryFlushBatchSize {
			flush()
		}
	}

	for {
		// Denials first. A single select over both channels picks at random
		// when both are ready, so giving denials their own channel only ever
		// bought them independent backpressure, never priority. This
		// non-blocking pass drains whatever denials are already buffered
		// before the blocking select below can pick up an ordinary event.
		//
		// It cannot starve the stop case: by the time Stop runs, main has
		// already shut the HTTP server down, so nothing is still producing.
		select {
		case ev := <-t.denials:
			add(ev)
			continue
		default:
		}

		select {
		case ev := <-t.denials:
			add(ev)
		case ev := <-t.events:
			add(ev)
		case <-ticker.C:
			flush()
		case <-t.stop:
		drain:
			for {
				select {
				case ev := <-t.denials:
					batch = append(batch, ev)
				case ev := <-t.events:
					batch = append(batch, ev)
				default:
					break drain
				}
			}
			flush()
			t.stopped.Done()
			select {}
		}
	}
}

func enrichBatch(batch []logEvent) {
	for i := range batch {
		ev := &batch[i]
		if ev.UserAgent == nil {
			continue
		}
		parsed := uadevice.Parse(*ev.UserAgent)
		if parsed.Browser != "" {
			ev.Browser = &parsed.Browser
		}
		if parsed.BrowserVersion != "" {
			ev.BrowserVersion = &parsed.BrowserVersion
		}
		if parsed.OS != "" {
			ev.OS = &parsed.OS
		}
		if parsed.OSVersion != "" {
			ev.OSVersion = &parsed.OSVersion
		}
		if parsed.DeviceType != "" {
			ev.DeviceType = &parsed.DeviceType
		}
	}
}

var requestLogColumns = []string{
	"id", "mesh_key_id", "user_id", "provider_id", "model_id", "requested_model",
	"outcome", "deny_reason", "source_ip", "user_agent",
	"browser", "browser_version", "os", "os_version", "device_type",
	"status_code", "latency_ms", "tokens_in", "tokens_out",
	"input_cost", "output_cost", "web_searches", "generated_seconds",
}

func (t *Telemetry) insertBatch(ctx context.Context, batch []logEvent) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pgBatch := &pgx.Batch{}
	for _, ev := range batch {
		pgBatch.Queue(insertRequestLogSQL,
			ev.ID, ev.MeshKeyID, ev.UserID, ev.ProviderID, ev.ModelID, ev.RequestedModel,
			ev.Outcome, ev.DenyReason, sourceAddr(ev.SourceIP), ev.UserAgent,
			ev.Browser, ev.BrowserVersion, ev.OS, ev.OSVersion, ev.DeviceType,
			ev.StatusCode, ev.LatencyMs, ev.TokensIn, ev.TokensOut,
			ev.InputCost, ev.OutputCost, ev.WebSearches, ev.GeneratedSeconds,
		)
	}

	results := t.pool.SendBatch(ctx, pgBatch)
	defer results.Close()
	for i := 0; i < pgBatch.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return nil
}

var insertRequestLogSQL = buildInsertSQL("request_logs", requestLogColumns)

func buildInsertSQL(table string, columns []string) string {
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = "$" + strconv.Itoa(i+1)
	}
	return "INSERT INTO " + table + " (" + strings.Join(columns, ", ") + ") VALUES (" +
		strings.Join(placeholders, ", ") + ")"
}

func sourceAddr(ip *string) any {
	if ip == nil {
		return nil
	}
	addr, err := netip.ParseAddr(*ip)
	if err != nil {
		return nil
	}
	return addr
}
