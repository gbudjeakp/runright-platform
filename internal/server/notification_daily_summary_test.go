package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sgbudje/runright-platform/internal/types"
)

func TestDispatchDailySummaries_SendsOncePerDayPerRuleScope(t *testing.T) {
	s := newPolicyTestServer(t)
	resetNotificationState(t, s)

	var (
		mu    sync.Mutex
		count int
	)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_ = r.Body.Close()
		mu.Lock()
		count++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	payload := validNotificationPayload()
	events := payload["events"].(map[string]any)
	events["daily_summary"] = true

	slack := payload["slack"].(map[string]any)
	dests := slack["destinations"].([]map[string]any)
	dests[0]["webhook_url"] = hook.URL

	payload["rules"] = []map[string]any{
		{
			"id":             "rule-daily-global",
			"name":           "Daily global summary",
			"type":           "event",
			"event":          "daily_summary",
			"scope":          "global",
			"repository":     "",
			"jobId":          "",
			"metric":         "max_cost_per_hour",
			"threshold":      0,
			"destinationIds": []string{"dest-primary"},
			"enabled":        true,
		},
	}

	if code := doJSON(t, s, http.MethodPut, "/api/v1/notifications/settings", payload, nil); code != http.StatusOK {
		t.Fatalf("upsert notification settings code = %d", code)
	}

	now := time.Now().UTC()
	summary := types.MetricsSummary{
		JobID:         "build-daily",
		Repository:    "owner/repo-daily",
		Status:        "completed",
		StartTime:     now.Add(-2 * time.Minute),
		EndTime:       now,
		CPUPercentP95: 34.5,
		MemUsedGiBP95: 1.8,
		MemTotalGiB:   8,
	}
	recs := []types.Recommendation{{
		CostDeltaPercent: -15,
		CurrentMonthly:   90,
		EstimatedMonthly: 76,
	}}

	// Use tomorrow's 09:00 as dispatch time so the 24h window [today 09:00, tomorrow 09:00]
	// is fully determined. Insert the job at dispatchAt-1h to guarantee it falls in window.
	dispatchAt := time.Date(now.Year(), now.Month(), now.Day()+1, 9, 0, 0, 0, time.UTC)
	jobTime := dispatchAt.Add(-1 * time.Hour)

	summaryJSON, _ := json.Marshal(summary)
	recsJSON, _ := json.Marshal(recs)
	if _, err := s.db.Exec(`
		INSERT INTO jobs (job_id, repository, start_time, end_time, duration_seconds, summary, recommendations, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'completed', $8)`,
		summary.JobID, summary.Repository, summary.StartTime, summary.EndTime, 120.0, summaryJSON, recsJSON, jobTime); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	s.dispatchDailySummaries(dispatchAt)
	// Second call same day should dedupe.
	s.dispatchDailySummaries(dispatchAt.Add(20 * time.Minute))

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("expected exactly 1 daily summary webhook for same day, got %d", count)
	}
}
