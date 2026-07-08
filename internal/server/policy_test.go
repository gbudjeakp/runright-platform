package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func newPolicyTestServer(t *testing.T) *Server {
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

func doJSON(t *testing.T, s *Server, method, path string, body any, out any) int {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if out != nil {
		if err := json.NewDecoder(rec.Body).Decode(out); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
		}
	}
	return rec.Code
}

func TestPolicyCRUDAndEvaluation(t *testing.T) {
	s := newPolicyTestServer(t)
	repo := "owner/repo-" + time.Now().Format("150405.000000000")
	job := "build"

	t.Cleanup(func() {
		_, _ = s.db.Exec(`DELETE FROM policy_rules WHERE repository = $1`, repo)
		_, _ = s.db.Exec(`DELETE FROM policy_rules WHERE repository = '' AND job_id = ''`)
	})

	code := doJSON(t, s, http.MethodPut, "/api/v1/policies", map[string]any{
		"repository": repo,
		"job_id": "",
		"max_cost_per_hour": 0.5,
		"enabled": true,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("repo policy upsert code = %d", code)
	}

	code = doJSON(t, s, http.MethodPut, "/api/v1/policies", map[string]any{
		"repository": repo,
		"job_id": job,
		"max_cost_per_hour": 0.25,
		"enabled": true,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("job policy upsert code = %d", code)
	}

	var policies []policyRule
	code = doJSON(t, s, http.MethodGet, "/api/v1/policies?repository="+repo, nil, &policies)
	if code != http.StatusOK {
		t.Fatalf("list policies code = %d", code)
	}
	if len(policies) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(policies))
	}

	var eval policyEvaluation
	code = doJSON(t, s, http.MethodPost, "/api/v1/policies/evaluate", map[string]any{
		"repository": repo,
		"job_id": job,
		"detected_price_per_hour": 0.30,
	}, &eval)
	if code != http.StatusOK {
		t.Fatalf("evaluate code = %d", code)
	}
	if !eval.Violated {
		t.Fatalf("expected violation, got false")
	}
	if eval.SourceScope != "job" {
		t.Fatalf("expected job scope, got %q", eval.SourceScope)
	}
	if eval.EffectiveMaxCostPerHour != 0.25 {
		t.Fatalf("expected 0.25 max, got %v", eval.EffectiveMaxCostPerHour)
	}
}

func TestPolicyPrecedenceFallsBackToRepositoryThenGlobal(t *testing.T) {
	s := newPolicyTestServer(t)
	repo := "owner/repo-" + time.Now().Add(time.Second).Format("150405.000000000")
	job := "lint"

	t.Cleanup(func() {
		_, _ = s.db.Exec(`DELETE FROM policy_rules WHERE repository = $1`, repo)
		_, _ = s.db.Exec(`DELETE FROM policy_rules WHERE repository = '' AND job_id = ''`)
	})

	_, _ = s.db.Exec(`INSERT INTO policy_rules (repository, job_id, max_cost_per_hour, enabled, updated_at) VALUES ('', '', 0.75, true, NOW())`)
	_, _ = s.db.Exec(`INSERT INTO policy_rules (repository, job_id, max_cost_per_hour, enabled, updated_at) VALUES ($1, '', 0.50, true, NOW())`, repo)

	var eval policyEvaluation
	code := doJSON(t, s, http.MethodPost, "/api/v1/policies/evaluate", map[string]any{
		"repository": repo,
		"job_id": job,
		"detected_price_per_hour": 0.60,
	}, &eval)
	if code != http.StatusOK {
		t.Fatalf("evaluate code = %d", code)
	}
	if eval.SourceScope != "repository" {
		t.Fatalf("expected repository scope, got %q", eval.SourceScope)
	}
	if eval.EffectiveMaxCostPerHour != 0.50 {
		t.Fatalf("expected repo max 0.50, got %v", eval.EffectiveMaxCostPerHour)
	}
	if !eval.Violated {
		t.Fatalf("expected violation against repo policy")
	}
}