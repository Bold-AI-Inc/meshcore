package reqqueue

import (
	"context"
	"runtime"
	"runtime/metrics"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"

	"mesh-server/internal/providerfamily"
	"mesh-server/internal/supervise"
)

const (
	CostText     int64 = 1
	CostImage    int64 = 5
	CostDocument int64 = 8
	CostAudio    int64 = 10
	CostVideo    int64 = 25
)

type Config struct {
	Capacity         int64
	AdmitTimeout     time.Duration
	PollInterval     time.Duration
	GoroutineCeiling int
	HeapCeilingBytes uint64
}

func DefaultConfig() Config {
	return Config{
		Capacity:         200,
		AdmitTimeout:     4 * time.Second,
		PollInterval:     3 * time.Second,
		GoroutineCeiling: 5000,
		HeapCeilingBytes: 1 << 30,
	}
}

type Queue struct {
	cfg Config

	sem *semaphore.Weighted

	inFlight   atomic.Int64
	loadFactor atomic.Int64
	stop       chan struct{}
}

func New(cfg Config) *Queue {
	q := &Queue{
		cfg:  cfg,
		sem:  semaphore.NewWeighted(cfg.Capacity),
		stop: make(chan struct{}),
	}
	q.loadFactor.Store(1)
	supervise.Go("reqqueue-load-poller", q.pollLoad)
	return q
}

func (q *Queue) Stop() {
	close(q.stop)
}

func (q *Queue) EffectiveCost(baseCost int64) int64 {
	cost := baseCost * q.loadFactor.Load()
	if cost > q.cfg.Capacity {
		cost = q.cfg.Capacity
	}
	if cost < 1 {
		cost = 1
	}
	return cost
}

type Reservation struct {
	q        *Queue
	held     atomic.Int64
	released atomic.Bool
}

func (r *Reservation) Grow(ctx context.Context, baseCost int64) bool {
	if r == nil {
		return true
	}
	want := r.q.EffectiveCost(baseCost)
	extra := want - r.held.Load()
	if extra <= 0 {
		return true
	}
	if err := r.q.sem.Acquire(ctx, extra); err != nil {
		return false
	}
	r.held.Add(extra)
	r.q.inFlight.Add(extra)
	return true
}

func (r *Reservation) Release() {
	if r == nil || !r.released.CompareAndSwap(false, true) {
		return
	}
	held := r.held.Swap(0)
	if held > 0 {
		r.q.sem.Release(held)
		r.q.inFlight.Add(-held)
	}
}

func (q *Queue) Acquire(ctx context.Context, baseCost int64) (res *Reservation, ok bool) {
	cost := q.EffectiveCost(baseCost)
	if err := q.sem.Acquire(ctx, cost); err != nil {
		return nil, false
	}
	q.inFlight.Add(cost)
	r := &Reservation{q: q}
	r.held.Store(cost)
	return r, true
}

type Stats struct {
	Capacity     int64 `json:"capacity"`
	InFlightCost int64 `json:"in_flight_cost"`
	LoadFactor   int64 `json:"load_factor"`
	Goroutines   int   `json:"goroutines"`
}

func (q *Queue) Stats() Stats {
	return Stats{
		Capacity:     q.cfg.Capacity,
		InFlightCost: q.inFlight.Load(),
		LoadFactor:   q.loadFactor.Load(),
		Goroutines:   runtime.NumGoroutine(),
	}
}

func (q *Queue) pollLoad() {
	ticker := time.NewTicker(q.cfg.PollInterval)
	defer ticker.Stop()

	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}

	for {
		select {
		case <-q.stop:
			select {}
		case <-ticker.C:
			metrics.Read(samples)
			heapInUse := samples[0].Value.Uint64()
			goroutinePressure := ratio(runtime.NumGoroutine(), q.cfg.GoroutineCeiling)
			heapPressure := ratioU64(heapInUse, q.cfg.HeapCeilingBytes)
			pressure := goroutinePressure
			if heapPressure > pressure {
				pressure = heapPressure
			}

			switch {
			case pressure >= 0.90:
				q.loadFactor.Store(4)
			case pressure >= 0.70:
				q.loadFactor.Store(2)
			default:
				q.loadFactor.Store(1)
			}
		}
	}
}

func ratio(value, ceiling int) float64 {
	if ceiling <= 0 {
		return 0
	}
	return float64(value) / float64(ceiling)
}

func ratioU64(value, ceiling uint64) float64 {
	if ceiling == 0 {
		return 0
	}
	return float64(value) / float64(ceiling)
}

func BlockCost(blockType string) int64 {
	switch blockType {
	case "image":
		return CostImage
	case "document":
		return CostDocument
	case "audio":
		return CostAudio
	case "video":
		return CostVideo
	default:
		return CostText
	}
}

func BytesCost(contentLength int64) int64 {
	if contentLength <= 0 {
		return CostText
	}
	cost := contentLength / (256 << 10)
	if cost < CostText {
		return CostText
	}
	return cost
}

func RequestCost(blocks []providerfamily.Block) int64 {
	var total int64
	for _, b := range blocks {
		total += BlockCost(string(b.Type))
	}
	if total < 1 {
		total = 1
	}
	return total
}
