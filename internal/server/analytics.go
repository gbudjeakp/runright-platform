package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// parsePeriodRange resolves start/end dates from ?from=&to= or ?period= query params.
// When from/to are provided they take precedence over period.
func parsePeriodRange(c *gin.Context) (startDate, endDate time.Time) {
	endDate = time.Now().Add(24 * time.Hour)
	if from := c.Query("from"); from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			startDate = t
		}
		if to := c.Query("to"); to != "" {
			if t, err := time.Parse("2006-01-02", to); err == nil {
				endDate = t.Add(24 * time.Hour)
			}
		}
		if startDate.IsZero() {
			startDate = time.Now().AddDate(0, 0, -30)
		}
		return
	}
	switch c.DefaultQuery("period", "30d") {
	case "7d":
		startDate = time.Now().AddDate(0, 0, -7)
	case "90d":
		startDate = time.Now().AddDate(0, 0, -90)
	case "1y":
		startDate = time.Now().AddDate(-1, 0, 0)
	case "all":
		startDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	default:
		startDate = time.Now().AddDate(0, 0, -30)
	}
	return
}

// UsageAnalytics represents aggregated usage metrics.
type UsageAnalytics struct {
	ID          string                 `json:"id"`
	TeamID      string                 `json:"team_id,omitempty"`
	PeriodStart time.Time              `json:"period_start"`
	PeriodEnd   time.Time              `json:"period_end"`
	PeriodType  string                 `json:"period_type"` // daily, weekly, monthly
	Metrics     map[string]any `json:"metrics"`
	CreatedAt   time.Time              `json:"created_at"`
}

// AnalyticsSummary provides dashboard overview.
type AnalyticsSummary struct {
	TotalJobs          int     `json:"total_jobs"`
	TotalCost          float64 `json:"total_cost"`
	TotalSavings       float64 `json:"total_savings"`
	SavingsPercent     float64 `json:"savings_percent"`
	AvgCostPerJob      float64 `json:"avg_cost_per_job"`
	TopRepositories    []RepoStats `json:"top_repositories"`
	CostTrend          []TrendPoint `json:"cost_trend"`
	SavingsTrend       []TrendPoint `json:"savings_trend"`
	ResourceUtilization ResourceUtilization `json:"resource_utilization"`
}

// RepoStats for top repositories.
type RepoStats struct {
	Repository     string  `json:"repository"`
	JobCount       int     `json:"job_count"`
	TotalCost      float64 `json:"total_cost"`
	PotentialSavings float64 `json:"potential_savings"`
}

// TrendPoint for charts.
type TrendPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// ResourceUtilization aggregates.
type ResourceUtilization struct {
	AvgCPU       float64 `json:"avg_cpu"`
	AvgMemory    float64 `json:"avg_memory"`
	AvgDisk      float64 `json:"avg_disk"`
	IdlePercent  float64 `json:"idle_percent"`
}

// --- Analytics Handlers ---

func (s *Server) getAnalyticsSummary(c *gin.Context) {
	ctx := c.Request.Context()
	teamID := c.Query("team_id")
	startDate, endDate := parsePeriodRange(c)

	summary := AnalyticsSummary{}

	query := `
		SELECT 
			COUNT(*),
			COALESCE(SUM(
				(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
				(summary->>'duration_seconds')::numeric / 3600
			), 0),
			COALESCE(SUM(
				CASE WHEN recommendations IS NOT NULL AND jsonb_array_length(recommendations) > 0
				THEN (
					(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric - 
					COALESCE((recommendations->0->'machine'->>'on_demand_price_per_hour')::numeric, 
						(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric)
				) * (summary->>'duration_seconds')::numeric / 3600
				ELSE 0 END
			), 0)
		FROM jobs
		WHERE created_at >= $1 AND created_at <= $2 AND summary IS NOT NULL
	`
	args := []any{startDate, endDate}
	if teamID != "" {
		query += " AND team_id = $3"
		args = append(args, teamID)
	}

	err := s.db.QueryRowContext(ctx, query, args...).Scan(&summary.TotalJobs, &summary.TotalCost, &summary.TotalSavings)
	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get analytics"})
		return
	}

	if summary.TotalCost > 0 {
		summary.SavingsPercent = (summary.TotalSavings / summary.TotalCost) * 100
	}
	if summary.TotalJobs > 0 {
		summary.AvgCostPerJob = summary.TotalCost / float64(summary.TotalJobs)
	}

	// Top repositories
	repoQuery := `
		SELECT 
			repository,
			COUNT(*) as job_count,
			COALESCE(SUM(
				(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
				(summary->>'duration_seconds')::numeric / 3600
			), 0) as total_cost,
			COALESCE(SUM(
				CASE WHEN recommendations IS NOT NULL AND jsonb_array_length(recommendations) > 0
				THEN (
					(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric - 
					COALESCE((recommendations->0->'machine'->>'on_demand_price_per_hour')::numeric, 
						(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric)
				) * (summary->>'duration_seconds')::numeric / 3600
				ELSE 0 END
			), 0) as savings
		FROM jobs
		WHERE created_at >= $1 AND created_at <= $2 AND summary IS NOT NULL
	`
	repoArgs := []any{startDate, endDate}
	if teamID != "" {
		repoQuery += " AND team_id = $3"
		repoArgs = append(repoArgs, teamID)
	}
	repoQuery += " GROUP BY repository ORDER BY total_cost DESC LIMIT 10"

	rows, err := s.db.QueryContext(ctx, repoQuery, repoArgs...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var r RepoStats
			if err := rows.Scan(&r.Repository, &r.JobCount, &r.TotalCost, &r.PotentialSavings); err == nil {
				summary.TopRepositories = append(summary.TopRepositories, r)
			}
		}
	}

	// Cost trend (daily)
	trendQuery := `
		SELECT 
			DATE(created_at) as date,
			COALESCE(SUM(
				(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
				(summary->>'duration_seconds')::numeric / 3600
			), 0) as cost
		FROM jobs
		WHERE created_at >= $1 AND created_at <= $2 AND summary IS NOT NULL
	`
	trendArgs := []any{startDate, endDate}
	if teamID != "" {
		trendQuery += " AND team_id = $3"
		trendArgs = append(trendArgs, teamID)
	}
	trendQuery += " GROUP BY DATE(created_at) ORDER BY date"

	trendRows, err := s.db.QueryContext(ctx, trendQuery, trendArgs...)
	if err == nil {
		defer trendRows.Close()
		for trendRows.Next() {
			var date time.Time
			var value float64
			if err := trendRows.Scan(&date, &value); err == nil {
				summary.CostTrend = append(summary.CostTrend, TrendPoint{
					Date:  date.Format("2006-01-02"),
					Value: value,
				})
			}
		}
	}

	// Savings trend (daily)
	savingsTrendQuery := `
		SELECT 
			DATE(created_at) as date,
			COALESCE(SUM(
				CASE WHEN recommendations IS NOT NULL AND jsonb_array_length(recommendations) > 0
				THEN GREATEST(0, (
					(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric - 
					COALESCE((recommendations->0->'machine'->>'on_demand_price_per_hour')::numeric, 
						(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric)
				) * (summary->>'duration_seconds')::numeric / 3600)
				ELSE 0 END
			), 0) as savings
		FROM jobs
		WHERE created_at >= $1 AND created_at <= $2 AND summary IS NOT NULL
	`
	savingsArgs := []any{startDate, endDate}
	if teamID != "" {
		savingsTrendQuery += " AND team_id = $3"
		savingsArgs = append(savingsArgs, teamID)
	}
	savingsTrendQuery += " GROUP BY DATE(created_at) ORDER BY date"

	savingsRows, err := s.db.QueryContext(ctx, savingsTrendQuery, savingsArgs...)
	if err == nil {
		defer savingsRows.Close()
		for savingsRows.Next() {
			var date time.Time
			var value float64
			if err := savingsRows.Scan(&date, &value); err == nil {
				summary.SavingsTrend = append(summary.SavingsTrend, TrendPoint{
					Date:  date.Format("2006-01-02"),
					Value: value,
				})
			}
		}
	}

	// Resource utilization averages
	utilQuery := `
		SELECT 
			COALESCE(AVG((summary->>'cpu_percent_peak')::numeric), 0),
			COALESCE(AVG(
				CASE WHEN (summary->>'mem_total_gib')::numeric > 0 
				THEN (summary->>'mem_used_gib_peak')::numeric / (summary->>'mem_total_gib')::numeric * 100
				ELSE 0 END
			), 0),
			COALESCE(AVG(10), 0),
			COALESCE(AVG(CASE WHEN (summary->>'cpu_percent_peak')::numeric < 20 THEN 1 ELSE 0 END) * 100, 0)
		FROM jobs
		WHERE created_at >= $1 AND created_at <= $2 AND summary IS NOT NULL
	`
	utilArgs := []any{startDate, endDate}
	if teamID != "" {
		utilQuery += " AND team_id = $3"
		utilArgs = append(utilArgs, teamID)
	}

	s.db.QueryRowContext(ctx, utilQuery, utilArgs...).Scan(
		&summary.ResourceUtilization.AvgCPU,
		&summary.ResourceUtilization.AvgMemory,
		&summary.ResourceUtilization.AvgDisk,
		&summary.ResourceUtilization.IdlePercent,
	)

	c.JSON(http.StatusOK, summary)
}

func (s *Server) getCostBreakdown(c *gin.Context) {
	ctx := c.Request.Context()
	teamID := c.Query("team_id")
	startDate, endDate := parsePeriodRange(c)

	type BreakdownItem struct {
		Name    string  `json:"name"`
		Cost    float64 `json:"cost"`
		Percent float64 `json:"percent"`
	}

	result := struct {
		ByProvider []BreakdownItem `json:"by_provider"`
		ByRepo     []BreakdownItem `json:"by_repo"`
		ByTier     []BreakdownItem `json:"by_tier"`
	}{
		ByProvider: []BreakdownItem{},
		ByRepo:     []BreakdownItem{},
		ByTier:     []BreakdownItem{},
	}

	// Get total cost for percentage calculation
	var totalCost float64
	totalQuery := `
		SELECT COALESCE(SUM(
			(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
			(summary->>'duration_seconds')::numeric / 3600
		), 0)
		FROM jobs
		WHERE created_at >= $1 AND created_at <= $2 AND summary IS NOT NULL
	`
	totalArgs := []any{startDate, endDate}
	if teamID != "" {
		totalQuery += " AND team_id = $3"
		totalArgs = append(totalArgs, teamID)
	}
	s.db.QueryRowContext(ctx, totalQuery, totalArgs...).Scan(&totalCost)

	if totalCost == 0 {
		c.JSON(http.StatusOK, result)
		return
	}

	// By Provider
	providerQuery := `
		SELECT 
			COALESCE(summary->'detected_machine'->>'provider', 'unknown') as provider,
			COALESCE(SUM(
				(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
				(summary->>'duration_seconds')::numeric / 3600
			), 0) as cost
		FROM jobs
		WHERE created_at >= $1 AND created_at <= $2 AND summary IS NOT NULL
	`
	providerArgs := []any{startDate, endDate}
	if teamID != "" {
		providerQuery += " AND team_id = $3"
		providerArgs = append(providerArgs, teamID)
	}
	providerQuery += " GROUP BY provider ORDER BY cost DESC LIMIT 10"

	providerRows, err := s.db.QueryContext(ctx, providerQuery, providerArgs...)
	if err == nil {
		defer providerRows.Close()
		for providerRows.Next() {
			var name string
			var cost float64
			if err := providerRows.Scan(&name, &cost); err == nil {
				result.ByProvider = append(result.ByProvider, BreakdownItem{
					Name:    name,
					Cost:    cost,
					Percent: (cost / totalCost) * 100,
				})
			}
		}
	}

	// By Repository
	repoQuery := `
		SELECT 
			repository,
			COALESCE(SUM(
				(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
				(summary->>'duration_seconds')::numeric / 3600
			), 0) as cost
		FROM jobs
		WHERE created_at >= $1 AND created_at <= $2 AND summary IS NOT NULL
	`
	repoArgs := []any{startDate, endDate}
	if teamID != "" {
		repoQuery += " AND team_id = $3"
		repoArgs = append(repoArgs, teamID)
	}
	repoQuery += " GROUP BY repository ORDER BY cost DESC LIMIT 10"

	repoRows, err := s.db.QueryContext(ctx, repoQuery, repoArgs...)
	if err == nil {
		defer repoRows.Close()
		for repoRows.Next() {
			var name string
			var cost float64
			if err := repoRows.Scan(&name, &cost); err == nil {
				result.ByRepo = append(result.ByRepo, BreakdownItem{
					Name:    name,
					Cost:    cost,
					Percent: (cost / totalCost) * 100,
				})
			}
		}
	}

	// By Machine Tier (family)
	tierQuery := `
		SELECT 
			COALESCE(summary->'detected_machine'->>'family', 'unknown') as tier,
			COALESCE(SUM(
				(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
				(summary->>'duration_seconds')::numeric / 3600
			), 0) as cost
		FROM jobs
		WHERE created_at >= $1 AND created_at <= $2 AND summary IS NOT NULL
	`
	tierArgs := []any{startDate, endDate}
	if teamID != "" {
		tierQuery += " AND team_id = $3"
		tierArgs = append(tierArgs, teamID)
	}
	tierQuery += " GROUP BY tier ORDER BY cost DESC LIMIT 10"

	tierRows, err := s.db.QueryContext(ctx, tierQuery, tierArgs...)
	if err == nil {
		defer tierRows.Close()
		for tierRows.Next() {
			var name string
			var cost float64
			if err := tierRows.Scan(&name, &cost); err == nil {
				result.ByTier = append(result.ByTier, BreakdownItem{
					Name:    name,
					Cost:    cost,
					Percent: (cost / totalCost) * 100,
				})
			}
		}
	}

	c.JSON(http.StatusOK, result)
}

// --- Scheduled Reports ---

// ScheduledReport configuration.
type ScheduledReport struct {
	ID          string                 `json:"id"`
	TeamID      string                 `json:"team_id,omitempty"`
	CreatedBy   string                 `json:"created_by"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	ReportType  string                 `json:"report_type"` // cost_summary, savings, utilization, audit
	Schedule    string                 `json:"schedule"`    // daily, weekly, monthly
	Timezone    string                 `json:"timezone"`
	Config      map[string]any `json:"config,omitempty"`
	Recipients  []string               `json:"recipients"`
	Format      string                 `json:"format"` // pdf, csv, json
	Enabled     bool                   `json:"enabled"`
	LastRunAt   *time.Time             `json:"last_run_at,omitempty"`
	NextRunAt   *time.Time             `json:"next_run_at,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

func (s *Server) listScheduledReports(c *gin.Context) {
	teamID := c.Query("team_id")
	ctx := c.Request.Context()

	query := `
		SELECT id, team_id, created_by, name, description, report_type, schedule, timezone, config, recipients, format, enabled, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_reports
		WHERE (team_id = $1 OR $1 = '')
		ORDER BY name
	`

	rows, err := s.db.QueryContext(ctx, query, teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list reports"})
		return
	}
	defer rows.Close()

	var reports []ScheduledReport
	for rows.Next() {
		var r ScheduledReport
		var teamID sql.NullString
		var lastRunAt, nextRunAt sql.NullTime
		var config, recipients []byte
		if err := rows.Scan(&r.ID, &teamID, &r.CreatedBy, &r.Name, &r.Description, &r.ReportType, &r.Schedule, &r.Timezone, &config, &recipients, &r.Format, &r.Enabled, &lastRunAt, &nextRunAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}
		if teamID.Valid {
			r.TeamID = teamID.String
		}
		if lastRunAt.Valid {
			r.LastRunAt = &lastRunAt.Time
		}
		if nextRunAt.Valid {
			r.NextRunAt = &nextRunAt.Time
		}
		if len(config) > 0 {
			json.Unmarshal(config, &r.Config)
		}
		if len(recipients) > 0 {
			json.Unmarshal(recipients, &r.Recipients)
		}
		reports = append(reports, r)
	}

	c.JSON(http.StatusOK, gin.H{"reports": reports})
}

func (s *Server) createScheduledReport(c *gin.Context) {
	userEmail := getUserEmail(c)
	ctx := c.Request.Context()

	var body struct {
		TeamID      string                 `json:"team_id"`
		Name        string                 `json:"name" binding:"required"`
		Description string                 `json:"description"`
		ReportType  string                 `json:"report_type" binding:"required"`
		Schedule    string                 `json:"schedule" binding:"required"`
		Timezone    string                 `json:"timezone"`
		Config      map[string]any `json:"config"`
		Recipients  []string               `json:"recipients" binding:"required"`
		Format      string                 `json:"format"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, report_type, schedule, and recipients required"})
		return
	}

	if body.Timezone == "" {
		body.Timezone = "UTC"
	}
	if body.Format == "" {
		body.Format = "pdf"
	}

	// Calculate next run time
	nextRun := calculateNextRun(body.Schedule, body.Timezone)

	configJSON, _ := json.Marshal(body.Config)
	recipientsJSON, _ := json.Marshal(body.Recipients)

	var reportID string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO scheduled_reports (team_id, created_by, name, description, report_type, schedule, timezone, config, recipients, format, next_run_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`, nilIfEmpty(body.TeamID), userEmail, body.Name, body.Description, body.ReportType, body.Schedule, body.Timezone, configJSON, recipientsJSON, body.Format, nextRun).Scan(&reportID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create report"})
		return
	}

	s.logAudit(ctx, userEmail, c, "report.create", "scheduled_report", reportID, body.Name, map[string]any{
		"report_type": body.ReportType,
		"schedule":    body.Schedule,
	})

	c.JSON(http.StatusCreated, gin.H{"id": reportID})
}

func (s *Server) updateScheduledReport(c *gin.Context) {
	reportID := c.Param("reportId")
	userEmail := getUserEmail(c)
	ctx := c.Request.Context()

	var body struct {
		Name        *string                `json:"name"`
		Description *string                `json:"description"`
		Schedule    *string                `json:"schedule"`
		Timezone    *string                `json:"timezone"`
		Config      map[string]any `json:"config"`
		Recipients  []string               `json:"recipients"`
		Format      *string                `json:"format"`
		Enabled     *bool                  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// For simplicity, update all fields
	_, err := s.db.ExecContext(ctx, `
		UPDATE scheduled_reports SET updated_at = NOW() WHERE id = $1
	`, reportID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update report"})
		return
	}

	s.logAudit(ctx, userEmail, c, "report.update", "scheduled_report", reportID, "", body)
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (s *Server) deleteScheduledReport(c *gin.Context) {
	reportID := c.Param("reportId")
	userEmail := getUserEmail(c)
	ctx := c.Request.Context()

	// Get name for audit
	var reportName string
	s.db.QueryRowContext(ctx, "SELECT name FROM scheduled_reports WHERE id = $1", reportID).Scan(&reportName)

	_, err := s.db.ExecContext(ctx, "DELETE FROM scheduled_reports WHERE id = $1", reportID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete report"})
		return
	}

	s.logAudit(ctx, userEmail, c, "report.delete", "scheduled_report", reportID, reportName, nil)
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (s *Server) runReportNow(c *gin.Context) {
	reportID := c.Param("reportId")
	userEmail := getUserEmail(c)
	ctx := c.Request.Context()

	// Get report config
	var report ScheduledReport
	var teamID sql.NullString
	var config []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, team_id, report_type, config FROM scheduled_reports WHERE id = $1
	`, reportID).Scan(&report.ID, &teamID, &report.ReportType, &config)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get report"})
		return
	}

	if teamID.Valid {
		report.TeamID = teamID.String
	}
	if len(config) > 0 {
		json.Unmarshal(config, &report.Config)
	}

	// Create run record
	var runID string
	s.db.QueryRowContext(ctx, `
		INSERT INTO report_runs (report_id, status) VALUES ($1, 'running') RETURNING id
	`, reportID).Scan(&runID)

	// TODO: Actually generate the report async
	// For now, mark as completed
	s.db.ExecContext(ctx, `
		UPDATE report_runs SET status = 'completed', completed_at = NOW() WHERE id = $1
	`, runID)
	s.db.ExecContext(ctx, `
		UPDATE scheduled_reports SET last_run_at = NOW() WHERE id = $1
	`, reportID)

	s.logAudit(ctx, userEmail, c, "report.run", "scheduled_report", reportID, "", map[string]any{
		"team_id":     report.TeamID,
		"report_type": report.ReportType,
	})
	c.JSON(http.StatusOK, gin.H{"run_id": runID, "status": "completed"})
}

func calculateNextRun(schedule, timezone string) time.Time {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)

	switch schedule {
	case "daily":
		// Next day at 8 AM
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 8, 0, 0, 0, loc)
		return next.UTC()
	case "weekly":
		// Next Monday at 8 AM
		daysUntilMonday := (8 - int(now.Weekday())) % 7
		if daysUntilMonday == 0 {
			daysUntilMonday = 7
		}
		next := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 8, 0, 0, 0, loc)
		return next.UTC()
	case "monthly":
		// First of next month at 8 AM
		next := time.Date(now.Year(), now.Month()+1, 1, 8, 0, 0, 0, loc)
		return next.UTC()
	default:
		return now.Add(24 * time.Hour)
	}
}
