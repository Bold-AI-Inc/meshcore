package adminapi

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"mesh-server/internal/gateway"
	"mesh-server/internal/memstate"
	"mesh-server/internal/middleware"
	"mesh-server/internal/reqqueue"
	"mesh-server/internal/store"
)

type Deps struct {
	Admins        *store.AdminStore
	Sessions      *store.SessionStore
	Users         *store.UserStore
	MeshKeys      *store.MeshKeyStore
	Providers     *store.ProviderStore
	Models        *store.ModelStore
	Access        *store.AccessStore
	Policies      *store.PolicyStore
	RequestLogs   *store.RequestLogStore
	Audit         *store.AuditStore
	MemState      *memstate.Store
	SessionCache  *middleware.SessionCache
	CookieSecure  bool
	MeshKeyPepper string
	MasterKey     string

	SuperAdminEmail string

	// MetricsToken, when non-empty, also authorises GET /admin/metrics via
	// `Authorization: Bearer <token>` so a scraper can reach it without a
	// browser session cookie.
	MetricsToken string

	TrustProxyHeader bool

	Telemetry   *gateway.Telemetry
	Queue       *reqqueue.Queue
	AdminPool   *pgxpool.Pool
	GatewayPool *pgxpool.Pool
}

func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()
	h := &authHandlers{deps: deps}
	u := &userHandlers{deps: deps}
	p := &providerHandlers{deps: deps}
	m := &modelHandlers{deps: deps}
	a := &accessHandlers{deps: deps}
	pol := &policyHandlers{deps: deps}
	lg := &logHandlers{deps: deps}
	st := &statusHandlers{deps: deps}

	loginLimiter := middleware.NewIPRateLimiter(rate.Every(time.Second), 5, deps.TrustProxyHeader)
	requireSession := middleware.RequireSession(deps.Sessions, deps.SessionCache)
	protected := func(next http.Handler) http.Handler {
		return requireSession(requireCSRF(next))
	}

	mux.Handle("POST /admin/login", loginLimiter.Limit(http.HandlerFunc(h.login)))
	mux.Handle("POST /admin/logout", protected(http.HandlerFunc(h.logout)))
	mux.Handle("GET /admin/me", protected(http.HandlerFunc(h.me)))
	mux.Handle("GET /admin/status", protected(http.HandlerFunc(st.status)))
	mux.Handle("GET /admin/metrics", deps.requireMetricsAuth(requireSession, http.HandlerFunc(st.metrics)))
	mux.Handle("POST /admin/change-password", protected(http.HandlerFunc(h.changePassword)))
	mux.Handle("POST /admin/kill-switch", protected(http.HandlerFunc(st.setKillSwitch)))

	mux.Handle("POST /admin/admins", protected(http.HandlerFunc(h.createAdmin)))
	mux.Handle("GET /admin/admins", protected(http.HandlerFunc(h.listAdmins)))
	mux.Handle("POST /admin/admins/{id}/pause", protected(http.HandlerFunc(h.pauseAdmin)))
	mux.Handle("POST /admin/admins/{id}/resume", protected(http.HandlerFunc(h.resumeAdmin)))
	mux.Handle("DELETE /admin/admins/{id}", protected(http.HandlerFunc(h.deleteAdmin)))

	mux.Handle("POST /admin/users", protected(http.HandlerFunc(u.create)))
	mux.Handle("GET /admin/users", protected(http.HandlerFunc(u.list)))
	mux.Handle("PUT /admin/users/{id}/expiry", protected(http.HandlerFunc(u.updateExpiry)))
	mux.Handle("POST /admin/users/{id}/revoke", protected(http.HandlerFunc(u.revoke)))
	mux.Handle("POST /admin/users/{id}/rotate-key", protected(http.HandlerFunc(u.rotateKey)))
	mux.Handle("DELETE /admin/users/{id}", protected(http.HandlerFunc(u.delete)))
	mux.Handle("GET /admin/users/{id}/policies", protected(http.HandlerFunc(pol.list)))
	mux.Handle("PUT /admin/users/{id}/policies/{modelId}", protected(http.HandlerFunc(pol.upsert)))
	mux.Handle("GET /admin/users/{id}/logs", protected(http.HandlerFunc(lg.list)))
	mux.Handle("GET /admin/users/{id}/logs/summary", protected(http.HandlerFunc(lg.summary)))

	mux.Handle("GET /admin/logs", protected(http.HandlerFunc(lg.list)))
	mux.Handle("GET /admin/logs/summary", protected(http.HandlerFunc(lg.summary)))

	mux.Handle("POST /admin/providers", protected(http.HandlerFunc(p.create)))
	mux.Handle("GET /admin/providers", protected(http.HandlerFunc(p.list)))
	mux.Handle("PUT /admin/providers/{id}", protected(http.HandlerFunc(p.update)))
	mux.Handle("DELETE /admin/providers/{id}", protected(http.HandlerFunc(p.delete)))

	mux.Handle("POST /admin/models", protected(http.HandlerFunc(m.create)))
	mux.Handle("GET /admin/models", protected(http.HandlerFunc(m.list)))
	mux.Handle("PUT /admin/models/{id}", protected(http.HandlerFunc(m.update)))
	mux.Handle("DELETE /admin/models/{id}", protected(http.HandlerFunc(m.delete)))

	mux.Handle("POST /admin/access", protected(http.HandlerFunc(a.grant)))
	mux.Handle("DELETE /admin/access", protected(http.HandlerFunc(a.revoke)))
	mux.Handle("GET /admin/access", protected(http.HandlerFunc(a.list)))

	return mux
}
