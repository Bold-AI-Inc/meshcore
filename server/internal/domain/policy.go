package domain

type ModelPolicy struct {
	UserID          string  `json:"user_id"`
	ModelID         string  `json:"model_id"`
	DailyCapUSD     float64 `json:"daily_cap_usd"`
	AllowedFrom     *string `json:"allowed_from"`
	AllowedTo       *string `json:"allowed_to"`
	Timezone        string  `json:"timezone"`
	ActiveDays      []int   `json:"active_days"`
	AlwaysOpen      bool    `json:"always_open"`
	MaxCallsPerHour *int    `json:"max_calls_per_hour"`

	SpentTodayUSD float64 `json:"spent_today_usd"`

	CallsThisHour int64 `json:"calls_this_hour"`
}
