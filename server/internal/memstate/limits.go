package memstate

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

type ProviderLimiter struct {
	mu      sync.Mutex
	rps     int
	maxConc int

	rl *rate.Limiter

	slots chan struct{}
}

func newProviderLimiter(rps, maxConc int) *ProviderLimiter {
	l := &ProviderLimiter{}
	l.SetLimits(rps, maxConc)
	return l
}

func (l *ProviderLimiter) SetLimits(rps, maxConc int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if rps != l.rps {
		l.rps = rps
		switch {
		case rps <= 0:
			l.rl = nil
		case l.rl == nil:
			l.rl = rate.NewLimiter(rate.Limit(rps), rps)
		default:
			l.rl.SetLimit(rate.Limit(rps))
			l.rl.SetBurst(rps)
		}
	}

	if maxConc != l.maxConc {
		l.maxConc = maxConc
		if maxConc <= 0 {
			l.slots = nil
		} else {
			slots := make(chan struct{}, maxConc)
			for i := 0; i < maxConc; i++ {
				slots <- struct{}{}
			}
			l.slots = slots
		}
	}
}

func (l *ProviderLimiter) Acquire(ctx context.Context) (release func(), err error) {
	if l == nil {
		return func() {}, nil
	}

	l.mu.Lock()
	rl, slots := l.rl, l.slots
	l.mu.Unlock()

	if rl != nil {
		if err := rl.Wait(ctx); err != nil {
			return nil, err
		}
	}

	if slots == nil {
		return func() {}, nil
	}

	select {
	case <-slots:
		var once sync.Once
		return func() {
			once.Do(func() {
				select {
				case slots <- struct{}{}:
				default:
				}
			})
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (l *ProviderLimiter) InFlight() (inFlight, limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.slots == nil {
		return 0, 0
	}
	return l.maxConc - len(l.slots), l.maxConc
}

func (s *Store) providerLimiter(providerID string, rps, maxConc int) *ProviderLimiter {
	if v, ok := s.providerLimiters.Load(providerID); ok {
		l := v.(*ProviderLimiter)
		l.SetLimits(rps, maxConc)
		return l
	}
	actual, loaded := s.providerLimiters.LoadOrStore(providerID, newProviderLimiter(rps, maxConc))
	l := actual.(*ProviderLimiter)
	if loaded {
		l.SetLimits(rps, maxConc)
	}
	return l
}

type ProviderLimiterStats struct {
	ProviderID  string `json:"provider_id"`
	InFlight    int    `json:"in_flight"`
	MaxInFlight int    `json:"max_in_flight"`
	MaxRPS      int    `json:"max_rps"`
}

func (s *Store) ProviderLimiterStats() []ProviderLimiterStats {
	var out []ProviderLimiterStats
	s.providerLimiters.Range(func(k, v any) bool {
		l := v.(*ProviderLimiter)
		inFlight, limit := l.InFlight()
		l.mu.Lock()
		rps := l.rps
		l.mu.Unlock()
		out = append(out, ProviderLimiterStats{
			ProviderID:  k.(string),
			InFlight:    inFlight,
			MaxInFlight: limit,
			MaxRPS:      rps,
		})
		return true
	})
	return out
}
