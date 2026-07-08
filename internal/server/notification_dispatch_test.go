package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sgbudje/runright-platform/internal/types"
)

func TestDispatchNotificationRules_SendsThresholdAlertToConfiguredDestination(t *testing.T) {
	s := newPolicyTestServer(t)
	resetNotificationState(t, s)

	var (
		mu      sync.Mutex
		count   int
		payload string
	)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		mu.Lock()
		count++
		payload = string(b)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	p := validNotificationPayload()
	slack := p["slack"].(map[string]any)
	dests := slack["destinations"].([]map[string]any)
	dests[0]["webhook_url"] = hook.URL
	rules := p["rules"].([]map[string]any)
	rules[0]["metric"] = "max_cost_per_hour"
	rules[0]["threshold"] = 0.5
	rules[0]["scope"] = "global"

	if code := doJSON(t, s, http.MethodPut, "/api/v1/notifications/settings", p, nil); code != http.StatusOK {
		t.Fatalf("upsert notification settings code = %d", code)
	}

	summary := types.MetricsSummary{
		JobID:      "build",
		Repository: "owner/repo-a",
		Status:     "completed",
		StartTime:  time.Now().Add(-2 * time.Minute),
		EndTime:    time.Now(),
		DetectedMachine: &types.MachineType{
			ID:                   "c5.xlarge",
			OnDemandPricePerHour: 0.8,
		},
		CPUPercentP95: 15,
		MemUsedGiBP95: 1.2,
		MemTotalGiB:   8,
	}
	recs := []types.Recommendation{{
		CostDeltaPercent: -40,
		CurrentMonthly:   120,
		EstimatedMonthly: 72,
	}}

	s.dispatchNotificationRules(summary, recs)

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 webhook call, got %d", count)
	}
	if payload == "" {
		t.Fatalf("expected webhook payload body to be non-empty")
	}
}

func TestDispatchNotificationRules_SendsPolicyViolationEventAlert(t *testing.T) {
	s := newPolicyTestServer(t)
	resetNotificationState(t, s)

	var (
		mu    sync.Mutex
		count int
	)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	repo := "owner/repo-policy"
	jobID := "build"
	if _, err := s.db.Exec(`
		INSERT INTO policy_rules (repository, job_id, max_cost_per_hour, enabled, updated_at)
		VALUES ($1, $2, 0.25, true, NOW())`, repo, jobID); err != nil {
		t.Fatalf("insert policy rule: %v", err)
	}
	defer func() {
		_, _ = s.db.Exec(`DELETE FROM policy_rules WHERE repository = $1 AND job_id = $2`, repo, jobID)
	}()

	p := validNotificationPayload()
	slack := p["slack"].(map[string]any)
	dests := slack["destinations"].([]map[string]any)
	dests[0]["webhook_url"] = hook.URL

	events := p["events"].(map[string]any)
	events["policy_violation"] = true
	rules := []map[string]any{
		{
			"id":             "rule-policy-event",
			"name":           "Policy violation event",
			"type":           "event",
			"event":          "policy_violation",
			"scope":          "job",
			"repository":     repo,
			"jobId":          jobID,
			"metric":         "max_cost_per_hour",
			"threshold":      0,
			"destinationIds": []string{"dest-primary"},
			"enabled":        true,
		},
	}
	p["rules"] = rules

	if code := doJSON(t, s, http.MethodPut, "/api/v1/notifications/settings", p, nil); code != http.StatusOK {
		t.Fatalf("upsert notification settings code = %d", code)
	}

	summary := types.MetricsSummary{
		JobID:      jobID,
		Repository: repo,
		Status:     "completed",
		StartTime:  time.Now().Add(-3 * time.Minute),
		EndTime:    time.Now(),
		DetectedMachine: &types.MachineType{
			ID:                   "c5.xlarge",
			OnDemandPricePerHour: 0.40,
		},
		CPUPercentP95: 30,
		MemUsedGiBP95: 2,
		MemTotalGiB:   8,
	}

	s.dispatchNotificationRules(summary, nil)

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 webhook call, got %d", count)
	}
}
