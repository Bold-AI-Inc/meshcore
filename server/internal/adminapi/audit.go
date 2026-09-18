package adminapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"mesh-server/internal/httputil"
	"mesh-server/internal/memstate"
	"mesh-server/internal/store"
)

func logAudit(ctx context.Context, audit *store.AuditStore, adminEmail, action, target string, detail any) {
	var detailJSON *string
	if detail != nil {
		encoded, err := json.Marshal(detail)
		if err != nil {
			slog.Error("audit log marshal failed", "action", action, "target", target, "err", err)
		} else {
			s := string(encoded)
			detailJSON = &s
		}
	}
	if err := audit.Log(ctx, adminEmail, action, target, detailJSON); err != nil {
		slog.Error("audit log write failed", "admin", adminEmail, "action", action, "target", target, "err", err)
	}
}

var (
	rebuildMu   sync.Mutex
	rebuildSeq  atomic.Uint64
	rebuildDone atomic.Uint64
)

func rebuildMemState(_ context.Context, mem *memstate.Store) {
	want := rebuildSeq.Add(1)
	for rebuildDone.Load() < want {
		rebuildMu.Lock()
		if rebuildDone.Load() >= want {
			rebuildMu.Unlock()
			return
		}
		covers := rebuildSeq.Load()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := mem.Rebuild(ctx)
		cancel()
		if err != nil {
			rebuildMu.Unlock()
			slog.Error("memstate rebuild failed", "err", err)
			return
		}
		rebuildDone.Store(covers)
		rebuildMu.Unlock()
	}
}

func (d Deps) clientIP(r *http.Request) string {
	return httputil.ClientIP(r, d.TrustProxyHeader)
}
