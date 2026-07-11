// cmd/seed/main.go — posts realistic demo jobs to the local backend.
//
// Usage:
//
//	go run ./cmd/seed [--url http://localhost:8080] [--api-key YOUR_KEY]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/sgbudje/runright-platform/internal/types"
)

func main() {
	url := flag.String("url", "http://localhost:8080", "Backend base URL")
	key := flag.String("api-key", "", "RUNRIGHT_API_KEY (leave blank if auth is disabled)")
	flag.Parse()

	jobs := buildJobs()
	ok, fail := 0, 0
	for _, j := range jobs {
		if err := post(*url, *key, j); err != nil {
			log.Printf("FAIL %s: %v", j.Summary.JobID, err)
			fail++
		} else {
			fmt.Printf("  OK  %s  (%.0fs, cpu p95 %.1f%%, mem p95 %.2f GiB)\n",
				j.Summary.JobID,
				j.Summary.DurationSeconds,
				j.Summary.CPUPercentP95,
				j.Summary.MemUsedGiBP95,
			)
			ok++
		}
	}
	fmt.Printf("\nSeeded %d jobs (%d failed)\n", ok, fail)

	// Seed a pending Auto-PR recommendation for gbudjeakp/DevOps-learn so the
	// Approve & Create PR flow can be tested immediately after seeding.
	if err := postRecommendation(*url, *key); err != nil {
		log.Printf("WARN recommendation seed: %v", err)
	} else {
		fmt.Println("  OK  devops-learn recommendation (pending, ubuntu-22.04 → ubuntu-latest)")
	}
}

// --------------------------------------------------------------------------
// HTTP helpers
// --------------------------------------------------------------------------

type payload struct {
	Summary         types.MetricsSummary   `json:"summary"`
	Recommendations []types.Recommendation `json:"recommendations"`
}

func post(base, key string, p payload) error {
	b, _ := json.Marshal(p)
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/jobs", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// postRecommendation seeds a pending Auto-PR recommendation for
// gbudjeakp/DevOps-learn so the Approve & Create PR flow can be demoed
// immediately without waiting for the background worker.
func postRecommendation(base, key string) error {
	body := map[string]any{
		"repository":                "gbudjeakp/DevOps-learn",
		"job_id":                    "devops-learn-github-hosted-build",
		"workflow_file":             ".github/workflows/ci-hosted.yml",
		"current_label":             "ubuntu-22.04",
		"current_vcpus":             2,
		"current_memory_gib":        7.0,
		"current_cost_per_hour":     0.008,
		"recommended_label":         "ubuntu-latest",
		"recommended_vcpus":         2,
		"recommended_memory_gib":    7.0,
		"recommended_cost_per_hour": 0.006,
		"p95_cpu_percent":           8.5,
		"p95_mem_percent":           1.2,
		"savings_percent":           25.0,
		"monthly_savings_usd":       4.32,
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/auto-pr/recommendations", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 409 means the recommendation already exists — that is fine.
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// --------------------------------------------------------------------------
// Seed data
// --------------------------------------------------------------------------

// awsT3Medium is the "detected" machine for most runs — a common GitHub-hosted runner size.
var awsT3Medium = types.MachineType{
	ID: "t3.medium", Provider: types.ProviderAWS,
	Family: "general-purpose", Series: "t3",
	VCPUs: 2, MemoryGiB: 4, NetworkGbps: 5, StorageType: "ebs",
	Architecture: "x86_64", OnDemandPricePerHour: 0.0416,
	Tags: []string{"burstable"},
}

var awsT3Large = types.MachineType{
	ID: "t3.large", Provider: types.ProviderAWS,
	Family: "general-purpose", Series: "t3",
	VCPUs: 2, MemoryGiB: 8, NetworkGbps: 5, StorageType: "ebs",
	Architecture: "x86_64", OnDemandPricePerHour: 0.0832,
	Tags: []string{"burstable"},
}

var awsC7gLarge = types.MachineType{
	ID: "c7g.large", Provider: types.ProviderAWS,
	Family: "compute-optimized", Series: "c7g",
	VCPUs: 2, MemoryGiB: 4, NetworkGbps: 12.5, StorageType: "ebs",
	Architecture: "arm64", OnDemandPricePerHour: 0.0725,
	Tags: []string{"graviton", "arm64"},
}

var awsM6iLarge = types.MachineType{
	ID: "m6i.large", Provider: types.ProviderAWS,
	Family: "general-purpose", Series: "m6i",
	VCPUs: 2, MemoryGiB: 8, NetworkGbps: 12.5, StorageType: "ebs",
	Architecture: "x86_64", OnDemandPricePerHour: 0.096,
	Tags: []string{"latest-gen"},
}

var gcpE2Medium = types.MachineType{
	ID: "e2-medium", Provider: types.ProviderGCP,
	Family: "general-purpose", Series: "e2",
	VCPUs: 2, MemoryGiB: 4, NetworkGbps: 4, StorageType: "pd-balanced",
	Architecture: "x86_64", OnDemandPricePerHour: 0.0335,
	Tags: []string{"shared-core"},
}

var gcpN2Standard2 = types.MachineType{
	ID: "n2-standard-2", Provider: types.ProviderGCP,
	Family: "general-purpose", Series: "n2",
	VCPUs: 2, MemoryGiB: 8, NetworkGbps: 10, StorageType: "pd-balanced",
	Architecture: "x86_64", OnDemandPricePerHour: 0.0971,
	Tags: []string{"latest-gen"},
}

var awsC7g2XLarge = types.MachineType{
	ID: "c7g.2xlarge", Provider: types.ProviderAWS,
	Family: "compute-optimized", Series: "c7g",
	VCPUs: 8, MemoryGiB: 16, NetworkGbps: 12.5, StorageType: "ebs",
	Architecture: "arm64", OnDemandPricePerHour: 0.58,
	Tags: []string{"graviton", "arm64", "compute-optimized"},
}

var awsP32XLarge = types.MachineType{
	ID: "p3.2xlarge", Provider: types.ProviderAWS,
	Family: "gpu-accelerated", Series: "p3",
	VCPUs: 8, MemoryGiB: 61, NetworkGbps: 10, StorageType: "ebs",
	Architecture: "x86_64", OnDemandPricePerHour: 3.06,
	Tags: []string{"gpu", "ml-training", "expensive"},
}

var awsG4dn12XLarge = types.MachineType{
	ID: "g4dn.12xlarge", Provider: types.ProviderAWS,
	Family: "gpu-accelerated", Series: "g4dn",
	VCPUs: 48, MemoryGiB: 192, NetworkGbps: 100, StorageType: "ebs",
	Architecture: "x86_64", OnDemandPricePerHour: 7.48,
	Tags: []string{"gpu", "inference", "very-expensive"},
}

var awsP4d24XLarge = types.MachineType{
	ID: "p4d.24xlarge", Provider: types.ProviderAWS,
	Family: "gpu-accelerated", Series: "p4d",
	VCPUs: 96, MemoryGiB: 1152, NetworkGbps: 400, StorageType: "ebs",
	Architecture: "x86_64", OnDemandPricePerHour: 32.77,
	Tags: []string{"gpu", "ml-training", "most-expensive"},
}

// githubUbuntu4Cores represents the GitHub-hosted ubuntu-latest-4-cores runner
// (4 vCPU, 16 GiB).  DevOps-learn currently uses this for its build step.
var githubUbuntu4Cores = types.MachineType{
	ID: "ubuntu-latest-4-cores", Provider: types.ProviderGitHub,
	Family: "general-purpose", Series: "github-hosted",
	VCPUs: 4, MemoryGiB: 16, NetworkGbps: 1, StorageType: "ssd",
	Architecture: "x86_64", OnDemandPricePerHour: 0.016, // $0.016/min
	Tags: []string{"github-hosted"},
}

// githubUbuntuLatest represents the standard 2-core GitHub-hosted runner
// — the right-sized target for the DevOps-learn build jobs.
var githubUbuntuLatest = types.MachineType{
	ID: "ubuntu-latest", Provider: types.ProviderGitHub,
	Family: "general-purpose", Series: "github-hosted",
	VCPUs: 2, MemoryGiB: 7, NetworkGbps: 1, StorageType: "ssd",
	Architecture: "x86_64", OnDemandPricePerHour: 0.008, // $0.008/min
	Tags: []string{"github-hosted"},
}

// rng is a seeded random source for reproducible jitter.
var rng = rand.New(rand.NewSource(42))

func jitter(base, spread float64) float64 {
	return base + (rng.Float64()*2-1)*spread
}

// buildJobs returns a realistic set of job runs spanning the past 30 days.
// Jobs are ordered oldest-first so the dashboard history looks natural.
func buildJobs() []payload {
	now := time.Now()
	var jobs []payload

	// ── "build" job: runs daily, gradually gets heavier over the month ──────
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i)
		growth := float64(30-i) / 30.0 // 0 → 1 over the month

		cpu := jitter(28+growth*22, 4)     // 28% → 50% over the month
		mem := jitter(1.1+growth*0.9, 0.1) // 1.1 → 2.0 GiB

		jobs = append(jobs, makeJob("build", "github", t, awsT3Medium, cpu, mem, 185+growth*75))
	}

	// ── "build-release" job: daily release builds, compute-intensive ────────
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(30 * time.Minute)
		jobs = append(jobs, makeJob("build-release", "github", t, awsC7g2XLarge,
			jitter(45, 5), jitter(6, 0.5), jitter(1200, 150))) // 20 min
	}

	// ── "unit-tests" job: fast, low-resource, stable ────────────────────────
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(5 * time.Minute)
		jobs = append(jobs, makeJob("unit-tests", "github", t, awsT3Medium,
			jitter(12, 3), jitter(0.6, 0.08), jitter(42, 8)))
	}

	// ── "integration-tests" job: medium load, runs every other day ──────────
	for i := 28; i >= 0; i -= 2 {
		t := now.AddDate(0, 0, -i).Add(12 * time.Minute)
		jobs = append(jobs, makeJob("integration-tests", "jenkins", t, awsC7gLarge,
			jitter(55, 8), jitter(2.1, 0.2), jitter(600, 60)))
	}

	// ── "e2e-tests" job: CPU-hungry, runs twice a week ──────────────────────
	for i := 28; i >= 0; i -= 4 {
		t := now.AddDate(0, 0, -i).Add(25 * time.Minute)
		jobs = append(jobs, makeJob("e2e-tests", "jenkins", t, awsT3Medium,
			jitter(78, 6), jitter(2.8, 0.25), jitter(620, 60)))
	}

	// ── "docker-build" job: I/O heavy, runs weekly ──────────────────────────
	for i := 28; i >= 0; i -= 7 {
		t := now.AddDate(0, 0, -i).Add(2 * time.Minute)
		jobs = append(jobs, makeJob("docker-build", "github", t, awsT3Medium,
			jitter(35, 5), jitter(1.4, 0.15), jitter(240, 30)))
	}

	// ── "lint" job: trivially light, every day ──────────────────────────────
	for i := 14; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(1 * time.Minute)
		jobs = append(jobs, makeJob("lint", "github", t, awsT3Medium,
			jitter(8, 2), jitter(0.35, 0.04), jitter(22, 4)))
	}

	// ── "security-scan" job: light load, runs twice a week ──────────────────
	for i := 28; i >= 0; i -= 3 {
		t := now.AddDate(0, 0, -i).Add(8 * time.Minute)
		jobs = append(jobs, makeJob("security-scan", "jenkins", t, awsT3Medium,
			jitter(14, 3), jitter(0.5, 0.06), jitter(55, 10)))
	}

	// ── "deploy-staging" job: moderate, once a day ──────────────────────────
	for i := 14; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(18 * time.Minute)
		jobs = append(jobs, makeJob("deploy-staging", "jenkins", t, awsT3Medium,
			jitter(30, 5), jitter(0.9, 0.1), jitter(95, 15)))
	}

	// ═══════════════════════════════════════════════════════════════════════
	// NEW REPOS & JOBS
	// ═══════════════════════════════════════════════════════════════════════

	// ── Infrastructure team: terraform-plan ─────────────────────────────────
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(6 * time.Minute)
		jobs = append(jobs, makeJob("terraform-plan", "github", t, awsT3Medium,
			jitter(15, 3), jitter(0.4, 0.05), jitter(45, 10)))
	}

	// ── Infrastructure team: k8s-deploy (every 2 days) ──────────────────────
	for i := 28; i >= 0; i -= 2 {
		t := now.AddDate(0, 0, -i).Add(20 * time.Minute)
		jobs = append(jobs, makeJob("k8s-deploy", "github", t, awsM6iLarge,
			jitter(35, 4), jitter(1.2, 0.1), jitter(1800, 200))) // 30 min
	}

	// ── Infrastructure team: disaster-recovery-drill (weekly, expensive) ─────
	for i := 28; i >= 0; i -= 7 {
		t := now.AddDate(0, 0, -i).Add(22 * time.Minute)
		jobs = append(jobs, makeJob("disaster-recovery-drill", "jenkins", t, awsC7g2XLarge,
			jitter(40, 5), jitter(6, 0.5), jitter(7200, 600))) // 2 hours
	}

	// ── Infrastructure team: infra-tests ────────────────────────────────────
	for i := 28; i >= 0; i -= 3 {
		t := now.AddDate(0, 0, -i).Add(30 * time.Minute)
		jobs = append(jobs, makeJob("infra-tests", "jenkins", t, gcpE2Medium,
			jitter(35, 5), jitter(1.2, 0.1), jitter(180, 25)))
	}

	// ── Security team: sast-scan (every day) ────────────────────────────────
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(4 * time.Minute)
		jobs = append(jobs, makeJob("sast-scan", "github", t, awsT3Medium,
			jitter(20, 4), jitter(0.5, 0.05), jitter(90, 15)))
	}

	// ── Security team: dast-scan (weekly) ───────────────────────────────────
	for i := 28; i >= 0; i -= 7 {
		t := now.AddDate(0, 0, -i).Add(2 * time.Hour)
		jobs = append(jobs, makeJob("dast-scan", "jenkins", t, awsT3Large,
			jitter(45, 6), jitter(2.5, 0.2), jitter(1800, 200)))
	}

	// ── Security team: dependency-audit (every 3 days) ──────────────────────
	for i := 27; i >= 0; i -= 3 {
		t := now.AddDate(0, 0, -i).Add(15 * time.Minute)
		jobs = append(jobs, makeJob("dependency-audit", "github", t, awsT3Medium,
			jitter(12, 2), jitter(0.4, 0.04), jitter(35, 8)))
	}

	// ── QA team: load-tests (weekly) ────────────────────────────────────────
	for i := 28; i >= 0; i -= 7 {
		t := now.AddDate(0, 0, -i).Add(3 * time.Hour)
		jobs = append(jobs, makeJob("load-tests", "jenkins", t, awsC7g2XLarge,
			jitter(65, 8), jitter(4.5, 0.4), jitter(7200, 600))) // 2 hours
	}

	// ── QA team: perf-regression (every 2 days, compute-intensive) ─────────────
	for i := 28; i >= 0; i -= 2 {
		t := now.AddDate(0, 0, -i).Add(4 * time.Hour)
		jobs = append(jobs, makeJob("perf-regression", "jenkins", t, awsC7g2XLarge,
			jitter(55, 6), jitter(5, 0.5), jitter(5400, 600))) // 1.5 hours
	}

	// ── QA team: smoke-tests (every day) ────────────────────────────────────
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(22 * time.Minute)
		jobs = append(jobs, makeJob("smoke-tests", "github", t, awsT3Medium,
			jitter(18, 3), jitter(0.6, 0.06), jitter(55, 10)))
	}

	// ── Mobile team: ios-build (every 2 days) ───────────────────────────────
	for i := 28; i >= 0; i -= 2 {
		t := now.AddDate(0, 0, -i).Add(40 * time.Minute)
		jobs = append(jobs, makeJob("ios-build", "github", t, awsM6iLarge,
			jitter(72, 6), jitter(5.5, 0.4), jitter(1800, 200))) // 30 min
	}

	// ── Mobile team: ios-release (weekly, compute-intensive) ─────────────────
	for i := 28; i >= 0; i -= 7 {
		t := now.AddDate(0, 0, -i).Add(50 * time.Minute)
		jobs = append(jobs, makeJob("ios-release", "github", t, awsC7g2XLarge,
			jitter(75, 5), jitter(8, 0.6), jitter(5400, 400))) // 1.5 hours
	}

	// ── Mobile team: android-build (every 2 days) ───────────────────────────
	for i := 28; i >= 0; i -= 2 {
		t := now.AddDate(0, 0, -i).Add(45 * time.Minute)
		jobs = append(jobs, makeJob("android-build", "github", t, awsM6iLarge,
			jitter(68, 5), jitter(6.2, 0.5), jitter(1200, 150)))
	}

	// ── Mobile team: mobile-tests (every 3 days) ────────────────────────────
	for i := 27; i >= 0; i -= 3 {
		t := now.AddDate(0, 0, -i).Add(1 * time.Hour)
		jobs = append(jobs, makeJob("mobile-tests", "jenkins", t, awsT3Large,
			jitter(42, 5), jitter(3.2, 0.3), jitter(480, 50)))
	}

	// ── API Gateway: api-tests (every day) ──────────────────────────────────
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(10 * time.Minute)
		jobs = append(jobs, makeJob("api-tests", "github", t, awsT3Medium,
			jitter(25, 4), jitter(0.8, 0.08), jitter(95, 15)))
	}

	// ── API Gateway: contract-tests (every 2 days) ──────────────────────────
	for i := 28; i >= 0; i -= 2 {
		t := now.AddDate(0, 0, -i).Add(14 * time.Minute)
		jobs = append(jobs, makeJob("contract-tests", "github", t, awsT3Medium,
			jitter(20, 3), jitter(0.5, 0.05), jitter(65, 10)))
	}

	// ── Frontend: storybook (every day) ─────────────────────────────────────
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(8 * time.Minute)
		jobs = append(jobs, makeJob("storybook", "github", t, awsT3Medium,
			jitter(32, 5), jitter(1.1, 0.1), jitter(180, 25)))
	}

	// ── Frontend: visual-tests (every 3 days) ───────────────────────────────
	for i := 27; i >= 0; i -= 3 {
		t := now.AddDate(0, 0, -i).Add(28 * time.Minute)
		jobs = append(jobs, makeJob("visual-tests", "jenkins", t, awsT3Large,
			jitter(38, 5), jitter(2.4, 0.2), jitter(320, 40)))
	}

	// ── ML Platform: train-model (every 3 days, very expensive) ─────────────
	for i := 27; i >= 0; i -= 3 {
		t := now.AddDate(0, 0, -i).Add(5 * time.Hour)
		jobs = append(jobs, makeJob("train-model", "jenkins", t, awsP32XLarge,
			jitter(55, 8), jitter(28, 4), jitter(18000, 2000)))
	}

	// ── ML Platform: model-eval (every 2 days) ──────────────────────────────
	for i := 28; i >= 0; i -= 2 {
		t := now.AddDate(0, 0, -i).Add(90 * time.Minute)
		jobs = append(jobs, makeJob("model-eval", "jenkins", t, awsC7g2XLarge,
			jitter(45, 6), jitter(8, 1), jitter(5400, 600)))
	}

	// ═══════════════════════════════════════════════════════════════════════

	// ── "benchmark" job: CPU-heavy, weekly on GCP ───────────────────────────
	for i := 28; i >= 0; i -= 7 {
		t := now.AddDate(0, 0, -i).Add(35 * time.Minute)
		jobs = append(jobs, makeJob("benchmark", "jenkins", t, gcpN2Standard2,
			jitter(88, 5), jitter(3.2, 0.2), jitter(750, 60)))
	}

	// ── "gcp-build" job: build running on GCP, every 3 days ─────────────────
	for i := 27; i >= 0; i -= 3 {
		t := now.AddDate(0, 0, -i).Add(3 * time.Minute)
		growth := float64(27-i) / 27.0
		jobs = append(jobs, makeJob("gcp-build", "github", t, gcpN2Standard2,
			jitter(32+growth*20, 4), jitter(1.2+growth*0.7, 0.1), jitter(200+growth*80, 30)))
	}

	// ── "python-tests" job: GCP, every other day ────────────────────────────
	for i := 28; i >= 0; i -= 2 {
		t := now.AddDate(0, 0, -i).Add(7 * time.Minute)
		jobs = append(jobs, makeJob("python-tests", "jenkins", t, gcpE2Medium,
			jitter(22, 4), jitter(0.8, 0.08), jitter(68, 12)))
	}

	// ── "ml-training" job: Heavy compute-optimized, runs weekly ──────────────
	// Uses c7g.2xlarge but only at 25% CPU most of the time.
	// Huge opportunity to downsize to m6i.xlarge or c7g.xlarge.
	for i := 27; i >= 0; i -= 7 {
		t := now.AddDate(0, 0, -i).Add(4 * time.Hour)
		jobs = append(jobs, makeJob("ml-training", "jenkins", t, awsC7g2XLarge,
			jitter(23, 5), jitter(2.5, 0.3), jitter(14400, 1200))) // 4 hours = 14400 seconds
	}

	// ── "gpu-inference" job: p4d.24xlarge (most expensive) but only 15% util ──
	// Runs twice a week, 4 hours each. Massive downsize opportunity.
	for i := 28; i >= 0; i -= 3 {
		t := now.AddDate(0, 0, -i).Add(6 * time.Hour)
		jobs = append(jobs, makeJob("gpu-inference", "jenkins", t, awsP4d24XLarge,
			jitter(14, 3), jitter(120, 15), jitter(14400, 1500))) // 4 hours, low GPU util
	}

	// ── "ml-batch" job: p3.2xlarge but only 25% GPU util, weekly ──────────────
	// 3-hour training runs with plenty of headroom.
	for i := 28; i >= 0; i -= 5 {
		t := now.AddDate(0, 0, -i).Add(2 * time.Hour)
		jobs = append(jobs, makeJob("ml-batch", "jenkins", t, awsP32XLarge,
			jitter(24, 4), jitter(15, 2), jitter(10800, 1200))) // 3 hours
	}

	// ── "data-transform" job: g4dn.12xlarge but only 20% GPU util, weekly ────
	// Long-running data pipeline with spare capacity.
	for i := 28; i >= 0; i -= 7 {
		t := now.AddDate(0, 0, -i).Add(14 * time.Hour)
		jobs = append(jobs, makeJob("data-transform", "jenkins", t, awsG4dn12XLarge,
			jitter(19, 4), jitter(40, 8), jitter(14400, 1200))) // 4 hours
	}

	// ── Interrupted runs ─────────────────────────────────────────────────────
	// Status="heartbeat" simulates jobs where the agent never sent a final
	// "completed" flush — either OOM-killed (SIGKILL) or runner disconnected.

	// OOM kill: build job 3 days ago — mem was near the 4 GiB ceiling
	jobs = append(jobs, makeInterruptedJob("build", "github",
		now.AddDate(0, 0, -3).Add(10*time.Second),
		awsT3Medium, jitter(44, 3), jitter(3.88, 0.05), 47))

	// Runner disconnect: e2e-tests 9 days ago — cut off 2 min into a 10+ min run
	jobs = append(jobs, makeInterruptedJob("e2e-tests", "jenkins",
		now.AddDate(0, 0, -9).Add(25*time.Minute),
		awsT3Medium, jitter(76, 4), jitter(2.6, 0.15), 118))

	// OOM kill: benchmark (GCP) 5 days ago — memory blew past instance ceiling
	jobs = append(jobs, makeInterruptedJob("benchmark", "jenkins",
		now.AddDate(0, 0, -5).Add(35*time.Minute),
		gcpN2Standard2, jitter(93, 2), jitter(6.9, 0.2), 203))

	// Runner disconnect: python-tests 2 days ago — runner went offline mid-suite
	jobs = append(jobs, makeInterruptedJob("python-tests", "jenkins",
		now.AddDate(0, 0, -2).Add(7*time.Minute),
		gcpE2Medium, jitter(24, 3), jitter(0.78, 0.06), 31))

	// ═════════════════════════════════════════════════════════════════════
	// DevOps-learn — GitHub-hosted runner history (gbudjeakp/DevOps-learn)
	//
	// All four stable job IDs from ci-hosted.yml are seeded.  The build job
	// is on ubuntu-latest-4-cores but uses ≈9 % CPU — a clear downsize target.
	// ═════════════════════════════════════════════════════════════════════

	// build: 4-core runner, Vite build, ≈45 s, runs every push (daily)
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(3 * time.Minute)
		jobs = append(jobs, makeDevOpsLearnJob("devops-learn-github-hosted-build", t,
			githubUbuntu4Cores, jitter(9, 2), jitter(0.9, 0.1), jitter(45, 8)))
	}

	// test: 2-core runner, Vitest, ≈30 s
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(2 * time.Minute)
		jobs = append(jobs, makeDevOpsLearnJob("devops-learn-github-hosted-test", t,
			githubUbuntuLatest, jitter(14, 3), jitter(0.6, 0.08), jitter(30, 5)))
	}

	// lint: 2-core runner, ESLint, ≈20 s
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(1 * time.Minute)
		jobs = append(jobs, makeDevOpsLearnJob("devops-learn-github-hosted-lint", t,
			githubUbuntuLatest, jitter(7, 2), jitter(0.4, 0.05), jitter(20, 4)))
	}

	// install: 2-core runner, npm ci, ≈25 s
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i)
		jobs = append(jobs, makeDevOpsLearnJob("devops-learn-github-hosted-install", t,
			githubUbuntuLatest, jitter(18, 3), jitter(0.5, 0.06), jitter(25, 5)))
	}

	return jobs
}

// makeInterruptedJob creates a job payload with Status="heartbeat", simulating
// a run where the agent was killed (OOM, SIGKILL) or the runner disconnected
// before it could send the final "completed" flush.
func makeInterruptedJob(
	jobID, ciPlatform string, at time.Time,
	detected types.MachineType,
	cpuP95, memP95GiB, durationSec float64,
) payload {
	p := makeJob(jobID, ciPlatform, at, detected, cpuP95, memP95GiB, durationSec)
	p.Summary.RunID = fmt.Sprintf("seed-%s-%x", jobID, rng.Int63())
	p.Summary.Status = "heartbeat"
	return p
}

// makeDevOpsLearnJob builds a job payload for the DevOps-learn CI pipeline.
// It hard-codes repository and ci_platform and forces the recommendation to
// suggest ubuntu-latest as the right-sized alternative.
func makeDevOpsLearnJob(
	jobID string, at time.Time,
	detected types.MachineType,
	cpuP95, memP95GiB, durationSec float64,
) payload {
	p := makeJob(jobID, "github", at, detected, cpuP95, memP95GiB, durationSec)
	p.Summary.Repository = "gbudjeakp/DevOps-learn"
	// For the 4-core build step, always recommend ubuntu-latest as the cheaper label
	if detected.ID == "ubuntu-latest-4-cores" {
		currentMonthly := detected.OnDemandPricePerHour * 720
		p.Recommendations = []types.Recommendation{
			{
				Machine:          githubUbuntuLatest,
				Tier:             types.TierCheaper,
				EstimatedMonthly: githubUbuntuLatest.OnDemandPricePerHour * 720,
				CurrentMonthly:   currentMonthly,
				CostDeltaPercent: (githubUbuntuLatest.OnDemandPricePerHour - detected.OnDemandPricePerHour) /
					detected.OnDemandPricePerHour * 100, // -50%
				Reasoning: fmt.Sprintf(
					"p95 CPU %.1f%% on %d vCPUs — ubuntu-latest (2 vCPU) is sufficient",
					cpuP95, detected.VCPUs,
				),
			},
		}
	}
	return p
}

func makeJob(
	jobID, ciPlatform string, at time.Time,
	detected types.MachineType,
	cpuP95, memP95GiB, durationSec float64,
) payload {
	start := at
	end := at.Add(time.Duration(durationSec) * time.Second)
	currentMonthly := detected.OnDemandPricePerHour * 720

	summary := types.MetricsSummary{
		JobID:           jobID,
		CIPlatform:      ciPlatform,
		Repository:      seededRepository(jobID),
		StartTime:       start,
		EndTime:         end,
		DurationSeconds: durationSec,
		DetectedMachine: &detected,

		CPUPercentP95:  cpuP95,
		CPUPercentPeak: jitter(cpuP95*1.12, 3),
		CPUPercentAvg:  cpuP95 * 0.72,

		MemUsedGiBP95:  memP95GiB,
		MemUsedGiBPeak: jitter(memP95GiB*1.1, 0.05),
		MemUsedGiBAvg:  memP95GiB * 0.8,
		MemTotalGiB:    detected.MemoryGiB,

		ProcessCountPeak: int(jitter(24, 4)),
		ThreadCountPeak:  int(jitter(80, 15)),

		DiskReadMBsPeak:  jitter(12, 4),
		DiskWriteMBsPeak: jitter(8, 3),
		NetRxMBsPeak:     jitter(5, 2),
		NetTxMBsPeak:     jitter(3, 1),

		SampleCount: int(durationSec / 5),
	}

	recs := recommend(summary, detected, currentMonthly)
	return payload{Summary: summary, Recommendations: recs}
}

func seededRepository(jobID string) string {
	switch jobID {
	// Backend/Core team
	case "build", "build-release", "unit-tests", "lint", "security-scan":
		return "runrightio/app-core"
	// Frontend team
	case "e2e-tests", "deploy-staging", "storybook", "visual-tests":
		return "runrightio/web-ui"
	// Data Science team - data pipeline
	case "benchmark", "gcp-build", "python-tests", "data-transform":
		return "runrightio/data-pipeline"
	// Data Science team - ML platform
	case "ml-training", "gpu-inference", "ml-batch", "train-model", "model-eval":
		return "runrightio/ml-platform"
	// Platform Engineering
	case "docker-build", "terraform-plan", "k8s-deploy", "infra-tests", "disaster-recovery-drill":
		return "runrightio/infrastructure"
	// Security team
	case "sast-scan", "dast-scan", "dependency-audit":
		return "runrightio/security-scanner"
	// QA team
	case "integration-tests", "load-tests", "smoke-tests", "perf-regression":
		return "runrightio/test-automation"
	// Mobile team
	case "ios-build", "android-build", "mobile-tests", "ios-release":
		return "runrightio/mobile-app"
	// API Gateway
	case "api-tests", "contract-tests":
		return "runrightio/api-gateway"
	// DevOps-learn
	case "devops-learn-github-hosted-build",
		"devops-learn-github-hosted-test",
		"devops-learn-github-hosted-lint",
		"devops-learn-github-hosted-install":
		return "gbudjeakp/DevOps-learn"
	default:
		return "runrightio/misc"
	}
}

// recommend generates plausible recommendations based on the summary.
func recommend(s types.MetricsSummary, detected types.MachineType, currentMonthly float64) []types.Recommendation {
	cpuPct := s.CPUPercentP95
	memGiB := s.MemUsedGiBP95

	var recs []types.Recommendation

	// Right-sized: best fit for the workload
	switch {
	case detected.ID == "p4d.24xlarge" && cpuPct < 20:
		// Way over-provisioned GPU: downsize dramatically
		recs = append(recs, rec(awsP32XLarge, types.TierCheaper, currentMonthly))  // $3.06/hr vs $32.77/hr
		recs = append(recs, rec(awsG4dn12XLarge, types.TierRightSized, currentMonthly)) // $7.48/hr
		recs = append(recs, rec(awsP4d24XLarge, types.TierMoreHeadroom, currentMonthly))

	case detected.ID == "p3.2xlarge" && cpuPct < 30:
		// Over-provisioned GPU: downsize to smaller GPU or compute
		recs = append(recs, rec(awsC7gLarge, types.TierCheaper, currentMonthly))
		recs = append(recs, rec(awsM6iLarge, types.TierRightSized, currentMonthly))
		recs = append(recs, rec(awsP32XLarge, types.TierMoreHeadroom, currentMonthly))

	case detected.ID == "g4dn.12xlarge" && cpuPct < 25:
		// Over-provisioned GPU: downsize to smaller GPU
		gdn4XL := types.MachineType{
			ID: "g4dn.xlarge", Provider: types.ProviderAWS,
			Family: "gpu-accelerated", Series: "g4dn",
			VCPUs: 4, MemoryGiB: 16, NetworkGbps: 25, StorageType: "ebs",
			Architecture: "x86_64", OnDemandPricePerHour: 0.526,
			Tags: []string{"gpu", "inference"},
		}
		recs = append(recs, rec(gdn4XL, types.TierCheaper, currentMonthly))
		recs = append(recs, rec(awsM6iLarge, types.TierRightSized, currentMonthly))
		recs = append(recs, rec(awsG4dn12XLarge, types.TierMoreHeadroom, currentMonthly))

	case cpuPct < 20 && memGiB < 0.8:
		// Very light — suggest t3.nano → not in our set, use t3.small equivalent
		m := types.MachineType{
			ID: "t3.small", Provider: types.ProviderAWS,
			Family: "general-purpose", Series: "t3",
			VCPUs: 2, MemoryGiB: 2, NetworkGbps: 5, StorageType: "ebs",
			Architecture: "x86_64", OnDemandPricePerHour: 0.0208, Tags: []string{"burstable"},
		}
		recs = append(recs, rec(m, types.TierRightSized, currentMonthly))
		recs = append(recs, rec(gcpE2Medium, types.TierCheaper, currentMonthly))

	case cpuPct >= 60 || memGiB > detected.MemoryGiB*0.7:
		// Heavy — needs more headroom
		recs = append(recs, rec(awsM6iLarge, types.TierRightSized, currentMonthly))
		recs = append(recs, rec(awsC7gLarge, types.TierCheaper, currentMonthly))
		recs = append(recs, rec(awsT3Large, types.TierMoreHeadroom, currentMonthly))

	case detected.ID == "c7g.2xlarge" && cpuPct < 30:
		// Over-provisioned compute: downsize significantly
		recs = append(recs, rec(awsC7gLarge, types.TierCheaper, currentMonthly))
		recs = append(recs, rec(awsM6iLarge, types.TierRightSized, currentMonthly))
		recs = append(recs, rec(awsC7g2XLarge, types.TierMoreHeadroom, currentMonthly))

	default:
		// Moderate — current is okay but cheaper options exist
		recs = append(recs, rec(awsT3Medium, types.TierRightSized, currentMonthly))
		recs = append(recs, rec(gcpE2Medium, types.TierCheaper, currentMonthly))
		recs = append(recs, rec(gcpN2Standard2, types.TierMoreHeadroom, currentMonthly))
	}

	return recs
}

func rec(m types.MachineType, tier types.RecommendationTier, currentMonthly float64) types.Recommendation {
	estimated := m.OnDemandPricePerHour * 720
	delta := (estimated - currentMonthly) / currentMonthly * 100
	spotFactor := 0.30
	if m.Provider == types.ProviderGCP {
		spotFactor = 0.20
	}
	spot := estimated * spotFactor
	spotDelta := (spot - currentMonthly) / currentMonthly * 100
	return types.Recommendation{
		Machine:           m,
		Tier:              tier,
		EstimatedMonthly:  estimated,
		SpotMonthly:       spot,
		CurrentMonthly:    currentMonthly,
		CostDeltaPercent:  delta,
		SpotDeltaPercent:  spotDelta,
		RequiredVCPUs:     m.VCPUs,
		RequiredMemoryGiB: m.MemoryGiB,
		Reasoning:         "Based on p95 CPU and memory usage patterns over recent runs.",
	}
}
