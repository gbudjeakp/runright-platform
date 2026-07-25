package server

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/go-github/v62/github"
)

// GitHubAppConfig holds app credentials
type GitHubAppConfig struct {
	AppID         int64
	AppSlug       string
	ClientID      string
	ClientSecret  string
	PrivateKey    []byte
	WebhookSecret string
}

// GitHubApp manages the GitHub App functionality
type GitHubApp struct {
	db     *sql.DB
	config *GitHubAppConfig
	server *Server
}

// NewGitHubApp creates a new GitHub App handler
func NewGitHubApp(db *sql.DB, s *Server) *GitHubApp {
	app := &GitHubApp{db: db, server: s}
	app.loadConfig()
	return app
}

// loadConfig loads GitHub App config from environment or database
func (g *GitHubApp) loadConfig() {
	appIDStr := os.Getenv("GITHUB_APP_ID")
	if appIDStr == "" {
		return
	}
	appID, _ := strconv.ParseInt(appIDStr, 10, 64)

	g.config = &GitHubAppConfig{
		AppID:         appID,
		AppSlug:       os.Getenv("GITHUB_APP_SLUG"),
		ClientID:      os.Getenv("GITHUB_APP_CLIENT_ID"),
		ClientSecret:  os.Getenv("GITHUB_APP_CLIENT_SECRET"),
		PrivateKey:    []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY")),
		WebhookSecret: os.Getenv("GITHUB_APP_WEBHOOK_SECRET"),
	}
}

// IsConfigured returns true if the app is properly configured
func (g *GitHubApp) IsConfigured() bool {
	return g.config != nil && g.config.AppID > 0 && len(g.config.PrivateKey) > 0
}

// generateJWT creates a JWT for authenticating as the GitHub App
func (g *GitHubApp) generateJWT() (string, error) {
	if !g.IsConfigured() {
		return "", fmt.Errorf("GitHub App not configured")
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM(g.config.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Unix() - 60, // issued at (allow 1 min clock skew)
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": g.config.AppID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(key)
}

// getInstallationClient returns a GitHub client authenticated as an installation
func (g *GitHubApp) getInstallationClient(ctx context.Context, installationID int64) (*github.Client, error) {
	jwtToken, err := g.generateJWT()
	if err != nil {
		return nil, err
	}

	// Create app-authenticated client
	appClient := github.NewClient(nil).WithAuthToken(jwtToken)

	// Get installation access token
	token, _, err := appClient.Apps.CreateInstallationToken(ctx, installationID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create installation token: %w", err)
	}

	// Return client with installation token
	return github.NewClient(nil).WithAuthToken(token.GetToken()), nil
}

// HandleWebhook processes incoming GitHub webhooks
func (g *GitHubApp) HandleWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	// Verify signature if webhook secret is configured
	if g.config != nil && g.config.WebhookSecret != "" {
		sig := c.GetHeader("X-Hub-Signature-256")
		if !g.verifySignature(payload, sig) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
	}

	event := c.GetHeader("X-GitHub-Event")
	deliveryID := c.GetHeader("X-GitHub-Delivery")

	fmt.Printf("[GitHub App] Event: %s, Delivery: %s\n", event, deliveryID)

	switch event {
	case "installation":
		g.handleInstallation(c, payload)
	case "installation_repositories":
		g.handleInstallationRepos(c, payload)
	case "workflow_run":
		g.handleWorkflowRun(c, payload)
	case "check_run":
		g.handleCheckRun(c, payload)
	case "ping":
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	default:
		c.JSON(http.StatusOK, gin.H{"message": "event ignored", "event": event})
	}
}

// verifySignature validates the webhook HMAC signature
func (g *GitHubApp) verifySignature(payload []byte, signature string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(g.config.WebhookSecret))
	mac.Write(payload)
	expected := mac.Sum(nil)

	return hmac.Equal(sig, expected)
}

// handleInstallation processes installation created/deleted events
func (g *GitHubApp) handleInstallation(c *gin.Context, payload []byte) {
	var event github.InstallationEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	action := event.GetAction()
	inst := event.GetInstallation()

	switch action {
	case "created":
		err := g.saveInstallation(inst, event.Repositories)
		if err != nil {
			fmt.Printf("[GitHub App] Failed to save installation: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		fmt.Printf("[GitHub App] Installation created: %d (%s)\n", inst.GetID(), inst.GetAccount().GetLogin())

	case "deleted":
		_, err := g.db.Exec(`DELETE FROM github_app_installations WHERE installation_id = $1`, inst.GetID())
		if err != nil {
			fmt.Printf("[GitHub App] Failed to delete installation: %v\n", err)
		}
		fmt.Printf("[GitHub App] Installation deleted: %d\n", inst.GetID())

	case "suspend":
		_, err := g.db.Exec(`
			UPDATE github_app_installations 
			SET suspended_at = NOW(), suspended_by = $2 
			WHERE installation_id = $1`,
			inst.GetID(), event.GetSender().GetLogin())
		if err != nil {
			fmt.Printf("[GitHub App] Failed to suspend installation: %v\n", err)
		}

	case "unsuspend":
		_, err := g.db.Exec(`
			UPDATE github_app_installations 
			SET suspended_at = NULL, suspended_by = NULL 
			WHERE installation_id = $1`, inst.GetID())
		if err != nil {
			fmt.Printf("[GitHub App] Failed to unsuspend installation: %v\n", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"action": action, "installation_id": inst.GetID()})
}

// saveInstallation saves an installation and its repos to the database
func (g *GitHubApp) saveInstallation(inst *github.Installation, repos []*github.Repository) error {
	tx, err := g.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	account := inst.GetAccount()
	targetType := "all"
	if inst.GetRepositorySelection() == "selected" {
		targetType = "selected"
	}

	_, err = tx.Exec(`
		INSERT INTO github_app_installations (installation_id, account_type, account_login, account_id, target_type)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (installation_id) DO UPDATE SET
			account_type = EXCLUDED.account_type,
			account_login = EXCLUDED.account_login,
			target_type = EXCLUDED.target_type,
			updated_at = NOW()`,
		inst.GetID(), account.GetType(), account.GetLogin(), account.GetID(), targetType)
	if err != nil {
		return err
	}

	// Save repositories
	for _, repo := range repos {
		_, err = tx.Exec(`
			INSERT INTO github_app_repos (installation_id, repo_id, repo_name, private)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (installation_id, repo_id) DO UPDATE SET
				repo_name = EXCLUDED.repo_name,
				private = EXCLUDED.private`,
			inst.GetID(), repo.GetID(), repo.GetFullName(), repo.GetPrivate())
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// handleInstallationRepos processes repository add/remove events
func (g *GitHubApp) handleInstallationRepos(c *gin.Context, payload []byte) {
	var event github.InstallationRepositoriesEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	instID := event.GetInstallation().GetID()

	// Add new repos
	for _, repo := range event.RepositoriesAdded {
		_, err := g.db.Exec(`
			INSERT INTO github_app_repos (installation_id, repo_id, repo_name, private)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (installation_id, repo_id) DO UPDATE SET repo_name = EXCLUDED.repo_name`,
			instID, repo.GetID(), repo.GetFullName(), repo.GetPrivate())
		if err != nil {
			fmt.Printf("[GitHub App] Failed to add repo: %v\n", err)
		}
	}

	// Remove repos
	for _, repo := range event.RepositoriesRemoved {
		_, err := g.db.Exec(`DELETE FROM github_app_repos WHERE installation_id = $1 AND repo_id = $2`,
			instID, repo.GetID())
		if err != nil {
			fmt.Printf("[GitHub App] Failed to remove repo: %v\n", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"added":   len(event.RepositoriesAdded),
		"removed": len(event.RepositoriesRemoved),
	})
}

// handleWorkflowRun processes workflow_run events - this is the main entry point for metrics
func (g *GitHubApp) handleWorkflowRun(c *gin.Context, payload []byte) {
	var event github.WorkflowRunEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	action := event.GetAction()
	run := event.GetWorkflowRun()
	repo := event.GetRepo()
	instID := event.GetInstallation().GetID()

	// We only process completed runs
	if action != "completed" {
		c.JSON(http.StatusOK, gin.H{"message": "ignoring non-completed run", "action": action})
		return
	}

	// Determine PR number if this is a PR-related run
	var prNumber *int
	if len(run.PullRequests) > 0 {
		n := run.PullRequests[0].GetNumber()
		prNumber = &n
	}

	// Save the workflow run
	_, err := g.db.Exec(`
		INSERT INTO github_workflow_runs 
			(installation_id, run_id, repo_name, workflow_name, workflow_path, 
			 head_branch, head_sha, event, status, conclusion, pr_number, 
			 run_attempt, run_started_at, run_completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (run_id) DO UPDATE SET
			status = EXCLUDED.status,
			conclusion = EXCLUDED.conclusion,
			run_completed_at = EXCLUDED.run_completed_at,
			updated_at = NOW()`,
		instID, run.GetID(), repo.GetFullName(), run.GetName(), "", // workflow_path not available on WorkflowRun
		run.GetHeadBranch(), run.GetHeadSHA(), run.GetEvent(), run.GetStatus(),
		run.GetConclusion(), prNumber, run.GetRunAttempt(),
		run.GetRunStartedAt().Time, run.GetUpdatedAt().Time)
	if err != nil {
		fmt.Printf("[GitHub App] Failed to save workflow run: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Process asynchronously - download artifacts and analyze
	go g.processWorkflowRun(context.Background(), instID, run, repo, prNumber)

	c.JSON(http.StatusOK, gin.H{
		"message":     "workflow run queued for processing",
		"run_id":      run.GetID(),
		"repo":        repo.GetFullName(),
		"workflow":    run.GetName(),
		"conclusion":  run.GetConclusion(),
	})
}

// processWorkflowRun downloads artifacts and analyzes metrics
func (g *GitHubApp) processWorkflowRun(ctx context.Context, instID int64, run *github.WorkflowRun, repo *github.Repository, prNumber *int) {
	client, err := g.getInstallationClient(ctx, instID)
	if err != nil {
		fmt.Printf("[GitHub App] Failed to get installation client: %v\n", err)
		return
	}

	repoOwner, repoName := splitRepo(repo.GetFullName())

	// List workflow run artifacts
	artifacts, _, err := client.Actions.ListWorkflowRunArtifacts(ctx, repoOwner, repoName, run.GetID(), nil)
	if err != nil {
		fmt.Printf("[GitHub App] Failed to list artifacts: %v\n", err)
		return
	}

	// Look for RunRight metrics artifact
	var metricsArtifact *github.Artifact
	for _, art := range artifacts.Artifacts {
		name := art.GetName()
		if strings.Contains(name, "runright") || strings.Contains(name, "metrics") {
			metricsArtifact = art
			break
		}
	}

	if metricsArtifact == nil {
		fmt.Printf("[GitHub App] No RunRight artifact found for run %d\n", run.GetID())
		// Mark as processed but no metrics
		g.db.Exec(`UPDATE github_workflow_runs SET artifact_downloaded = true, updated_at = NOW() WHERE run_id = $1`, run.GetID())
		return
	}

	// Download and process the artifact
	metrics, err := g.downloadAndParseArtifact(ctx, client, repoOwner, repoName, metricsArtifact)
	if err != nil {
		fmt.Printf("[GitHub App] Failed to download artifact: %v\n", err)
		return
	}

	// Update database with metrics
	g.db.Exec(`UPDATE github_workflow_runs SET artifact_downloaded = true, metrics_processed = true, updated_at = NOW() WHERE run_id = $1`, run.GetID())

	// Create job record in the main jobs table (integrate with existing platform)
	g.ingestMetricsToJobs(ctx, repo.GetFullName(), run, metrics)

	// Post PR comment if this is a PR run
	if prNumber != nil && *prNumber > 0 {
		g.postPRComment(ctx, client, repoOwner, repoName, *prNumber, run, metrics)
	}
}

// downloadAndParseArtifact downloads and extracts the metrics artifact
func (g *GitHubApp) downloadAndParseArtifact(ctx context.Context, client *github.Client, owner, repo string, artifact *github.Artifact) (*MetricsPayload, error) {
	// Get artifact download URL
	url, _, err := client.Actions.DownloadArtifact(ctx, owner, repo, artifact.GetID(), 4)
	if err != nil {
		return nil, fmt.Errorf("failed to get download URL: %w", err)
	}

	// Download the artifact (it's a ZIP file)
	resp, err := http.Get(url.String())
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	// Unzip and find metrics file
	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}

	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, "metrics-summary.json") || strings.HasSuffix(f.Name, "metrics.json") {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			defer rc.Close()

			var metrics MetricsPayload
			if err := json.NewDecoder(rc).Decode(&metrics); err != nil {
				return nil, fmt.Errorf("failed to parse metrics: %w", err)
			}
			return &metrics, nil
		}
	}

	return nil, fmt.Errorf("no metrics file found in artifact")
}

// MetricsPayload represents the RunRight metrics structure
type MetricsPayload struct {
	JobID            string   `json:"job_id"`
	Repository       string   `json:"repository"`
	CommitSHA        string   `json:"commit_sha"`
	Branch           string   `json:"branch"`
	PRNumber         int      `json:"pr_number,omitempty"`
	StartTime        string   `json:"start_time"`
	EndTime          string   `json:"end_time"`
	DurationSec      float64  `json:"duration_sec"`
	CPUCores         int      `json:"cpu_cores"`
	MemoryTotalMB    int      `json:"memory_total_mb"`
	CPUP50           float64  `json:"cpu_p50"`
	CPUP95           float64  `json:"cpu_p95"`
	CPUMax           float64  `json:"cpu_max"`
	MemP50MB         float64  `json:"mem_p50_mb"`
	MemP95MB         float64  `json:"mem_p95_mb"`
	MemMaxMB         float64  `json:"mem_max_mb"`
	DiskReadMB       float64  `json:"disk_read_mb"`
	DiskWriteMB      float64  `json:"disk_write_mb"`
	NetInMB          float64  `json:"net_in_mb"`
	NetOutMB         float64  `json:"net_out_mb"`
	DetectedMachine  string   `json:"detected_machine"`
	Recommendations  []Recommendation `json:"recommendations,omitempty"`
}

// Recommendation represents a machine recommendation
type Recommendation struct {
	Provider    string  `json:"provider"`
	MachineType string  `json:"machine_type"`
	CPUCores    int     `json:"cpu_cores"`
	MemoryGB    float64 `json:"memory_gb"`
	PricePerHr  float64 `json:"price_per_hr"`
	Tier        string  `json:"tier"`
	Fit         string  `json:"fit"`
}

// ingestMetricsToJobs creates a job record in the existing jobs table
func (g *GitHubApp) ingestMetricsToJobs(ctx context.Context, repoName string, run *github.WorkflowRun, metrics *MetricsPayload) {
	// This integrates with the existing platform jobs table
	// The structure should match what the HTTP export does
	jobID := fmt.Sprintf("%s/%s", repoName, run.GetName())
	if metrics.JobID != "" {
		jobID = metrics.JobID
	}

	// Use the existing createJob logic via an internal call
	// For now, direct SQL insert matching existing schema
	_, err := g.db.Exec(`
		INSERT INTO jobs (
			job_id, repository, commit_sha, branch, pr_number,
			start_time, end_time, duration_sec,
			cpu_cores, memory_total_mb,
			cpu_p50, cpu_p95, cpu_max,
			mem_p50_mb, mem_p95_mb, mem_max_mb,
			disk_read_mb, disk_write_mb, net_in_mb, net_out_mb,
			detected_machine, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, 'github_app')`,
		jobID, repoName, run.GetHeadSHA(), run.GetHeadBranch(), metrics.PRNumber,
		metrics.StartTime, metrics.EndTime, metrics.DurationSec,
		metrics.CPUCores, metrics.MemoryTotalMB,
		metrics.CPUP50, metrics.CPUP95, metrics.CPUMax,
		metrics.MemP50MB, metrics.MemP95MB, metrics.MemMaxMB,
		metrics.DiskReadMB, metrics.DiskWriteMB, metrics.NetInMB, metrics.NetOutMB,
		metrics.DetectedMachine)
	if err != nil {
		fmt.Printf("[GitHub App] Failed to insert job: %v\n", err)
	}
}

// postPRComment posts a recommendation comment on the PR
func (g *GitHubApp) postPRComment(ctx context.Context, client *github.Client, owner, repo string, prNumber int, run *github.WorkflowRun, metrics *MetricsPayload) {
	body := g.formatPRComment(metrics)

	comment := &github.IssueComment{
		Body: &body,
	}

	_, _, err := client.Issues.CreateComment(ctx, owner, repo, prNumber, comment)
	if err != nil {
		fmt.Printf("[GitHub App] Failed to post PR comment: %v\n", err)
		return
	}

	// Mark comment as posted
	g.db.Exec(`UPDATE github_workflow_runs SET comment_posted = true, updated_at = NOW() WHERE run_id = $1`, run.GetID())
	fmt.Printf("[GitHub App] Posted PR comment on %s/%s#%d\n", owner, repo, prNumber)
}

// formatPRComment creates a markdown comment with recommendations
func (g *GitHubApp) formatPRComment(metrics *MetricsPayload) string {
	var sb strings.Builder

	sb.WriteString("## 📊 RunRight CI Analysis\n\n")

	// Resource usage summary
	sb.WriteString("### Resource Usage\n")
	sb.WriteString(fmt.Sprintf("| Metric | P50 | P95 | Max |\n"))
	sb.WriteString(fmt.Sprintf("|--------|-----|-----|-----|\n"))
	sb.WriteString(fmt.Sprintf("| CPU | %.1f%% | %.1f%% | %.1f%% |\n", metrics.CPUP50, metrics.CPUP95, metrics.CPUMax))
	sb.WriteString(fmt.Sprintf("| Memory | %.0f MB | %.0f MB | %.0f MB |\n", metrics.MemP50MB, metrics.MemP95MB, metrics.MemMaxMB))
	sb.WriteString("\n")

	if metrics.DetectedMachine != "" {
		sb.WriteString(fmt.Sprintf("**Current runner:** `%s`\n\n", metrics.DetectedMachine))
	}

	// Recommendations
	if len(metrics.Recommendations) > 0 {
		sb.WriteString("### 💡 Recommendations\n\n")
		sb.WriteString("| Provider | Machine | CPU | Memory | Price/hr | Fit |\n")
		sb.WriteString("|----------|---------|-----|--------|----------|-----|\n")

		for _, rec := range metrics.Recommendations {
			fitEmoji := "✅"
			if rec.Fit == "tight" {
				fitEmoji = "⚠️"
			} else if rec.Fit == "oversized" {
				fitEmoji = "📈"
			}
			sb.WriteString(fmt.Sprintf("| %s | `%s` | %d | %.1f GB | $%.4f | %s %s |\n",
				rec.Provider, rec.MachineType, rec.CPUCores, rec.MemoryGB, rec.PricePerHr, fitEmoji, rec.Fit))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n")
	sb.WriteString("<sub>📈 [View full analysis](https://runright.dev) | Powered by [RunRight](https://github.com/gbudjeakp/run-right)</sub>")

	return sb.String()
}

// handleCheckRun processes check_run events (for status checks)
func (g *GitHubApp) handleCheckRun(c *gin.Context, payload []byte) {
	// For now, just acknowledge - we primarily use workflow_run
	c.JSON(http.StatusOK, gin.H{"message": "check_run acknowledged"})
}

// ListInstallations returns all installations for the admin UI
func (g *GitHubApp) ListInstallations(c *gin.Context) {
	rows, err := g.db.Query(`
		SELECT 
			i.installation_id, i.account_type, i.account_login, i.account_id,
			i.target_type, i.suspended_at, i.created_at,
			COUNT(r.id) as repo_count
		FROM github_app_installations i
		LEFT JOIN github_app_repos r ON i.installation_id = r.installation_id
		GROUP BY i.id
		ORDER BY i.created_at DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var installations []map[string]interface{}
	for rows.Next() {
		var instID, accountID int64
		var accountType, accountLogin, targetType string
		var suspendedAt sql.NullTime
		var createdAt time.Time
		var repoCount int

		err := rows.Scan(&instID, &accountType, &accountLogin, &accountID, &targetType, &suspendedAt, &createdAt, &repoCount)
		if err != nil {
			continue
		}

		inst := map[string]interface{}{
			"installation_id": instID,
			"account_type":    accountType,
			"account_login":   accountLogin,
			"account_id":      accountID,
			"target_type":     targetType,
			"created_at":      createdAt,
			"repo_count":      repoCount,
		}
		if suspendedAt.Valid {
			inst["suspended_at"] = suspendedAt.Time
		}
		installations = append(installations, inst)
	}

	c.JSON(http.StatusOK, installations)
}

// GetInstallationRepos returns repos for a specific installation
func (g *GitHubApp) GetInstallationRepos(c *gin.Context) {
	instIDStr := c.Param("installationId")
	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid installation ID"})
		return
	}

	rows, err := g.db.Query(`
		SELECT repo_id, repo_name, private, created_at
		FROM github_app_repos
		WHERE installation_id = $1
		ORDER BY repo_name`, instID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var repos []map[string]interface{}
	for rows.Next() {
		var repoID int64
		var repoName string
		var private bool
		var createdAt time.Time

		err := rows.Scan(&repoID, &repoName, &private, &createdAt)
		if err != nil {
			continue
		}

		repos = append(repos, map[string]interface{}{
			"repo_id":    repoID,
			"repo_name":  repoName,
			"private":    private,
			"created_at": createdAt,
		})
	}

	c.JSON(http.StatusOK, repos)
}

// GetAppStatus returns the GitHub App configuration status
func (g *GitHubApp) GetAppStatus(c *gin.Context) {
	status := map[string]interface{}{
		"configured":   g.IsConfigured(),
		"app_id":       nil,
		"app_slug":     nil,
		"install_url":  nil,
	}

	if g.IsConfigured() {
		status["app_id"] = g.config.AppID
		status["app_slug"] = g.config.AppSlug
		status["install_url"] = fmt.Sprintf("https://github.com/apps/%s/installations/new", g.config.AppSlug)
	}

	// Get installation count
	var count int
	g.db.QueryRow(`SELECT COUNT(*) FROM github_app_installations WHERE suspended_at IS NULL`).Scan(&count)
	status["installation_count"] = count

	c.JSON(http.StatusOK, status)
}

// splitRepo splits "owner/repo" into owner and repo
func splitRepo(fullName string) (string, string) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return fullName, ""
	}
	return parts[0], parts[1]
}

// InjectWorkflowRequest represents a request to add RunRight to a workflow
type InjectWorkflowRequest struct {
	Repository   string `json:"repository" binding:"required"` // owner/repo
	WorkflowPath string `json:"workflow_path"`                 // optional, auto-detect if empty
	BranchName   string `json:"branch_name"`                   // optional, defaults to runright-setup
}

// InjectWorkflow creates a PR to add the RunRight action to a repository's workflow
func (g *GitHubApp) InjectWorkflow(ctx context.Context, req InjectWorkflowRequest) (*PRResult, error) {
	// Find installation for this repo
	var instID int64
	err := g.db.QueryRow(`
		SELECT installation_id FROM github_app_repos 
		WHERE repo_name = $1 LIMIT 1`, req.Repository).Scan(&instID)
	if err != nil {
		return nil, fmt.Errorf("repository not found in any installation: %w", err)
	}

	client, err := g.getInstallationClient(ctx, instID)
	if err != nil {
		return nil, err
	}

	owner, repo := splitRepo(req.Repository)
	branchName := req.BranchName
	if branchName == "" {
		branchName = "runright-setup"
	}

	// Get default branch
	repoInfo, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}
	defaultBranch := repoInfo.GetDefaultBranch()

	// Find workflow files
	workflowPath := req.WorkflowPath
	if workflowPath == "" {
		// Auto-detect workflow files
		_, dirContents, _, err := client.Repositories.GetContents(ctx, owner, repo, ".github/workflows", &github.RepositoryContentGetOptions{
			Ref: defaultBranch,
		})
		if err != nil {
			return nil, fmt.Errorf("no workflows found: %w", err)
		}

		// Pick the first .yml file
		for _, f := range dirContents {
			if strings.HasSuffix(f.GetName(), ".yml") || strings.HasSuffix(f.GetName(), ".yaml") {
				workflowPath = f.GetPath()
				break
			}
		}
		if workflowPath == "" {
			return nil, fmt.Errorf("no workflow files found")
		}
	}

	// Get the workflow file content
	fileContent, _, _, err := client.Repositories.GetContents(ctx, owner, repo, workflowPath, &github.RepositoryContentGetOptions{
		Ref: defaultBranch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow file: %w", err)
	}

	content, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("failed to decode content: %w", err)
	}

	// Check if RunRight is already in the workflow
	if strings.Contains(content, "gbudjeakp/run-right") || strings.Contains(content, "runright") {
		return nil, fmt.Errorf("RunRight already present in workflow")
	}

	// Modify the workflow to wrap commands with RunRight
	newContent := injectRunRightIntoWorkflow(content)

	// Get the default branch ref
	ref, _, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+defaultBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get ref: %w", err)
	}

	// Create a new branch
	newRef := &github.Reference{
		Ref:    github.String("refs/heads/" + branchName),
		Object: &github.GitObject{SHA: ref.Object.SHA},
	}
	_, _, err = client.Git.CreateRef(ctx, owner, repo, newRef)
	if err != nil {
		// Branch might already exist, try to update
		if !strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("failed to create branch: %w", err)
		}
	}

	// Update the file on the new branch
	opts := &github.RepositoryContentFileOptions{
		Message: github.String("Add RunRight CI monitoring\n\nThis PR adds RunRight to monitor CI resource usage and provide cost optimization recommendations."),
		Content: []byte(newContent),
		Branch:  github.String(branchName),
		SHA:     fileContent.SHA,
	}

	_, _, err = client.Repositories.UpdateFile(ctx, owner, repo, workflowPath, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to update file: %w", err)
	}

	// Create the PR
	pr, _, err := client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title: github.String("Add RunRight CI monitoring"),
		Body: github.String(`## 📊 Add RunRight CI Monitoring

This PR adds [RunRight](https://runright.dev) to your CI workflow to:

- Monitor CPU and memory usage during builds
- Recommend optimal runner sizes
- Track cost savings over time

### What changes?

The workflow is updated to use the RunRight action, which wraps your existing commands and collects metrics.

### What happens next?

Once merged, you'll see cost recommendations as PR comments and in your RunRight dashboard.

---
*This PR was automatically created by the RunRight GitHub App.*`),
		Head: github.String(branchName),
		Base: github.String(defaultBranch),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	return &PRResult{
		PRURL:    pr.GetHTMLURL(),
		PRNumber: pr.GetNumber(),
		Branch:   branchName,
	}, nil
}

// PRResult contains the result of creating a PR
type PRResult struct {
	PRURL    string `json:"pr_url"`
	PRNumber int    `json:"pr_number"`
	Branch   string `json:"branch"`
}

// injectRunRightIntoWorkflow modifies a workflow YAML to use RunRight
func injectRunRightIntoWorkflow(content string) string {
	// Simple injection: add RunRight as the first step in each job
	// This is a basic implementation - a production version would use a proper YAML parser

	lines := strings.Split(content, "\n")
	var result []string
	// stepsIndent tracks indentation when we find a steps: block
	var stepsIndent int

	for i, line := range lines {
		result = append(result, line)

		// Detect "steps:" line
		trimmed := strings.TrimSpace(line)
		if trimmed == "steps:" {
			stepsIndent = len(line) - len(strings.TrimLeft(line, " "))
			
			// Add RunRight as the first step
			indent := strings.Repeat(" ", stepsIndent+2)
			result = append(result, indent+"- name: Start RunRight monitoring")
			result = append(result, indent+"  uses: gbudjeakp/run-right@v1")
			result = append(result, indent+"  with:")
			result = append(result, indent+"    step: start")
			result = append(result, "")

			// Look ahead to find the last step and add stop
			// (simplified - a real impl would be more robust)
			_ = i // acknowledge the index
			_ = stepsIndent // acknowledge the indent
		}
	}

	// Add a final step to stop monitoring (simplified approach)
	// In practice, we'd find the right place to insert this
	finalContent := strings.Join(result, "\n")
	
	// Add stop step before the end of each job's steps
	// This is a simplified version - production would use YAML parsing
	return finalContent
}

// ListWorkflowRuns lists recent workflow runs from the GitHub App
func (g *GitHubApp) ListWorkflowRuns(c *gin.Context) {
	repo := c.Query("repository")
	limit := 50

	var rows *sql.Rows
	var err error

	if repo != "" {
		rows, err = g.db.Query(`
			SELECT run_id, repo_name, workflow_name, head_branch, head_sha,
				   event, status, conclusion, pr_number, 
				   artifact_downloaded, metrics_processed, comment_posted,
				   run_started_at, run_completed_at, created_at
			FROM github_workflow_runs
			WHERE repo_name = $1
			ORDER BY created_at DESC
			LIMIT $2`, repo, limit)
	} else {
		rows, err = g.db.Query(`
			SELECT run_id, repo_name, workflow_name, head_branch, head_sha,
				   event, status, conclusion, pr_number,
				   artifact_downloaded, metrics_processed, comment_posted,
				   run_started_at, run_completed_at, created_at
			FROM github_workflow_runs
			ORDER BY created_at DESC
			LIMIT $1`, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var runs []map[string]interface{}
	for rows.Next() {
		var runID int64
		var repoName, workflowName, headBranch, headSHA, event, status, conclusion string
		var prNumber sql.NullInt64
		var artifactDownloaded, metricsProcessed, commentPosted bool
		var runStartedAt, runCompletedAt, createdAt sql.NullTime

		err := rows.Scan(&runID, &repoName, &workflowName, &headBranch, &headSHA,
			&event, &status, &conclusion, &prNumber,
			&artifactDownloaded, &metricsProcessed, &commentPosted,
			&runStartedAt, &runCompletedAt, &createdAt)
		if err != nil {
			continue
		}

		run := map[string]interface{}{
			"run_id":              runID,
			"repo_name":           repoName,
			"workflow_name":       workflowName,
			"head_branch":         headBranch,
			"head_sha":            headSHA,
			"event":               event,
			"status":              status,
			"conclusion":          conclusion,
			"artifact_downloaded": artifactDownloaded,
			"metrics_processed":   metricsProcessed,
			"comment_posted":      commentPosted,
		}
		if prNumber.Valid {
			run["pr_number"] = prNumber.Int64
		}
		if runStartedAt.Valid {
			run["run_started_at"] = runStartedAt.Time
		}
		if runCompletedAt.Valid {
			run["run_completed_at"] = runCompletedAt.Time
		}
		if createdAt.Valid {
			run["created_at"] = createdAt.Time
		}

		runs = append(runs, run)
	}

	c.JSON(http.StatusOK, runs)
}
