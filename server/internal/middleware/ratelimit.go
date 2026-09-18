package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"mesh-server/internal/httputil"
)

const (
	rateLimiterIdleTTL   = 10 * time.Minute
	rateLimiterMaxIdle   = 10000
	rateLimiterSweepEach = time.Minute
)

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type IPRateLimiter struct {
	mu         sync.Mutex
	limiters   map[string]*ipLimiter
	r          rate.Limit
	burst      int
	trustProxy bool
	lastSweep  time.Time
}

func NewIPRateLimiter(r rate.Limit, burst int, trustProxy bool) *IPRateLimiter {
	return &IPRateLimiter{
		limiters:   make(map[string]*ipLimiter),
		r:          r,
		burst:      burst,
		trustProxy: trustProxy,
		lastSweep:  time.Now(),
	}
}

func (i *IPRateLimiter) get(ip string) *rate.Limiter {
	now := time.Now()
	i.mu.Lock()
	defer i.mu.Unlock()

	if now.Sub(i.lastSweep) >= rateLimiterSweepEach || len(i.limiters) > rateLimiterMaxIdle {
		for k, e := range i.limiters {
			if now.Sub(e.lastSeen) > rateLimiterIdleTTL {
				delete(i.limiters, k)
			}
		}
		i.lastSweep = now
	}

	entry, exists := i.limiters[ip]
	if !exists {
		entry = &ipLimiter{limiter: rate.NewLimiter(i.r, i.burst)}
		i.limiters[ip] = entry
	}
	entry.lastSeen = now
	return entry.limiter
}

func (i *IPRateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !i.get(httputil.ClientIP(r, i.trustProxy)).Allow() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
