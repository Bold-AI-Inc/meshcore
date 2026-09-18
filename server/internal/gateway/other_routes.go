package gateway

import (
	"net/http"
	"time"

	"mesh-server/internal/memstate"
)

func (h *Handler) Models(w http.ResponseWriter, r *http.Request) {
	resolved, ok := h.authenticate(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, errInvalidOrRevokedKey, nil)
		return
	}

	now := time.Now()
	if code, ok := checkUserStatus(resolved, now); !ok {
		writeError(w, http.StatusUnauthorized, code, nil)
		return
	}
	total := len(resolved.Access) + len(resolved.EmbeddingAccess) + len(resolved.GenerationAccess)
	models := make([]map[string]any, 0, total)
	for name, access := range resolved.Access {
		if m, ok := h.describeModel(now, resolved.UserID, name, "chat", access); ok {
			models = append(models, m)
		}
	}
	for name, access := range resolved.EmbeddingAccess {
		if m, ok := h.describeModel(now, resolved.UserID, name, "embedding", access); ok {
			models = append(models, m)
		}
	}
	for name, access := range resolved.GenerationAccess {
		if m, ok := h.describeModel(now, resolved.UserID, name, "generation", access); ok {
			models = append(models, m)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (h *Handler) describeModel(now time.Time, userID, name, kind string, access memstate.ModelAccess) (map[string]any, bool) {
	if access.ModelBlocked || access.ModelStatus == "retired" {
		return nil, false
	}

	formats := []string{"text"}
	if access.SupportsImageIn {
		formats = append(formats, "image")
	}
	if access.SupportsDocumentIn {
		formats = append(formats, "document")
	}
	if access.SupportsAudioIn {
		formats = append(formats, "audio")
	}
	if access.SupportsVideoIn {
		formats = append(formats, "video")
	}

	status := "available"
	var resumeAt *string

	if access.Provider.MaxCallsPerHour != nil && h.mem.ProviderHourlyCount(access.Provider.ID) >= int64(*access.Provider.MaxCallsPerHour) {
		status = "rate_limited"
		t := now.Truncate(time.Hour).Add(time.Hour).Format(time.RFC3339)
		resumeAt = &t
	}
	callsThisHour := h.mem.UserModelHourlyCount(userID, access.ModelID)
	if status == "available" && access.Policy.MaxCallsPerHour != nil && callsThisHour >= int64(*access.Policy.MaxCallsPerHour) {
		status = "rate_limited"
		t := now.Truncate(time.Hour).Add(time.Hour).Format(time.RFC3339)
		resumeAt = &t
	}
	if status == "available" && !access.Policy.AlwaysOpen {
		if t, withinWindow := checkTimeWindow(now, access.Policy); !withinWindow {
			status = "outside_hours"
			formatted := t.Format(time.RFC3339)
			resumeAt = &formatted
		}
	}
	spentUSD := h.mem.DailySpendUSD(userID, access.ModelID)
	if status == "available" && access.Policy.DailyCapUSD > 0 && spentUSD >= access.Policy.DailyCapUSD {
		status = "budget_exceeded"
		t := now.Truncate(24 * time.Hour).Add(24 * time.Hour).Format(time.RFC3339)
		resumeAt = &t
	}

	webSearch := access.SupportsWebSearch && access.Provider.SupportsWebSearch &&
		len(access.Provider.WebSearchRequestBodyTemplate) > 0

	var dailyCap *float64
	if access.Policy.DailyCapUSD > 0 {
		capUSD := access.Policy.DailyCapUSD
		dailyCap = &capUSD
	}
	entry := map[string]any{
		"model":            name,
		"kind":             kind,
		"formats_accepted": formats,
		"web_search":       webSearch,
		"status":           status,
		"usage": map[string]any{
			"calls_this_hour":      callsThisHour,
			"calls_per_hour_limit": access.Policy.MaxCallsPerHour,
			"spent_today_usd":      spentUSD,
			"daily_cap_usd":        dailyCap,
		},
	}
	if access.OutputMediaType != "" {
		entry["output_media_type"] = access.OutputMediaType
	}
	if resumeAt != nil {
		entry["resume_at"] = *resumeAt
	}
	if !access.Policy.AlwaysOpen && access.Policy.AllowedFrom != nil && access.Policy.AllowedTo != nil {
		entry["allowed_hours"] = map[string]any{
			"from":     *access.Policy.AllowedFrom,
			"to":       *access.Policy.AllowedTo,
			"timezone": access.Policy.Timezone,
			"days":     access.Policy.ActiveDays,
		}
	}
	return entry, true
}

func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	resolved, ok := h.authenticate(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, errInvalidOrRevokedKey, nil)
		return
	}

	if code, ok := checkUserStatus(resolved, time.Now()); !ok {
		writeError(w, http.StatusUnauthorized, code, nil)
		return
	}

	total := len(resolved.Access) + len(resolved.EmbeddingAccess) + len(resolved.GenerationAccess)
	usage := make([]map[string]any, 0, total)
	add := func(kind string, models map[string]memstate.ModelAccess) {
		for name, access := range models {
			spentUSD := h.mem.DailySpendUSD(resolved.UserID, access.ModelID)
			var capUSD, remaining *float64
			if access.Policy.DailyCapUSD > 0 {
				c := access.Policy.DailyCapUSD
				rem := c - spentUSD
				capUSD, remaining = &c, &rem
			}
			usage = append(usage, map[string]any{
				"model":         name,
				"kind":          kind,
				"spent_usd":     spentUSD,
				"daily_cap_usd": capUSD,
				"remaining_usd": remaining,
			})
		}
	}
	add("chat", resolved.Access)
	add("embedding", resolved.EmbeddingAccess)
	add("generation", resolved.GenerationAccess)

	writeJSON(w, http.StatusOK, map[string]any{"usage": usage})
}
