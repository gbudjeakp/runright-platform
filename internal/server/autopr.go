package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ghlib "github.com/sgbudje/runright-platform/internal/github"
	"github.com/sgbudje/runright-platform/internal/types"
)

// LabelMapping maps a runner label to instance specs
type LabelMapping struct {
	ID           string    `json:"id"`
	TeamID       string    `json:"team_id,omitempty"`
	Repository   string    `json:"repository"`
	Label        string    `json:"label"`
	Provider     string    `json:"provider"`
	InstanceType string    `json:"instance_type"`
	VCPUs        int       `json:"vcpus"`
	MemoryGiB    float64   `json:"memory_gib"`
	CostPerHour  float64   `json:"cost_per_hour"`
	IsGPU        bool      `json:"is_gpu"`
	GPUType      string    `json:"gpu_type,omitempty"`
	GPUCount     int       `json:"gpu_count,omitempty"`
	GPUMemoryGiB float64   `json:"gpu_memory_gib,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AutoPRSettings controls auto-PR behavior
type AutoPRSettings struct {
	TeamID               string   `json:"team_id,omitempty"`
	Enabled              bool     `json:"enabled"`
	MinSavingsPercent    float64  `json:"min_savings_percent"`
	MinMonthlySavings    float64  `json:"min_monthly_savings"`
	RequireConsecutive   int      `json:"require_consecutive_runs"`
	GPUPRsEnabled        bool     `json:"gpu_prs_enabled"`
	GPUMinSavingsPercent float64  `json:"gpu_min_savings_percent"`
	ExcludeRepositories  []string `json:"exclude_repositories"`
	ExcludeJobPatterns   []string `json:"exclude_job_patterns"`
}

// PRRecommendation represents a suggested optimization
type PRRecommendation struct {
	ID                       string    `json:"id"`
	TeamID                   string    `json:"team_id,omitempty"`
	Repository               string    `json:"repository"`
	JobID                    string    `json:"job_id"`
	WorkflowFile             string    `json:"workflow_file,omitempty"`
	CurrentLabel             string    `json:"current_label"`
	CurrentVCPUs             int       `json:"current_vcpus"`
	CurrentMemoryGiB         float64   `json:"current_memory_gib"`
	CurrentCostPerHour       float64   `json:"current_cost_per_hour"`
	RecommendedLabel         string    `json:"recommended_label"`
	RecommendedVCPUs         int       `json:"recommended_vcpus"`
	RecommendedMemoryGiB     float64   `json:"recommended_memory_gib"`
	RecommendedCostPerHour   float64   `json:"recommended_cost_per_hour"`
	P95CPUPercent            float64   `json:"p95_cpu_percent"`
	P95MemPercent            float64   `json:"p95_mem_percent"`
	RunCount                 int       `json:"run_count"`
	ConsecutiveUnderutilized int       `json:"consecutive_underutilized"`
	IsGPUJob                 bool      `json:"is_gpu_job"`
	CurrentGPUType           string    `json:"current_gpu_type,omitempty"`
	RecommendedGPUType       string    `json:"recommended_gpu_type,omitempty"`
	P95GPUUtilPercent        float64   `json:"p95_gpu_util_percent,omitempty"`
	P95GPUMemPercent         float64   `json:"p95_gpu_mem_percent,omitempty"`
	SavingsPercent           float64   `json:"savings_percent"`
	MonthlySavingsUSD        float64   `json:"monthly_savings_usd"`
	Status                   string    `json:"status"`
	PRUrl                    string    `json:"pr_url,omitempty"`
	PRNumber                 int       `json:"pr_number,omitempty"`
	DismissedReason          string    `json:"dismissed_reason,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// ══════════════════════════════════════════════════════════════════════════════
// LABEL MAPPINGS
// ══════════════════════════════════════════════════════════════════════════════

func (s *Server) listLabelMappings(c *gin.Context) {
	ctx := c.Request.Context()
	repository := c.Query("repository")
	isGPU := c.Query("gpu") == "true"

	query := `
		SELECT id, COALESCE(team_id, ''), repository, label, provider, instance_type,
		       vcpus, memory_gib, cost_per_hour, is_gpu, 
		       COALESCE(gpu_type, ''), COALESCE(gpu_count, 0), COALESCE(gpu_memory_gib, 0),
		       created_at, updated_at
		FROM label_mappings
		WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if repository != "" {
		query += ` AND (repository = $1 OR repository = '*')`
		args = append(args, repository)
		argIdx++
	}
	if isGPU {
		query += ` AND is_gpu = TRUE`
	}
	query += ` ORDER BY repository, label`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	mappings := []LabelMapping{}
	for rows.Next() {
		var m LabelMapping
		if err := rows.Scan(
			&m.ID, &m.TeamID, &m.Repository, &m.Label, &m.Provider, &m.InstanceType,
			&m.VCPUs, &m.MemoryGiB, &m.CostPerHour, &m.IsGPU,
			&m.GPUType, &m.GPUCount, &m.GPUMemoryGiB,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		mappings = append(mappings, m)
	}
	c.JSON(http.StatusOK, mappings)
}

func (s *Server) upsertLabelMapping(c *gin.Context) {
	ctx := c.Request.Context()
	var m LabelMapping
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if m.Label == "" || m.Provider == "" || m.InstanceType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "label, provider, and instance_type required"})
		return
	}
	if m.Repository == "" {
		m.Repository = "*"
	}

	id := uuid.New().String()
	if m.ID != "" {
		id = m.ID
	}

	query := `
		INSERT INTO label_mappings (
			id, repository, label, provider, instance_type, vcpus, memory_gib, 
			cost_per_hour, is_gpu, gpu_type, gpu_count, gpu_memory_gib
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (team_id, repository, label) DO UPDATE SET
			provider = EXCLUDED.provider,
			instance_type = EXCLUDED.instance_type,
			vcpus = EXCLUDED.vcpus,
			memory_gib = EXCLUDED.memory_gib,
			cost_per_hour = EXCLUDED.cost_per_hour,
			is_gpu = EXCLUDED.is_gpu,
			gpu_type = EXCLUDED.gpu_type,
			gpu_count = EXCLUDED.gpu_count,
			gpu_memory_gib = EXCLUDED.gpu_memory_gib,
			updated_at = NOW()
		RETURNING id`

	err := s.db.QueryRowContext(ctx, query,
		id, m.Repository, m.Label, m.Provider, m.InstanceType, m.VCPUs, m.MemoryGiB,
		m.CostPerHour, m.IsGPU, m.GPUType, m.GPUCount, m.GPUMemoryGiB,
	).Scan(&m.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": m.ID, "status": "ok"})
}

func (s *Server) deleteLabelMapping(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
		return
	}

	_, err := s.db.ExecContext(ctx, `DELETE FROM label_mappings WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ══════════════════════════════════════════════════════════════════════════════
// AUTO-PR SETTINGS
// ══════════════════════════════════════════════════════════════════════════════

func (s *Server) getAutoPRSettings(c *gin.Context) {
	ctx := c.Request.Context()

	var settings AutoPRSettings
	var excludeReposJSON, excludePatternsJSON []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT team_id, enabled, min_savings_percent, min_monthly_savings,
		       require_consecutive_runs, gpu_prs_enabled, gpu_min_savings_percent,
		       COALESCE(exclude_repositories, '[]'), COALESCE(exclude_job_patterns, '[]')
		FROM auto_pr_settings
		LIMIT 1
	`).Scan(
		&settings.TeamID, &settings.Enabled, &settings.MinSavingsPercent,
		&settings.MinMonthlySavings, &settings.RequireConsecutive,
		&settings.GPUPRsEnabled, &settings.GPUMinSavingsPercent,
		&excludeReposJSON, &excludePatternsJSON,
	)

	if err == sql.ErrNoRows {
		// Return defaults
		settings = AutoPRSettings{
			Enabled:              false,
			MinSavingsPercent:    20,
			MinMonthlySavings:    10,
			RequireConsecutive:   3,
			GPUPRsEnabled:        true,
			GPUMinSavingsPercent: 15,
			ExcludeRepositories:  []string{},
			ExcludeJobPatterns:   []string{},
		}
		c.JSON(http.StatusOK, settings)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	json.Unmarshal(excludeReposJSON, &settings.ExcludeRepositories)
	json.Unmarshal(excludePatternsJSON, &settings.ExcludeJobPatterns)

	c.JSON(http.StatusOK, settings)
}

func (s *Server) upsertAutoPRSettings(c *gin.Context) {
	ctx := c.Request.Context()
	var settings AutoPRSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	excludeReposJSON, _ := json.Marshal(settings.ExcludeRepositories)
	excludePatternsJSON, _ := json.Marshal(settings.ExcludeJobPatterns)

	// First ensure we have a default team if none exists
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO teams (id, name, created_at, updated_at)
		VALUES ('default', 'Default Team', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO auto_pr_settings (
			team_id, enabled, min_savings_percent, min_monthly_savings,
			require_consecutive_runs, gpu_prs_enabled, gpu_min_savings_percent,
			exclude_repositories, exclude_job_patterns, updated_at
		) VALUES ('default', $1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (team_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			min_savings_percent = EXCLUDED.min_savings_percent,
			min_monthly_savings = EXCLUDED.min_monthly_savings,
			require_consecutive_runs = EXCLUDED.require_consecutive_runs,
			gpu_prs_enabled = EXCLUDED.gpu_prs_enabled,
			gpu_min_savings_percent = EXCLUDED.gpu_min_savings_percent,
			exclude_repositories = EXCLUDED.exclude_repositories,
			exclude_job_patterns = EXCLUDED.exclude_job_patterns,
			updated_at = NOW()
	`, settings.Enabled, settings.MinSavingsPercent, settings.MinMonthlySavings,
		settings.RequireConsecutive, settings.GPUPRsEnabled, settings.GPUMinSavingsPercent,
		excludeReposJSON, excludePatternsJSON)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ══════════════════════════════════════════════════════════════════════════════
// PR RECOMMENDATIONS
// ══════════════════════════════════════════════════════════════════════════════

func (s *Server) listPRRecommendations(c *gin.Context) {
	ctx := c.Request.Context()
	status := c.DefaultQuery("status", "pending")
	gpuOnly := c.Query("gpu") == "true"

	query := `
		SELECT id, COALESCE(team_id, ''), repository, job_id, COALESCE(workflow_file, ''),
		       current_label, current_vcpus, current_memory_gib, current_cost_per_hour,
		       recommended_label, recommended_vcpus, recommended_memory_gib, recommended_cost_per_hour,
		       COALESCE(p95_cpu_percent, 0), COALESCE(p95_mem_percent, 0),
		       run_count, consecutive_underutilized,
		       is_gpu_job, COALESCE(current_gpu_type, ''), COALESCE(recommended_gpu_type, ''),
		       COALESCE(p95_gpu_util_percent, 0), COALESCE(p95_gpu_mem_percent, 0),
		       savings_percent, monthly_savings_usd, status,
		       COALESCE(pr_url, ''), COALESCE(pr_number, 0), COALESCE(dismissed_reason, ''),
		       created_at, updated_at
		FROM pr_recommendations
		WHERE status = $1`

	if gpuOnly {
		query += ` AND is_gpu_job = TRUE`
	}
	query += ` ORDER BY monthly_savings_usd DESC`

	rows, err := s.db.QueryContext(ctx, query, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	recommendations := []PRRecommendation{}
	for rows.Next() {
		var r PRRecommendation
		if err := rows.Scan(
			&r.ID, &r.TeamID, &r.Repository, &r.JobID, &r.WorkflowFile,
			&r.CurrentLabel, &r.CurrentVCPUs, &r.CurrentMemoryGiB, &r.CurrentCostPerHour,
			&r.RecommendedLabel, &r.RecommendedVCPUs, &r.RecommendedMemoryGiB, &r.RecommendedCostPerHour,
			&r.P95CPUPercent, &r.P95MemPercent,
			&r.RunCount, &r.ConsecutiveUnderutilized,
			&r.IsGPUJob, &r.CurrentGPUType, &r.RecommendedGPUType,
			&r.P95GPUUtilPercent, &r.P95GPUMemPercent,
			&r.SavingsPercent, &r.MonthlySavingsUSD, &r.Status,
			&r.PRUrl, &r.PRNumber, &r.DismissedReason,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		recommendations = append(recommendations, r)
	}
	c.JSON(http.StatusOK, recommendations)
}

func (s *Server) createPRRecommendation(c *gin.Context) {
	ctx := c.Request.Context()
	var r PRRecommendation
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if r.Repository == "" || r.JobID == "" || r.CurrentLabel == "" || r.RecommendedLabel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repository, job_id, current_label, recommended_label required"})
		return
	}

	id := uuid.New().String()
	query := `
		INSERT INTO pr_recommendations (
			id, repository, job_id, workflow_file,
			current_label, current_vcpus, current_memory_gib, current_cost_per_hour,
			recommended_label, recommended_vcpus, recommended_memory_gib, recommended_cost_per_hour,
			p95_cpu_percent, p95_mem_percent, run_count, consecutive_underutilized,
			is_gpu_job, current_gpu_type, recommended_gpu_type,
			p95_gpu_util_percent, p95_gpu_mem_percent,
			savings_percent, monthly_savings_usd, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, 'pending')
		ON CONFLICT (team_id, repository, job_id) DO UPDATE SET
			current_label = EXCLUDED.current_label,
			current_vcpus = EXCLUDED.current_vcpus,
			current_memory_gib = EXCLUDED.current_memory_gib,
			current_cost_per_hour = EXCLUDED.current_cost_per_hour,
			recommended_label = EXCLUDED.recommended_label,
			recommended_vcpus = EXCLUDED.recommended_vcpus,
			recommended_memory_gib = EXCLUDED.recommended_memory_gib,
			recommended_cost_per_hour = EXCLUDED.recommended_cost_per_hour,
			p95_cpu_percent = EXCLUDED.p95_cpu_percent,
			p95_mem_percent = EXCLUDED.p95_mem_percent,
			run_count = pr_recommendations.run_count + 1,
			consecutive_underutilized = pr_recommendations.consecutive_underutilized + 1,
			is_gpu_job = EXCLUDED.is_gpu_job,
			current_gpu_type = EXCLUDED.current_gpu_type,
			recommended_gpu_type = EXCLUDED.recommended_gpu_type,
			p95_gpu_util_percent = EXCLUDED.p95_gpu_util_percent,
			p95_gpu_mem_percent = EXCLUDED.p95_gpu_mem_percent,
			savings_percent = EXCLUDED.savings_percent,
			monthly_savings_usd = EXCLUDED.monthly_savings_usd,
			updated_at = NOW()
		RETURNING id`

	err := s.db.QueryRowContext(ctx, query,
		id, r.Repository, r.JobID, r.WorkflowFile,
		r.CurrentLabel, r.CurrentVCPUs, r.CurrentMemoryGiB, r.CurrentCostPerHour,
		r.RecommendedLabel, r.RecommendedVCPUs, r.RecommendedMemoryGiB, r.RecommendedCostPerHour,
		r.P95CPUPercent, r.P95MemPercent, r.RunCount, r.ConsecutiveUnderutilized,
		r.IsGPUJob, r.CurrentGPUType, r.RecommendedGPUType,
		r.P95GPUUtilPercent, r.P95GPUMemPercent,
		r.SavingsPercent, r.MonthlySavingsUSD,
	).Scan(&r.ID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": r.ID, "status": "ok"})
}

func (s *Server) approvePRRecommendation(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	// Fetch the recommendation details
	var rec PRRecommendation
	err := s.db.QueryRowContext(ctx, `
		SELECT id, repository, job_id, COALESCE(workflow_file, ''),
		       current_label, current_vcpus, current_memory_gib,
		       recommended_label, recommended_vcpus, recommended_memory_gib,
		       p95_cpu_percent, p95_mem_percent, run_count, consecutive_underutilized,
		       savings_percent, monthly_savings_usd
		FROM pr_recommendations WHERE id = $1
	`, id).Scan(
		&rec.ID, &rec.Repository, &rec.JobID, &rec.WorkflowFile,
		&rec.CurrentLabel, &rec.CurrentVCPUs, &rec.CurrentMemoryGiB,
		&rec.RecommendedLabel, &rec.RecommendedVCPUs, &rec.RecommendedMemoryGiB,
		&rec.P95CPUPercent, &rec.P95MemPercent, &rec.RunCount, &rec.ConsecutiveUnderutilized,
		&rec.SavingsPercent, &rec.MonthlySavingsUSD,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch recommendation: " + err.Error()})
		return
	}

	// Try to create a PR via GitHub API
	prURL := ""
	prNumber := 0

	ghClient, err := s.getGitHubClient()
	if err == nil {
		prResult, err := ghClient.CreateRunnerRightSizePR(ctx, ghlib.PROptions{
			Repository:       rec.Repository,
			JobID:            rec.JobID,
			WorkflowFile:     rec.WorkflowFile,
			CurrentLabel:     rec.CurrentLabel,
			NewLabel:         rec.RecommendedLabel,
			CurrentVCPUs:     rec.CurrentVCPUs,
			CurrentMemoryGiB: rec.CurrentMemoryGiB,
			NewVCPUs:         rec.RecommendedVCPUs,
			NewMemoryGiB:     rec.RecommendedMemoryGiB,
			P95CPU:           rec.P95CPUPercent,
			P95Memory:        rec.P95MemPercent,
			RunCount:         rec.RunCount,
			MonthlySavings:   rec.MonthlySavingsUSD,
			SavingsPercent:   rec.SavingsPercent,
			ConsecutiveRuns:  rec.ConsecutiveUnderutilized,
		})
		if err != nil {
			// Log the error but don't fail - still mark as approved
			fmt.Printf("warning: failed to create PR for recommendation %s: %v\n", id, err)
		} else {
			prURL = prResult.PRURL
			prNumber = prResult.PRNumber

			// Also insert into PR history
			s.db.ExecContext(ctx, `
				INSERT INTO pr_history (id, recommendation_id, team_id, repository, job_id, 
				                        pr_number, pr_url, old_label, new_label, 
				                        savings_percent, monthly_savings_usd, is_gpu_job, status)
				VALUES ($1, $2, (SELECT team_id FROM pr_recommendations WHERE id = $2), $3, $4, $5, $6, $7, $8, $9, $10, $11, 'open')
			`, uuid.New().String(), id, rec.Repository, rec.JobID, prNumber, prURL,
				rec.CurrentLabel, rec.RecommendedLabel, rec.SavingsPercent, rec.MonthlySavingsUSD, rec.IsGPUJob)
		}
	}

	// Update the recommendation status
	_, err = s.db.ExecContext(ctx, `
		UPDATE pr_recommendations 
		SET status = 'approved', pr_url = $2, pr_number = $3, updated_at = NOW()
		WHERE id = $1
	`, id, sql.NullString{String: prURL, Valid: prURL != ""}, sql.NullInt64{Int64: int64(prNumber), Valid: prNumber > 0})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Broadcast update to WebSocket clients
	if s.wsHub != nil {
		s.wsHub.Broadcast("recommendation_approved", map[string]interface{}{
			"id":        id,
			"pr_url":    prURL,
			"pr_number": prNumber,
		}, rec.TeamID)
	}

	response := gin.H{"status": "approved"}
	if prURL != "" {
		response["pr_url"] = prURL
		response["pr_number"] = prNumber
	}
	c.JSON(http.StatusOK, response)
}

func (s *Server) dismissPRRecommendation(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	var body struct {
		Reason string `json:"reason"`
	}
	c.ShouldBindJSON(&body)

	_, err := s.db.ExecContext(ctx, `
		UPDATE pr_recommendations 
		SET status = 'dismissed', dismissed_reason = $2, updated_at = NOW()
		WHERE id = $1
	`, id, body.Reason)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Broadcast update to WebSocket clients
	if s.wsHub != nil {
		s.wsHub.Broadcast("recommendation_dismissed", map[string]interface{}{
			"id":     id,
			"reason": body.Reason,
		}, "")
	}

	c.JSON(http.StatusOK, gin.H{"status": "dismissed"})
}

// ══════════════════════════════════════════════════════════════════════════════
// GPU ANALYSIS
// ══════════════════════════════════════════════════════════════════════════════

// GPUTier represents a GPU option with specs and pricing
type GPUTier struct {
	Type        string  `json:"type"`
	Name        string  `json:"name"`
	MemoryGiB   int     `json:"memory_gib"`
	CostPerHour float64 `json:"cost_per_hour"`
	Provider    string  `json:"provider"`
}

var gpuTiers = []GPUTier{
	{Type: "nvidia-t4", Name: "NVIDIA T4", MemoryGiB: 16, CostPerHour: 0.526, Provider: "aws"},
	{Type: "nvidia-l4", Name: "NVIDIA L4", MemoryGiB: 24, CostPerHour: 0.81, Provider: "gcp"},
	{Type: "nvidia-a10g", Name: "NVIDIA A10G", MemoryGiB: 24, CostPerHour: 1.006, Provider: "aws"},
	{Type: "nvidia-l40s", Name: "NVIDIA L40S", MemoryGiB: 48, CostPerHour: 1.50, Provider: "aws"},
	{Type: "nvidia-a100-40gb", Name: "NVIDIA A100 40GB", MemoryGiB: 40, CostPerHour: 3.67, Provider: "aws"},
	{Type: "nvidia-a100-80gb", Name: "NVIDIA A100 80GB", MemoryGiB: 80, CostPerHour: 4.10, Provider: "aws"},
	{Type: "nvidia-h100", Name: "NVIDIA H100", MemoryGiB: 80, CostPerHour: 5.67, Provider: "aws"},
}

func (s *Server) listGPUTiers(c *gin.Context) {
	c.JSON(http.StatusOK, gpuTiers)
}

// GPURecommendationRequest is the input for GPU analysis
type GPURecommendationRequest struct {
	CurrentGPU     string  `json:"current_gpu"`
	P95UtilPercent float64 `json:"p95_util_percent"`
	P95MemPercent  float64 `json:"p95_mem_percent"`
	PeakMemoryGiB  float64 `json:"peak_memory_gib"`
	AvgDurationSec float64 `json:"avg_duration_sec"`
	RunsPerMonth   int     `json:"runs_per_month"`
}

// GPURecommendationResponse is the output of GPU analysis
type GPURecommendationResponse struct {
	CurrentGPU        string  `json:"current_gpu"`
	RecommendedGPU    string  `json:"recommended_gpu"`
	Reason            string  `json:"reason"`
	SavingsPercent    float64 `json:"savings_percent"`
	MonthlySavingsUSD float64 `json:"monthly_savings_usd"`
	Action            string  `json:"action"` // downsize, upsize, keep
}

func (s *Server) getGPURecommendation(c *gin.Context) {
	var req GPURecommendationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find current tier
	var currentTier *GPUTier
	for i := range gpuTiers {
		if gpuTiers[i].Type == req.CurrentGPU {
			currentTier = &gpuTiers[i]
			break
		}
	}
	if currentTier == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown GPU type: " + req.CurrentGPU})
		return
	}

	recommendation := GPURecommendationResponse{
		CurrentGPU: req.CurrentGPU,
	}

	const (
		lowUtilThreshold  = 30.0
		lowMemThreshold   = 40.0
		highUtilThreshold = 85.0
		highMemThreshold  = 90.0
	)

	if req.P95UtilPercent < lowUtilThreshold && req.P95MemPercent < lowMemThreshold {
		// Find smaller GPU that fits memory needs
		for i := range gpuTiers {
			t := gpuTiers[i]
			if float64(t.MemoryGiB) >= req.PeakMemoryGiB*1.2 && t.CostPerHour < currentTier.CostPerHour {
				recommendation.RecommendedGPU = t.Type
				recommendation.Action = "downsize"
				recommendation.SavingsPercent = ((currentTier.CostPerHour - t.CostPerHour) / currentTier.CostPerHour) * 100
				hoursPerMonth := (req.AvgDurationSec / 3600) * float64(req.RunsPerMonth)
				recommendation.MonthlySavingsUSD = (currentTier.CostPerHour - t.CostPerHour) * hoursPerMonth
				recommendation.Reason = "GPU underutilized - compute and memory both below 40%"
				break
			}
		}
		if recommendation.RecommendedGPU == "" {
			recommendation.RecommendedGPU = req.CurrentGPU
			recommendation.Action = "keep"
			recommendation.Reason = "No smaller GPU available that meets memory requirements"
		}
	} else if req.P95UtilPercent > highUtilThreshold || req.P95MemPercent > highMemThreshold {
		// Find larger GPU
		for i := len(gpuTiers) - 1; i >= 0; i-- {
			t := gpuTiers[i]
			if t.CostPerHour > currentTier.CostPerHour {
				recommendation.RecommendedGPU = t.Type
				recommendation.Action = "upsize"
				recommendation.Reason = "GPU saturated - upgrade may reduce job duration"
				break
			}
		}
		if recommendation.RecommendedGPU == "" {
			recommendation.RecommendedGPU = req.CurrentGPU
			recommendation.Action = "keep"
			recommendation.Reason = "Already using the largest available GPU"
		}
	} else {
		recommendation.RecommendedGPU = req.CurrentGPU
		recommendation.Action = "keep"
		recommendation.Reason = "GPU utilization is appropriate"
	}

	c.JSON(http.StatusOK, recommendation)
}

// ══════════════════════════════════════════════════════════════════════════════
// PR HISTORY
// ══════════════════════════════════════════════════════════════════════════════

// PRHistory tracks created PRs
type PRHistory struct {
	ID               string    `json:"id"`
	RecommendationID string    `json:"recommendation_id,omitempty"`
	TeamID           string    `json:"team_id,omitempty"`
	Repository       string    `json:"repository"`
	JobID            string    `json:"job_id"`
	PRNumber         int       `json:"pr_number"`
	PRUrl            string    `json:"pr_url"`
	OldLabel         string    `json:"old_label"`
	NewLabel         string    `json:"new_label"`
	SavingsPercent   float64   `json:"savings_percent"`
	MonthlySavingsUSD float64  `json:"monthly_savings_usd"`
	IsGPUJob         bool      `json:"is_gpu_job"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	MergedAt         *time.Time `json:"merged_at,omitempty"`
	ClosedAt         *time.Time `json:"closed_at,omitempty"`
}

func (s *Server) listPRHistory(c *gin.Context) {
	ctx := c.Request.Context()
	status := c.DefaultQuery("status", "")
	gpuOnly := c.Query("gpu") == "true"

	query := `
		SELECT id, COALESCE(recommendation_id::text, ''), COALESCE(team_id, ''),
		       repository, job_id, pr_number, pr_url, old_label, new_label,
		       COALESCE(savings_percent, 0), COALESCE(monthly_savings_usd, 0),
		       is_gpu_job, status, created_at, merged_at, closed_at
		FROM pr_history
		WHERE 1=1`

	args := []interface{}{}
	argIdx := 1

	if status != "" {
		query += ` AND status = $` + string(rune('0'+argIdx))
		args = append(args, status)
		argIdx++
	}
	if gpuOnly {
		query += ` AND is_gpu_job = TRUE`
	}
	query += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	history := []PRHistory{}
	for rows.Next() {
		var h PRHistory
		if err := rows.Scan(
			&h.ID, &h.RecommendationID, &h.TeamID,
			&h.Repository, &h.JobID, &h.PRNumber, &h.PRUrl, &h.OldLabel, &h.NewLabel,
			&h.SavingsPercent, &h.MonthlySavingsUSD,
			&h.IsGPUJob, &h.Status, &h.CreatedAt, &h.MergedAt, &h.ClosedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		history = append(history, h)
	}
	c.JSON(http.StatusOK, history)
}

// getGitHubClient returns a GitHub API client if GITHUB_TOKEN is set
func (s *Server) getGitHubClient() (*ghlib.Client, error) {
	return ghlib.New()
}

// checkAutoPRCandidate is called after each completed job insertion.
// It scans the last N completed runs of the same job_id+repository and, when
// consecutive underutilisation is detected, upserts a pr_recommendations row
// so it surfaces on the Auto PR page.
func (s *Server) checkAutoPRCandidate(jobID, repository string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// ── Load settings (fall back to defaults if not configured) ──────────────
	var minSavingsPct float64 = 20
	var requireConsec = 5
	_ = s.db.QueryRowContext(ctx,
		`SELECT min_savings_percent, require_consecutive_runs FROM auto_pr_settings LIMIT 1`,
	).Scan(&minSavingsPct, &requireConsec)

	// ── Query the last N completed runs ───────────────────────────────────────
	type runRow struct {
		summaryJSON []byte
		recsJSON    []byte
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT summary, recommendations
		FROM jobs
		WHERE job_id = $1
		  AND ($2 = '' OR repository = $2)
		  AND status = 'completed'
		ORDER BY created_at DESC
		LIMIT $3
	`, jobID, repository, requireConsec)
	if err != nil {
		return
	}
	defer rows.Close()

	var runs []runRow
	for rows.Next() {
		var r runRow
		if err := rows.Scan(&r.summaryJSON, &r.recsJSON); err == nil {
			runs = append(runs, r)
		}
	}
	if len(runs) < requireConsec {
		return // not enough history yet
	}

	// ── Check consecutive underutilisation (p95 CPU < 25 % on all N runs) ────
	const cpuThreshold = 25.0
	for _, r := range runs {
		var summary types.MetricsSummary
		if err := json.Unmarshal(r.summaryJSON, &summary); err != nil {
			return
		}
		if summary.CPUPercentP95 > cpuThreshold {
			return
		}
	}

	// ── Use the most recent run's data ────────────────────────────────────────
	var latestSummary types.MetricsSummary
	var latestRecs []types.Recommendation
	if err := json.Unmarshal(runs[0].summaryJSON, &latestSummary); err != nil {
		return
	}
	_ = json.Unmarshal(runs[0].recsJSON, &latestRecs)

	if latestSummary.DetectedMachine == nil || len(latestRecs) == 0 {
		return
	}

	// Find cheapest recommendation with a meaningful saving
	var bestRec *types.Recommendation
	for i := range latestRecs {
		if latestRecs[i].CostDeltaPercent < 0 {
			if bestRec == nil || latestRecs[i].CostDeltaPercent < bestRec.CostDeltaPercent {
				bestRec = &latestRecs[i]
			}
		}
	}
	if bestRec == nil {
		return
	}

	savingsPct := -bestRec.CostDeltaPercent
	if savingsPct < minSavingsPct {
		return
	}

	// ── Resolve runner labels via label_mappings (best-effort) ───────────────
	currentLabel := latestSummary.DetectedMachine.ID
	recommendedLabel := bestRec.Machine.ID

	var mapped string
	if err := s.db.QueryRowContext(ctx,
		`SELECT label FROM label_mappings WHERE instance_type = $1 ORDER BY created_at LIMIT 1`,
		latestSummary.DetectedMachine.ID,
	).Scan(&mapped); err == nil && mapped != "" {
		currentLabel = mapped
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT label FROM label_mappings WHERE instance_type = $1 ORDER BY created_at LIMIT 1`,
		bestRec.Machine.ID,
	).Scan(&mapped); err == nil && mapped != "" {
		recommendedLabel = mapped
	}

	// ── Estimate monthly savings (assumes ~22 runs / month) ───────────────────
	const runsPerMonth = 22.0
	runHours := latestSummary.DurationSeconds / 3600.0
	currentMonthly := latestSummary.DetectedMachine.OnDemandPricePerHour * runHours * runsPerMonth
	recommendedMonthly := bestRec.Machine.OnDemandPricePerHour * runHours * runsPerMonth
	monthlySavings := currentMonthly - recommendedMonthly

	// ── Upsert: update existing pending row, or insert new one ────────────────
	res, err := s.db.ExecContext(ctx, `
		UPDATE pr_recommendations
		SET run_count              = run_count + 1,
		    consecutive_underutilized = consecutive_underutilized + 1,
		    p95_cpu_percent        = $1,
		    p95_mem_percent        = $2,
		    savings_percent        = $3,
		    monthly_savings_usd    = $4,
		    updated_at             = NOW()
		WHERE repository = $5
		  AND job_id     = $6
		  AND status     = 'pending'
		  AND team_id IS NULL
	`, latestSummary.CPUPercentP95, latestSummary.MemUsedGiBP95,
		savingsPct, monthlySavings, repository, jobID)
	if err != nil {
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return // updated existing
	}

	// Insert new recommendation
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO pr_recommendations (
			id, repository, job_id,
			current_label, current_vcpus, current_memory_gib, current_cost_per_hour,
			recommended_label, recommended_vcpus, recommended_memory_gib, recommended_cost_per_hour,
			p95_cpu_percent, p95_mem_percent,
			run_count, consecutive_underutilized,
			savings_percent, monthly_savings_usd, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14,$15,$16,'pending')
	`, uuid.New().String(), repository, jobID,
		currentLabel,
		latestSummary.DetectedMachine.VCPUs,
		latestSummary.DetectedMachine.MemoryGiB,
		latestSummary.DetectedMachine.OnDemandPricePerHour,
		recommendedLabel,
		bestRec.Machine.VCPUs,
		bestRec.Machine.MemoryGiB,
		bestRec.Machine.OnDemandPricePerHour,
		latestSummary.CPUPercentP95,
		latestSummary.MemUsedGiBP95,
		requireConsec,
		savingsPct,
		monthlySavings,
	)
}
