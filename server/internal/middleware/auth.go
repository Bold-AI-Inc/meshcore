package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"mesh-server/internal/security"
	"mesh-server/internal/store"
)

type contextKey string

const AdminIDKey contextKey = "adminID"
const AdminEmailKey contextKey = "adminEmail"

const (
	sessionCacheTTL = 5 * time.Second

	sessionCacheMaxEntries = 4096

	sessionCacheSweepInterval = time.Minute
)

type sessionCacheEntry struct {
	admin     store.AuthedAdmin
	expiresAt time.Time
}

type SessionCache struct {
	mu    sync.Mutex
	items map[string]sessionCacheEntry
	stop  chan struct{}
}

func NewSessionCache() *SessionCache {
	c := &SessionCache{
		items: make(map[string]sessionCacheEntry),
		stop:  make(chan struct{}),
	}
	go c.sweep()
	return c
}

func (c *SessionCache) sweep() {
	ticker := time.NewTicker(sessionCacheSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case now := <-ticker.C:
			c.mu.Lock()
			for k, entry := range c.items {
				if now.After(entry.expiresAt) {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *SessionCache) Close() { close(c.stop) }

func (c *SessionCache) get(tokenHash string) (store.AuthedAdmin, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[tokenHash]
	if !ok {
		return store.AuthedAdmin{}, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.items, tokenHash)
		return store.AuthedAdmin{}, false
	}
	return entry.admin, true
}

func (c *SessionCache) set(tokenHash string, admin store.AuthedAdmin) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= sessionCacheMaxEntries {
		now := time.Now()
		for k, entry := range c.items {
			if now.After(entry.expiresAt) {
				delete(c.items, k)
			}
		}
		if len(c.items) >= sessionCacheMaxEntries {
			return
		}
	}
	c.items[tokenHash] = sessionCacheEntry{admin: admin, expiresAt: time.Now().Add(sessionCacheTTL)}
}

func (c *SessionCache) Invalidate(tokenHash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, tokenHash)
}

func (c *SessionCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.items)
}

func RequireSession(sessions *store.SessionStore, cache *SessionCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("mesh_session")
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			tokenHash := security.HashToken(cookie.Value)

			admin, ok := cache.get(tokenHash)
			if !ok {
				found, err := sessions.FindActiveAdmin(r.Context(), tokenHash)
				if err != nil || found.Status != "active" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				admin = *found
				cache.set(tokenHash, admin)
			}

			ctx := context.WithValue(r.Context(), AdminIDKey, admin.ID)
			ctx = context.WithValue(ctx, AdminEmailKey, admin.Email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
