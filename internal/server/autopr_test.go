package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/sgbudje/runright-platform/internal/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newAutoPRTestServer(t *testing.T) *Server {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://runright:runright@localhost:5435/runright?sslmode=disable"
	}
	s, err := New(Config{DSN: dsn, APIKey: ""})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return s
}

// insertTestJob inserts a completed job row directly into the DB and returns
// the generated row ID.
func insertTestJob(t *testing.T, s *Server, jobID, repo string, cpuP95, memP95, durationSec float64) string {
	t.Helper()
	detected := types.MachineType{
		ID:                   "ubuntu-latest-4-cores",
		Provider:             types.ProviderGitHub,
		VCPUs:                4,
		MemoryGiB:            16,
		OnDemandPricePerHour: 0.016,
	}
	recommended := types.MachineType{
		ID:                   "ubuntu-latest",
		Provider:             types.ProviderGitHub,
		VCPUs:                2,
		MemoryGiB:            7,
		OnDemandPricePerHour: 0.008,
	}
	currentMonthly := detected.OnDemandPricePerHour * 720
	summary := types.MetricsSummary{
		JobID:           jobID,
		CIPlatform:      "github",
		Repository:      repo,
		StartTime:       time.Now().Add(-time.Duration(durationSec) * time.Second),
		EndTime:         time.Now(),
		DurationSeconds: durationSec,
		DetectedMachine: &detected,
		CPUPercentP95:   cpuP95,
		MemUsedGiBP95:   memP95,
		MemTotalGiB:     detected.MemoryGiB,
	}
	recs := []types.Recommendation{
		{
			Machine:          recommended,
			Tier:             types.TierCheaper,
			CurrentMonthly:   currentMonthly,
			EstimatedMonthly: recommended.OnDemandPricePerHour * 720,
			CostDeltaPercent: -50,
		},
	}
	summaryJSON, _ := json.Marshal(summary)
	recsJSON, _ := json.Marshal(recs)

	var id string
	err := s.db.QueryRow(`
		INSERT INTO jobs (job_id, repository, start_time, end_time, duration_seconds, summary, recommendations, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'completed', $3) RETURNING id`,
		jobID, repo,
		summary.StartTime, summary.EndTime, durationSec,
		summaryJSON, recsJSON,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insertTestJob: %v", err)
	}
	return id
}

// ── checkAutoPRCandidate unit tests ──────────────────────────────────────────

func TestCheckAutoPRCandidate_NotEnoughRuns(t *testing.T) {
	s := newAutoPRTestServer(t)
	repo := fmt.Sprintf("test-org/repo-%d", time.Now().UnixNano())
	jobID := "build-not-enough"

	t.Cleanup(func() {
		s.db.Exec(`DELETE FROM jobs WHERE job_id = $1 AND repository = $2`, jobID, repo)
		s.db.Exec(`DELETE FROM pr_recommendations WHERE job_id = $1 AND repository = $2`, jobID, repo)
	})

	// Insert only 2 runs (default threshold is 5)
	for range 2 {
		insertTestJob(t, s, jobID, repo, 8.0, 0.9, 45)
	}

	s.checkAutoPRCandidate(jobID, repo)

	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM pr_recommendations WHERE job_id = $1 AND repository = $2`, jobID, repo).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 recommendations, got %d", count)
	}
}

func TestCheckAutoPRCandidate_HighCPUSkips(t *testing.T) {
	s := newAutoPRTestServer(t)
	repo := fmt.Sprintf("test-org/repo-%d", time.Now().UnixNano())
	jobID := "build-high-cpu"

	t.Cleanup(func() {
		s.db.Exec(`DELETE FROM jobs WHERE job_id = $1 AND repository = $2`, jobID, repo)
		s.db.Exec(`DELETE FROM pr_recommendations WHERE job_id = $1 AND repository = $2`, jobID, repo)
	})

	// 4 low-CPU runs + 1 high-CPU run — the high-CPU run breaks the streak
	for range 4 {
		insertTestJob(t, s, jobID, repo, 8.0, 0.9, 45)
	}
	insertTestJob(t, s, jobID, repo, 75.0, 3.0, 45) // high CPU

	s.checkAutoPRCandidate(jobID, repo)

	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM pr_recommendations WHERE job_id = $1 AND repository = $2`, jobID, repo).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 recommendations (high-CPU run should block), got %d", count)
	}
}

func TestCheckAutoPRCandidate_CreatesRecommendation(t *testing.T) {
	s := newAutoPRTestServer(t)
	repo := fmt.Sprintf("test-org/repo-%d", time.Now().UnixNano())
	jobID := "build-underutilised"

	t.Cleanup(func() {
		s.db.Exec(`DELETE FROM jobs WHERE job_id = $1 AND repository = $2`, jobID, repo)
		s.db.Exec(`DELETE FROM pr_recommendations WHERE job_id = $1 AND repository = $2`, jobID, repo)
	})

	// 5 consistently underutilised runs (p95 CPU ≈ 9%)
	for range 5 {
		insertTestJob(t, s, jobID, repo, 9.0, 0.9, 45)
	}

	s.checkAutoPRCandidate(jobID, repo)

	var rec PRRecommendation
	err := s.db.QueryRow(`
		SELECT id, repository, job_id, current_label, recommended_label,
		       savings_percent, run_count, status
		FROM pr_recommendations WHERE job_id = $1 AND repository = $2
	`, jobID, repo).Scan(
		&rec.ID, &rec.Repository, &rec.JobID,
		&rec.CurrentLabel, &rec.RecommendedLabel,
		&rec.SavingsPercent, &rec.RunCount, &rec.Status,
	)
	if err != nil {
		t.Fatalf("expected a recommendation row, got: %v", err)
	}
	if rec.Status != "pending" {
		t.Errorf("status = %q, want pending", rec.Status)
	}
	if rec.SavingsPercent <= 0 {
		t.Errorf("savings_percent = %.2f, want > 0", rec.SavingsPercent)
	}
	if rec.CurrentLabel == "" || rec.RecommendedLabel == "" {
		t.Errorf("labels empty: current=%q recommended=%q", rec.CurrentLabel, rec.RecommendedLabel)
	}
	if rec.RunCount < 1 {
		t.Errorf("run_count = %d, want >= 1", rec.RunCount)
	}
}

func TestCheckAutoPRCandidate_IncrementsCounters(t *testing.T) {
	s := newAutoPRTestServer(t)
	repo := fmt.Sprintf("test-org/repo-%d", time.Now().UnixNano())
	jobID := "build-increment"

	t.Cleanup(func() {
		s.db.Exec(`DELETE FROM jobs WHERE job_id = $1 AND repository = $2`, jobID, repo)
		s.db.Exec(`DELETE FROM pr_recommendations WHERE job_id = $1 AND repository = $2`, jobID, repo)
	})

	// First batch — creates the recommendation
	for range 5 {
		insertTestJob(t, s, jobID, repo, 9.0, 0.9, 45)
	}
	s.checkAutoPRCandidate(jobID, repo)

	var firstCount int
	s.db.QueryRow(`SELECT run_count FROM pr_recommendations WHERE job_id = $1 AND repository = $2 AND status = 'pending'`, jobID, repo).Scan(&firstCount)

	// Add one more run and re-analyse — worker would call this again next tick
	insertTestJob(t, s, jobID, repo, 7.0, 0.8, 45)
	s.checkAutoPRCandidate(jobID, repo)

	var secondCount int
	s.db.QueryRow(`SELECT run_count FROM pr_recommendations WHERE job_id = $1 AND repository = $2 AND status = 'pending'`, jobID, repo).Scan(&secondCount)

	if secondCount <= firstCount {
		t.Errorf("run_count did not increment: before=%d after=%d", firstCount, secondCount)
	}
}

// ── HTTP endpoint tests ───────────────────────────────────────────────────────

func TestAutoPRSettings_GetAndUpsert(t *testing.T) {
	s := newAutoPRTestServer(t)

	// Ensure default team exists (required for settings FK)
	s.db.Exec(`INSERT INTO teams (id, name, slug, created_at, updated_at) VALUES ('default', 'Default', 'default', NOW(), NOW()) ON CONFLICT DO NOTHING`)

	t.Cleanup(func() {
		s.db.Exec(`DELETE FROM auto_pr_settings WHERE team_id = 'default'`)
	})

	// GET before any settings → returns defaults
	var defaults AutoPRSettings
	code := doJSON(t, s, http.MethodGet, "/api/v1/auto-pr/settings", nil, &defaults)
	if code != http.StatusOK {
		t.Fatalf("GET settings code = %d", code)
	}
	if defaults.RequireConsecutive != 3 {
		t.Errorf("default require_consecutive_runs = %d, want 3", defaults.RequireConsecutive)
	}

	// PUT updated settings
	code = doJSON(t, s, http.MethodPut, "/api/v1/auto-pr/settings", map[string]any{
		"enabled":                true,
		"min_savings_percent":    15.0,
		"min_monthly_savings":    5.0,
		"require_consecutive_runs": 7,
		"gpu_prs_enabled":        false,
		"gpu_min_savings_percent": 10.0,
		"exclude_repositories":   []string{},
		"exclude_job_patterns":   []string{},
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("PUT settings code = %d", code)
	}

	// GET again — should reflect the new values
	var updated AutoPRSettings
	doJSON(t, s, http.MethodGet, "/api/v1/auto-pr/settings", nil, &updated)
	if !updated.Enabled {
		t.Error("expected enabled = true after PUT")
	}
	if updated.RequireConsecutive != 7 {
		t.Errorf("require_consecutive_runs = %d, want 7", updated.RequireConsecutive)
	}
}

func TestAutoPRRecommendations_CreateListDismiss(t *testing.T) {
	s := newAutoPRTestServer(t)
	repo := fmt.Sprintf("test-org/http-repo-%d", time.Now().UnixNano())
	jobID := "http-build"

	t.Cleanup(func() {
		s.db.Exec(`DELETE FROM pr_recommendations WHERE job_id = $1 AND repository = $2`, jobID, repo)
	})

	// Create via POST
	var created map[string]string
	code := doJSON(t, s, http.MethodPost, "/api/v1/auto-pr/recommendations", map[string]any{
		"repository":              repo,
		"job_id":                  jobID,
		"current_label":           "ubuntu-latest-4-cores",
		"current_vcpus":           4,
		"current_memory_gib":      16.0,
		"current_cost_per_hour":   0.016,
		"recommended_label":       "ubuntu-latest",
		"recommended_vcpus":       2,
		"recommended_memory_gib":  7.0,
		"recommended_cost_per_hour": 0.008,
		"p95_cpu_percent":         9.0,
		"p95_mem_percent":         12.0,
		"run_count":               5,
		"consecutive_underutilized": 5,
		"savings_percent":         50.0,
		"monthly_savings_usd":     2.88,
	}, &created)
	if code != http.StatusOK {
		t.Fatalf("POST recommendations code = %d", code)
	}
	recID, ok := created["id"]
	if !ok || recID == "" {
		t.Fatal("POST did not return an id")
	}

	// List — should appear with status=pending
	var list []PRRecommendation
	code = doJSON(t, s, http.MethodGet, "/api/v1/auto-pr/recommendations?status=pending", nil, &list)
	if code != http.StatusOK {
		t.Fatalf("GET recommendations code = %d", code)
	}
	found := false
	for _, r := range list {
		if r.ID == recID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created recommendation %s not found in list", recID)
	}

	// Dismiss
	code = doJSON(t, s, http.MethodPost,
		"/api/v1/auto-pr/recommendations/"+recID+"/dismiss",
		map[string]any{"reason": "already handled"},
		nil,
	)
	if code != http.StatusOK {
		t.Fatalf("POST dismiss code = %d", code)
	}

	// Verify dismissed status
	var dismissed PRRecommendation
	s.db.QueryRow(`SELECT status FROM pr_recommendations WHERE id = $1`, recID).Scan(&dismissed.Status)
	if dismissed.Status != "dismissed" {
		t.Errorf("status after dismiss = %q, want dismissed", dismissed.Status)
	}
}

func TestAutoPRRecommendations_Approve(t *testing.T) {
	s := newAutoPRTestServer(t)
	repo := fmt.Sprintf("test-org/approve-repo-%d", time.Now().UnixNano())
	jobID := "approve-build"

	t.Cleanup(func() {
		s.db.Exec(`DELETE FROM pr_recommendations WHERE job_id = $1 AND repository = $2`, jobID, repo)
	})

	var created map[string]string
	doJSON(t, s, http.MethodPost, "/api/v1/auto-pr/recommendations", map[string]any{
		"repository":               repo,
		"job_id":                   jobID,
		"current_label":            "ubuntu-latest-4-cores",
		"current_vcpus":            4,
		"current_memory_gib":       16.0,
		"current_cost_per_hour":    0.016,
		"recommended_label":        "ubuntu-latest",
		"recommended_vcpus":        2,
		"recommended_memory_gib":   7.0,
		"recommended_cost_per_hour": 0.008,
		"savings_percent":          50.0,
		"monthly_savings_usd":      2.88,
	}, &created)
	recID := created["id"]

	// Approve without a GitHub token → now returns 422 with a clear message
	// rather than silently marking approved.
	var errResp map[string]any
	code := doJSON(t, s, http.MethodPost,
		"/api/v1/auto-pr/recommendations/"+recID+"/approve",
		nil, &errResp,
	)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("POST approve without token: want 422, got %d", code)
	}
	if errMsg, _ := errResp["error"].(string); errMsg == "" {
		t.Error("expected non-empty error message in response body")
	}

	// DB status must still be pending (recommendation was NOT auto-approved)
	var dbStatus string
	s.db.QueryRow(`SELECT status FROM pr_recommendations WHERE id = $1`, recID).Scan(&dbStatus)
	if dbStatus != "pending" {
		t.Errorf("DB status without token = %q, want pending", dbStatus)
	}
}

func TestAutoPRLabelMappings_CRUD(t *testing.T) {
	s := newAutoPRTestServer(t)

	t.Cleanup(func() {
		s.db.Exec(`DELETE FROM label_mappings WHERE label LIKE 'test-label-%'`)
	})

	label := fmt.Sprintf("test-label-%d", time.Now().UnixNano())

	// Create (endpoint is PUT /api/v1/labels — upsert)
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	code := doJSON(t, s, http.MethodPut, "/api/v1/labels", map[string]any{
		"repository":    "*",
		"label":         label,
		"provider":      "github",
		"instance_type": "ubuntu-latest-4-cores",
		"vcpus":         4,
		"memory_gib":    16.0,
		"cost_per_hour": 0.016,
		"is_gpu":        false,
	}, &created)
	if code != http.StatusOK {
		t.Fatalf("PUT labels code = %d", code)
	}
	if created.ID == "" {
		t.Fatal("PUT did not return id")
	}

	// List — should appear
	var list []LabelMapping
	code = doJSON(t, s, http.MethodGet, "/api/v1/labels", nil, &list)
	if code != http.StatusOK {
		t.Fatalf("GET labels code = %d", code)
	}
	found := false
	for _, m := range list {
		if m.Label == label {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created label mapping %q not found in list", label)
	}

	// Delete
	code = doJSON(t, s, http.MethodDelete, "/api/v1/labels/"+created.ID, nil, nil)
	if code != http.StatusOK && code != http.StatusNoContent {
		t.Fatalf("DELETE label code = %d", code)
	}
}
