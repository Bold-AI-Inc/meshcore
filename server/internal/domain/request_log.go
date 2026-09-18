package domain

import "time"

type RequestLog struct {
	ID             string    `json:"id"`
	UserID         *string   `json:"user_id"`
	UserEmail      *string   `json:"user_email"`
	ModelID        *string   `json:"model_id"`
	ModelName      *string   `json:"model_name"`
	ProviderName   *string   `json:"provider_name"`
	RequestedModel *string   `json:"requested_model"`
	Outcome        string    `json:"outcome"`
	DenyReason     *string   `json:"deny_reason"`
	SourceIP       *string   `json:"source_ip"`
	UserAgent      *string   `json:"user_agent"`
	Browser        *string   `json:"browser"`
	BrowserVersion *string   `json:"browser_version"`
	OS             *string   `json:"os"`
	OSVersion      *string   `json:"os_version"`
	DeviceType     *string   `json:"device_type"`
	StatusCode     int       `json:"status_code"`
	LatencyMs      int       `json:"latency_ms"`
	TokensIn       int       `json:"tokens_in"`
	TokensOut      int       `json:"tokens_out"`
	InputCost      float64   `json:"input_cost"`
	OutputCost     float64   `json:"output_cost"`
	CreatedAt      time.Time `json:"created_at"`
}

type RequestLogFilter struct {
	UserID      string
	From        time.Time
	To          time.Time
	ModelID     string
	ProviderID  string
	Status      string
	DenyReason  string
	ExcludeTest bool
	Search      string
	SortBy      string
	SortDir     string
	Timezone    string
	Page        int
	PageSize    int
}

type LogsTimeBucket struct {
	Bucket time.Time `json:"bucket"`
	Calls  int       `json:"calls"`
	Errors int       `json:"errors"`
	AvgMs  float64   `json:"avg_latency_ms"`
}

type LogsModelBreakdown struct {
	ModelID      string  `json:"model_id"`
	ModelName    string  `json:"model_name"`
	ProviderName string  `json:"provider_name"`
	Calls        int     `json:"calls"`
	Errors       int     `json:"errors"`
	TokensIn     int64   `json:"tokens_in"`
	TokensOut    int64   `json:"tokens_out"`
	CostUSD      float64 `json:"cost_usd"`
	AvgMs        float64 `json:"avg_latency_ms"`
}

type LogsUserBreakdown struct {
	UserID    string  `json:"user_id"`
	UserEmail string  `json:"user_email"`
	Calls     int     `json:"calls"`
	Errors    int     `json:"errors"`
	CostUSD   float64 `json:"cost_usd"`
}

type LogsDenyBreakdown struct {
	Reason string `json:"reason"`
	Calls  int    `json:"calls"`
}

type LogsLatencyBucket struct {
	UpperMs int    `json:"upper_ms"`
	Label   string `json:"label"`
	Calls   int    `json:"calls"`
}

type LogsStatusBreakdown struct {
	StatusCode int `json:"status_code"`
	Calls      int `json:"calls"`
}

type LogsSummary struct {
	TotalCalls      int                   `json:"total_calls"`
	ErrorCalls      int                   `json:"error_calls"`
	AvgLatencyMs    float64               `json:"avg_latency_ms"`
	TotalTokensIn   int64                 `json:"total_tokens_in"`
	TotalTokensOut  int64                 `json:"total_tokens_out"`
	TotalInputCost  float64               `json:"total_input_cost"`
	TotalOutputCost float64               `json:"total_output_cost"`
	DeniedCalls     int                   `json:"denied_calls"`
	TimeSeries      []LogsTimeBucket      `json:"time_series"`
	ByModel         []LogsModelBreakdown  `json:"by_model"`
	ByStatus        []LogsStatusBreakdown `json:"by_status"`
	ByUser          []LogsUserBreakdown   `json:"by_user"`
	ByDenyReason    []LogsDenyBreakdown   `json:"by_deny_reason"`
	LatencyBuckets  []LogsLatencyBucket   `json:"latency_buckets"`
}
