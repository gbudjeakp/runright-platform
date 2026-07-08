package types

import "time"

// Provider represents a cloud provider.
type Provider string

const (
	ProviderAWS    Provider = "aws"
	ProviderGCP    Provider = "gcp"
	ProviderGitHub Provider = "github"
)

// MachineType describes a cloud provider instance type with its specs and pricing.
type MachineType struct {
	ID                   string   `json:"id"`
	Provider             Provider `json:"provider"`
	Family               string   `json:"family"`
	Series               string   `json:"series"`
	VCPUs                int      `json:"vcpus"`
	MemoryGiB            float64  `json:"memory_gib"`
	NetworkGbps          float64  `json:"network_gbps"`
	StorageType          string   `json:"storage_type"`
	Architecture         string   `json:"architecture"`
	OnDemandPricePerHour float64  `json:"on_demand_price_per_hour"`
	// SpotPricePerHour is an explicitly sourced spot/preemptible price.
	// If zero, spot data is considered unavailable and must not be inferred.
	SpotPricePerHour float64 `json:"spot_price_per_hour,omitempty"`
	// SpotInterruptionRatePct is an explicitly sourced interruption/preemption rate
	// (0..100). If zero, interruption risk should not be inferred.
	SpotInterruptionRatePct float64  `json:"spot_interruption_rate_pct,omitempty"`
	Tags                    []string `json:"tags"`
	// SpotRisk indicates interruption likelihood for spot/preemptible: low, medium, or high.
	SpotRisk string `json:"spot_risk,omitempty"`
}

// MetricSnapshot is a single point-in-time reading from the metrics agent.
type MetricSnapshot struct {
	Timestamp    time.Time `json:"timestamp"`
	CPUPercent   float64   `json:"cpu_percent"`
	MemUsedGiB   float64   `json:"mem_used_gib"`
	MemTotalGiB  float64   `json:"mem_total_gib"`
	ProcessCount int       `json:"process_count"`
	ThreadCount  int       `json:"thread_count"`
	DiskReadMBs  float64   `json:"disk_read_mbs"`
	DiskWriteMBs float64   `json:"disk_write_mbs"`
	NetRxMBs     float64   `json:"net_rx_mbs"`
	NetTxMBs     float64   `json:"net_tx_mbs"`
}

// MetricsSummary aggregates a collection of snapshots into peak/avg/p95 values.
type MetricsSummary struct {
	JobID             string    `json:"job_id"`
	RunID             string    `json:"run_id,omitempty"`              // unique per agent invocation; used for upserts
	Status            string    `json:"status,omitempty"`              // "heartbeat" | "completed"
	CIPlatform        string    `json:"ci_platform,omitempty"`         // "github" | "jenkins" | "gitlab" | "circleci" | "bitbucket" | "local"
	Repository        string    `json:"repository,omitempty"`          // e.g. "owner/repo" from GITHUB_REPOSITORY
	AllowedMachineIDs []string  `json:"allowed_machine_ids,omitempty"` // optional explicit machine allow-list (e.g. ["c7g.2xlarge","m7i.xlarge"])
	AllowedSeries     []string  `json:"allowed_series,omitempty"`      // optional series allow-list (e.g. ["c7g","m7i"])
	AllowedFamilies   []string  `json:"allowed_families,omitempty"`    // optional family prefixes (e.g. ["c","m","r"])
	StartTime         time.Time `json:"start_time"`
	EndTime           time.Time `json:"end_time"`
	DurationSeconds   float64   `json:"duration_seconds"`

	// Detected machine at the time of the run.
	DetectedMachine *MachineType `json:"detected_machine,omitempty"`
	// Detection confidence for DetectedMachine in range 0..1.
	DetectedMachineConfidence float64 `json:"detected_machine_confidence,omitempty"`
	// Detection confidence label: high, medium, low, unknown.
	DetectedMachineConfidenceLevel string `json:"detected_machine_confidence_level,omitempty"`
	// Human-readable reason for the machine match decision.
	DetectedMachineMatchReason string `json:"detected_machine_match_reason,omitempty"`
	// Best-effort runtime storage class probe (ssd|hdd|unknown).
	RuntimeStorageClass string `json:"runtime_storage_class,omitempty"`

	CPUPercentPeak float64 `json:"cpu_percent_peak"`
	CPUPercentAvg  float64 `json:"cpu_percent_avg"`
	CPUPercentP95  float64 `json:"cpu_percent_p95"`

	MemUsedGiBPeak float64 `json:"mem_used_gib_peak"`
	MemUsedGiBAvg  float64 `json:"mem_used_gib_avg"`
	MemUsedGiBP95  float64 `json:"mem_used_gib_p95"`
	MemTotalGiB    float64 `json:"mem_total_gib"`

	ProcessCountPeak int `json:"process_count_peak"`
	ThreadCountPeak  int `json:"thread_count_peak"`

	DiskReadMBsPeak  float64 `json:"disk_read_mbs_peak"`
	DiskWriteMBsPeak float64 `json:"disk_write_mbs_peak"`
	NetRxMBsPeak     float64 `json:"net_rx_mbs_peak"`
	NetTxMBsPeak     float64 `json:"net_tx_mbs_peak"`

	SampleCount int `json:"sample_count"`

	// GPU metrics (Tier 3 feature)
	GPU *GPUSummary `json:"gpu,omitempty"`

	// Container-level breakdown (Tier 3 feature)
	Containers *ContainerSummary `json:"containers,omitempty"`

	// Build cache efficiency (Tier 3 feature)
	Cache *CacheStats `json:"cache,omitempty"`

	// Network egress cost estimation (Tier 3 feature)
	Egress *EgressSummary `json:"egress,omitempty"`
}

// GPUSummary aggregates GPU metrics across a run.
type GPUSummary struct {
	Count                 int     `json:"count"`
	TotalMemoryGiB        float64 `json:"total_memory_gib"`
	AvgUtilizationPct     float64 `json:"avg_utilization_pct"`
	PeakUtilizationPct    float64 `json:"peak_utilization_pct"`
	P95UtilizationPct     float64 `json:"p95_utilization_pct"`
	AvgMemoryUtilPct      float64 `json:"avg_memory_util_pct"`
	PeakMemoryUtilPct     float64 `json:"peak_memory_util_pct"`
	P95MemoryUtilPct      float64 `json:"p95_memory_util_pct"`
	AvgPowerDrawW         float64 `json:"avg_power_draw_w"`
	PeakPowerDrawW        float64 `json:"peak_power_draw_w"`
	IdleSamplesPct        float64 `json:"idle_samples_pct"`
	UnderutilizedPct      float64 `json:"underutilized_pct"`
	GPUType               string  `json:"gpu_type,omitempty"`
}

// ContainerSummary aggregates container metrics across a run.
type ContainerSummary struct {
	Containers         []ContainerAggregates `json:"containers"`
	TotalContainers    int                   `json:"total_containers"`
	TopCPUContainer    string                `json:"top_cpu_container,omitempty"`
	TopMemoryContainer string                `json:"top_memory_container,omitempty"`
}

// ContainerAggregates holds aggregated metrics for a single container.
type ContainerAggregates struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Image             string  `json:"image,omitempty"`
	CPUPercentAvg     float64 `json:"cpu_percent_avg"`
	CPUPercentPeak    float64 `json:"cpu_percent_peak"`
	CPUPercentP95     float64 `json:"cpu_percent_p95"`
	MemoryUsedMiBAvg  float64 `json:"memory_used_mib_avg"`
	MemoryUsedMiBPeak float64 `json:"memory_used_mib_peak"`
	MemoryLimitMiB    float64 `json:"memory_limit_mib,omitempty"`
	NetRxMBTotal      float64 `json:"net_rx_mb_total"`
	NetTxMBTotal      float64 `json:"net_tx_mb_total"`
	BlockReadMBTotal  float64 `json:"block_read_mb_total"`
	BlockWriteMBTotal float64 `json:"block_write_mb_total"`
	SampleCount       int     `json:"sample_count"`
}

// CacheStats captures build cache efficiency metrics.
type CacheStats struct {
	DockerLayerCacheHits   int     `json:"docker_layer_cache_hits,omitempty"`
	DockerLayerCacheMisses int     `json:"docker_layer_cache_misses,omitempty"`
	NPMCacheHitRate        float64 `json:"npm_cache_hit_rate,omitempty"`
	PIPCacheHitRate        float64 `json:"pip_cache_hit_rate,omitempty"`
	GoCacheHitRate         float64 `json:"go_cache_hit_rate,omitempty"`
	MavenCacheHitRate      float64 `json:"maven_cache_hit_rate,omitempty"`
	GradleCacheHitRate     float64 `json:"gradle_cache_hit_rate,omitempty"`
	CIPlatformCacheHit     bool    `json:"ci_platform_cache_hit,omitempty"`
	CIPlatformCacheSize    int64   `json:"ci_platform_cache_size_bytes,omitempty"`
	CacheRestoreTimeMs     int64   `json:"cache_restore_time_ms,omitempty"`
	CacheSaveTimeMs        int64   `json:"cache_save_time_ms,omitempty"`
	OverallCacheHitRate    float64 `json:"overall_cache_hit_rate,omitempty"`
	EstimatedTimeSaved     float64 `json:"estimated_time_saved_sec,omitempty"`
}

// EgressSummary provides run-level egress cost analysis.
type EgressSummary struct {
	TotalEgressGB        float64 `json:"total_egress_gb"`
	EstimatedCostUSD     float64 `json:"estimated_cost_usd"`
	CostPerRunUSD        float64 `json:"cost_per_run_usd"`
	MonthlyProjectionUSD float64 `json:"monthly_projection_usd,omitempty"`
	Provider             string  `json:"provider"`
	RecommendCaching     bool    `json:"recommend_caching,omitempty"`
	RecommendCompression bool    `json:"recommend_compression,omitempty"`
	PotentialSavingsUSD  float64 `json:"potential_savings_usd,omitempty"`
}

// RecommendationTier describes how a recommended machine compares to current usage.
type RecommendationTier string

const (
	TierRightSized   RecommendationTier = "right-sized"
	TierCheaper      RecommendationTier = "cheaper-option"
	TierMoreHeadroom RecommendationTier = "more-headroom"
)

// KubernetesResources contains suggested resource requests and limits for
// a Kubernetes runner pod based on observed p95 usage.
type KubernetesResources struct {
	CPURequest    string `json:"cpu_request"`    // e.g. "1200m"
	CPULimit      string `json:"cpu_limit"`      // e.g. "2000m"
	MemoryRequest string `json:"memory_request"` // e.g. "2Gi"
	MemoryLimit   string `json:"memory_limit"`   // e.g. "3Gi"
}

// Recommendation is a single machine type suggestion with cost and reasoning.
type Recommendation struct {
	Machine               MachineType        `json:"machine"`
	Tier                  RecommendationTier `json:"tier"`
	EstimatedMonthly      float64            `json:"estimated_monthly_usd"`
	SpotMonthly           float64            `json:"spot_monthly_usd"`
	CurrentMonthly        float64            `json:"current_monthly_usd"`
	CostDeltaPercent      float64            `json:"cost_delta_percent"`
	SpotDeltaPercent      float64            `json:"spot_delta_percent"`
	RequiredVCPUs         int                `json:"required_vcpus"`
	RequiredMemoryGiB     float64            `json:"required_memory_gib"`
	Reasoning             string             `json:"reasoning"`
	DurationRegressionPct *float64           `json:"duration_regression_pct,omitempty"`
	// DurationRiskNote is set when the recommended machine has significantly fewer vCPUs
	// than the current one; a CPU-bound job may run slower after the change.
	DurationRiskNote string `json:"duration_risk_note,omitempty"`
	// SpotRisk is inherited from the recommended machine's spot interruption likelihood.
	SpotRisk            string               `json:"spot_risk,omitempty"`
	KubernetesResources *KubernetesResources `json:"kubernetes_resources,omitempty"`
}

// UserSettings holds user-specific preferences and configurations.
type UserSettings struct {
	OtelEndpoint      string   `json:"otel_endpoint"`
	AllowedMachineIDs []string `json:"allowed_machine_ids"`
	AllowedSeries     []string `json:"allowed_series"`
	AllowedFamilies   []string `json:"allowed_families"`
}

// ─── AI Assistant Types ─────────────────────────────────────────────────────

// Conversation represents a chat conversation with the AI assistant.
type Conversation struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	UserID         string    `json:"user_id,omitempty"`
	Summary        string    `json:"summary,omitempty"`        // compacted memory of older messages
	MessageCount   int       `json:"message_count,omitempty"` // total stored messages
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"` // "user" | "assistant" | "system"
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
	// Metadata stores additional info like data sources used, cost of API call, etc.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ChatRequest is the payload for sending a message to the assistant.
type ChatRequest struct {
	ConversationID string `json:"conversation_id,omitempty"` // empty = new conversation
	Message        string `json:"message"`
	// Context filters to scope the assistant's data access
	Repository string `json:"repository,omitempty"`
	JobID      string `json:"job_id,omitempty"`
}

// ChatResponse is the assistant's reply.
type ChatResponse struct {
	ConversationID string       `json:"conversation_id"`
	Message        ChatMessage  `json:"message"`
	DataSources    []DataSource `json:"data_sources,omitempty"` // what data was used to answer
}

// DataSource describes data the assistant accessed to answer a question.
type DataSource struct {
	Type        string `json:"type"`        // "jobs", "savings", "policies", "catalog", etc.
	Description string `json:"description"` // human-readable description
	Count       int    `json:"count,omitempty"`
}

// AssistantContext aggregates all data the assistant can access.
type AssistantContext struct {
	Jobs            []MetricsSummary             `json:"jobs,omitempty"`
	Recommendations map[string][]Recommendation  `json:"recommendations,omitempty"` // job_id -> recommendations
	Savings         *SavingsSnapshot             `json:"savings,omitempty"`
	Policies        []PolicyRule                 `json:"policies,omitempty"`
	Repositories    []string                     `json:"repositories,omitempty"`
	SemanticContext string                       `json:"semantic_context,omitempty"` // RAG: relevant job text from semantic search
}

// SavingsSnapshot captures current savings state for the assistant.
type SavingsSnapshot struct {
	TotalPotentialSavingsUSD float64 `json:"total_potential_savings_usd"`
	TotalCurrentCostUSD      float64 `json:"total_current_cost_usd"`
	SavingsPercent           float64 `json:"savings_percent"`
	TopSavingsOpportunities  []SavingsOpportunity `json:"top_savings_opportunities,omitempty"`
}

// SavingsOpportunity represents a single cost-saving opportunity.
type SavingsOpportunity struct {
	JobID            string  `json:"job_id"`
	Repository       string  `json:"repository,omitempty"`
	CurrentCostUSD   float64 `json:"current_cost_usd"`
	RecommendedCostUSD float64 `json:"recommended_cost_usd"`
	SavingsUSD       float64 `json:"savings_usd"`
	SavingsPercent   float64 `json:"savings_percent"`
	RecommendedMachine string `json:"recommended_machine"`
}

// EmbeddingSearchResult represents a semantic search result from RAG.
type EmbeddingSearchResult struct {
	JobID        string    `json:"job_id"`
	Repository   string    `json:"repository"`
	EmbeddedText string    `json:"embedded_text"`
	MachineType  string    `json:"machine_type,omitempty"`
	CostUSD      float64   `json:"cost_usd"`
	StartTime    time.Time `json:"start_time"`
	Similarity   float64   `json:"similarity"` // 0-1, higher is more similar
}

// PolicyRule represents a cost policy (re-exported for assistant context).
type PolicyRule struct {
	Repository     string  `json:"repository"`
	JobID          string  `json:"job_id,omitempty"`
	MaxCostPerHour float64 `json:"max_cost_per_hour"`
	Enabled        bool    `json:"enabled"`
}
