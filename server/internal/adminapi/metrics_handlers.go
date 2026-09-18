package adminapi

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
)

// metricSample is one labelled value of a metric family: labels are flat
// key/value pairs, so {"provider_id", "abc"} renders as {provider_id="abc"}.
type metricSample struct {
	labels []string
	value  float64
}

func formatLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels)/2)
	for i := 0; i+1 < len(labels); i += 2 {
		parts = append(parts, labels[i]+"="+strconv.Quote(labels[i+1]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func (h *statusHandlers) metrics(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	writeSeries := func(name, typ, help string, samples ...metricSample) {
		b.WriteString("# HELP " + name + " " + help + "\n")
		b.WriteString("# TYPE " + name + " " + typ + "\n")
		for _, s := range samples {
			b.WriteString(name)
			b.WriteString(formatLabels(s.labels))
			b.WriteString(" " + strconv.FormatFloat(s.value, 'f', -1, 64) + "\n")
		}
	}

	write := func(name, typ, help string, value float64, labels ...string) {
		writeSeries(name, typ, help, metricSample{labels: labels, value: value})
	}

	write("mesh_goroutines", "gauge", "Goroutines currently running.", float64(runtime.NumGoroutine()))

	if h.deps.MemState != nil {
		tripped := 0.0
		if h.deps.MemState.KillSwitchTripped() {
			tripped = 1
		}
		write("mesh_kill_switch", "gauge", "1 when an admin has paused all gateway traffic.", tripped)
	}

	if h.deps.Telemetry != nil {
		s := h.deps.Telemetry.Stats()
		write("mesh_telemetry_queued_events", "gauge", "Success events buffered awaiting a flush.", float64(s.QueuedEvents))
		write("mesh_telemetry_queued_denials", "gauge", "Denial events buffered awaiting a flush.", float64(s.QueuedDenials))
		write("mesh_telemetry_written_total", "counter", "Request log rows written.", float64(s.Written))
		write("mesh_telemetry_dropped_success_total", "counter", "Success events dropped because the channel was full.", float64(s.DroppedSuccess))
		write("mesh_telemetry_dropped_denied_total", "counter", "Denial events dropped because the channel was full.", float64(s.DroppedDenied))
		write("mesh_telemetry_insert_failures_total", "counter", "Request log rows lost to a failed insert.", float64(s.InsertFailures))
	}

	if h.deps.Queue != nil {
		s := h.deps.Queue.Stats()
		write("mesh_queue_capacity", "gauge", "Total admission budget units.", float64(s.Capacity))
		write("mesh_queue_in_flight_cost", "gauge", "Admission budget units currently held.", float64(s.InFlightCost))
		write("mesh_queue_load_factor", "gauge", "Multiplier applied to every request's admission cost.", float64(s.LoadFactor))
	}

	if h.deps.MemState != nil {
		stats := h.deps.MemState.ProviderLimiterStats()
		inFlight := make([]metricSample, 0, len(stats))
		maxInFlight := make([]metricSample, 0, len(stats))
		maxRPS := make([]metricSample, 0, len(stats))
		for _, p := range stats {
			l := []string{"provider_id", p.ProviderID}
			inFlight = append(inFlight, metricSample{l, float64(p.InFlight)})
			maxInFlight = append(maxInFlight, metricSample{l, float64(p.MaxInFlight)})
			maxRPS = append(maxRPS, metricSample{l, float64(p.MaxRPS)})
		}
		writeSeries("mesh_provider_in_flight", "gauge", "Upstream calls currently open to a provider.", inFlight...)
		writeSeries("mesh_provider_max_in_flight", "gauge", "Configured concurrency ceiling (0 = unlimited).", maxInFlight...)
		writeSeries("mesh_provider_max_rps", "gauge", "Configured outbound rate ceiling (0 = unlimited).", maxRPS...)
	}

	pool := func(prefix string, acquired, idle, total, max, wait float64) {
		write(prefix+"_acquired", "gauge", "Connections currently checked out.", acquired)
		write(prefix+"_idle", "gauge", "Connections currently idle.", idle)
		write(prefix+"_total", "gauge", "Connections currently open.", total)
		write(prefix+"_max", "gauge", "Maximum connections this pool may open.", max)
		write(prefix+"_empty_acquire_total", "counter", "Acquires that had to wait for a free connection.", wait)
	}
	if h.deps.GatewayPool != nil {
		s := h.deps.GatewayPool.Stat()
		pool("mesh_gateway_pool", float64(s.AcquiredConns()), float64(s.IdleConns()),
			float64(s.TotalConns()), float64(s.MaxConns()), float64(s.EmptyAcquireCount()))
	}
	if h.deps.AdminPool != nil {
		s := h.deps.AdminPool.Stat()
		pool("mesh_admin_pool", float64(s.AcquiredConns()), float64(s.IdleConns()),
			float64(s.TotalConns()), float64(s.MaxConns()), float64(s.EmptyAcquireCount()))
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprint(w, b.String())
}
