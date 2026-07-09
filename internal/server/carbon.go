package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// CarbonMetrics holds carbon footprint estimates.
type CarbonMetrics struct {
	TotalKgCO2         float64 `json:"total_kg_co2"`
	SavedKgCO2         float64 `json:"saved_kg_co2"`
	EquivalentTreeDays float64 `json:"equivalent_tree_days"`
	EquivalentMiles    float64 `json:"equivalent_car_miles"`
}

// PublicStats holds publicly shareable aggregate stats.
type PublicStats struct {
	TotalJobs          int            `json:"total_jobs"`
	TotalOrgs          int            `json:"total_orgs"`
	TotalSavingsUSD    float64        `json:"total_savings_usd"`
	TotalCarbonSavedKg float64        `json:"total_carbon_saved_kg"`
	LastUpdated        time.Time      `json:"last_updated"`
	ByProvider         []ProviderStat `json:"by_provider,omitempty"`
}

// ProviderStat shows stats per cloud provider.
type ProviderStat struct {
	Provider   string  `json:"provider"`
	JobCount   int     `json:"job_count"`
	SavingsUSD float64 `json:"savings_usd"`
}

// RunDiff represents a comparison between two runs.
type RunDiff struct {
	Before        RunSnapshot   `json:"before"`
	After         RunSnapshot   `json:"after"`
	CPUDelta      float64       `json:"cpu_delta_percent"`
	MemoryDelta   float64       `json:"memory_delta_gib"`
	DurationDelta float64       `json:"duration_delta_seconds"`
	CostDelta     float64       `json:"cost_delta_usd"`
	CarbonDelta   float64       `json:"carbon_delta_kg"`
	Insights      []DiffInsight `json:"insights"`
}

// RunSnapshot is a simplified view of a run for comparison.
type RunSnapshot struct {
	ID              int       `json:"id"`
	RunID           string    `json:"run_id"`
	JobID           string    `json:"job_id"`
	StartTime       time.Time `json:"start_time"`
	DurationSeconds float64   `json:"duration_seconds"`
	CPUPercentP95   float64   `json:"cpu_percent_p95"`
	MemUsedGiBP95   float64   `json:"mem_used_gib_p95"`
	DetectedMachine string    `json:"detected_machine,omitempty"`
	TopRecommend    string    `json:"top_recommend,omitempty"`
	CostDeltaPct    float64   `json:"cost_delta_percent"`
	EstCarbonKg     float64   `json:"est_carbon_kg"`
}

// DiffInsight provides actionable insights from run comparison.
type DiffInsight struct {
	Type    string `json:"type"` // warning, improvement, neutral
	Message string `json:"message"`
}

// Carbon emission factors (kg CO2 per kWh by region/provider)
// Source: EPA, cloud provider sustainability reports
var carbonFactors = map[string]float64{
	"aws":    0.417, // US average
	"gcp":    0.180, // GCP is more renewable-heavy
	"azure":  0.350, // Azure average
	"github": 0.417, // GitHub Actions runs on Azure/AWS mix
	"":       0.400, // Default
}

// estimateCarbonKg estimates carbon emissions for a job.
// Uses approximate power consumption based on machine specs.
func estimateCarbonKg(provider string, vcpus int, memGiB float64, durationHrs float64) float64 {
	// Rough power estimates:
	// - ~10W per vCPU under load
	// - ~0.3W per GiB RAM
	// - Base system overhead ~20W
	powerW := 20.0 + float64(vcpus)*10.0 + memGiB*0.3
	kWh := (powerW / 1000.0) * durationHrs

	factor := carbonFactors[provider]
	if factor == 0 {
		factor = carbonFactors[""]
	}

	return kWh * factor
}

// --- Carbon Footprint Handler ---

func (s *Server) getCarbonFootprint(c *gin.Context) {
	ctx := c.Request.Context()
	teamID := c.Query("team_id")
	startDate, endDate := parsePeriodRange(c)

	// Calculate actual carbon from jobs
	query := `
		SELECT 
			COALESCE(summary->'detected_machine'->>'provider', 'unknown') as provider,
			COALESCE((summary->'detected_machine'->>'vcpus')::int, 2) as vcpus,
			COALESCE((summary->>'mem_total_gib')::numeric, 4) as mem_gib,
			COALESCE((summary->>'duration_seconds')::numeric / 3600, 0) as duration_hrs,
			COALESCE((summary->'detected_machine'->>'on_demand_price_per_hour')::numeric, 0.1) as current_price,
			COALESCE((recommendations->0->'machine'->>'on_demand_price_per_hour')::numeric, 
				(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric) as rec_price,
			COALESCE((recommendations->0->'machine'->>'vcpus')::int, 
				(summary->'detected_machine'->>'vcpus')::int) as rec_vcpus,
			COALESCE((recommendations->0->'machine'->>'memory_gib')::numeric, 
				(summary->>'mem_total_gib')::numeric) as rec_mem
		FROM jobs
		WHERE created_at >= $1 AND created_at <= $2 AND summary IS NOT NULL
	`
	args := []any{startDate, endDate}
	if teamID != "" {
		query += " AND team_id = $3"
		args = append(args, teamID)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to calculate carbon"})
		return
	}
	defer rows.Close()

	var totalCarbon, savedCarbon float64

	for rows.Next() {
		var provider string
		var vcpus, recVcpus int
		var memGiB, durationHrs, currentPrice, recPrice, recMem float64

		if err := rows.Scan(&provider, &vcpus, &memGiB, &durationHrs, &currentPrice, &recPrice, &recVcpus, &recMem); err != nil {
			continue
		}

		// Current carbon
		currentCarbon := estimateCarbonKg(provider, vcpus, memGiB, durationHrs)
		totalCarbon += currentCarbon

		// Carbon if using recommended machine
		if recPrice < currentPrice && recVcpus > 0 {
			recCarbon := estimateCarbonKg(provider, recVcpus, recMem, durationHrs)
			savedCarbon += currentCarbon - recCarbon
		}
	}

	metrics := CarbonMetrics{
		TotalKgCO2:         totalCarbon,
		SavedKgCO2:         savedCarbon,
		EquivalentTreeDays: savedCarbon / 0.06,  // Avg tree absorbs ~21kg CO2/year = 0.06/day
		EquivalentMiles:    savedCarbon / 0.404, // Avg car emits 404g CO2/mile
	}

	c.JSON(http.StatusOK, metrics)
}

// --- Public Stats Handler (no auth required) ---

func (s *Server) getPublicStats(c *gin.Context) {
	ctx := c.Request.Context()

	stats := PublicStats{
		LastUpdated: time.Now(),
	}

	// Total jobs and orgs
	err := s.db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*),
			COUNT(DISTINCT team_id)
		FROM jobs WHERE summary IS NOT NULL
	`).Scan(&stats.TotalJobs, &stats.TotalOrgs)
	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	// Total savings
	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(
			CASE WHEN recommendations IS NOT NULL AND jsonb_array_length(recommendations) > 0
			THEN (
				(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric - 
				COALESCE((recommendations->0->'machine'->>'on_demand_price_per_hour')::numeric, 
					(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric)
			) * (summary->>'duration_seconds')::numeric / 3600
			ELSE 0 END
		), 0) * 720  -- Extrapolate to monthly
		FROM jobs WHERE summary IS NOT NULL
	`).Scan(&stats.TotalSavingsUSD)
	if err != nil && err != sql.ErrNoRows {
		// Continue anyway
	}

	// Calculate total carbon saved
	rows, err := s.db.QueryContext(ctx, `
		SELECT 
			COALESCE(summary->'detected_machine'->>'provider', 'unknown'),
			COALESCE((summary->'detected_machine'->>'vcpus')::int, 2),
			COALESCE((summary->>'mem_total_gib')::numeric, 4),
			COALESCE((summary->>'duration_seconds')::numeric / 3600, 0),
			COALESCE((recommendations->0->'machine'->>'vcpus')::int, (summary->'detected_machine'->>'vcpus')::int),
			COALESCE((recommendations->0->'machine'->>'memory_gib')::numeric, (summary->>'mem_total_gib')::numeric),
			COALESCE((summary->'detected_machine'->>'on_demand_price_per_hour')::numeric, 0.1),
			COALESCE((recommendations->0->'machine'->>'on_demand_price_per_hour')::numeric, 
				(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric)
		FROM jobs 
		WHERE summary IS NOT NULL AND recommendations IS NOT NULL
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var provider string
			var vcpus, recVcpus int
			var memGiB, durationHrs, recMem, currentPrice, recPrice float64
			if err := rows.Scan(&provider, &vcpus, &memGiB, &durationHrs, &recVcpus, &recMem, &currentPrice, &recPrice); err == nil {
				if recPrice < currentPrice && recVcpus > 0 {
					currentCarbon := estimateCarbonKg(provider, vcpus, memGiB, durationHrs)
					recCarbon := estimateCarbonKg(provider, recVcpus, recMem, durationHrs)
					stats.TotalCarbonSavedKg += currentCarbon - recCarbon
				}
			}
		}
	}

	// By provider breakdown
	providerRows, err := s.db.QueryContext(ctx, `
		SELECT 
			COALESCE(summary->'detected_machine'->>'provider', 'unknown') as provider,
			COUNT(*) as job_count,
			COALESCE(SUM(
				CASE WHEN recommendations IS NOT NULL AND jsonb_array_length(recommendations) > 0
				THEN (
					(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric - 
					COALESCE((recommendations->0->'machine'->>'on_demand_price_per_hour')::numeric, 
						(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric)
				) * (summary->>'duration_seconds')::numeric / 3600
				ELSE 0 END
			), 0) * 720 as savings
		FROM jobs 
		WHERE summary IS NOT NULL
		GROUP BY provider
		ORDER BY savings DESC
	`)
	if err == nil {
		defer providerRows.Close()
		for providerRows.Next() {
			var ps ProviderStat
			if err := providerRows.Scan(&ps.Provider, &ps.JobCount, &ps.SavingsUSD); err == nil {
				stats.ByProvider = append(stats.ByProvider, ps)
			}
		}
	}

	c.JSON(http.StatusOK, stats)
}

// --- Run Diff Handler ---

func (s *Server) compareRuns(c *gin.Context) {
	ctx := c.Request.Context()

	beforeID, err := strconv.Atoi(c.Query("before"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before id"})
		return
	}
	afterID, err := strconv.Atoi(c.Query("after"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after id"})
		return
	}

	// Load both runs
	before, err := s.loadRunSnapshot(ctx, beforeID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "before run not found"})
		return
	}
	after, err := s.loadRunSnapshot(ctx, afterID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "after run not found"})
		return
	}

	diff := RunDiff{
		Before:        *before,
		After:         *after,
		CPUDelta:      after.CPUPercentP95 - before.CPUPercentP95,
		MemoryDelta:   after.MemUsedGiBP95 - before.MemUsedGiBP95,
		DurationDelta: after.DurationSeconds - before.DurationSeconds,
		CarbonDelta:   after.EstCarbonKg - before.EstCarbonKg,
		CostDelta:     after.CostDeltaPct - before.CostDeltaPct,
	}

	// Generate insights
	if diff.CPUDelta > 10 {
		diff.Insights = append(diff.Insights, DiffInsight{
			Type:    "warning",
			Message: fmt.Sprintf("CPU usage increased significantly (+%.1f%%). Consider investigating build changes.", diff.CPUDelta),
		})
	} else if diff.CPUDelta < -10 {
		diff.Insights = append(diff.Insights, DiffInsight{
			Type:    "improvement",
			Message: fmt.Sprintf("CPU usage decreased (%.1f%%). Your optimizations are working!", diff.CPUDelta),
		})
	}

	if diff.MemoryDelta > 1 {
		diff.Insights = append(diff.Insights, DiffInsight{
			Type:    "warning",
			Message: fmt.Sprintf("Memory usage increased by %.2f GiB. Check for memory leaks or new dependencies.", diff.MemoryDelta),
		})
	} else if diff.MemoryDelta < -1 {
		diff.Insights = append(diff.Insights, DiffInsight{
			Type:    "improvement",
			Message: fmt.Sprintf("Memory usage decreased by %.2f GiB. Good job on optimization!", -diff.MemoryDelta),
		})
	}

	if diff.DurationDelta > 60 {
		diff.Insights = append(diff.Insights, DiffInsight{
			Type:    "warning",
			Message: fmt.Sprintf("Build time increased by %.0f seconds. Consider caching improvements.", diff.DurationDelta),
		})
	} else if diff.DurationDelta < -60 {
		diff.Insights = append(diff.Insights, DiffInsight{
			Type:    "improvement",
			Message: fmt.Sprintf("Build time decreased by %.0f seconds. Nice speedup!", -diff.DurationDelta),
		})
	}

	c.JSON(http.StatusOK, diff)
}

func (s *Server) loadRunSnapshot(ctx context.Context, id int) (*RunSnapshot, error) {
	var snapshot RunSnapshot
	var summaryJSON, recsJSON []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT id, job_id, summary, recommendations, created_at
		FROM jobs WHERE id = $1
	`, id).Scan(&snapshot.ID, &snapshot.JobID, &summaryJSON, &recsJSON, &snapshot.StartTime)
	if err != nil {
		return nil, err
	}

	// Parse summary
	var summary map[string]any
	if err := json.Unmarshal(summaryJSON, &summary); err == nil {
		if v, ok := summary["run_id"].(string); ok {
			snapshot.RunID = v
		}
		if v, ok := summary["duration_seconds"].(float64); ok {
			snapshot.DurationSeconds = v
		}
		if v, ok := summary["cpu_percent_p95"].(float64); ok {
			snapshot.CPUPercentP95 = v
		}
		if v, ok := summary["mem_used_gib_p95"].(float64); ok {
			snapshot.MemUsedGiBP95 = v
		}
		if dm, ok := summary["detected_machine"].(map[string]any); ok {
			if v, ok := dm["id"].(string); ok {
				snapshot.DetectedMachine = v
			}
		}
	}

	// Parse recommendations
	var recs []map[string]any
	if err := json.Unmarshal(recsJSON, &recs); err == nil && len(recs) > 0 {
		if m, ok := recs[0]["machine"].(map[string]any); ok {
			if v, ok := m["id"].(string); ok {
				snapshot.TopRecommend = v
			}
		}
		if v, ok := recs[0]["cost_delta_percent"].(float64); ok {
			snapshot.CostDeltaPct = v
		}
	}

	// Estimate carbon
	provider := ""
	vcpus := 2
	memGiB := 4.0
	if summary != nil {
		if dm, ok := summary["detected_machine"].(map[string]any); ok {
			if v, ok := dm["provider"].(string); ok {
				provider = v
			}
			if v, ok := dm["vcpus"].(float64); ok {
				vcpus = int(v)
			}
		}
		if v, ok := summary["mem_total_gib"].(float64); ok {
			memGiB = v
		}
	}
	snapshot.EstCarbonKg = estimateCarbonKg(provider, vcpus, memGiB, snapshot.DurationSeconds/3600)

	return &snapshot, nil
}

// --- Run History Handler ---

func (s *Server) getRunHistory(c *gin.Context) {
	ctx := c.Request.Context()
	jobID := c.Query("job_id")
	repository := c.Query("repository")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit > 200 {
		limit = 200
	}

	query := `
		SELECT 
			id, job_id, 
			summary->>'run_id' as run_id,
			summary->>'repository' as repository,
			created_at,
			COALESCE((summary->>'duration_seconds')::numeric, 0) as duration,
			COALESCE((summary->>'cpu_percent_p95')::numeric, 0) as cpu_p95,
			COALESCE((summary->>'mem_used_gib_p95')::numeric, 0) as mem_p95,
			summary->'detected_machine'->>'id' as detected_machine,
			recommendations->0->'machine'->>'id' as top_recommend,
			COALESCE((recommendations->0->>'cost_delta_percent')::numeric, 0) as cost_delta
		FROM jobs
		WHERE summary IS NOT NULL
	`
	args := []any{}
	argIdx := 1

	if jobID != "" {
		query += " AND job_id = $" + strconv.Itoa(argIdx)
		args = append(args, jobID)
		argIdx++
	}
	if repository != "" {
		query += " AND summary->>'repository' = $" + strconv.Itoa(argIdx)
		args = append(args, repository)
		argIdx++
	}

	query += " ORDER BY created_at DESC"
	query += " LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get history"})
		return
	}
	defer rows.Close()

	var history []RunSnapshot
	for rows.Next() {
		var r RunSnapshot
		var repo, detected, topRec sql.NullString
		err := rows.Scan(
			&r.ID, &r.JobID, &r.RunID, &repo, &r.StartTime,
			&r.DurationSeconds, &r.CPUPercentP95, &r.MemUsedGiBP95,
			&detected, &topRec, &r.CostDeltaPct,
		)
		if err != nil {
			continue
		}
		r.DetectedMachine = detected.String
		r.TopRecommend = topRec.String
		history = append(history, r)
	}

	c.JSON(http.StatusOK, gin.H{"runs": history})
}
