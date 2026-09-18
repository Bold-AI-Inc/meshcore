package memstate

import (
	"time"
)

type Snapshot struct {
	KeyIndex map[[32]byte]ResolvedKey

	SilentProviders map[string]bool
}

type ResolvedKey struct {
	KeyID         string
	UserID        string
	UserStatus    string
	UserExpiresAt *time.Time
	KeyExpiresAt  *time.Time

	Access map[string]ModelAccess

	EmbeddingAccess map[string]ModelAccess

	GenerationAccess map[string]ModelAccess
}

type ModelAccess struct {
	ModelID                string
	ModelName              string
	ResolvedModel          string
	ModelBlocked           bool
	ModelStatus            string
	InputPricePer1kTokens  float64
	OutputPricePer1kTokens float64
	Provider               *ProviderConfig
	Policy                 PolicySnapshot

	SupportsImageIn    bool
	SupportsDocumentIn bool
	SupportsAudioIn    bool
	SupportsVideoIn    bool

	SupportsWebSearch bool

	PricePerSecond float64

	OutputMediaType string
}

type ProviderConfig struct {
	ID                    string
	Name                  string
	EndpointURL           string
	HTTPMethod            string
	HeaderTemplate        []byte
	RequestBodyTemplate   []byte
	ResponseDeltaPath     string
	ResponseDoneSignal    *string
	UsageInputTokensPath  *string
	UsageOutputTokensPath *string

	DeltaPathSegs         []string
	UsageInPathSegs       []string
	UsageOutPathSegs      []string
	APIKey                string
	MaxOutboundRPS        int
	MaxConcurrentUpstream int
	RetryEnabled          bool
	MaxRetries            int
	RetryBackoffMs        int
	MaxCallsPerHour       *int

	ProviderFamily string

	SupportsEmbeddings           bool
	EmbeddingEndpointURL         *string
	EmbeddingRequestBodyTemplate []byte
	EmbeddingResponseVectorPath  *string

	SupportsGeneration            bool
	GenerationEndpointURL         *string
	GenerationRequestBodyTemplate []byte
	GenerationResponseMediaPath   *string

	GenerationMode               string
	GenerationJobIDPath          *string
	GenerationStatusURLTemplate  *string
	GenerationStatusPath         *string
	GenerationContentURLTemplate *string
	GenerationStatusSuccess      []string
	GenerationStatusFailure      []string
	GenerationPollIntervalMs     int
	GenerationMaxWaitSeconds     int

	GenerationJobIDSegs    []string
	GenerationStatusSegs   []string
	GenerationDurationSegs []string

	EmbeddingUsageTokensPath        *string
	GenerationUsageInputTokensPath  *string
	GenerationUsageOutputTokensPath *string
	GenerationDurationSecondsPath   *string

	EmbeddingUsageSegs     []string
	GenerationUsageInSegs  []string
	GenerationUsageOutSegs []string

	SupportsWebSearch            bool
	WebSearchRequestBodyTemplate []byte

	WebSearchPricePerCall float64

	Limiter *ProviderLimiter
}

type PolicySnapshot struct {
	DailyCapUSD float64
	AllowedFrom *string
	AllowedTo   *string
	Timezone    string

	Location *time.Location

	FromMinutes int
	ToMinutes   int
	HasWindow   bool

	ActiveDays      []int
	AlwaysOpen      bool
	MaxCallsPerHour *int
}
