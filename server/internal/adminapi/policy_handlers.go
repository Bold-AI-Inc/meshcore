package adminapi

import (
	"net/http"
	"strconv"
	"time"

	"mesh-server/internal/domain"
	"mesh-server/internal/httputil"
	"mesh-server/internal/middleware"
)

type policyHandlers struct {
	deps Deps
}

func (h *policyHandlers) list(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	policies, err := h.deps.Policies.ListForUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err)
		return
	}
	for i := range policies {
		policies[i].CallsThisHour = h.deps.MemState.UserModelHourlyCount(userID, policies[i].ModelID)
	}
	httputil.WriteJSON(w, http.StatusOK, policies)
}

type policyRequest struct {
	DailyCapUSD     float64 `json:"daily_cap_usd"`
	AllowedFrom     *string `json:"allowed_from"`
	AllowedTo       *string `json:"allowed_to"`
	Timezone        string  `json:"timezone"`
	ActiveDays      []int   `json:"active_days"`
	AlwaysOpen      bool    `json:"always_open"`
	MaxCallsPerHour *int    `json:"max_calls_per_hour"`
}

func (h *policyHandlers) upsert(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	modelID := r.PathValue("modelId")
	var req policyRequest
	if err := httputil.DecodeJSON(r, &req); err != nil || req.DailyCapUSD < 0 || req.Timezone == "" ||
		(req.MaxCallsPerHour != nil && *req.MaxCallsPerHour < 1) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	policy := domain.ModelPolicy{
		UserID:          userID,
		ModelID:         modelID,
		DailyCapUSD:     req.DailyCapUSD,
		AllowedFrom:     req.AllowedFrom,
		AllowedTo:       req.AllowedTo,
		Timezone:        req.Timezone,
		ActiveDays:      req.ActiveDays,
		AlwaysOpen:      req.AlwaysOpen,
		MaxCallsPerHour: req.MaxCallsPerHour,
	}
	if err := h.deps.Policies.Upsert(r.Context(), policy); err != nil {
		httputil.WriteInternalError(w, err)
		return
	}

	adminEmail, _ := r.Context().Value(middleware.AdminEmailKey).(string)
	logAudit(r.Context(), h.deps.Audit, adminEmail, "update_policy", userID+":"+modelID, nil)
	rebuildMemState(r.Context(), h.deps.MemState)

	resp := map[string]string{"status": "ok"}
	if _, err := time.LoadLocation(req.Timezone); err != nil {
		resp["warning"] = "Unrecognised timezone " + strconv.Quote(req.Timezone) +
			" — allowed hours will be enforced in GMT."
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}
