package adminapi

import (
	"net/http"
	"strconv"
	"time"

	"mesh-server/internal/domain"
	"mesh-server/internal/httputil"
)

type logHandlers struct {
	deps Deps
}

func parseLogFilter(r *http.Request) domain.RequestLogFilter {
	q := r.URL.Query()

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -7)
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	tz := q.Get("tz")
	if _, err := time.LoadLocation(tz); err != nil {
		tz = ""
	}

	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	userID := r.PathValue("id")
	global := userID == ""
	if global {
		userID = q.Get("user_id")
	}

	excludeTest := global && q.Get("include_test") != "true"

	return domain.RequestLogFilter{
		UserID:      userID,
		From:        from,
		To:          to,
		ModelID:     q.Get("model_id"),
		ProviderID:  q.Get("provider_id"),
		Status:      q.Get("status"),
		DenyReason:  q.Get("deny_reason"),
		Search:      q.Get("search"),
		ExcludeTest: excludeTest,
		SortBy:      q.Get("sort_by"),
		SortDir:     q.Get("sort_dir"),
		Timezone:    tz,
		Page:        page,
		PageSize:    pageSize,
	}
}

func (h *logHandlers) list(w http.ResponseWriter, r *http.Request) {
	filter := parseLogFilter(r)
	logs, total, err := h.deps.RequestLogs.List(r.Context(), filter)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	pageSize := filter.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 25
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"logs":      logs,
		"total":     total,
		"page":      filter.Page,
		"page_size": pageSize,
	})
}

func (h *logHandlers) summary(w http.ResponseWriter, r *http.Request) {
	filter := parseLogFilter(r)
	summary, err := h.deps.RequestLogs.Summary(r.Context(), filter)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, summary)
}
