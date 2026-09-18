package memstate

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const NanosPerUSD = 1_000_000_000

func USDToNanos(usd float64) int64 {
	if usd <= 0 {
		return 0
	}
	return int64(usd*NanosPerUSD + 0.5)
}

func NanosToUSD(nanos int64) float64 {
	return float64(nanos) / NanosPerUSD
}

func spendKey(userID, modelID string) string { return userID + ":" + modelID }

func ensureCounter(m *sync.Map, key string) *atomic.Int64 {
	if v, ok := m.Load(key); ok {
		return v.(*atomic.Int64)
	}
	actual, _ := m.LoadOrStore(key, new(atomic.Int64))
	return actual.(*atomic.Int64)
}

func (s *Store) EnsureProviderCounter(providerID string) {
	ensureCounter(&s.providerHourlyCalls, providerID)
}

func (s *Store) EnsureSpendCounter(userID, modelID string) {
	ensureCounter(&s.dailySpendNanos, spendKey(userID, modelID))
	ensureCounter(&s.dailySpendSynced, spendKey(userID, modelID))
}

func (s *Store) IncrProviderHourly(providerID string) int64 {
	return ensureCounter(&s.providerHourlyCalls, providerID).Add(1)
}

func (s *Store) ProviderHourlyCount(providerID string) int64 {
	return ensureCounter(&s.providerHourlyCalls, providerID).Load()
}

func (s *Store) AddDailySpendNanos(userID, modelID string, nanos int64) int64 {
	return ensureCounter(&s.dailySpendNanos, spendKey(userID, modelID)).Add(nanos)
}

func (s *Store) AddDailySpendUSD(userID, modelID string, usd float64) int64 {
	return s.AddDailySpendNanos(userID, modelID, USDToNanos(usd))
}

func (s *Store) DailySpendNanos(userID, modelID string) int64 {
	return ensureCounter(&s.dailySpendNanos, spendKey(userID, modelID)).Load()
}

func (s *Store) DailySpendUSD(userID, modelID string) float64 {
	return NanosToUSD(s.DailySpendNanos(userID, modelID))
}

func (s *Store) EnsureUserModelHourlyCounter(userID, modelID string) {
	ensureCounter(&s.userModelHourlyCalls, spendKey(userID, modelID))
}

func (s *Store) IncrUserModelHourly(userID, modelID string) int64 {
	return ensureCounter(&s.userModelHourlyCalls, spendKey(userID, modelID)).Add(1)
}

func (s *Store) UserModelHourlyCount(userID, modelID string) int64 {
	return ensureCounter(&s.userModelHourlyCalls, spendKey(userID, modelID)).Load()
}

func (s *Store) CheckpointDailySpend(ctx context.Context, today time.Time) error {
	return s.checkpointDailySpend(ctx, today, false)
}

type spendRow struct {
	userID, modelID string
	key             string
	current         int64
	delta           int64
}

func (s *Store) checkpointDailySpend(ctx context.Context, day time.Time, reset bool) error {
	var rows []spendRow
	s.dailySpendNanos.Range(func(k, v any) bool {
		key := k.(string)
		current := v.(*atomic.Int64).Load()
		synced := ensureCounter(&s.dailySpendSynced, key).Load()
		delta := current - synced
		if delta == 0 && !reset {
			return true
		}
		userID, modelID, ok := splitSpendKey(key)
		if ok {
			rows = append(rows, spendRow{userID, modelID, key, current, delta})
		}
		return true
	})
	if len(rows) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(
			`INSERT INTO daily_spend (user_id, model_id, date, total_cost_usd)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (user_id, model_id, date)
			 DO UPDATE SET total_cost_usd = daily_spend.total_cost_usd + $4
			 RETURNING total_cost_usd`,
			r.userID, r.modelID, day.Format("2006-01-02"), NanosToUSD(r.delta),
		)
	}

	results := s.pool.SendBatch(ctx, batch)
	totals := make([]float64, len(rows))
	var firstErr error
	for i := range rows {
		var total float64
		if err := results.QueryRow().Scan(&total); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			totals[i] = -1
			continue
		}
		totals[i] = total
	}
	if err := results.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if firstErr != nil {
		firstErr = nil
		for i, r := range rows {
			if totals[i] >= 0 {
				continue
			}
			var total float64
			err := s.pool.QueryRow(ctx,
				`INSERT INTO daily_spend (user_id, model_id, date, total_cost_usd)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT (user_id, model_id, date)
				 DO UPDATE SET total_cost_usd = daily_spend.total_cost_usd + $4
				 RETURNING total_cost_usd`,
				r.userID, r.modelID, day.Format("2006-01-02"), NanosToUSD(r.delta),
			).Scan(&total)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				totals[i] = -1
				continue
			}
			totals[i] = total
		}
	}

	for i, r := range rows {
		if totals[i] < 0 {
			continue
		}
		counter := ensureCounter(&s.dailySpendNanos, r.key)
		synced := ensureCounter(&s.dailySpendSynced, r.key)
		if reset {
			counter.Add(-r.current)
			synced.Store(0)
			continue
		}
		dbNanos := USDToNanos(totals[i])
		counter.Add(dbNanos - r.current)
		synced.Store(dbNanos)
	}
	return firstErr
}

func (s *Store) CheckpointAndResetDailySpend(ctx context.Context, day time.Time) error {
	return s.checkpointDailySpend(ctx, day, true)
}

func (s *Store) CheckpointAndResetHourly(ctx context.Context, hourStart time.Time) error {
	type row struct {
		providerID string
		count      int64
	}
	var rows []row
	s.providerHourlyCalls.Range(func(k, v any) bool {
		counter := v.(*atomic.Int64)
		count := counter.Swap(0)
		rows = append(rows, row{k.(string), count})
		return true
	})
	if len(rows) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(
			`INSERT INTO provider_hourly_usage (provider_id, hour_start, call_count)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (provider_id, hour_start) DO UPDATE SET call_count = provider_hourly_usage.call_count + $3`,
			r.providerID, hourStart, r.count,
		)
	}
	if err := runBatch(ctx, s.pool, batch); err != nil {
		for _, r := range rows {
			ensureCounter(&s.providerHourlyCalls, r.providerID).Add(r.count)
		}
		return err
	}
	return nil
}

func (s *Store) CheckpointAndResetUserModelHourly(ctx context.Context, hourStart time.Time) error {
	type row struct {
		userID, modelID string
		count           int64
	}
	var rows []row
	s.userModelHourlyCalls.Range(func(k, v any) bool {
		counter := v.(*atomic.Int64)
		count := counter.Swap(0)
		userID, modelID, ok := splitSpendKey(k.(string))
		if ok {
			rows = append(rows, row{userID, modelID, count})
		}
		return true
	})
	if len(rows) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(
			`INSERT INTO user_model_hourly_usage (user_id, model_id, hour_start, call_count)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (user_id, model_id, hour_start) DO UPDATE SET call_count = user_model_hourly_usage.call_count + $4`,
			r.userID, r.modelID, hourStart, r.count,
		)
	}
	if err := runBatch(ctx, s.pool, batch); err != nil {
		for _, r := range rows {
			ensureCounter(&s.userModelHourlyCalls, spendKey(r.userID, r.modelID)).Add(r.count)
		}
		return err
	}
	return nil
}

func runBatch(ctx context.Context, pool *pgxpool.Pool, batch *pgx.Batch) error {
	results := pool.SendBatch(ctx, batch)
	defer results.Close()
	for i := 0; i < batch.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SeedFromPostgres(ctx context.Context, now time.Time) error {
	rows, err := s.pool.Query(ctx,
		`SELECT user_id, model_id, total_cost_usd FROM daily_spend WHERE date = $1`,
		now.Format("2006-01-02"),
	)
	if err != nil {
		return err
	}
	for rows.Next() {
		var userID, modelID string
		var totalUSD float64
		if err := rows.Scan(&userID, &modelID, &totalUSD); err != nil {
			rows.Close()
			return err
		}
		nanos := USDToNanos(totalUSD)
		ensureCounter(&s.dailySpendNanos, spendKey(userID, modelID)).Store(nanos)
		ensureCounter(&s.dailySpendSynced, spendKey(userID, modelID)).Store(nanos)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	hourStart := now.Truncate(time.Hour)
	rows, err = s.pool.Query(ctx,
		`SELECT provider_id, call_count FROM provider_hourly_usage WHERE hour_start = $1`,
		hourStart,
	)
	if err != nil {
		return err
	}
	for rows.Next() {
		var providerID string
		var count int64
		if err := rows.Scan(&providerID, &count); err != nil {
			rows.Close()
			return err
		}
		ensureCounter(&s.providerHourlyCalls, providerID).Store(count)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = s.pool.Query(ctx,
		`SELECT user_id, model_id, call_count FROM user_model_hourly_usage WHERE hour_start = $1`,
		hourStart,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var userID, modelID string
		var count int64
		if err := rows.Scan(&userID, &modelID, &count); err != nil {
			return err
		}
		ensureCounter(&s.userModelHourlyCalls, spendKey(userID, modelID)).Store(count)
	}
	return rows.Err()
}

func splitSpendKey(key string) (userID, modelID string, ok bool) {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return key[:i], key[i+1:], true
		}
	}
	return "", "", false
}

func (s *Store) Prune(activeSpendKeys, activeProviderIDs map[string]bool) {
	pruneMap := func(m *sync.Map, active map[string]bool) {
		m.Range(func(k, _ any) bool {
			if !active[k.(string)] {
				m.Delete(k)
			}
			return true
		})
	}
	pruneMap(&s.dailySpendNanos, activeSpendKeys)
	pruneMap(&s.dailySpendSynced, activeSpendKeys)
	pruneMap(&s.userModelHourlyCalls, activeSpendKeys)
	pruneMap(&s.providerHourlyCalls, activeProviderIDs)
	pruneMap(&s.providerLimiters, activeProviderIDs)
}
