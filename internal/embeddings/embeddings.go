// Package embeddings provides vector embedding generation and semantic search for RunRight.
// It uses pgvector for efficient similarity search and supports multiple embedding providers.
package embeddings

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sgbudje/runright-platform/internal/types"
)

// Provider identifies the embedding model provider.
type Provider string

const (
	ProviderOllama Provider = "ollama"
	ProviderOpenAI Provider = "openai"
)

// Config holds embedding configuration.
type Config struct {
	Provider   Provider
	Model      string
	BaseURL    string
	APIKey     string // Only needed for OpenAI
	Dimensions int    // Vector dimensions (768 for nomic, 1536 for OpenAI)
}

// Service provides embedding generation and semantic search.
type Service struct {
	db     *sql.DB
	cfg    Config
	client *http.Client
}

// New creates a new embedding service.
func New(db *sql.DB, cfg Config) *Service {
	if cfg.Dimensions == 0 {
		cfg.Dimensions = 768 // Default for nomic-embed-text
	}
	if cfg.Model == "" {
		cfg.Model = "nomic-embed-text"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	// Default to Ollama if provider not specified but BaseURL is
	if cfg.Provider == "" && cfg.BaseURL != "" {
		cfg.Provider = ProviderOllama
	}
	return &Service{
		db:     db,
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// NewFromEnv creates an embedding service using environment variables.
func NewFromEnv(db *sql.DB) *Service {
	provider := Provider(os.Getenv("RUNRIGHT_EMBED_PROVIDER"))
	if provider == "" {
		provider = ProviderOllama
	}
	
	dimensions := 768
	if provider == ProviderOpenAI {
		dimensions = 1536
	}
	
	return New(db, Config{
		Provider:   provider,
		Model:      os.Getenv("RUNRIGHT_EMBED_MODEL"),
		BaseURL:    os.Getenv("RUNRIGHT_AI_BASE_URL"), // Reuse AI base URL for Ollama
		APIKey:     os.Getenv("RUNRIGHT_AI_API_KEY"),
		Dimensions: dimensions,
	})
}

// IsConfigured returns true if the embedding service is ready.
func (s *Service) IsConfigured() bool {
	switch s.cfg.Provider {
	case ProviderOllama:
		return s.cfg.BaseURL != ""
	case ProviderOpenAI:
		return s.cfg.APIKey != ""
	}
	return false
}

// JobSummary represents the key fields we embed for a job.
type JobSummary struct {
	JobID          string    `json:"job_id"`
	Repository     string    `json:"repository"`
	MachineName    string    `json:"machine_name,omitempty"`
	CPUPercentPeak float64   `json:"cpu_percent_peak"`
	CPUPercentAvg  float64   `json:"cpu_percent_avg"`
	MemUsedGiBPeak float64   `json:"mem_used_gib_peak"`
	MemTotalGiB    float64   `json:"mem_total_gib"`
	GPUPercentPeak float64   `json:"gpu_percent_peak,omitempty"`
	DurationSecs   int       `json:"duration_secs"`
	CostUSD        float64   `json:"cost_usd"`
	StartTime      time.Time `json:"start_time"`
}

// GenerateEmbedding creates a vector embedding for the given text.
func (s *Service) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	switch s.cfg.Provider {
	case ProviderOllama:
		return s.embedOllama(ctx, text)
	case ProviderOpenAI:
		return s.embedOpenAI(ctx, text)
	default:
		return s.embedOllama(ctx, text)
	}
}

// embedOllama generates embeddings using Ollama's API.
func (s *Service) embedOllama(ctx context.Context, text string) ([]float32, error) {
	type request struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	type response struct {
		Embedding []float32 `json:"embedding"`
		Error     string    `json:"error,omitempty"`
	}

	reqBody := request{
		Model:  s.cfg.Model,
		Prompt: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimSuffix(s.cfg.BaseURL, "/") + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embedding error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse ollama embedding response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", result.Error)
	}

	return result.Embedding, nil
}

// embedOpenAI generates embeddings using OpenAI's API.
func (s *Service) embedOpenAI(ctx context.Context, text string) ([]float32, error) {
	type request struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}
	type response struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	model := s.cfg.Model
	if model == "" {
		model = "text-embedding-3-small"
	}

	reqBody := request{
		Model: model,
		Input: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse openai embedding response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("openai error: %s", result.Error.Message)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned from OpenAI")
	}

	return result.Data[0].Embedding, nil
}

// JobToText converts a job summary to searchable text.
func JobToText(job types.MetricsSummary) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Job: %s", job.JobID))
	parts = append(parts, fmt.Sprintf("Repository: %s", job.Repository))

	if job.DetectedMachine != nil {
		parts = append(parts, fmt.Sprintf("Machine: %s (%d vCPUs, %.1f GB RAM)",
			job.DetectedMachine.ID, job.DetectedMachine.VCPUs, job.DetectedMachine.MemoryGiB))
	}

	// Resource utilization
	parts = append(parts, fmt.Sprintf("CPU: peak %.1f%%, avg %.1f%%, p95 %.1f%%",
		job.CPUPercentPeak, job.CPUPercentAvg, job.CPUPercentP95))
	
	memUsagePercent := 0.0
	if job.MemTotalGiB > 0 {
		memUsagePercent = (job.MemUsedGiBPeak / job.MemTotalGiB) * 100
	}
	parts = append(parts, fmt.Sprintf("Memory: peak %.2f GB (%.1f%% of %.1f GB)",
		job.MemUsedGiBPeak, memUsagePercent, job.MemTotalGiB))

	// GPU if present
	if job.GPU != nil && job.GPU.PeakUtilizationPct > 0 {
		parts = append(parts, fmt.Sprintf("GPU: %.1f%% utilization, %.2f GB memory (%d GPUs, %s)",
			job.GPU.PeakUtilizationPct, job.GPU.TotalMemoryGiB, job.GPU.Count, job.GPU.GPUType))
	}

	// Duration
	duration := job.EndTime.Sub(job.StartTime)
	parts = append(parts, fmt.Sprintf("Duration: %.1f minutes", duration.Minutes()))

	// Cost estimation
	if job.DetectedMachine != nil {
		cost := job.DetectedMachine.OnDemandPricePerHour * duration.Hours()
		parts = append(parts, fmt.Sprintf("Estimated cost: $%.4f", cost))
	}

	// Performance indicators
	if job.CPUPercentPeak > 90 {
		parts = append(parts, "HIGH CPU: Job is CPU-bound, may benefit from more cores")
	} else if job.CPUPercentPeak < 30 {
		parts = append(parts, "LOW CPU: Job is underutilizing CPU, consider smaller instance")
	}

	if memUsagePercent > 85 {
		parts = append(parts, "HIGH MEMORY: Job is memory-bound, may need more RAM")
	} else if memUsagePercent < 30 {
		parts = append(parts, "LOW MEMORY: Job is underutilizing memory, consider smaller instance")
	}

	return strings.Join(parts, "\n")
}

// IndexJob creates an embedding for a job and stores it.
func (s *Service) IndexJob(ctx context.Context, job types.MetricsSummary) error {
	text := JobToText(job)

	embedding, err := s.GenerateEmbedding(ctx, text)
	if err != nil {
		return fmt.Errorf("generate embedding: %w", err)
	}

	// Convert embedding to pgvector format
	embeddingStr := formatVector(embedding)

	duration := int(job.EndTime.Sub(job.StartTime).Seconds())
	cost := 0.0
	machineType := ""
	if job.DetectedMachine != nil {
		cost = job.DetectedMachine.OnDemandPricePerHour * job.EndTime.Sub(job.StartTime).Hours()
		machineType = job.DetectedMachine.ID
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO job_embeddings (id, job_id, repository, embedded_text, embedding, start_time, duration_seconds, cost_usd, machine_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (job_id, start_time) DO UPDATE SET
			embedded_text = EXCLUDED.embedded_text,
			embedding = EXCLUDED.embedding,
			duration_seconds = EXCLUDED.duration_seconds,
			cost_usd = EXCLUDED.cost_usd,
			machine_type = EXCLUDED.machine_type,
			updated_at = NOW()
	`, uuid.New().String(), job.JobID, job.Repository, text, embeddingStr, job.StartTime, duration, cost, machineType)

	return err
}

// Search finds jobs similar to the query using vector similarity.
func (s *Service) Search(ctx context.Context, query string, limit int, repository string) ([]types.EmbeddingSearchResult, error) {
	if limit == 0 {
		limit = 10
	}

	// Generate embedding for query
	embedding, err := s.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("generate query embedding: %w", err)
	}

	embeddingStr := formatVector(embedding)

	// Search using cosine similarity
	sqlQuery := `
		SELECT 
			job_id, 
			repository, 
			embedded_text, 
			machine_type, 
			COALESCE(cost_usd, 0) as cost_usd, 
			start_time,
			1 - (embedding <=> $1::vector) as similarity
		FROM job_embeddings
		WHERE 1=1
	`
	args := []any{embeddingStr}
	argNum := 2

	if repository != "" {
		sqlQuery += fmt.Sprintf(" AND repository = $%d", argNum)
		args = append(args, repository)
		argNum++
	}

	sqlQuery += fmt.Sprintf(`
		ORDER BY embedding <=> $1::vector
		LIMIT $%d
	`, argNum)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var results []types.EmbeddingSearchResult
	for rows.Next() {
		var r types.EmbeddingSearchResult
		var machineType sql.NullString
		if err := rows.Scan(&r.JobID, &r.Repository, &r.EmbeddedText, &machineType, &r.CostUSD, &r.StartTime, &r.Similarity); err != nil {
			return nil, err
		}
		if machineType.Valid {
			r.MachineType = machineType.String
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// GetIndexStats returns statistics about the embedding index.
func (s *Service) GetIndexStats(ctx context.Context) (map[string]any, error) {
	stats := make(map[string]any)

	// Total embeddings
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_embeddings`).Scan(&total)
	if err != nil {
		return nil, err
	}
	stats["total_embeddings"] = total

	// Embeddings by repository
	rows, err := s.db.QueryContext(ctx, `
		SELECT repository, COUNT(*) 
		FROM job_embeddings 
		GROUP BY repository 
		ORDER BY COUNT(*) DESC 
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	repoStats := make(map[string]int)
	for rows.Next() {
		var repo string
		var count int
		if err := rows.Scan(&repo, &count); err != nil {
			return nil, err
		}
		repoStats[repo] = count
	}
	stats["by_repository"] = repoStats

	// Date range
	var minDate, maxDate sql.NullTime
	err = s.db.QueryRowContext(ctx, `
		SELECT MIN(start_time), MAX(start_time) FROM job_embeddings
	`).Scan(&minDate, &maxDate)
	if err != nil {
		return nil, err
	}
	if minDate.Valid {
		stats["earliest_job"] = minDate.Time
	}
	if maxDate.Valid {
		stats["latest_job"] = maxDate.Time
	}

	return stats, nil
}

// IndexAllJobs indexes all jobs that don't have embeddings yet.
func (s *Service) IndexAllJobs(ctx context.Context, batchSize int) (int, error) {
	if batchSize == 0 {
		batchSize = 50
	}

	// Find jobs without embeddings
	rows, err := s.db.QueryContext(ctx, `
		SELECT j.summary, j.recommendations
		FROM jobs j
		LEFT JOIN job_embeddings e ON 
			j.summary->>'job_id' = e.job_id AND 
			(j.summary->>'start_time')::timestamp = e.start_time
		WHERE e.id IS NULL
		LIMIT $1
	`, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	indexed := 0
	for rows.Next() {
		var summaryJSON, recsJSON sql.NullString
		if err := rows.Scan(&summaryJSON, &recsJSON); err != nil {
			return indexed, err
		}

		if !summaryJSON.Valid {
			continue
		}

		var summary types.MetricsSummary
		if err := json.Unmarshal([]byte(summaryJSON.String), &summary); err != nil {
			continue
		}

		if err := s.IndexJob(ctx, summary); err != nil {
			// Log but continue
			fmt.Printf("Failed to index job %s: %v\n", summary.JobID, err)
			continue
		}
		indexed++
	}

	return indexed, rows.Err()
}

// formatVector converts a slice of floats to pgvector format.
func formatVector(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%f", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
