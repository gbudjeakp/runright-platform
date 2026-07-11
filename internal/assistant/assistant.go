// Package assistant provides AI-powered analysis and Q&A capabilities for RunRight.
// It integrates with LLM providers (OpenAI, Anthropic, Ollama) and aggregates system data
// to answer natural language questions about CI/CD costs and resource usage.
package assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sgbudje/runright-platform/internal/types"
)

// LLMProvider identifies the AI model provider.
type LLMProvider string

const (
	ProviderOpenAI    LLMProvider = "openai"
	ProviderAnthropic LLMProvider = "anthropic"
	ProviderOllama    LLMProvider = "ollama"
)

// Config holds assistant configuration.
type Config struct {
	Provider       LLMProvider
	APIKey         string // Not required for Ollama
	BaseURL        string // Custom endpoint URL (required for Ollama, optional for others)
	Model          string // e.g., "gpt-4o", "claude-sonnet-4-20250514", "llama3.2", "mistral"
	MaxContextJobs int    // max jobs to include in context (default 100)
	UseRAG         bool   // Enable RAG (semantic search) for context building
}

// EmbeddingService interface for semantic search.
type EmbeddingService interface {
	Search(ctx context.Context, query string, limit int, repository string) ([]types.EmbeddingSearchResult, error)
	IsConfigured() bool
}

// Assistant is the AI assistant service.
type Assistant struct {
	db         *sql.DB
	cfg        Config
	client     *http.Client
	embeddings EmbeddingService
	provider   Provider // Abstracted LLM provider
}

// New creates a new Assistant with the given config.
func New(db *sql.DB, cfg Config) *Assistant {
	if cfg.MaxContextJobs == 0 {
		cfg.MaxContextJobs = 100
	}
	if cfg.Model == "" {
		switch cfg.Provider {
		case ProviderOpenAI:
			cfg.Model = "gpt-4o"
		case ProviderAnthropic:
			cfg.Model = "claude-sonnet-4-20250514"
		case ProviderOllama:
			cfg.Model = "llama3.2"
		default:
			cfg.Model = "gpt-4o"
		}
	}
	// Default Ollama URL if not specified
	if cfg.Provider == ProviderOllama && cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}

	httpClient := &http.Client{Timeout: 180 * time.Second}

	// Create the LLM provider
	provider := NewProvider(cfg.Provider, ProviderConfig{
		APIKey:      cfg.APIKey,
		BaseURL:     cfg.BaseURL,
		Model:       cfg.Model,
		MaxTokens:   2048,
		Temperature: 0.3,
		HTTPClient:  httpClient,
	})

	return &Assistant{
		db:       db,
		cfg:      cfg,
		client:   httpClient,
		provider: provider,
	}
}

// NewFromEnv creates an Assistant using environment variables.
// RUNRIGHT_AI_PROVIDER: "openai", "anthropic", or "ollama"
// RUNRIGHT_AI_API_KEY: API key for the provider (not required for Ollama)
// RUNRIGHT_AI_BASE_URL: Custom endpoint URL (required for remote Ollama)
// RUNRIGHT_AI_MODEL: model name (optional)
// RUNRIGHT_AI_USE_RAG: "true" to enable RAG
func NewFromEnv(db *sql.DB) *Assistant {
	provider := LLMProvider(os.Getenv("RUNRIGHT_AI_PROVIDER"))
	if provider == "" {
		provider = ProviderOpenAI
	}
	// Check both RUNRIGHT_USE_RAG and RUNRIGHT_AI_USE_RAG for compatibility
	useRAG := os.Getenv("RUNRIGHT_USE_RAG") == "true" || os.Getenv("RUNRIGHT_AI_USE_RAG") == "true"
	return New(db, Config{
		Provider: provider,
		APIKey:   os.Getenv("RUNRIGHT_AI_API_KEY"),
		BaseURL:  os.Getenv("RUNRIGHT_AI_BASE_URL"),
		Model:    os.Getenv("RUNRIGHT_AI_MODEL"),
		UseRAG:   useRAG,
	})
}

// SetEmbeddingService sets the embedding service for RAG.
func (a *Assistant) SetEmbeddingService(svc EmbeddingService) {
	a.embeddings = svc
}

// IsConfigured returns true if the assistant is properly configured.
func (a *Assistant) IsConfigured() bool {
	switch a.cfg.Provider {
	case ProviderOllama:
		// Ollama just needs a base URL (defaults to localhost)
		return a.cfg.BaseURL != ""
	default:
		// Other providers need an API key
		return a.cfg.APIKey != ""
	}
}

// GetProviderInfo returns info about the configured provider.
func (a *Assistant) GetProviderInfo() map[string]string {
	info := map[string]string{
		"provider": string(a.cfg.Provider),
		"model":    a.cfg.Model,
	}
	if a.cfg.Provider == ProviderOllama {
		info["base_url"] = a.cfg.BaseURL
	}
	return info
}

// systemPrompt returns the base system prompt for the assistant.
func (a *Assistant) systemPrompt() string {
	base := `You are RunRight AI, an intelligent assistant for the RunRight CI/CD cost optimization platform.

Your role is to help users understand and optimize their CI/CD resource usage and costs.

## CRITICAL: USE THE PROVIDED DATA

You have access to REAL job metrics and recommendations in the context below. When answering:

1. **ALWAYS reference specific data** from the context:
   - Job IDs, repository names, machine types
   - Actual CPU/memory/GPU utilization numbers
   - Specific dollar amounts and percentages
   - RunRight's machine recommendations (e.g., "switch from m5.4xlarge to m5.2xlarge")

2. **Quote RunRight recommendations directly**:
   - "RunRight recommends switching job X from [current] to [recommended], saving $Y/mo"
   - Reference the "Recommended:" lines in job data
   - Use the savings figures from "Top savings opportunities"

3. **Be specific and actionable**:
   - BAD: "Consider switching to spot instances"
   - GOOD: "RunRight recommends switching gpu-inference from p3.2xlarge to p3.xlarge, saving $125.90/mo. The job uses only 45% GPU utilization."

4. **Format responses clearly**:
   - Use tables for cost comparisons
   - Bold the key numbers: **$1505.66 total**, **save $125.90/mo**
   - Link recommendations to actual utilization data

## Data you have access to:

- Job execution metrics (CPU, memory, GPU utilization - peak, avg, p95)
- Current machine assignments and costs
- RunRight's specific machine recommendations with savings estimates
- Top savings opportunities ranked by impact
- Cost policies and alert rules

## When users ask about costs or optimization:

1. First, cite the ACTUAL numbers from the data
2. Then, provide RunRight's specific recommendations
3. Finally, explain WHY based on utilization metrics

Example response format:
"**runrightio/ml-platform** is your most expensive repo at **$1505.66 total**.

Top savings opportunities:
| Job | Current | Recommended | Savings |
|-----|---------|-------------|---------|
| gpu-inference | p3.2xlarge ($3.06/hr) | p3.xlarge ($1.53/hr) | **$125.90/mo** |
| ml-training | m5.4xlarge ($0.77/hr) | m5.2xlarge ($0.38/hr) | **$48.20/mo** |

The gpu-inference job only uses **45% GPU** and **32% memory**, so the smaller instance is sufficient."

Always be helpful, accurate, and focused on helping users reduce costs with specific RunRight recommendations.`

	return base + a.ToolsSystemPromptAddition()
}

// Chat processes a chat message and returns a response.
func (a *Assistant) Chat(ctx context.Context, req types.ChatRequest, userID string) (*types.ChatResponse, error) {
	if !a.IsConfigured() {
		return nil, fmt.Errorf("AI assistant not configured: set RUNRIGHT_AI_API_KEY")
	}

	// Get or create conversation
	convID := req.ConversationID
	if convID == "" {
		convID = uuid.New().String()
		// Create new conversation
		title := summarizeQuestion(req.Message)
		if err := a.createConversation(ctx, convID, title, userID); err != nil {
			return nil, fmt.Errorf("create conversation: %w", err)
		}
	} else {
		// Verify the caller owns this conversation before appending to it.
		var ownerID string
		err := a.db.QueryRowContext(ctx,
			`SELECT user_id FROM assistant_conversations WHERE id = $1`, convID,
		).Scan(&ownerID)
		if err != nil || ownerID != userID {
			return nil, fmt.Errorf("conversation not found")
		}
	}

	// Store user message
	userMsg := types.ChatMessage{
		ID:             uuid.New().String(),
		ConversationID: convID,
		Role:           "user",
		Content:        req.Message,
		CreatedAt:      time.Now(),
	}
	if err := a.storeMessage(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("store user message: %w", err)
	}

	// Build context from RunRight data (with optional RAG)
	assistantCtx, dataSources, err := a.buildContext(ctx, req.Repository, req.JobID, req.Message, req.PageContext)
	if err != nil {
		return nil, fmt.Errorf("build context: %w", err)
	}

	// Apply token-budget-aware trimming so we don't overflow context windows,
	// especially important for local Ollama models with smaller context sizes.
	budgets := a.tokenBudgets()
	assistantCtx = trimContext(assistantCtx, budgets.contextBudget)

	// Load compacted memory (summary of older turns + recent verbatim messages).
	mem, err := a.buildMemory(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("build memory: %w", err)
	}

	// Trim verbatim history to fit the history token budget.
	history := trimHistory(mem.RecentMessages, budgets.historyBudget)

	// Call LLM with compacted memory summary injected alongside history.
	response, err := a.callLLM(ctx, mem.Summary, history, assistantCtx, req.Message, userID)
	if err != nil {
		return nil, fmt.Errorf("call LLM: %w", err)
	}

	// Store assistant response
	assistantMsg := types.ChatMessage{
		ID:             uuid.New().String(),
		ConversationID: convID,
		Role:           "assistant",
		Content:        response,
		CreatedAt:      time.Now(),
		Metadata: map[string]any{
			"model":    a.cfg.Model,
			"provider": string(a.cfg.Provider),
		},
	}
	if err := a.storeMessage(ctx, assistantMsg); err != nil {
		return nil, fmt.Errorf("store assistant message: %w", err)
	}

	// Update conversation timestamp, then asynchronously compact if the conversation
	// has grown beyond the threshold — avoids adding latency to the user response.
	a.updateConversationTimestamp(ctx, convID)
	go a.compactIfNeeded(context.Background(), convID)

	return &types.ChatResponse{
		ConversationID: convID,
		Message:        assistantMsg,
		DataSources:    dataSources,
	}, nil
}

// buildContext aggregates RunRight data for the LLM.
// If RAG is enabled and embeddings are configured, it uses semantic search
// to find relevant jobs based on the user's question.
func (a *Assistant) buildContext(ctx context.Context, repository, jobID, userMessage string, pageCtx *types.PageContext) (*types.AssistantContext, []types.DataSource, error) {
	var dataSources []types.DataSource
	assistantCtx := &types.AssistantContext{}

	// Include page context so the assistant knows what the user is looking at
	if pageCtx != nil {
		assistantCtx.PageContext = pageCtx
		dataSources = append(dataSources, types.DataSource{
			Type:        "page_context",
			Description: fmt.Sprintf("User is viewing: %s", pageCtx.Page),
		})
	}

	// Use RAG if enabled and configured
	if a.cfg.UseRAG && a.embeddings != nil && a.embeddings.IsConfigured() {
		// Semantic search for relevant jobs
		results, err := a.embeddings.Search(ctx, userMessage, 20, repository)
		if err == nil && len(results) > 0 {
			// Add semantic search results to context
			var relevantText strings.Builder
			relevantText.WriteString("Semantically relevant jobs based on your question:\n\n")
			for i, r := range results {
				relevantText.WriteString(fmt.Sprintf("--- Job %d (similarity: %.2f) ---\n%s\n\n", i+1, r.Similarity, r.EmbeddedText))
			}
			assistantCtx.SemanticContext = relevantText.String()
			dataSources = append(dataSources, types.DataSource{
				Type:        "semantic_search",
				Description: "AI-retrieved relevant jobs",
				Count:       len(results),
			})
		}
	}

	// Fetch jobs with recommendations (reduced if RAG provided context)
	jobLimit := a.cfg.MaxContextJobs
	if assistantCtx.SemanticContext != "" {
		jobLimit = 20 // Reduce regular job fetch when we have semantic results
	}
	jobs, recs, err := a.fetchJobsWithRecommendations(ctx, repository, jobID, jobLimit)
	if err != nil {
		return nil, nil, err
	}
	assistantCtx.Jobs = jobs
	assistantCtx.Recommendations = recs
	if len(jobs) > 0 {
		dataSources = append(dataSources, types.DataSource{
			Type:        "jobs",
			Description: "Job execution metrics and recommendations",
			Count:       len(jobs),
		})
	}

	// Fetch savings summary
	savings, err := a.fetchSavingsSummary(ctx, repository)
	if err != nil {
		return nil, nil, err
	}
	assistantCtx.Savings = savings
	if savings != nil {
		dataSources = append(dataSources, types.DataSource{
			Type:        "savings",
			Description: "Cost savings opportunities",
			Count:       len(savings.TopSavingsOpportunities),
		})
	}

	// Fetch policies
	policies, err := a.fetchPolicies(ctx, repository)
	if err != nil {
		return nil, nil, err
	}
	assistantCtx.Policies = policies
	if len(policies) > 0 {
		dataSources = append(dataSources, types.DataSource{
			Type:        "policies",
			Description: "Cost policy rules",
			Count:       len(policies),
		})
	}

	// Fetch alerts if on alerts page or user mentions alerts
	if pageCtx != nil && pageCtx.Page == "alerts" {
		alerts, err := a.fetchAlerts(ctx)
		if err == nil && len(alerts) > 0 {
			assistantCtx.Alerts = alerts
			dataSources = append(dataSources, types.DataSource{
				Type:        "alerts",
				Description: "Alert rules",
				Count:       len(alerts),
			})
		}
		destinations, err := a.fetchDestinations(ctx)
		if err == nil && len(destinations) > 0 {
			assistantCtx.Destinations = destinations
			dataSources = append(dataSources, types.DataSource{
				Type:        "destinations",
				Description: "Alert destinations",
				Count:       len(destinations),
			})
		}
	}

	// Fetch repositories
	repos, err := a.fetchRepositories(ctx)
	if err != nil {
		return nil, nil, err
	}
	assistantCtx.Repositories = repos

	return assistantCtx, dataSources, nil
}

// fetchJobsWithRecommendations retrieves recent jobs with their recommendations.
func (a *Assistant) fetchJobsWithRecommendations(ctx context.Context, repository, jobID string, limit int) ([]types.MetricsSummary, map[string][]types.Recommendation, error) {
	query := `
		SELECT 
			summary,
			recommendations
		FROM jobs
		WHERE 1=1
	`
	args := []any{}
	argNum := 1

	if repository != "" {
		query += fmt.Sprintf(" AND summary->>'repository' = $%d", argNum)
		args = append(args, repository)
		argNum++
	}
	if jobID != "" {
		query += fmt.Sprintf(" AND summary->>'job_id' = $%d", argNum)
		args = append(args, jobID)
		argNum++
	}

	query += " ORDER BY (summary->>'start_time')::timestamp DESC"
	query += fmt.Sprintf(" LIMIT $%d", argNum)
	args = append(args, limit)

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var jobs []types.MetricsSummary
	recommendations := make(map[string][]types.Recommendation)

	for rows.Next() {
		var summaryJSON, recsJSON sql.NullString
		if err := rows.Scan(&summaryJSON, &recsJSON); err != nil {
			return nil, nil, err
		}

		if summaryJSON.Valid {
			var summary types.MetricsSummary
			if err := json.Unmarshal([]byte(summaryJSON.String), &summary); err == nil {
				jobs = append(jobs, summary)

				if recsJSON.Valid && recsJSON.String != "" {
					var recs []types.Recommendation
					if err := json.Unmarshal([]byte(recsJSON.String), &recs); err == nil && len(recs) > 0 {
						recommendations[summary.JobID] = recs
					}
				}
			}
		}
	}

	return jobs, recommendations, rows.Err()
}

// fetchSavingsSummary calculates current savings opportunities.
func (a *Assistant) fetchSavingsSummary(ctx context.Context, repository string) (*types.SavingsSnapshot, error) {
	query := `
		WITH job_costs AS (
			SELECT 
				summary->>'job_id' as job_id,
				summary->>'repository' as repository,
				(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
					(summary->>'duration_seconds')::numeric / 3600 as current_cost,
				CASE WHEN recommendations IS NOT NULL AND jsonb_array_length(recommendations) > 0
					THEN (recommendations->0->'machine'->>'on_demand_price_per_hour')::numeric *
						(summary->>'duration_seconds')::numeric / 3600
					ELSE (summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
						(summary->>'duration_seconds')::numeric / 3600
				END as recommended_cost,
				recommendations->0->'machine'->>'id' as recommended_machine
			FROM jobs
			WHERE summary->'detected_machine'->>'on_demand_price_per_hour' IS NOT NULL
	`
	args := []any{}
	if repository != "" {
		query += " AND summary->>'repository' = $1"
		args = append(args, repository)
	}
	query += `
		)
		SELECT 
			COALESCE(SUM(current_cost), 0) as total_current,
			COALESCE(SUM(recommended_cost), 0) as total_recommended,
			job_id,
			repository,
			current_cost,
			recommended_cost,
			recommended_machine
		FROM job_costs
		GROUP BY job_id, repository, current_cost, recommended_cost, recommended_machine
		ORDER BY (current_cost - recommended_cost) DESC
		LIMIT 20
	`

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var totalCurrent, totalRecommended float64
	var opportunities []types.SavingsOpportunity

	for rows.Next() {
		var tc, tr float64
		var opp types.SavingsOpportunity
		var recMachine sql.NullString
		if err := rows.Scan(&tc, &tr, &opp.JobID, &opp.Repository, &opp.CurrentCostUSD, &opp.RecommendedCostUSD, &recMachine); err != nil {
			return nil, err
		}
		totalCurrent = tc
		totalRecommended = tr
		opp.SavingsUSD = opp.CurrentCostUSD - opp.RecommendedCostUSD
		if opp.CurrentCostUSD > 0 {
			opp.SavingsPercent = (opp.SavingsUSD / opp.CurrentCostUSD) * 100
		}
		if recMachine.Valid {
			opp.RecommendedMachine = recMachine.String
		}
		if opp.SavingsUSD > 0 {
			opportunities = append(opportunities, opp)
		}
	}

	snapshot := &types.SavingsSnapshot{
		TotalCurrentCostUSD:      totalCurrent,
		TotalPotentialSavingsUSD: totalCurrent - totalRecommended,
		TopSavingsOpportunities:  opportunities,
	}
	if totalCurrent > 0 {
		snapshot.SavingsPercent = (snapshot.TotalPotentialSavingsUSD / totalCurrent) * 100
	}

	return snapshot, rows.Err()
}

// fetchPolicies retrieves policy rules.
func (a *Assistant) fetchPolicies(ctx context.Context, repository string) ([]types.PolicyRule, error) {
	query := `SELECT repository, job_id, max_cost_per_hour, enabled FROM policy_rules`
	args := []any{}
	if repository != "" {
		query += " WHERE repository = $1 OR repository = ''"
		args = append(args, repository)
	}

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		// Table might not exist
		return nil, nil
	}
	defer rows.Close()

	var policies []types.PolicyRule
	for rows.Next() {
		var p types.PolicyRule
		var jobID sql.NullString
		if err := rows.Scan(&p.Repository, &jobID, &p.MaxCostPerHour, &p.Enabled); err != nil {
			return nil, err
		}
		if jobID.Valid {
			p.JobID = jobID.String
		}
		policies = append(policies, p)
	}

	return policies, rows.Err()
}

// fetchAlerts retrieves alert rules.
func (a *Assistant) fetchAlerts(ctx context.Context) ([]types.AlertRule, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(repository, ''), COALESCE(job_id, ''), 
		       condition_type, threshold_value, channel, destination, enabled
		FROM alert_rules
		ORDER BY created_at DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	var alerts []types.AlertRule
	for rows.Next() {
		var a types.AlertRule
		if err := rows.Scan(&a.ID, &a.Name, &a.Repository, &a.JobID,
			&a.ConditionType, &a.Threshold, &a.Channel, &a.Destination, &a.Enabled); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// fetchDestinations retrieves alert destinations.
func (a *Assistant) fetchDestinations(ctx context.Context) ([]types.AlertDestination, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, name, type, COALESCE(config, ''), verified
		FROM alert_destinations
		ORDER BY name
	`)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	var destinations []types.AlertDestination
	for rows.Next() {
		var d types.AlertDestination
		if err := rows.Scan(&d.ID, &d.Name, &d.Type, &d.Config, &d.Verified); err != nil {
			return nil, err
		}
		destinations = append(destinations, d)
	}
	return destinations, rows.Err()
}

// fetchRepositories gets all known repositories.
func (a *Assistant) fetchRepositories(ctx context.Context) ([]string, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT DISTINCT summary->>'repository' as repo
		FROM jobs
		WHERE summary->>'repository' IS NOT NULL AND summary->>'repository' != ''
		ORDER BY repo
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []string
	for rows.Next() {
		var repo string
		if err := rows.Scan(&repo); err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}

	return repos, rows.Err()
}

// callLLM sends the request to the configured LLM provider using the provider interface.
// memorySummary is the compacted digest of older conversation turns; it is injected
// as an additional system message so the model retains long-term context efficiently.
func (a *Assistant) callLLM(ctx context.Context, memorySummary string, history []types.ChatMessage, assistantCtx *types.AssistantContext, userMessage string, userID string) (string, error) {
	// Detect if this is an action request that should force tool use
	forceToolUse := isActionRequest(userMessage)

	// Build the unified chat request
	req := ChatCompletionRequest{
		SystemPrompt:    a.systemPrompt(),
		MemorySummary:   memorySummary,
		SemanticContext: assistantCtx.SemanticContext,
		DataContext:     formatContextAsText(assistantCtx),
		History:         history,
		UserMessage:     userMessage,
		Tools:           a.AvailableTools(),
		MaxIterations:   5,
		ForceToolUse:    forceToolUse,
	}

	// Use the provider to execute the chat with tool support
	return a.provider.Chat(ctx, req, func(ctx context.Context, call ToolCall) (*ToolResult, error) {
		return a.ExecuteTool(ctx, call, userID, "")
	})
}

// isActionRequest detects if the user message is asking for an action (create, delete, etc.)
// or confirming a pending action. These requests should force tool use.
func isActionRequest(msg string) bool {
	lower := strings.ToLower(msg)
	
	// Action words that indicate the user wants something done
	actionWords := []string{
		"create", "make", "add", "set up", "setup", "configure",
		"delete", "remove", "revoke",
		"update", "change", "modify", "edit",
		"list", "show", "get", "view", "display",
		"enable", "disable", "toggle",
		"snooze", "archive", "unarchive",
		"assign", "unassign",
		"alert", "policy", "role", "user", "key", "label",
	}
	for _, word := range actionWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	
	// Confirmation words - user is confirming a pending action
	// Only match if the message is short (likely a confirmation, not a complex sentence)
	if len(lower) < 50 {
		confirmWords := []string{
			"yes", "yeah", "yep", "yup", "sure", "ok", "okay",
			"go ahead", "do it", "proceed", "confirm", "approved",
			"sounds good", "that's right", "correct", "exactly",
		}
		for _, word := range confirmWords {
			if strings.Contains(lower, word) {
				return true
			}
		}
	}
	
	return false
}

// formatContextAsText converts AssistantContext into human-readable structured text
// that LLMs can reason about far more effectively than raw JSON.
func formatContextAsText(ctx *types.AssistantContext) string {
	var b strings.Builder

	// --- Page Context (what the user is looking at) ---
	if ctx.PageContext != nil {
		b.WriteString("=== CURRENT PAGE CONTEXT ===\n")
		b.WriteString(fmt.Sprintf("  User is viewing: %s\n", ctx.PageContext.Page))
		if ctx.PageContext.EntityType != "" && ctx.PageContext.EntityID != "" {
			b.WriteString(fmt.Sprintf("  Viewing specific %s: %s\n", ctx.PageContext.EntityType, ctx.PageContext.EntityID))
		}
		if len(ctx.PageContext.Metadata) > 0 {
			b.WriteString("  Page data:\n")
			for k, v := range ctx.PageContext.Metadata {
				b.WriteString(fmt.Sprintf("    %s: %v\n", k, v))
			}
		}
		b.WriteString("\n")
		b.WriteString("NOTE: Analyze this page's content when answering. If the user asks about ")
		b.WriteString("\"this\", \"these\", or \"what I'm looking at\", refer to the above context.\n\n")
	}

	// --- Repositories ---
	if len(ctx.Repositories) > 0 {
		b.WriteString("=== REPOSITORIES ===\n")
		for _, r := range ctx.Repositories {
			b.WriteString(fmt.Sprintf("  • %s\n", r))
		}
		b.WriteString("\n")
	}

	// --- Jobs ---
	if len(ctx.Jobs) > 0 {
		b.WriteString(fmt.Sprintf("=== JOBS (%d most recent runs) ===\n", len(ctx.Jobs)))
		for _, j := range ctx.Jobs {
			machine := "unknown"
			machinePrice := 0.0
			machineVCPUs := 0
			machineMemGiB := 0.0
			if j.DetectedMachine != nil {
				machine = j.DetectedMachine.ID
				machinePrice = j.DetectedMachine.OnDemandPricePerHour
				machineVCPUs = j.DetectedMachine.VCPUs
				machineMemGiB = j.DetectedMachine.MemoryGiB
			}
			repo := j.Repository
			if repo == "" {
				repo = "(no repo)"
			}
			b.WriteString(fmt.Sprintf("JOB: %s  (repo: %s)\n", j.JobID, repo))
			b.WriteString(fmt.Sprintf("  Machine: %s  (%d vCPU, %.1f GiB RAM, $%.4f/hr)\n", machine, machineVCPUs, machineMemGiB, machinePrice))
			b.WriteString(fmt.Sprintf("  CPU:     peak %.1f%%  avg %.1f%%  p95 %.1f%%\n", j.CPUPercentPeak, j.CPUPercentAvg, j.CPUPercentP95))
			b.WriteString(fmt.Sprintf("  Memory:  peak %.2f GiB / %.1f GiB total (%.0f%% peak utilization)\n",
				j.MemUsedGiBPeak, j.MemTotalGiB, pct(j.MemUsedGiBPeak, j.MemTotalGiB)))
			if j.GPU != nil && j.GPU.Count > 0 {
				b.WriteString(fmt.Sprintf("  GPU:     %d GPU(s)  util peak %.1f%%  avg %.1f%%  mem peak %.1f%%\n",
					j.GPU.Count, j.GPU.PeakUtilizationPct, j.GPU.AvgUtilizationPct, j.GPU.PeakMemoryUtilPct))
			}
			b.WriteString(fmt.Sprintf("  Duration: %.0fs  (%.1f min)\n", j.DurationSeconds, j.DurationSeconds/60))
			// Recommendations for this job
			if recs, ok := ctx.Recommendations[j.JobID]; ok && len(recs) > 0 {
				r := recs[0]
				b.WriteString(fmt.Sprintf("  Current cost: $%.2f/mo  →  Recommended: %s ($%.2f/mo, %+.0f%%)\n",
					r.CurrentMonthly, r.Machine.ID, r.EstimatedMonthly, r.CostDeltaPercent))
				if r.CostDeltaPercent < -0.5 {
					b.WriteString(fmt.Sprintf("  Potential savings: $%.2f/mo  ($%.2f/yr)\n",
						r.CurrentMonthly-r.EstimatedMonthly, (r.CurrentMonthly-r.EstimatedMonthly)*12))
				}
				if r.Reasoning != "" {
					b.WriteString(fmt.Sprintf("  Reason: %s\n", r.Reasoning))
				}
			}
			b.WriteString("\n")
		}
	}

	// --- Savings summary ---
	if ctx.Savings != nil {
		b.WriteString("=== SAVINGS SUMMARY ===\n")
		b.WriteString(fmt.Sprintf("  Total current monthly cost: $%.2f\n", ctx.Savings.TotalCurrentCostUSD))
		savingsAmt := ctx.Savings.TotalPotentialSavingsUSD
		if savingsAmt > 0 {
			b.WriteString(fmt.Sprintf("  Potential monthly savings: $%.2f (%.1f%%)\n", savingsAmt, ctx.Savings.SavingsPercent))
			b.WriteString(fmt.Sprintf("  Projected annual savings: $%.2f\n", savingsAmt*12))
		}
		if len(ctx.Savings.TopSavingsOpportunities) > 0 {
			b.WriteString("  Top savings opportunities:\n")
			for _, opp := range ctx.Savings.TopSavingsOpportunities {
				b.WriteString(fmt.Sprintf("    • %s (%s): save $%.2f/mo → switch to %s\n",
					opp.JobID, opp.Repository, opp.SavingsUSD, opp.RecommendedMachine))
			}
		}
		b.WriteString("\n")
	}

	// --- Policies ---
	if len(ctx.Policies) > 0 {
		b.WriteString("=== COST POLICIES ===\n")
		for _, p := range ctx.Policies {
			scope := "global"
			if p.Repository != "" && p.JobID != "" {
				scope = fmt.Sprintf("%s / %s", p.Repository, p.JobID)
			} else if p.Repository != "" {
				scope = p.Repository
			}
			status := "enabled"
			if !p.Enabled {
				status = "disabled"
			}
			b.WriteString(fmt.Sprintf("  • %s: max $%.4f/hr  [%s]\n", scope, p.MaxCostPerHour, status))
		}
		b.WriteString("\n")
	}

	// --- Alerts ---
	if len(ctx.Alerts) > 0 {
		b.WriteString("=== ALERT RULES ===\n")
		for _, a := range ctx.Alerts {
			scope := "global"
			if a.Repository != "" && a.JobID != "" {
				scope = fmt.Sprintf("%s / %s", a.Repository, a.JobID)
			} else if a.Repository != "" {
				scope = a.Repository
			}
			status := "enabled"
			if !a.Enabled {
				status = "disabled"
			}
			b.WriteString(fmt.Sprintf("  • %s (%s): %s > %.0f → %s [%s]\n",
				a.Name, scope, a.ConditionType, a.Threshold, a.Destination, status))
		}
		b.WriteString("\n")
	}

	// --- Destinations ---
	if len(ctx.Destinations) > 0 {
		b.WriteString("=== ALERT DESTINATIONS ===\n")
		for _, d := range ctx.Destinations {
			verified := ""
			if d.Verified {
				verified = " ✓"
			}
			b.WriteString(fmt.Sprintf("  • %s (%s)%s\n", d.Name, d.Type, verified))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// pct computes (value/total)*100, returning 0 when total is 0.
func pct(value, total float64) float64 {
	if total == 0 {
		return 0
	}
	return value / total * 100
}

// Database operations

func (a *Assistant) createConversation(ctx context.Context, id, title, userID string) error {
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO assistant_conversations (id, title, user_id, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
	`, id, title, userID)
	return err
}

func (a *Assistant) storeMessage(ctx context.Context, msg types.ChatMessage) error {
	metaJSON, _ := json.Marshal(msg.Metadata)
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO assistant_messages (id, conversation_id, role, content, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, msg.ID, msg.ConversationID, msg.Role, msg.Content, metaJSON, msg.CreatedAt)
	return err
}

func (a *Assistant) getConversationHistory(ctx context.Context, convID string, limit int) ([]types.ChatMessage, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, conversation_id, role, content, metadata, created_at
		FROM assistant_messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC
		LIMIT $2
	`, convID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []types.ChatMessage
	for rows.Next() {
		var msg types.ChatMessage
		var metaJSON sql.NullString
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &metaJSON, &msg.CreatedAt); err != nil {
			return nil, err
		}
		if metaJSON.Valid {
			json.Unmarshal([]byte(metaJSON.String), &msg.Metadata)
		}
		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

func (a *Assistant) updateConversationTimestamp(ctx context.Context, convID string) {
	a.db.ExecContext(ctx, `UPDATE assistant_conversations SET updated_at = NOW() WHERE id = $1`, convID)
}

// GetConversations returns all conversations for a user, including message counts.
func (a *Assistant) GetConversations(ctx context.Context, userID string) ([]types.Conversation, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT
			c.id, c.title, c.user_id, c.summary, c.created_at, c.updated_at,
			COUNT(m.id) AS message_count
		FROM assistant_conversations c
		LEFT JOIN assistant_messages m ON m.conversation_id = c.id
		WHERE c.user_id = $1
		GROUP BY c.id, c.title, c.user_id, c.summary, c.created_at, c.updated_at
		ORDER BY c.updated_at DESC
		LIMIT 100
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []types.Conversation
	for rows.Next() {
		var c types.Conversation
		var uid, summary sql.NullString
		if err := rows.Scan(&c.ID, &c.Title, &uid, &summary, &c.CreatedAt, &c.UpdatedAt, &c.MessageCount); err != nil {
			return nil, err
		}
		if uid.Valid {
			c.UserID = uid.String
		}
		if summary.Valid {
			c.Summary = summary.String
		}
		convs = append(convs, c)
	}

	return convs, rows.Err()
}

// GetConversation returns a single conversation with its messages.
// Returns an error if the conversation does not belong to userID.
// If the conversation was compacted, conv.Summary contains the digest of older turns.
func (a *Assistant) GetConversation(ctx context.Context, convID, userID string) (*types.Conversation, []types.ChatMessage, error) {
	var conv types.Conversation
	var uid, summary sql.NullString
	err := a.db.QueryRowContext(ctx, `
		SELECT
			c.id, c.title, c.user_id, c.summary, c.created_at, c.updated_at,
			COUNT(m.id) AS message_count
		FROM assistant_conversations c
		LEFT JOIN assistant_messages m ON m.conversation_id = c.id
		WHERE c.id = $1 AND c.user_id = $2
		GROUP BY c.id, c.title, c.user_id, c.summary, c.created_at, c.updated_at
	`, convID, userID).Scan(&conv.ID, &conv.Title, &uid, &summary, &conv.CreatedAt, &conv.UpdatedAt, &conv.MessageCount)
	if err != nil {
		return nil, nil, err
	}
	if uid.Valid {
		conv.UserID = uid.String
	}
	if summary.Valid {
		conv.Summary = summary.String
	}

	messages, err := a.getConversationHistory(ctx, convID, 200)
	if err != nil {
		return nil, nil, err
	}

	return &conv, messages, nil
}

// DeleteConversation removes a conversation and all its messages.
// The messages table has ON DELETE CASCADE so deleting the conversation is enough.
// Only deletes if the conversation belongs to userID.
func (a *Assistant) DeleteConversation(ctx context.Context, convID, userID string) error {
	_, err := a.db.ExecContext(ctx, `DELETE FROM assistant_conversations WHERE id = $1 AND user_id = $2`, convID, userID)
	return err
}

// DeleteAllConversations removes every conversation (and cascades messages) for a user.
func (a *Assistant) DeleteAllConversations(ctx context.Context, userID string) (int64, error) {
	res, err := a.db.ExecContext(ctx,
		`DELETE FROM assistant_conversations WHERE user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// summarizeQuestion creates a short title from the user's first question.
func summarizeQuestion(q string) string {
	q = strings.TrimSpace(q)
	if len(q) > 50 {
		// Find a good break point
		if idx := strings.LastIndex(q[:50], " "); idx > 20 {
			return q[:idx] + "..."
		}
		return q[:47] + "..."
	}
	return q
}

// QuickStats returns quick statistics for the assistant to reference.
type QuickStats struct {
	TotalJobs         int     `json:"total_jobs"`
	TotalRepositories int     `json:"total_repositories"`
	TotalCostLast30d  float64 `json:"total_cost_last_30d"`
	PotentialSavings  float64 `json:"potential_savings"`
	TopSpendingRepo   string  `json:"top_spending_repo,omitempty"`
	TopSpendingJob    string  `json:"top_spending_job,omitempty"`
	AvgCPUUtilization float64 `json:"avg_cpu_utilization"`
	AvgMemUtilization float64 `json:"avg_mem_utilization"`
}

// GetQuickStats returns dashboard-style quick stats.
func (a *Assistant) GetQuickStats(ctx context.Context) (*QuickStats, error) {
	stats := &QuickStats{}

	// Total jobs
	a.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT summary->>'job_id') FROM jobs`).Scan(&stats.TotalJobs)

	// Total repositories
	a.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT summary->>'repository') 
		FROM jobs 
		WHERE summary->>'repository' IS NOT NULL AND summary->>'repository' != ''
	`).Scan(&stats.TotalRepositories)

	// Cost in last 30 days
	a.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(
			(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
			(summary->>'duration_seconds')::numeric / 3600
		), 0)
		FROM jobs
		WHERE (summary->>'start_time')::timestamp > NOW() - INTERVAL '30 days'
	`).Scan(&stats.TotalCostLast30d)

	// Potential savings
	a.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(
			((summary->'detected_machine'->>'on_demand_price_per_hour')::numeric - 
			 COALESCE((recommendations->0->'machine'->>'on_demand_price_per_hour')::numeric, 
			          (summary->'detected_machine'->>'on_demand_price_per_hour')::numeric)) *
			(summary->>'duration_seconds')::numeric / 3600
		), 0)
		FROM jobs
		WHERE recommendations IS NOT NULL AND jsonb_array_length(recommendations) > 0
	`).Scan(&stats.PotentialSavings)

	// Top spending repository
	a.db.QueryRowContext(ctx, `
		SELECT summary->>'repository'
		FROM jobs
		WHERE summary->>'repository' IS NOT NULL AND summary->>'repository' != ''
		GROUP BY summary->>'repository'
		ORDER BY SUM(
			(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
			(summary->>'duration_seconds')::numeric / 3600
		) DESC
		LIMIT 1
	`).Scan(&stats.TopSpendingRepo)

	// Top spending job
	a.db.QueryRowContext(ctx, `
		SELECT summary->>'job_id'
		FROM jobs
		GROUP BY summary->>'job_id'
		ORDER BY SUM(
			(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
			(summary->>'duration_seconds')::numeric / 3600
		) DESC
		LIMIT 1
	`).Scan(&stats.TopSpendingJob)

	// Average utilization
	a.db.QueryRowContext(ctx, `
		SELECT 
			COALESCE(AVG((summary->>'cpu_percent_avg')::numeric), 0),
			COALESCE(AVG(
				CASE WHEN (summary->>'mem_total_gib')::numeric > 0
				THEN (summary->>'mem_used_gib_avg')::numeric / (summary->>'mem_total_gib')::numeric * 100
				ELSE 0 END
			), 0)
		FROM jobs
	`).Scan(&stats.AvgCPUUtilization, &stats.AvgMemUtilization)

	return stats, nil
}

// SuggestedQuestions returns contextual suggested questions.
func (a *Assistant) SuggestedQuestions(ctx context.Context) []string {
	questions := []string{
		"Where are we spending the most money?",
		"Which jobs are over-provisioned?",
		"How much could we save with spot instances?",
		"What's our overall resource utilization?",
	}

	// Add context-specific questions based on data
	stats, err := a.GetQuickStats(ctx)
	if err == nil {
		if stats.TopSpendingRepo != "" {
			questions = append(questions, fmt.Sprintf("What jobs in %s are costing the most?", stats.TopSpendingRepo))
		}
		if stats.AvgCPUUtilization < 50 {
			questions = append(questions, "Why is our CPU utilization so low?")
		}
		if stats.PotentialSavings > 100 {
			questions = append(questions, "What are the top opportunities to reduce costs?")
		}
	}

	// Return top 5 questions
	if len(questions) > 5 {
		questions = questions[:5]
	}

	return questions
}
