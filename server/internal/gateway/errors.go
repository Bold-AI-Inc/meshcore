package gateway

import (
	"encoding/json"
	"net/http"
)

func writeError(w http.ResponseWriter, status int, code string, extra map[string]any) {
	body := map[string]any{"error": code}
	for k, v := range extra {
		body[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

const (
	errModelBlocked            = "model_blocked"
	errModelNotPermitted       = "model_not_permitted"
	errProviderHourlyLimit     = "provider_hourly_limit_reached"
	errUserModelHourlyLimit    = "user_model_hourly_limit_reached"
	errOutsideAllowedHours     = "outside_allowed_hours"
	errBudgetExceeded          = "budget_exceeded"
	errInvalidOrRevokedKey     = "invalid_or_revoked_key"
	errPausedByAdmin           = "paused_by_admin"
	errInvalidRequest          = "invalid_request"
	errContentTypeNotSupported = "content_type_not_supported_by_model"
	errWebSearchNotSupported   = "web_search_not_supported_by_model"
	errServerBusy              = "server_busy"
	errProviderBusy            = "provider_busy"
)
