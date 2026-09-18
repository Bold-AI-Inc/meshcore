package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mesh-server/internal/domain"
)

type RequestLogStore struct {
	pool *pgxpool.Pool
}

func NewRequestLogStore(pool *pgxpool.Pool) *RequestLogStore {
	return &RequestLogStore{pool: pool}
}

const logsFromJoin = `FROM request_logs r
	LEFT JOIN models m ON m.id = r.model_id
	LEFT JOIN providers p ON p.id = r.provider_id
	LEFT JOIN users u ON u.id = r.user_id`

var sortColumns = map[string]string{
	"created_at":  "r.created_at",
	"latency_ms":  "r.latency_ms",
	"status_code": "r.status_code",
	"tokens_in":   "r.tokens_in",
	"tokens_out":  "r.tokens_out",
}

const testTrafficClause = `(
	COALESCE(p.name, '') ILIKE 'testserve%' OR COALESCE(p.name, '') ILIKE 'test-%'
	OR COALESCE(m.name, '') ILIKE 'ts-%'
	OR COALESCE(r.requested_model, '') ILIKE 'ts-%'
	OR COALESCE(u.email, '') ILIKE '%@local.test'
)`

const maxSearchLen = 128

func buildLogsWhere(f domain.RequestLogFilter) (string, []any) {
	args := []any{f.From, f.To}
	clauses := []string{"r.created_at >= $1", "r.created_at < $2"}

	if f.UserID != "" {
		args = append(args, f.UserID)
		clauses = append(clauses, fmt.Sprintf("r.user_id = $%d", len(args)))
	}
	if f.ModelID != "" {
		args = append(args, f.ModelID)
		clauses = append(clauses, fmt.Sprintf("r.model_id = $%d", len(args)))
	}
	if f.ProviderID != "" {
		args = append(args, f.ProviderID)
		clauses = append(clauses, fmt.Sprintf("r.provider_id = $%d", len(args)))
	}
	if f.DenyReason != "" {
		args = append(args, f.DenyReason)
		clauses = append(clauses, fmt.Sprintf("r.deny_reason = $%d", len(args)))
	}
	if f.ExcludeTest {
		clauses = append(clauses, "NOT "+testTrafficClause)
	}
	switch f.Status {
	case "success":
		clauses = append(clauses, "r.outcome = 'success'")
	case "error":
		clauses = append(clauses, "r.outcome IN ('upstream_error', 'denied')")
	case "denied":
		clauses = append(clauses, "r.outcome = 'denied'")
	}
	if search := f.Search; search != "" {
		if len(search) > maxSearchLen {
			search = search[:maxSearchLen]
		}
		args = append(args, "%"+search+"%")
		i := len(args)
		clauses = append(clauses, fmt.Sprintf(
			`(m.name ILIKE $%d OR p.name ILIKE $%d OR u.email ILIKE $%d
			  OR r.requested_model ILIKE $%d OR r.deny_reason ILIKE $%d
			  OR host(r.source_ip) ILIKE $%d OR r.user_agent ILIKE $%d
			  OR r.browser ILIKE $%d OR r.os ILIKE $%d OR r.device_type ILIKE $%d)`,
			i, i, i, i, i, i, i, i, i, i,
		))
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func (s *RequestLogStore) List(ctx context.Context, f domain.RequestLogFilter) ([]domain.RequestLog, int, error) {
	where, args := buildLogsWhere(f)

	sortCol, ok := sortColumns[f.SortBy]
	if !ok {
		sortCol = "r.created_at"
	}
	dir := "DESC"
	if strings.ToLower(f.SortDir) == "asc" {
		dir = "ASC"
	}

	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 25
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	pageArgs := append(append([]any{}, args...), pageSize, offset)
	query := fmt.Sprintf(
		`SELECT r.id, r.user_id, u.email, r.model_id, m.name, p.name, r.requested_model, r.outcome, r.deny_reason,
		        host(r.source_ip), r.user_agent,
		        r.browser, r.browser_version, r.os, r.os_version, r.device_type,
		        r.status_code, r.latency_ms, r.tokens_in, r.tokens_out,
		        r.input_cost, r.output_cost, r.created_at,
		        count(*) OVER () AS total
		 %s %s
		 ORDER BY %s %s
		 LIMIT $%d OFFSET $%d`,
		logsFromJoin, where, sortCol, dir, len(pageArgs)-1, len(pageArgs),
	)

	rows, err := s.pool.Query(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs := []domain.RequestLog{}
	total := 0
	for rows.Next() {
		var l domain.RequestLog
		var sourceIP *string
		if err := rows.Scan(&l.ID, &l.UserID, &l.UserEmail, &l.ModelID, &l.ModelName, &l.ProviderName, &l.RequestedModel, &l.Outcome, &l.DenyReason,
			&sourceIP, &l.UserAgent,
			&l.Browser, &l.BrowserVersion, &l.OS, &l.OSVersion, &l.DeviceType,
			&l.StatusCode, &l.LatencyMs, &l.TokensIn, &l.TokensOut,
			&l.InputCost, &l.OutputCost, &l.CreatedAt, &total); err != nil {
			return nil, 0, err
		}
		l.SourceIP = sourceIP
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if len(logs) == 0 && page > 1 {
		countQuery := `SELECT count(*) ` + logsFromJoin + ` ` + where
		if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, err
		}
	}
	return logs, total, nil
}

func (s *RequestLogStore) Summary(ctx context.Context, f domain.RequestLogFilter) (*domain.LogsSummary, error) {
	where, args := buildLogsWhere(f)
	summary := &domain.LogsSummary{
		TimeSeries:     []domain.LogsTimeBucket{},
		ByModel:        []domain.LogsModelBreakdown{},
		ByStatus:       []domain.LogsStatusBreakdown{},
		ByUser:         []domain.LogsUserBreakdown{},
		ByDenyReason:   []domain.LogsDenyBreakdown{},
		LatencyBuckets: []domain.LogsLatencyBucket{},
	}

	bucketUnit := "day"
	if f.To.Sub(f.From).Hours() <= 48 {
		bucketUnit = "hour"
	}

	batch := &pgx.Batch{}
	batch.Queue(fmt.Sprintf(
		`SELECT count(*), COALESCE(sum(CASE WHEN r.status_code >= 400 THEN 1 ELSE 0 END), 0),
		        COALESCE(avg(r.latency_ms), 0), COALESCE(sum(r.tokens_in), 0), COALESCE(sum(r.tokens_out), 0),
		        COALESCE(sum(r.input_cost), 0), COALESCE(sum(r.output_cost), 0),
		        COALESCE(sum(CASE WHEN r.outcome = 'denied' THEN 1 ELSE 0 END), 0)
		 %s %s`, logsFromJoin, where), args...)
	tz := f.Timezone
	if tz == "" {
		tz = "UTC"
	}
	tzArgs := append(append([]any{}, args...), tz)
	tzIdx := len(tzArgs)
	batch.Queue(fmt.Sprintf(
		`SELECT date_trunc('%s', r.created_at AT TIME ZONE $%d) AT TIME ZONE $%d AS bucket, count(*),
		        COALESCE(sum(CASE WHEN r.status_code >= 400 THEN 1 ELSE 0 END), 0),
		        COALESCE(avg(r.latency_ms), 0)
		 %s %s
		 GROUP BY bucket ORDER BY bucket`, bucketUnit, tzIdx, tzIdx, logsFromJoin, where), tzArgs...)
	batch.Queue(fmt.Sprintf(
		`SELECT COALESCE(r.model_id::text, ''),
		        CASE WHEN r.model_id IS NULL THEN 'No model resolved' ELSE COALESCE(m.name, 'Deleted model') END,
		        CASE WHEN r.model_id IS NULL THEN '—' ELSE COALESCE(p.name, 'Deleted provider') END,
		        count(*),
		        COALESCE(sum(CASE WHEN r.status_code >= 400 THEN 1 ELSE 0 END), 0),
		        COALESCE(sum(r.tokens_in), 0), COALESCE(sum(r.tokens_out), 0),
		        COALESCE(sum(r.input_cost + r.output_cost), 0),
		        COALESCE(avg(r.latency_ms), 0)
		 %s %s
		 GROUP BY r.model_id, m.name, p.name ORDER BY count(*) DESC LIMIT 20`, logsFromJoin, where), args...)
	batch.Queue(fmt.Sprintf(
		`SELECT r.status_code, count(*) %s %s GROUP BY r.status_code ORDER BY count(*) DESC`,
		logsFromJoin, where), args...)
	batch.Queue(fmt.Sprintf(
		`SELECT COALESCE(r.user_id::text, ''), COALESCE(u.email, 'Unknown'), count(*),
		        COALESCE(sum(CASE WHEN r.status_code >= 400 THEN 1 ELSE 0 END), 0),
		        COALESCE(sum(r.input_cost + r.output_cost), 0)
		 %s %s
		 GROUP BY r.user_id, u.email ORDER BY count(*) DESC LIMIT 20`, logsFromJoin, where), args...)
	batch.Queue(fmt.Sprintf(
		`SELECT r.deny_reason, count(*) %s %s AND r.deny_reason IS NOT NULL
		 GROUP BY r.deny_reason ORDER BY count(*) DESC`, logsFromJoin, where), args...)
	batch.Queue(fmt.Sprintf(
		`SELECT width_bucket(r.latency_ms, ARRAY[100, 250, 500, 1000, 2500, 5000, 10000, 30000]), count(*)
		 %s %s AND r.latency_ms IS NOT NULL
		 GROUP BY 1 ORDER BY 1`, logsFromJoin, where), args...)

	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()

	if err := results.QueryRow().Scan(
		&summary.TotalCalls, &summary.ErrorCalls, &summary.AvgLatencyMs,
		&summary.TotalTokensIn, &summary.TotalTokensOut,
		&summary.TotalInputCost, &summary.TotalOutputCost, &summary.DeniedCalls,
	); err != nil {
		return nil, err
	}

	seriesRows, err := results.Query()
	if err != nil {
		return nil, err
	}
	for seriesRows.Next() {
		var b domain.LogsTimeBucket
		if err := seriesRows.Scan(&b.Bucket, &b.Calls, &b.Errors, &b.AvgMs); err != nil {
			seriesRows.Close()
			return nil, err
		}
		summary.TimeSeries = append(summary.TimeSeries, b)
	}
	seriesRows.Close()
	if err := seriesRows.Err(); err != nil {
		return nil, err
	}

	modelRows, err := results.Query()
	if err != nil {
		return nil, err
	}
	for modelRows.Next() {
		var b domain.LogsModelBreakdown
		if err := modelRows.Scan(&b.ModelID, &b.ModelName, &b.ProviderName, &b.Calls,
			&b.Errors, &b.TokensIn, &b.TokensOut, &b.CostUSD, &b.AvgMs); err != nil {
			modelRows.Close()
			return nil, err
		}
		summary.ByModel = append(summary.ByModel, b)
	}
	modelRows.Close()
	if err := modelRows.Err(); err != nil {
		return nil, err
	}

	statusRows, err := results.Query()
	if err != nil {
		return nil, err
	}
	for statusRows.Next() {
		var b domain.LogsStatusBreakdown
		if err := statusRows.Scan(&b.StatusCode, &b.Calls); err != nil {
			statusRows.Close()
			return nil, err
		}
		summary.ByStatus = append(summary.ByStatus, b)
	}
	statusRows.Close()
	if err := statusRows.Err(); err != nil {
		return nil, err
	}

	userRows, err := results.Query()
	if err != nil {
		return nil, err
	}
	for userRows.Next() {
		var b domain.LogsUserBreakdown
		if err := userRows.Scan(&b.UserID, &b.UserEmail, &b.Calls, &b.Errors, &b.CostUSD); err != nil {
			userRows.Close()
			return nil, err
		}
		summary.ByUser = append(summary.ByUser, b)
	}
	userRows.Close()
	if err := userRows.Err(); err != nil {
		return nil, err
	}

	denyRows, err := results.Query()
	if err != nil {
		return nil, err
	}
	for denyRows.Next() {
		var b domain.LogsDenyBreakdown
		if err := denyRows.Scan(&b.Reason, &b.Calls); err != nil {
			denyRows.Close()
			return nil, err
		}
		summary.ByDenyReason = append(summary.ByDenyReason, b)
	}
	denyRows.Close()
	if err := denyRows.Err(); err != nil {
		return nil, err
	}

	latencyRows, err := results.Query()
	if err != nil {
		return nil, err
	}
	counts := make([]int, len(latencyLabels))
	for latencyRows.Next() {
		var idx, n int
		if err := latencyRows.Scan(&idx, &n); err != nil {
			latencyRows.Close()
			return nil, err
		}
		if idx >= 0 && idx < len(counts) {
			counts[idx] += n
		}
	}
	latencyRows.Close()
	if err := latencyRows.Err(); err != nil {
		return nil, err
	}
	for i, label := range latencyLabels {
		upper := 0
		if i < len(latencyEdges) {
			upper = latencyEdges[i]
		}
		summary.LatencyBuckets = append(summary.LatencyBuckets, domain.LogsLatencyBucket{
			UpperMs: upper, Label: label, Calls: counts[i],
		})
	}

	return summary, nil
}

var (
	latencyEdges  = []int{100, 250, 500, 1000, 2500, 5000, 10000, 30000}
	latencyLabels = []string{"<100ms", "100-250ms", "250-500ms", "0.5-1s", "1-2.5s", "2.5-5s", "5-10s", "10-30s", ">30s"}
)

func (s *RequestLogStore) DeleteOlderThan(ctx context.Context, cutoff time.Time, chunkSize int, maxChunks int) (int64, error) {
	if chunkSize <= 0 {
		chunkSize = 5000
	}
	var total int64
	for i := 0; i < maxChunks; i++ {
		tag, err := s.pool.Exec(ctx,
			`DELETE FROM request_logs
			 WHERE ctid IN (
			     SELECT ctid FROM request_logs WHERE created_at < $1 LIMIT $2
			 )`,
			cutoff, chunkSize,
		)
		if err != nil {
			return total, err
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() < int64(chunkSize) {
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	return total, nil
}
