package adminapi

import (
	"crypto/subtle"
	"net/http"
	"runtime"

	"mesh-server/internal/httputil"
)

type statusHandlers struct {
	deps Deps
}

func (h *statusHandlers) status(w http.ResponseWriter, r *http.Request) {
	payload := map[string]any{
		"goroutines": runtime.NumGoroutine(),
	}

	if h.deps.MemState != nil {
		payload["kill_switch"] = h.deps.MemState.KillSwitchTripped()
	}

	if h.deps.Telemetry != nil {
		payload["telemetry"] = h.deps.Telemetry.Stats()
	}
	if h.deps.Queue != nil {
		payload["queue"] = h.deps.Queue.Stats()
	}
	if h.deps.MemState != nil {
		payload["providers"] = h.deps.MemState.ProviderLimiterStats()
	}
	if h.deps.GatewayPool != nil {
		s := h.deps.GatewayPool.Stat()
		payload["gateway_pool"] = map[string]any{
			"acquired":   s.AcquiredConns(),
			"idle":       s.IdleConns(),
			"total":      s.TotalConns(),
			"max":        s.MaxConns(),
			"wait_count": s.EmptyAcquireCount(),
		}
	}
	if h.deps.AdminPool != nil {
		s := h.deps.AdminPool.Stat()
		payload["admin_pool"] = map[string]any{
			"acquired":   s.AcquiredConns(),
			"idle":       s.IdleConns(),
			"total":      s.TotalConns(),
			"max":        s.MaxConns(),
			"wait_count": s.EmptyAcquireCount(),
		}
	}

	httputil.WriteJSON(w, http.StatusOK, payload)
}

type killSwitchRequest struct {
	Tripped bool `json:"tripped"`
}

// setKillSwitch flips the gateway-wide pause that makes every /v1 call answer
// with paused_by_admin. The flag is in-process and deliberately not persisted:
// it is an incident brake, so it clears on restart, and each instance behind a
// load balancer has to be tripped separately.
func (h *statusHandlers) setKillSwitch(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.deps.requireSuperAdmin(w, r)
	if !ok {
		return
	}

	var req killSwitchRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if h.deps.MemState == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "memstate_unavailable")
		return
	}

	h.deps.MemState.SetKillSwitch(req.Tripped)

	action := "resume_gateway"
	if req.Tripped {
		action = "pause_gateway"
	}
	logAudit(r.Context(), h.deps.Audit, actor, action, "gateway", map[string]string{"ip": h.deps.clientIP(r)})

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"kill_switch": req.Tripped})
}

// requireMetricsAuth lets a scraper reach /admin/metrics with a bearer token
// instead of an admin session cookie, which is the only credential a
// Prometheus scrape config can actually present. With METRICS_TOKEN unset the
// endpoint stays session-only, exactly as before.
func (d Deps) requireMetricsAuth(sessionAuth func(http.Handler) http.Handler, next http.Handler) http.Handler {
	viaSession := sessionAuth(next)
	if d.MetricsToken == "" {
		return viaSession
	}
	want := []byte("Bearer " + d.MetricsToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		viaSession.ServeHTTP(w, r)
	})
}
