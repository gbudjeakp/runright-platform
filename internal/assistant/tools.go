// Package assistant provides AI-powered analysis and Q&A capabilities for RunRight.
// This file implements agentic tool execution - allowing the AI to take actions.
package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Tool represents a function the AI can call.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall represents a tool invocation from the AI.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult represents the result of executing a tool.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Success    bool   `json:"success"`
	Result     string `json:"result"`
	Error      string `json:"error,omitempty"`
}

// ActionLog records actions taken by the AI for audit purposes.
type ActionLog struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	UserID         string    `json:"user_id"`
	ToolName       string    `json:"tool_name"`
	Arguments      string    `json:"arguments"`
	Result         string    `json:"result"`
	Success        bool      `json:"success"`
	CreatedAt      time.Time `json:"created_at"`
}

// AvailableTools returns the list of tools the AI can use.
func (a *Assistant) AvailableTools() []Tool {
	return []Tool{
		{
			Name:        "create_alert_rule",
			Description: "Create a new alert rule to notify teams when thresholds are exceeded. Use this when the user wants to set up alerts for cost, waste, or utilization thresholds.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Human-readable name for the alert rule",
					},
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Repository to scope the alert to (optional, empty for global)",
					},
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "Job ID to scope the alert to (optional)",
					},
					"condition_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"cost_threshold", "waste_threshold", "cpu_threshold", "memory_threshold", "duration_threshold"},
						"description": "Type of condition that triggers the alert",
					},
					"threshold": map[string]interface{}{
						"type":        "number",
						"description": "Numeric threshold value (e.g., 0.50 for $0.50/hr, 30 for 30% waste)",
					},
					"channel": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"slack", "email", "webhook"},
						"description": "Notification channel",
					},
					"destination": map[string]interface{}{
						"type":        "string",
						"description": "Channel-specific destination (Slack channel, email address, or webhook URL)",
					},
				},
				"required": []string{"name", "condition_type", "threshold", "channel", "destination"},
			},
		},
		{
			Name:        "create_policy",
			Description: "Create a cost policy to enforce spending limits. Use this when the user wants to set max cost guardrails for jobs or repositories.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Repository to apply the policy to (empty string for global policy)",
					},
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "Job ID to apply the policy to (empty string for repo-level)",
					},
					"max_cost_per_hour": map[string]interface{}{
						"type":        "number",
						"description": "Maximum allowed cost per hour in USD",
					},
					"enabled": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the policy is enabled",
					},
				},
				"required": []string{"max_cost_per_hour"},
			},
		},
		{
			Name:        "snooze_job",
			Description: "Snooze a job to temporarily hide it from recommendations. Use this when the user wants to defer action on a job.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "The job_id to snooze",
					},
					"duration_days": map[string]interface{}{
						"type":        "integer",
						"description": "Number of days to snooze (default 7)",
					},
					"reason": map[string]interface{}{
						"type":        "string",
						"description": "Reason for snoozing",
					},
				},
				"required": []string{"job_id"},
			},
		},
		{
			Name:        "archive_job",
			Description: "Archive a job to permanently hide it from active views. Use this for deprecated or obsolete jobs.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "The job_id to archive",
					},
					"reason": map[string]interface{}{
						"type":        "string",
						"description": "Reason for archiving",
					},
				},
				"required": []string{"job_id"},
			},
		},
		{
			Name:        "get_job_details",
			Description: "Get detailed metrics and recommendations for a specific job. Use this to gather information before suggesting optimizations.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "The job_id to get details for",
					},
				},
				"required": []string{"job_id"},
			},
		},
		{
			Name:        "list_high_waste_jobs",
			Description: "List jobs with the highest resource waste. Use this to identify optimization opportunities.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Filter by repository (optional)",
					},
					"min_waste_percent": map[string]interface{}{
						"type":        "number",
						"description": "Minimum waste percentage to include (default 30)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results (default 10)",
					},
				},
			},
		},
		{
			Name:        "generate_savings_report",
			Description: "Generate a summary report of potential savings across jobs. Use this when the user asks for an overview of optimization opportunities.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Filter by repository (optional)",
					},
					"time_range_days": map[string]interface{}{
						"type":        "integer",
						"description": "Number of days to analyze (default 30)",
					},
				},
			},
		},
	}
}

// ExecuteTool runs a tool and returns the result.
func (a *Assistant) ExecuteTool(ctx context.Context, call ToolCall, userID, conversationID string) (*ToolResult, error) {
	result := &ToolResult{
		ToolCallID: call.ID,
	}

	// Log the action attempt
	actionLog := ActionLog{
		ID:             uuid.New().String(),
		ConversationID: conversationID,
		UserID:         userID,
		ToolName:       call.Name,
		Arguments:      string(call.Arguments),
		CreatedAt:      time.Now(),
	}

	var err error
	switch call.Name {
	case "create_alert_rule":
		result.Result, err = a.executeCreateAlertRule(ctx, call.Arguments)
	case "create_policy":
		result.Result, err = a.executeCreatePolicy(ctx, call.Arguments)
	case "snooze_job":
		result.Result, err = a.executeSnoozeJob(ctx, call.Arguments)
	case "archive_job":
		result.Result, err = a.executeArchiveJob(ctx, call.Arguments)
	case "get_job_details":
		result.Result, err = a.executeGetJobDetails(ctx, call.Arguments)
	case "list_high_waste_jobs":
		result.Result, err = a.executeListHighWasteJobs(ctx, call.Arguments)
	case "generate_savings_report":
		result.Result, err = a.executeGenerateSavingsReport(ctx, call.Arguments)
	default:
		err = fmt.Errorf("unknown tool: %s", call.Name)
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		actionLog.Success = false
		actionLog.Result = err.Error()
	} else {
		result.Success = true
		actionLog.Success = true
		actionLog.Result = result.Result
	}

	// Store the action log
	if a.db != nil {
		_, _ = a.db.ExecContext(ctx, `
			INSERT INTO assistant_action_log (id, conversation_id, user_id, tool_name, arguments, result, success, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, actionLog.ID, actionLog.ConversationID, actionLog.UserID, actionLog.ToolName,
			actionLog.Arguments, actionLog.Result, actionLog.Success, actionLog.CreatedAt)
	}

	return result, nil
}

// Tool execution implementations

func (a *Assistant) executeCreateAlertRule(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Name          string  `json:"name"`
		Repository    string  `json:"repository"`
		JobID         string  `json:"job_id"`
		ConditionType string  `json:"condition_type"`
		Threshold     float64 `json:"threshold"`
		Channel       string  `json:"channel"`
		Destination   string  `json:"destination"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	id := uuid.New().String()
	
	// Insert the alert rule
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO alert_rules (id, name, repository, job_id, condition_type, threshold_value, channel, destination, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, NOW(), NOW())
	`, id, params.Name, params.Repository, params.JobID, params.ConditionType, params.Threshold, params.Channel, params.Destination)
	if err != nil {
		return "", fmt.Errorf("failed to create alert rule: %w", err)
	}

	// Build scope description
	scope := "global"
	if params.Repository != "" && params.JobID != "" {
		scope = fmt.Sprintf("job `%s` in `%s`", params.JobID, params.Repository)
	} else if params.Repository != "" {
		scope = fmt.Sprintf("repository `%s`", params.Repository)
	}

	return fmt.Sprintf(`✅ **Created alert rule "%s"**

| Setting | Value |
|---------|-------|
| Scope | %s |
| Condition | %s > %.0f%% |
| Notify via | %s → %s |

🔗 [View in Alerts Dashboard](/app/alerts)`, 
		params.Name, scope, params.ConditionType, params.Threshold, params.Channel, params.Destination), nil
}

func (a *Assistant) executeCreatePolicy(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Repository     string  `json:"repository"`
		JobID          string  `json:"job_id"`
		MaxCostPerHour float64 `json:"max_cost_per_hour"`
		Enabled        *bool   `json:"enabled"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	enabled := true
	if params.Enabled != nil {
		enabled = *params.Enabled
	}

	// Upsert the policy
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO policy_rules (repository, job_id, max_cost_per_hour, enabled, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (repository, job_id) DO UPDATE
		SET max_cost_per_hour = EXCLUDED.max_cost_per_hour, enabled = EXCLUDED.enabled, updated_at = NOW()
	`, params.Repository, params.JobID, params.MaxCostPerHour, enabled)
	if err != nil {
		return "", fmt.Errorf("failed to create policy: %w", err)
	}

	scope := "global"
	if params.Repository != "" && params.JobID != "" {
		scope = fmt.Sprintf("job `%s` in `%s`", params.JobID, params.Repository)
	} else if params.Repository != "" {
		scope = fmt.Sprintf("repository `%s`", params.Repository)
	}

	return fmt.Sprintf(`✅ **Created cost policy**

| Setting | Value |
|---------|-------|
| Scope | %s |
| Max Cost | $%.2f/hr |
| Status | Enabled |

CI jobs exceeding this limit will be flagged.

🔗 [View in Policies Dashboard](/app/policies)`, scope, params.MaxCostPerHour), nil
}

func (a *Assistant) executeSnoozeJob(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		JobID        string `json:"job_id"`
		DurationDays int    `json:"duration_days"`
		Reason       string `json:"reason"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if params.DurationDays == 0 {
		params.DurationDays = 7
	}

	snoozeUntil := time.Now().AddDate(0, 0, params.DurationDays)

	_, err := a.db.ExecContext(ctx, `
		UPDATE job_summaries 
		SET snoozed_until = $1, snooze_reason = $2
		WHERE job_id = $3
	`, snoozeUntil, params.Reason, params.JobID)
	if err != nil {
		return "", fmt.Errorf("failed to snooze job: %w", err)
	}

	return fmt.Sprintf("✅ Snoozed job '%s' until %s (%d days). Reason: %s",
		params.JobID, snoozeUntil.Format("Jan 2, 2006"), params.DurationDays, params.Reason), nil
}

func (a *Assistant) executeArchiveJob(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		JobID  string `json:"job_id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	_, err := a.db.ExecContext(ctx, `
		UPDATE job_summaries SET archived = true WHERE job_id = $1
	`, params.JobID)
	if err != nil {
		return "", fmt.Errorf("failed to archive job: %w", err)
	}

	return fmt.Sprintf("✅ Archived job '%s'. It will no longer appear in active job lists.", params.JobID), nil
}

func (a *Assistant) executeGetJobDetails(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	var result struct {
		JobID           string  `db:"job_id"`
		Repository      string  `db:"repository"`
		RunCount        int     `db:"run_count"`
		AvgDuration     float64 `db:"avg_duration"`
		CPUP95          float64 `db:"cpu_p95"`
		MemP95          float64 `db:"mem_p95"`
		MonthlySavings  float64 `db:"monthly_savings"`
		WastePercent    float64 `db:"waste_percent"`
		DetectedMachine string  `db:"detected_machine"`
		TopRecommend    string  `db:"top_recommend"`
	}

	err := a.db.QueryRowContext(ctx, `
		SELECT 
			j.job_id, j.repository, j.run_count, 
			j.avg_duration_seconds as avg_duration,
			j.cpu_percent_p95 as cpu_p95, j.mem_used_gib_p95 as mem_p95,
			COALESCE(j.monthly_savings_usd, 0) as monthly_savings,
			COALESCE(j.waste_percent, 0) as waste_percent,
			COALESCE(j.detected_machine, '') as detected_machine,
			COALESCE(j.top_recommend, '') as top_recommend
		FROM job_summaries j
		WHERE j.job_id = $1
	`, params.JobID).Scan(
		&result.JobID, &result.Repository, &result.RunCount,
		&result.AvgDuration, &result.CPUP95, &result.MemP95,
		&result.MonthlySavings, &result.WastePercent,
		&result.DetectedMachine, &result.TopRecommend,
	)
	if err != nil {
		return "", fmt.Errorf("job not found: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Job: %s**\n", result.JobID))
	sb.WriteString(fmt.Sprintf("Repository: %s\n", result.Repository))
	sb.WriteString(fmt.Sprintf("Run count: %d\n", result.RunCount))
	sb.WriteString(fmt.Sprintf("Avg duration: %.0f seconds\n", result.AvgDuration))
	sb.WriteString(fmt.Sprintf("CPU p95: %.1f%%\n", result.CPUP95))
	sb.WriteString(fmt.Sprintf("Memory p95: %.2f GiB\n", result.MemP95))
	sb.WriteString(fmt.Sprintf("Current machine: %s\n", result.DetectedMachine))
	sb.WriteString(fmt.Sprintf("Recommended: %s\n", result.TopRecommend))
	sb.WriteString(fmt.Sprintf("Waste: %.1f%%\n", result.WastePercent))
	sb.WriteString(fmt.Sprintf("Monthly savings potential: $%.2f\n", result.MonthlySavings))

	return sb.String(), nil
}

func (a *Assistant) executeListHighWasteJobs(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Repository      string  `json:"repository"`
		MinWastePercent float64 `json:"min_waste_percent"`
		Limit           int     `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if params.MinWastePercent == 0 {
		params.MinWastePercent = 30
	}
	if params.Limit == 0 {
		params.Limit = 10
	}

	query := `
		SELECT job_id, repository, waste_percent, monthly_savings_usd
		FROM job_summaries
		WHERE waste_percent >= $1 AND archived = false AND snoozed_until IS NULL
	`
	queryArgs := []interface{}{params.MinWastePercent}

	if params.Repository != "" {
		query += " AND repository = $2"
		queryArgs = append(queryArgs, params.Repository)
	}
	query += " ORDER BY monthly_savings_usd DESC LIMIT $" + fmt.Sprintf("%d", len(queryArgs)+1)
	queryArgs = append(queryArgs, params.Limit)

	rows, err := a.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return "", fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**High-Waste Jobs (>%.0f%% waste)**\n\n", params.MinWastePercent))

	count := 0
	for rows.Next() {
		var jobID, repo string
		var waste, savings float64
		if err := rows.Scan(&jobID, &repo, &waste, &savings); err != nil {
			continue
		}
		count++
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n", count, jobID, repo))
		sb.WriteString(fmt.Sprintf("   Waste: %.1f%% | Potential savings: $%.2f/mo\n\n", waste, savings))
	}

	if count == 0 {
		return "No high-waste jobs found matching the criteria.", nil
	}

	return sb.String(), nil
}

func (a *Assistant) executeGenerateSavingsReport(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Repository    string `json:"repository"`
		TimeRangeDays int    `json:"time_range_days"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if params.TimeRangeDays == 0 {
		params.TimeRangeDays = 30
	}

	// Aggregate savings data
	query := `
		SELECT 
			COUNT(*) as total_jobs,
			COUNT(*) FILTER (WHERE monthly_savings_usd > 0) as jobs_with_savings,
			COALESCE(SUM(monthly_savings_usd), 0) as total_monthly_savings,
			COALESCE(AVG(waste_percent), 0) as avg_waste
		FROM job_summaries
		WHERE archived = false
	`
	queryArgs := []interface{}{}

	if params.Repository != "" {
		query += " AND repository = $1"
		queryArgs = append(queryArgs, params.Repository)
	}

	var totalJobs, jobsWithSavings int
	var totalMonthlySavings, avgWaste float64

	err := a.db.QueryRowContext(ctx, query, queryArgs...).Scan(
		&totalJobs, &jobsWithSavings, &totalMonthlySavings, &avgWaste,
	)
	if err != nil {
		return "", fmt.Errorf("query failed: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("## 📊 Savings Report\n\n")
	if params.Repository != "" {
		sb.WriteString(fmt.Sprintf("**Repository:** %s\n", params.Repository))
	}
	sb.WriteString(fmt.Sprintf("**Time Range:** Last %d days\n\n", params.TimeRangeDays))
	sb.WriteString("### Summary\n")
	sb.WriteString(fmt.Sprintf("- **Total Jobs Analyzed:** %d\n", totalJobs))
	sb.WriteString(fmt.Sprintf("- **Jobs with Savings Potential:** %d (%.0f%%)\n", jobsWithSavings, float64(jobsWithSavings)/float64(totalJobs)*100))
	sb.WriteString(fmt.Sprintf("- **Estimated Monthly Savings:** $%.2f\n", totalMonthlySavings))
	sb.WriteString(fmt.Sprintf("- **Projected Annual Savings:** $%.2f\n", totalMonthlySavings*12))
	sb.WriteString(fmt.Sprintf("- **Average Waste:** %.1f%%\n", avgWaste))

	return sb.String(), nil
}

// ToolsSystemPromptAddition returns the addition to the system prompt that describes tool capabilities.
func (a *Assistant) ToolsSystemPromptAddition() string {
	return `

## Agentic Capabilities

You can take actions on behalf of the user. When the user asks you to do something (not just answer a question), use the appropriate tool. Always confirm what action you're taking.

Available actions:
- **Create alerts**: Set up notifications for cost thresholds, waste levels, or performance issues
- **Create policies**: Set spending guardrails at global, repository, or job level  
- **Snooze/archive jobs**: Temporarily hide or permanently archive jobs from recommendations
- **Generate reports**: Create savings reports and identify high-waste jobs

Before taking an action, briefly explain what you're about to do. After completing an action, confirm the result.

Example interaction:
User: "Set up an alert when job build-test costs more than $0.50/hr"
You: "I'll create a cost threshold alert for the build-test job. [calls create_alert_rule tool]"
Result: "✅ Created alert rule..."
You: "Done! I've set up an alert that will notify you when build-test exceeds $0.50/hr."
`
}
