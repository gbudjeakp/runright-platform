// Package assistant provides AI-powered analysis and Q&A capabilities for RunRight.
// This file implements agentic tool execution - allowing the AI to take actions.
package assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
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
						"enum":        []string{"cost_threshold", "waste_threshold", "cpu_threshold", "memory_threshold", "gpu_threshold", "duration_threshold"},
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
				"required": []string{"name", "condition_type", "threshold"},
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
		{
			Name:        "list_alerts",
			Description: "List all alert rules. Use this when the user wants to see existing alerts.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Filter by repository (optional)",
					},
				},
			},
		},
		{
			Name:        "delete_alert",
			Description: "Delete an alert rule by ID or name. Use this when the user wants to remove an alert.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"alert_id": map[string]interface{}{
						"type":        "string",
						"description": "The alert ID to delete",
					},
					"alert_name": map[string]interface{}{
						"type":        "string",
						"description": "The alert name to delete (used if ID not provided)",
					},
				},
			},
		},
		{
			Name:        "toggle_alert",
			Description: "Enable or disable an alert rule. Use this when the user wants to turn an alert on/off.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"alert_id": map[string]interface{}{
						"type":        "string",
						"description": "The alert ID to toggle",
					},
					"alert_name": map[string]interface{}{
						"type":        "string",
						"description": "The alert name to toggle (used if ID not provided)",
					},
					"enabled": map[string]interface{}{
						"type":        "boolean",
						"description": "Set to true to enable, false to disable",
					},
				},
				"required": []string{"enabled"},
			},
		},
		{
			Name:        "update_policy",
			Description: "Update an existing cost policy. Use this when the user wants to change a policy's settings.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Repository the policy applies to",
					},
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "Job ID the policy applies to (optional)",
					},
					"max_cost_per_hour": map[string]interface{}{
						"type":        "number",
						"description": "New maximum allowed cost per hour in USD",
					},
					"enabled": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the policy should be enabled",
					},
				},
				"required": []string{"repository"},
			},
		},
		{
			Name:        "delete_policy",
			Description: "Delete a cost policy. Use this when the user wants to remove a policy.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Repository the policy applies to",
					},
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "Job ID the policy applies to (optional, empty for repo-level)",
					},
				},
				"required": []string{"repository"},
			},
		},
		{
			Name:        "analyze_page",
			Description: "Analyze what the user is currently viewing on the page and provide insights. Use this when the user asks about 'this', 'what I'm looking at', or wants analysis of the current view.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{},
			},
		},
		// ─── Role & User Management ─────────────────────────────────────────
		{
			Name:        "create_role",
			Description: "Create a new role with specified permissions. Use this when the user wants to set up custom access levels.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Name for the role (e.g., 'ml-team-lead', 'billing-admin')",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Human-readable description of what this role is for",
					},
					"permissions": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "List of permissions: read, write, delete, admin, manage_alerts, manage_policies, manage_users, manage_billing, view_costs",
					},
				},
				"required": []string{"name", "permissions"},
			},
		},
		{
			Name:        "list_roles",
			Description: "List all roles in the system. Use this to see available roles and their permissions.",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "delete_role",
			Description: "Delete a custom role. System roles cannot be deleted.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"role_id": map[string]interface{}{
						"type":        "string",
						"description": "The role ID to delete",
					},
					"role_name": map[string]interface{}{
						"type":        "string",
						"description": "The role name to delete (used if ID not provided)",
					},
				},
			},
		},
		{
			Name:        "list_users",
			Description: "List all users in the system with their roles.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"role": map[string]interface{}{
						"type":        "string",
						"description": "Filter by role (optional)",
					},
				},
			},
		},
		{
			Name:        "update_user_role",
			Description: "Change a user's role. Use this to promote/demote users or change their access level.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"user_email": map[string]interface{}{
						"type":        "string",
						"description": "Email of the user to update",
					},
					"new_role": map[string]interface{}{
						"type":        "string",
						"description": "New role to assign (e.g., 'admin', 'developer', 'viewer')",
					},
				},
				"required": []string{"user_email", "new_role"},
			},
		},
		// ─── API Key Management ─────────────────────────────────────────────
		{
			Name:        "create_api_key",
			Description: "Create a new API key for programmatic access. Use this when setting up integrations or CI/CD pipelines.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Human-readable name for the API key (e.g., 'GitHub Actions', 'Jenkins CI')",
					},
					"scopes": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Permission scopes: read, write, admin",
					},
					"expires_days": map[string]interface{}{
						"type":        "integer",
						"description": "Number of days until expiration (0 for no expiration)",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "list_api_keys",
			Description: "List all API keys. Shows name, scopes, and status but not the key values.",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "revoke_api_key",
			Description: "Revoke an API key to disable it immediately.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key_id": map[string]interface{}{
						"type":        "string",
						"description": "The API key ID to revoke",
					},
					"key_name": map[string]interface{}{
						"type":        "string",
						"description": "The API key name to revoke (used if ID not provided)",
					},
				},
			},
		},
		// ─── Repository Ownership ───────────────────────────────────────────
		{
			Name:        "set_repository_ownership",
			Description: "Assign a team as owner of a repository. Owners receive alerts for that repo.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Repository name (e.g., 'org/repo-name')",
					},
					"team_name": map[string]interface{}{
						"type":        "string",
						"description": "Team name to assign as owner",
					},
					"notification_destinations": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Notification destination IDs for this team's alerts",
					},
				},
				"required": []string{"repository", "team_name"},
			},
		},
		{
			Name:        "list_repository_ownership",
			Description: "List repository ownership assignments.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Filter by repository (optional)",
					},
				},
			},
		},
		{
			Name:        "remove_repository_ownership",
			Description: "Remove a team's ownership of a repository.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Repository name",
					},
					"team_name": map[string]interface{}{
						"type":        "string",
						"description": "Team name to remove",
					},
				},
				"required": []string{"repository", "team_name"},
			},
		},
		// Label Mapping Tools
		{
			Name:        "create_label",
			Description: "Create a runner label mapping to define the specs and cost of a CI runner label (e.g., ubuntu-latest-16-cores). Use this when the user wants to add or update a label's specifications.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"label": map[string]interface{}{
						"type":        "string",
						"description": "Runner label name (e.g., 'ubuntu-latest-16-cores', 'self-hosted-gpu')",
					},
					"provider": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"github", "gitlab", "azure", "bitbucket", "circleci"},
						"description": "CI provider (default: github)",
					},
					"instance_type": map[string]interface{}{
						"type":        "string",
						"description": "Cloud instance type (e.g., 'Standard_D16as_v5', 'm5.4xlarge')",
					},
					"vcpus": map[string]interface{}{
						"type":        "integer",
						"description": "Number of vCPUs",
					},
					"memory_gib": map[string]interface{}{
						"type":        "number",
						"description": "Memory in GiB",
					},
					"cost_per_hour": map[string]interface{}{
						"type":        "number",
						"description": "Cost per hour in USD",
					},
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Repository to scope this label to (default: '*' for global)",
					},
					"is_gpu": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether this is a GPU runner",
					},
					"gpu_type": map[string]interface{}{
						"type":        "string",
						"description": "GPU type (e.g., 'nvidia-a100', 'nvidia-t4')",
					},
					"gpu_count": map[string]interface{}{
						"type":        "integer",
						"description": "Number of GPUs",
					},
				},
				"required": []string{"label", "vcpus", "memory_gib", "cost_per_hour"},
			},
		},
		{
			Name:        "list_labels",
			Description: "List all runner label mappings.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"gpu_only": map[string]interface{}{
						"type":        "boolean",
						"description": "Filter to show only GPU labels",
					},
				},
			},
		},
		{
			Name:        "delete_label",
			Description: "Delete a runner label mapping.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"label": map[string]interface{}{
						"type":        "string",
						"description": "Label name to delete",
					},
				},
				"required": []string{"label"},
			},
		},
		// === Cost Analysis Tools ===
		{
			Name:        "get_repository_costs",
			Description: "Get cost breakdown by repository to find the most expensive repos. Use this when the user asks about 'most expensive', 'highest cost', or 'top spending' repositories.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Number of top repos to return (default 10)",
					},
				},
			},
		},
		{
			Name:        "get_recommendations",
			Description: "Get RunRight's specific machine recommendations for a repository. ALWAYS use this when the user asks 'how to save', 'how to reduce costs', 'what can we do', or 'what are the recommendations'. Returns specific machine switches with savings amounts.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repository": map[string]interface{}{
						"type":        "string",
						"description": "Repository to get recommendations for (e.g., 'runrightio/ml-platform')",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Number of recommendations to return (default 10)",
					},
				},
			},
		},
		// === Audit Log Tools ===
		{
			Name:        "search_audit_logs",
			Description: "Search audit logs to find who did what action. Use this to answer questions like 'who deleted alert X', 'who created policy Y', or 'what did user Z do today'.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"description": "Action type to filter by (e.g., 'alert.delete', 'policy.create', 'apikey.create', 'role.update'). Partial match supported.",
					},
					"actor": map[string]interface{}{
						"type":        "string",
						"description": "Email of the user who performed the action",
					},
					"resource_type": map[string]interface{}{
						"type":        "string",
						"description": "Type of resource (e.g., 'alert', 'policy', 'apikey', 'role', 'job')",
					},
					"resource_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the resource to search for",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results to return (default: 20)",
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
	case "create_alert_rule", "create_alert":
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
	case "list_alerts":
		result.Result, err = a.executeListAlerts(ctx, call.Arguments)
	case "delete_alert":
		result.Result, err = a.executeDeleteAlert(ctx, call.Arguments)
	case "toggle_alert":
		result.Result, err = a.executeToggleAlert(ctx, call.Arguments)
	case "update_policy":
		result.Result, err = a.executeUpdatePolicy(ctx, call.Arguments)
	case "delete_policy":
		result.Result, err = a.executeDeletePolicy(ctx, call.Arguments)
	case "analyze_page":
		result.Result, err = a.executeAnalyzePage(ctx, call.Arguments)
	// Role & User Management
	case "create_role":
		result.Result, err = a.executeCreateRole(ctx, call.Arguments)
	case "list_roles":
		result.Result, err = a.executeListRoles(ctx, call.Arguments)
	case "delete_role":
		result.Result, err = a.executeDeleteRole(ctx, call.Arguments)
	case "list_users":
		result.Result, err = a.executeListUsers(ctx, call.Arguments)
	case "update_user_role":
		result.Result, err = a.executeUpdateUserRole(ctx, call.Arguments)
	// API Key Management
	case "create_api_key":
		result.Result, err = a.executeCreateAPIKey(ctx, call.Arguments, userID)
	case "list_api_keys":
		result.Result, err = a.executeListAPIKeys(ctx, call.Arguments)
	case "revoke_api_key":
		result.Result, err = a.executeRevokeAPIKey(ctx, call.Arguments)
	// Repository Ownership
	case "set_repository_ownership":
		result.Result, err = a.executeSetRepositoryOwnership(ctx, call.Arguments)
	case "list_repository_ownership":
		result.Result, err = a.executeListRepositoryOwnership(ctx, call.Arguments)
	case "remove_repository_ownership":
		result.Result, err = a.executeRemoveRepositoryOwnership(ctx, call.Arguments)
	// Label Mapping
	case "create_label":
		result.Result, err = a.executeCreateLabel(ctx, call.Arguments)
	case "list_labels":
		result.Result, err = a.executeListLabels(ctx, call.Arguments)
	case "delete_label":
		result.Result, err = a.executeDeleteLabel(ctx, call.Arguments)
	// Cost Analysis
	case "get_repository_costs":
		result.Result, err = a.executeGetRepositoryCosts(ctx, call.Arguments)
	case "get_recommendations":
		result.Result, err = a.executeGetRecommendations(ctx, call.Arguments)
	// Audit Logs
	case "search_audit_logs":
		result.Result, err = a.executeSearchAuditLogs(ctx, call.Arguments)
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
	// Parse into map first to handle different field name variations
	var rawParams map[string]interface{}
	if err := json.Unmarshal(args, &rawParams); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	
	// Helper to get string from map with fallback keys
	getString := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := rawParams[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	
	// Helper to get float from map with fallback keys
	getFloat := func(keys ...string) float64 {
		for _, k := range keys {
			if v, ok := rawParams[k].(float64); ok {
				return v
			}
			// Also handle string numbers
			if v, ok := rawParams[k].(string); ok {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					return f
				}
			}
		}
		return 0
	}
	
	// Extract params with fallback field names
	params := struct {
		Name          string
		Repository    string
		JobID         string
		ConditionType string
		Threshold     float64
		Channel       string
		Destination   string
	}{
		Name:          getString("name", "alert_name", "alertName"),
		Repository:    getString("repository", "repo"),
		JobID:         getString("job_id", "jobId", "job"),
		ConditionType: getString("condition_type", "conditionType", "type"),
		Threshold:     getFloat("threshold", "max_cost_per_hour", "min_waste_percent", "threshold_value"),
		Channel:       getString("channel", "notification_channel"),
		Destination:   getString("destination", "notification_destinations", "dest"),
	}

	// Infer condition_type from which parameters were used
	if params.ConditionType == "" {
		// Check alert_type, condition, or trigger_type field
		alertType := ""
		for _, key := range []string{"alert_type", "condition", "trigger_type", "type"} {
			if at, ok := rawParams[key].(string); ok && at != "" {
				alertType = at
				break
			}
		}
		if alertType != "" {
			alertTypeLower := strings.ToLower(alertType)
			if strings.Contains(alertTypeLower, "cost") {
				params.ConditionType = "cost"
			} else if strings.Contains(alertTypeLower, "waste") {
				params.ConditionType = "waste"
			} else if strings.Contains(alertTypeLower, "cpu") {
				params.ConditionType = "cpu"
			} else if strings.Contains(alertTypeLower, "memory") {
				params.ConditionType = "memory"
			}
		}
		// Also check threshold field names
		if params.ConditionType == "" {
			if rawParams["max_cost_per_hour"] != nil {
				params.ConditionType = "cost"
			} else if rawParams["min_waste_percent"] != nil {
				params.ConditionType = "waste"
			}
		}
	}

	// Normalize condition_type to match DB constraint
	conditionMap := map[string]string{
		"waste":              "waste_threshold",
		"waste_threshold":    "waste_threshold",
		"high_waste":         "waste_threshold",
		"cost":               "cost_threshold",
		"cost_threshold":     "cost_threshold",
		"high_cost":          "cost_threshold",
		"cpu":                "cpu_threshold",
		"cpu_threshold":      "cpu_threshold",
		"high_cpu":           "cpu_threshold",
		"memory":             "memory_threshold",
		"memory_threshold":   "memory_threshold",
		"high_memory":        "memory_threshold",
		"gpu":                "gpu_threshold",
		"gpu_threshold":      "gpu_threshold",
		"high_gpu":           "gpu_threshold",
		"duration":           "duration_threshold",
		"duration_threshold": "duration_threshold",
		"long_duration":      "duration_threshold",
	}
	if normalized, ok := conditionMap[strings.ToLower(params.ConditionType)]; ok {
		params.ConditionType = normalized
	} else if params.ConditionType == "" || strings.Contains(strings.ToLower(params.ConditionType), "waste") {
		params.ConditionType = "waste_threshold" // default to waste
	} else if strings.Contains(strings.ToLower(params.ConditionType), "cost") {
		params.ConditionType = "cost_threshold"
	} else {
		params.ConditionType = "waste_threshold" // fallback default
	}

	// Validate required fields
	if params.Name == "" {
		return "", fmt.Errorf("name is required - please specify a name for the alert")
	}

	// Check if alert with same name already exists
	var existingID string
	err := a.db.QueryRowContext(ctx, "SELECT id FROM alert_rules WHERE name = $1", params.Name).Scan(&existingID)
	if err == nil {
		return fmt.Sprintf("Alert '%s' already exists", params.Name), nil
	} else if err != sql.ErrNoRows {
		return "", fmt.Errorf("failed to check existing alert: %w", err)
	}

	// Apply sensible defaults
	if params.Channel == "" {
		params.Channel = "slack"
	}
	if params.Destination == "" {
		params.Destination = "#ci-costs"
	}

	id := uuid.New().String()
	
	// Insert the alert rule
	_, err = a.db.ExecContext(ctx, `
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

	// Format condition nicely
	conditionDisplay := strings.TrimSuffix(params.ConditionType, "_threshold")

	return fmt.Sprintf(`✅ **Created alert rule "%s"**

| Setting | Value |
|---------|-------|
| Scope | %s |
| Condition | %s > %.0f%% |
| Notify via | %s → %s |

🔗 [View in Alerts Dashboard](/app/alerts)`, 
		params.Name, scope, conditionDisplay, params.Threshold, params.Channel, params.Destination), nil
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

func (a *Assistant) executeListAlerts(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Repository string `json:"repository"`
	}
	_ = json.Unmarshal(args, &params)

	query := `
		SELECT id, name, COALESCE(repository, 'global'), COALESCE(job_id, ''),
		       condition_type, threshold_value, channel, destination, enabled
		FROM alert_rules
		ORDER BY created_at DESC
	`
	rows, err := a.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to list alerts: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("## 🔔 Alert Rules\n\n")
	sb.WriteString("| Name | Scope | Condition | Destination | Status |\n")
	sb.WriteString("|------|-------|-----------|-------------|--------|\n")

	count := 0
	for rows.Next() {
		var id, name, repo, jobID, condType, channel, dest string
		var threshold float64
		var enabled bool
		if err := rows.Scan(&id, &name, &repo, &jobID, &condType, &threshold, &channel, &dest, &enabled); err != nil {
			continue
		}
		if params.Repository != "" && repo != params.Repository {
			continue
		}
		scope := repo
		if jobID != "" {
			scope = fmt.Sprintf("%s/%s", repo, jobID)
		}
		status := "✅ Enabled"
		if !enabled {
			status = "⏸️ Disabled"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s > %.0f | %s → %s | %s |\n",
			name, scope, condType, threshold, channel, dest, status))
		count++
	}

	if count == 0 {
		return "No alert rules found.", nil
	}

	sb.WriteString(fmt.Sprintf("\n%d alert(s) total\n\n🔗 [Manage Alerts](/app/alerts)", count))
	return sb.String(), nil
}

func (a *Assistant) executeDeleteAlert(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		AlertID   string `json:"alert_id"`
		AlertName string `json:"alert_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	var result sql.Result
	var err error
	var identifier string

	if params.AlertID != "" {
		result, err = a.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = $1`, params.AlertID)
		identifier = params.AlertID
	} else if params.AlertName != "" {
		result, err = a.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE name = $1`, params.AlertName)
		identifier = params.AlertName
	} else {
		return "", fmt.Errorf("must provide alert_id or alert_name")
	}

	if err != nil {
		return "", fmt.Errorf("failed to delete alert: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Sprintf("⚠️ No alert found with identifier: %s", identifier), nil
	}

	return fmt.Sprintf("✅ **Deleted alert** `%s`\n\n🔗 [View Alerts](/app/alerts)", identifier), nil
}

func (a *Assistant) executeToggleAlert(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		AlertID   string `json:"alert_id"`
		AlertName string `json:"alert_name"`
		Enabled   bool   `json:"enabled"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	var result sql.Result
	var err error
	var identifier string

	if params.AlertID != "" {
		result, err = a.db.ExecContext(ctx, `UPDATE alert_rules SET enabled = $1, updated_at = NOW() WHERE id = $2`,
			params.Enabled, params.AlertID)
		identifier = params.AlertID
	} else if params.AlertName != "" {
		result, err = a.db.ExecContext(ctx, `UPDATE alert_rules SET enabled = $1, updated_at = NOW() WHERE name = $2`,
			params.Enabled, params.AlertName)
		identifier = params.AlertName
	} else {
		return "", fmt.Errorf("must provide alert_id or alert_name")
	}

	if err != nil {
		return "", fmt.Errorf("failed to toggle alert: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Sprintf("⚠️ No alert found with identifier: %s", identifier), nil
	}

	status := "enabled ✅"
	if !params.Enabled {
		status = "disabled ⏸️"
	}
	return fmt.Sprintf("✅ **Alert `%s` is now %s**\n\n🔗 [View Alerts](/app/alerts)", identifier, status), nil
}

func (a *Assistant) executeUpdatePolicy(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Repository     string   `json:"repository"`
		JobID          string   `json:"job_id"`
		MaxCostPerHour *float64 `json:"max_cost_per_hour"`
		Enabled        *bool    `json:"enabled"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// Build dynamic update
	updates := []string{}
	updateArgs := []interface{}{}
	argNum := 1

	if params.MaxCostPerHour != nil {
		updates = append(updates, fmt.Sprintf("max_cost_per_hour = $%d", argNum))
		updateArgs = append(updateArgs, *params.MaxCostPerHour)
		argNum++
	}
	if params.Enabled != nil {
		updates = append(updates, fmt.Sprintf("enabled = $%d", argNum))
		updateArgs = append(updateArgs, *params.Enabled)
		argNum++
	}

	if len(updates) == 0 {
		return "", fmt.Errorf("no updates specified")
	}

	updates = append(updates, "updated_at = NOW()")
	updateArgs = append(updateArgs, params.Repository, params.JobID)

	query := fmt.Sprintf(`UPDATE policy_rules SET %s WHERE repository = $%d AND COALESCE(job_id, '') = $%d`,
		strings.Join(updates, ", "), argNum, argNum+1)

	result, err := a.db.ExecContext(ctx, query, updateArgs...)
	if err != nil {
		return "", fmt.Errorf("failed to update policy: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Sprintf("⚠️ No policy found for repository: %s", params.Repository), nil
	}

	scope := params.Repository
	if params.JobID != "" {
		scope = fmt.Sprintf("%s/%s", params.Repository, params.JobID)
	}
	return fmt.Sprintf("✅ **Updated policy for `%s`**\n\n🔗 [View Policies](/app/policies)", scope), nil
}

func (a *Assistant) executeDeletePolicy(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Repository string `json:"repository"`
		JobID      string `json:"job_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	result, err := a.db.ExecContext(ctx, `DELETE FROM policy_rules WHERE repository = $1 AND COALESCE(job_id, '') = $2`,
		params.Repository, params.JobID)
	if err != nil {
		return "", fmt.Errorf("failed to delete policy: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Sprintf("⚠️ No policy found for repository: %s", params.Repository), nil
	}

	scope := params.Repository
	if params.JobID != "" {
		scope = fmt.Sprintf("%s/%s", params.Repository, params.JobID)
	}
	return fmt.Sprintf("✅ **Deleted policy for `%s`**\n\n🔗 [View Policies](/app/policies)", scope), nil
}

func (a *Assistant) executeAnalyzePage(_ context.Context, _ json.RawMessage) (string, error) {
	// This is a special tool - it doesn't actually do anything,
	// it's just a signal that the assistant should analyze the page context
	// that was passed in the request.
	return "I'll analyze what you're currently viewing based on the page context provided.", nil
}

// ─── Role & User Management ─────────────────────────────────────────────────

func (a *Assistant) executeCreateRole(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// Validate permissions
	validPerms := map[string]bool{
		"read": true, "write": true, "delete": true, "admin": true,
		"manage_alerts": true, "manage_policies": true, "manage_users": true,
		"manage_billing": true, "view_costs": true,
	}
	for _, p := range params.Permissions {
		if !validPerms[p] {
			return "", fmt.Errorf("invalid permission: %s", p)
		}
	}

	permsJSON, _ := json.Marshal(params.Permissions)
	id := uuid.New().String()

	_, err := a.db.ExecContext(ctx, `
		INSERT INTO roles (id, name, description, permissions, is_system, created_at, updated_at)
		VALUES ($1, $2, $3, $4, false, NOW(), NOW())
	`, id, params.Name, params.Description, permsJSON)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			return "", fmt.Errorf("role '%s' already exists", params.Name)
		}
		return "", fmt.Errorf("failed to create role: %w", err)
	}

	return fmt.Sprintf(`✅ **Created role "%s"**

| Setting | Value |
|---------|-------|
| Description | %s |
| Permissions | %s |

🔗 [Manage Roles](/app/settings/roles)`, params.Name, params.Description, strings.Join(params.Permissions, ", ")), nil
}

func (a *Assistant) executeListRoles(ctx context.Context, _ json.RawMessage) (string, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, name, description, permissions, is_system
		FROM roles
		ORDER BY is_system DESC, name ASC
	`)
	if err != nil {
		return "", fmt.Errorf("failed to list roles: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("## 👥 Roles\n\n")
	sb.WriteString("| Name | Description | Permissions | Type |\n")
	sb.WriteString("|------|-------------|-------------|------|\n")

	count := 0
	for rows.Next() {
		var id, name, desc string
		var permsJSON []byte
		var isSystem bool
		if err := rows.Scan(&id, &name, &desc, &permsJSON, &isSystem); err != nil {
			continue
		}
		var perms []string
		json.Unmarshal(permsJSON, &perms)
		roleType := "Custom"
		if isSystem {
			roleType = "🔒 System"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", name, desc, strings.Join(perms, ", "), roleType))
		count++
	}

	if count == 0 {
		return "No roles found.", nil
	}

	sb.WriteString(fmt.Sprintf("\n%d role(s) total\n\n🔗 [Manage Roles](/app/settings/roles)", count))
	return sb.String(), nil
}

func (a *Assistant) executeDeleteRole(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		RoleID   string `json:"role_id"`
		RoleName string `json:"role_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	var query string
	var identifier string
	var queryArg interface{}

	if params.RoleID != "" {
		query = `DELETE FROM roles WHERE id = $1 AND is_system = false`
		identifier = params.RoleID
		queryArg = params.RoleID
	} else if params.RoleName != "" {
		query = `DELETE FROM roles WHERE name = $1 AND is_system = false`
		identifier = params.RoleName
		queryArg = params.RoleName
	} else {
		return "", fmt.Errorf("must provide role_id or role_name")
	}

	result, err := a.db.ExecContext(ctx, query, queryArg)
	if err != nil {
		return "", fmt.Errorf("failed to delete role: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Sprintf("⚠️ Role not found or is a system role (cannot delete): %s", identifier), nil
	}

	return fmt.Sprintf("✅ **Deleted role `%s`**\n\n🔗 [Manage Roles](/app/settings/roles)", identifier), nil
}

func (a *Assistant) executeListUsers(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Role string `json:"role"`
	}
	_ = json.Unmarshal(args, &params)

	query := `
		SELECT id, email, name, role, provider, last_login_at
		FROM sso_users
		ORDER BY last_login_at DESC
	`
	rows, err := a.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("## 👤 Users\n\n")
	sb.WriteString("| Name | Email | Role | Provider | Last Login |\n")
	sb.WriteString("|------|-------|------|----------|------------|\n")

	count := 0
	for rows.Next() {
		var id, email, name, role, provider string
		var lastLogin time.Time
		if err := rows.Scan(&id, &email, &name, &role, &provider, &lastLogin); err != nil {
			continue
		}
		if params.Role != "" && role != params.Role {
			continue
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			name, email, role, provider, lastLogin.Format("Jan 2, 2006")))
		count++
	}

	if count == 0 {
		return "No users found.", nil
	}

	sb.WriteString(fmt.Sprintf("\n%d user(s) total\n\n🔗 [Manage Users](/app/settings/users)", count))
	return sb.String(), nil
}

func (a *Assistant) executeUpdateUserRole(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		UserEmail string `json:"user_email"`
		NewRole   string `json:"new_role"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	result, err := a.db.ExecContext(ctx, `
		UPDATE sso_users SET role = $1 WHERE email = $2
	`, params.NewRole, params.UserEmail)
	if err != nil {
		return "", fmt.Errorf("failed to update user role: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Sprintf("⚠️ User not found: %s", params.UserEmail), nil
	}

	return fmt.Sprintf("✅ **Updated user `%s` to role `%s`**\n\n🔗 [Manage Users](/app/settings/users)",
		params.UserEmail, params.NewRole), nil
}

// ─── API Key Management ─────────────────────────────────────────────────────

func (a *Assistant) executeCreateAPIKey(ctx context.Context, args json.RawMessage, userEmail string) (string, error) {
	var params struct {
		Name        string   `json:"name"`
		Scopes      []string `json:"scopes"`
		ExpiresDays int      `json:"expires_days"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if len(params.Scopes) == 0 {
		params.Scopes = []string{"read"}
	}

	// Use the actual user's email, fallback to assistant if not provided
	if userEmail == "" {
		userEmail = "assistant@runright.io"
	}

	// Generate a key (in reality you'd use crypto/rand and hash it)
	keyPrefix := fmt.Sprintf("rr_%s", uuid.New().String()[:8])
	keyHash := uuid.New().String() // In production, hash a real key

	scopesJSON, _ := json.Marshal(params.Scopes)
	id := uuid.New().String()

	var expiresAt interface{} = nil
	if params.ExpiresDays > 0 {
		exp := time.Now().AddDate(0, 0, params.ExpiresDays)
		expiresAt = exp
	}

	_, err := a.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, user_email, name, key_prefix, key_hash, scopes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`, id, userEmail, params.Name, keyPrefix, keyHash, scopesJSON, expiresAt)
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	expiresText := "Never"
	if params.ExpiresDays > 0 {
		expiresText = fmt.Sprintf("%d days", params.ExpiresDays)
	}

	return fmt.Sprintf(`✅ **Created API key "%s"**

| Setting | Value |
|---------|-------|
| Key Prefix | %s... |
| Scopes | %s |
| Expires | %s |

⚠️ **Important:** The full API key is only shown once. Store it securely!

🔗 [Manage API Keys](/app/settings/api-keys)`, params.Name, keyPrefix, strings.Join(params.Scopes, ", "), expiresText), nil
}

func (a *Assistant) executeListAPIKeys(ctx context.Context, _ json.RawMessage) (string, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, name, key_prefix, scopes, expires_at, revoked_at, last_used_at, created_at
		FROM api_keys
		ORDER BY created_at DESC
	`)
	if err != nil {
		return "", fmt.Errorf("failed to list API keys: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("## 🔑 API Keys\n\n")
	sb.WriteString("| Name | Key | Scopes | Status | Last Used |\n")
	sb.WriteString("|------|-----|--------|--------|----------|\n")

	count := 0
	for rows.Next() {
		var id, name, prefix string
		var scopesJSON []byte
		var expiresAt, revokedAt, lastUsed, createdAt sql.NullTime
		if err := rows.Scan(&id, &name, &prefix, &scopesJSON, &expiresAt, &revokedAt, &lastUsed, &createdAt); err != nil {
			continue
		}
		var scopes []string
		json.Unmarshal(scopesJSON, &scopes)

		status := "✅ Active"
		if revokedAt.Valid {
			status = "🚫 Revoked"
		} else if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			status = "⏰ Expired"
		}

		lastUsedStr := "Never"
		if lastUsed.Valid {
			lastUsedStr = lastUsed.Time.Format("Jan 2")
		}

		sb.WriteString(fmt.Sprintf("| %s | %s... | %s | %s | %s |\n",
			name, prefix, strings.Join(scopes, ", "), status, lastUsedStr))
		count++
	}

	if count == 0 {
		return "No API keys found.", nil
	}

	sb.WriteString(fmt.Sprintf("\n%d key(s) total\n\n🔗 [Manage API Keys](/app/settings/api-keys)", count))
	return sb.String(), nil
}

func (a *Assistant) executeRevokeAPIKey(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		KeyID   string `json:"key_id"`
		KeyName string `json:"key_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	var result sql.Result
	var err error
	var identifier string

	if params.KeyID != "" {
		result, err = a.db.ExecContext(ctx, `UPDATE api_keys SET revoked_at = NOW() WHERE id = $1`, params.KeyID)
		identifier = params.KeyID
	} else if params.KeyName != "" {
		result, err = a.db.ExecContext(ctx, `UPDATE api_keys SET revoked_at = NOW() WHERE name = $1`, params.KeyName)
		identifier = params.KeyName
	} else {
		return "", fmt.Errorf("must provide key_id or key_name")
	}

	if err != nil {
		return "", fmt.Errorf("failed to revoke API key: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Sprintf("⚠️ API key not found: %s", identifier), nil
	}

	return fmt.Sprintf("✅ **Revoked API key `%s`**\n\nThis key can no longer be used for authentication.\n\n🔗 [Manage API Keys](/app/settings/api-keys)", identifier), nil
}

// ─── Repository Ownership ───────────────────────────────────────────────────

func (a *Assistant) executeSetRepositoryOwnership(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Repository               string   `json:"repository"`
		TeamName                 string   `json:"team_name"`
		NotificationDestinations []string `json:"notification_destinations"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if params.NotificationDestinations == nil {
		params.NotificationDestinations = []string{}
	}
	destJSON, _ := json.Marshal(params.NotificationDestinations)

	_, err := a.db.ExecContext(ctx, `
		INSERT INTO repository_ownership (repository, team_name, destination_ids, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (repository, team_name) DO UPDATE
		SET destination_ids = EXCLUDED.destination_ids, updated_at = NOW()
	`, params.Repository, params.TeamName, destJSON)
	if err != nil {
		return "", fmt.Errorf("failed to set ownership: %w", err)
	}

	return fmt.Sprintf(`✅ **Set ownership for %s**

| Setting | Value |
|---------|-------|
| Repository | %s |
| Team | %s |
| Alert Destinations | %d configured |

🔗 [Manage Ownership](/app/settings/ownership)`, params.Repository, params.Repository, params.TeamName, len(params.NotificationDestinations)), nil
}

func (a *Assistant) executeListRepositoryOwnership(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Repository string `json:"repository"`
	}
	_ = json.Unmarshal(args, &params)

	query := `SELECT repository, team_name, destination_ids FROM repository_ownership ORDER BY repository`
	rows, err := a.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to list ownership: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("## 🏢 Repository Ownership\n\n")
	sb.WriteString("| Repository | Team | Alert Destinations |\n")
	sb.WriteString("|------------|------|--------------------|\n")

	count := 0
	for rows.Next() {
		var repo, team string
		var destJSON []byte
		if err := rows.Scan(&repo, &team, &destJSON); err != nil {
			continue
		}
		if params.Repository != "" && repo != params.Repository {
			continue
		}
		var dests []string
		json.Unmarshal(destJSON, &dests)
		sb.WriteString(fmt.Sprintf("| %s | %s | %d |\n", repo, team, len(dests)))
		count++
	}

	if count == 0 {
		return "No repository ownership configured.", nil
	}

	sb.WriteString(fmt.Sprintf("\n%d assignment(s) total\n\n🔗 [Manage Ownership](/app/settings/ownership)", count))
	return sb.String(), nil
}

func (a *Assistant) executeRemoveRepositoryOwnership(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Repository string `json:"repository"`
		TeamName   string `json:"team_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	result, err := a.db.ExecContext(ctx, `
		DELETE FROM repository_ownership WHERE repository = $1 AND team_name = $2
	`, params.Repository, params.TeamName)
	if err != nil {
		return "", fmt.Errorf("failed to remove ownership: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Sprintf("⚠️ No ownership found for %s / %s", params.Repository, params.TeamName), nil
	}

	return fmt.Sprintf("✅ **Removed %s ownership of `%s`**\n\n🔗 [Manage Ownership](/app/settings/ownership)",
		params.TeamName, params.Repository), nil
}

// === Label Mapping Tools ===

func (a *Assistant) executeCreateLabel(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Label        string  `json:"label"`
		Provider     string  `json:"provider"`
		InstanceType string  `json:"instance_type"`
		VCPUs        int     `json:"vcpus"`
		MemoryGiB    float64 `json:"memory_gib"`
		CostPerHour  float64 `json:"cost_per_hour"`
		Repository   string  `json:"repository"`
		IsGPU        bool    `json:"is_gpu"`
		GPUType      string  `json:"gpu_type"`
		GPUCount     int     `json:"gpu_count"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// Apply defaults
	if params.Provider == "" {
		params.Provider = "github"
	}
	if params.Repository == "" {
		params.Repository = "*"
	}
	if params.InstanceType == "" {
		params.InstanceType = "custom"
	}
	if params.IsGPU && params.GPUCount == 0 {
		params.GPUCount = 1
	}

	// Upsert the label mapping
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO label_mappings (repository, label, provider, instance_type, vcpus, memory_gib, cost_per_hour, is_gpu, gpu_type, gpu_count, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (team_id, repository, label) DO UPDATE
		SET provider = EXCLUDED.provider, instance_type = EXCLUDED.instance_type, vcpus = EXCLUDED.vcpus, 
		    memory_gib = EXCLUDED.memory_gib, cost_per_hour = EXCLUDED.cost_per_hour, is_gpu = EXCLUDED.is_gpu,
		    gpu_type = EXCLUDED.gpu_type, gpu_count = EXCLUDED.gpu_count, updated_at = NOW()
	`, params.Repository, params.Label, params.Provider, params.InstanceType, params.VCPUs, params.MemoryGiB, params.CostPerHour, params.IsGPU, params.GPUType, params.GPUCount)
	if err != nil {
		return "", fmt.Errorf("failed to create label mapping: %w", err)
	}

	gpuInfo := ""
	if params.IsGPU {
		gpuInfo = fmt.Sprintf("\n| GPU | %s x%d |", params.GPUType, params.GPUCount)
	}

	scope := "global"
	if params.Repository != "*" {
		scope = fmt.Sprintf("repository `%s`", params.Repository)
	}

	return fmt.Sprintf(`✅ **Created label mapping "%s"**

| Setting | Value |
|---------|-------|
| Provider | %s |
| Instance | %s |
| vCPUs | %d |
| Memory | %.1f GiB |
| Cost | $%.4f/hr |
| Scope | %s |%s

🔗 [View Labels](/app/auto-pr)`, params.Label, params.Provider, params.InstanceType, params.VCPUs, params.MemoryGiB, params.CostPerHour, scope, gpuInfo), nil
}

func (a *Assistant) executeListLabels(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		GPUOnly bool `json:"gpu_only"`
	}
	json.Unmarshal(args, &params)

	query := `
		SELECT label, provider, instance_type, vcpus, memory_gib, cost_per_hour, repository, is_gpu, gpu_type, gpu_count
		FROM label_mappings
	`
	if params.GPUOnly {
		query += " WHERE is_gpu = true"
	}
	query += " ORDER BY label"

	rows, err := a.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to list labels: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("## 🏷️ Runner Labels\n\n")
	sb.WriteString("| Label | Provider | vCPUs | Memory | $/hr | Scope |\n")
	sb.WriteString("|-------|----------|-------|--------|------|-------|\n")

	count := 0
	for rows.Next() {
		var label, provider, instanceType, repository string
		var vcpus, gpuCount int
		var memoryGiB, costPerHour float64
		var isGPU bool
		var gpuType sql.NullString
		if err := rows.Scan(&label, &provider, &instanceType, &vcpus, &memoryGiB, &costPerHour, &repository, &isGPU, &gpuType, &gpuCount); err != nil {
			continue
		}

		scope := "global"
		if repository != "*" {
			scope = repository
		}

		gpuMark := ""
		if isGPU {
			gpuMark = " 🎮"
		}

		sb.WriteString(fmt.Sprintf("| %s%s | %s | %d | %.0f GiB | $%.4f | %s |\n",
			label, gpuMark, provider, vcpus, memoryGiB, costPerHour, scope))
		count++
	}

	if count == 0 {
		return "No label mappings found. Use `create_label` to add one.\n\n🔗 [Manage Labels](/app/auto-pr)", nil
	}

	sb.WriteString(fmt.Sprintf("\n*%d label(s) found*\n\n🔗 [Manage Labels](/app/auto-pr)", count))
	return sb.String(), nil
}

func (a *Assistant) executeDeleteLabel(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Label string `json:"label"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	result, err := a.db.ExecContext(ctx, `DELETE FROM label_mappings WHERE label = $1`, params.Label)
	if err != nil {
		return "", fmt.Errorf("failed to delete label: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Sprintf("⚠️ Label not found: %s", params.Label), nil
	}

	return fmt.Sprintf("✅ **Deleted label `%s`**\n\n🔗 [Manage Labels](/app/auto-pr)", params.Label), nil
}

// === Audit Log Tools ===

func (a *Assistant) executeSearchAuditLogs(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Action       string `json:"action"`
		Actor        string `json:"actor"`
		ResourceType string `json:"resource_type"`
		ResourceName string `json:"resource_name"`
		Limit        int    `json:"limit"`
	}
	json.Unmarshal(args, &params)

	if params.Limit <= 0 || params.Limit > 50 {
		params.Limit = 20
	}

	// Build query dynamically
	query := `
		SELECT id, actor_email, action, resource_type, resource_name, status, details, created_at
		FROM audit_logs
		WHERE 1=1
	`
	argList := []any{}
	argIdx := 1

	if params.Action != "" {
		query += fmt.Sprintf(" AND action LIKE $%d", argIdx)
		argList = append(argList, "%"+params.Action+"%")
		argIdx++
	}
	if params.Actor != "" {
		query += fmt.Sprintf(" AND actor_email ILIKE $%d", argIdx)
		argList = append(argList, "%"+params.Actor+"%")
		argIdx++
	}
	if params.ResourceType != "" {
		query += fmt.Sprintf(" AND resource_type = $%d", argIdx)
		argList = append(argList, params.ResourceType)
		argIdx++
	}
	if params.ResourceName != "" {
		query += fmt.Sprintf(" AND resource_name ILIKE $%d", argIdx)
		argList = append(argList, "%"+params.ResourceName+"%")
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", argIdx)
	argList = append(argList, params.Limit)

	rows, err := a.db.QueryContext(ctx, query, argList...)
	if err != nil {
		return "", fmt.Errorf("failed to search audit logs: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("## 📋 Audit Log Results\n\n")

	count := 0
	for rows.Next() {
		var id, actorEmail, action, resourceType, status string
		var resourceName sql.NullString
		var details []byte
		var createdAt time.Time

		if err := rows.Scan(&id, &actorEmail, &action, &resourceType, &resourceName, &status, &details, &createdAt); err != nil {
			continue
		}

		statusEmoji := "✅"
		if status != "success" {
			statusEmoji = "❌"
		}

		// Parse action for better display
		actionParts := strings.Split(action, ".")
		actionVerb := actionParts[len(actionParts)-1]
		actionVerb = strings.Title(strings.ToLower(actionVerb))

		resourceDisplay := resourceType
		if resourceName.Valid && resourceName.String != "" {
			resourceDisplay = fmt.Sprintf("%s \"%s\"", resourceType, resourceName.String)
		}

		sb.WriteString(fmt.Sprintf("### %s %s %s\n", statusEmoji, actionVerb, resourceDisplay))
		sb.WriteString(fmt.Sprintf("- **Actor:** %s\n", actorEmail))
		sb.WriteString(fmt.Sprintf("- **When:** %s\n", createdAt.Format("Jan 2, 2006 3:04 PM")))

		// Include details if present
		if len(details) > 0 && string(details) != "{}" && string(details) != "null" {
			var detailsMap map[string]any
			if json.Unmarshal(details, &detailsMap) == nil && len(detailsMap) > 0 {
				sb.WriteString("- **Details:**\n")
				for k, v := range detailsMap {
					sb.WriteString(fmt.Sprintf("  - %s: %v\n", k, v))
				}
			}
		}
		sb.WriteString("\n")
		count++
	}

	if count == 0 {
		filters := []string{}
		if params.Action != "" {
			filters = append(filters, fmt.Sprintf("action=%q", params.Action))
		}
		if params.Actor != "" {
			filters = append(filters, fmt.Sprintf("actor=%q", params.Actor))
		}
		if params.ResourceType != "" {
			filters = append(filters, fmt.Sprintf("type=%q", params.ResourceType))
		}
		if params.ResourceName != "" {
			filters = append(filters, fmt.Sprintf("name=%q", params.ResourceName))
		}
		filterStr := ""
		if len(filters) > 0 {
			filterStr = " matching " + strings.Join(filters, ", ")
		}
		return fmt.Sprintf("No audit log entries found%s.\n\n🔗 [View All Audit Logs](/app/settings?tab=audit)", filterStr), nil
	}

	sb.WriteString(fmt.Sprintf("*Found %d matching entries*\n\n🔗 [View All Audit Logs](/app/settings?tab=audit)", count))
	return sb.String(), nil
}

// executeGetRepositoryCosts returns cost breakdown by repository.
func (a *Assistant) executeGetRepositoryCosts(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Limit int `json:"limit"`
	}
	if len(args) > 0 {
		json.Unmarshal(args, &params)
	}
	if params.Limit <= 0 {
		params.Limit = 10
	}

	query := `
		SELECT 
			repository,
			COUNT(*) as job_count,
			ROUND(SUM((summary->'detected_machine'->>'on_demand_price_per_hour')::numeric * 
				(summary->>'duration_seconds')::numeric / 3600)::numeric, 2) as total_cost
		FROM jobs 
		WHERE summary->'detected_machine'->>'on_demand_price_per_hour' IS NOT NULL
		GROUP BY repository 
		ORDER BY total_cost DESC 
		LIMIT $1
	`

	rows, err := a.db.QueryContext(ctx, query, params.Limit)
	if err != nil {
		return "", fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("## Repository Costs\n\n")
	sb.WriteString("| Repository | Jobs | Total Cost |\n")
	sb.WriteString("|------------|------|------------|\n")

	var totalCost float64
	var topRepo string
	var topCost float64
	rank := 0

	for rows.Next() {
		var repo string
		var jobCount int
		var cost float64
		if err := rows.Scan(&repo, &jobCount, &cost); err != nil {
			return "", err
		}
		rank++
		if rank == 1 {
			topRepo = repo
			topCost = cost
		}
		totalCost += cost
		sb.WriteString(fmt.Sprintf("| %s | %d | $%.2f |\n", repo, jobCount, cost))
	}

	sb.WriteString(fmt.Sprintf("\n**Total across all repos:** $%.2f\n", totalCost))
	if topRepo != "" {
		sb.WriteString(fmt.Sprintf("\n**Most expensive:** %s ($%.2f)\n", topRepo, topCost))
	}

	return sb.String(), nil
}

// executeGetRecommendations returns RunRight's machine recommendations for a repository.
func (a *Assistant) executeGetRecommendations(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Repository string `json:"repository"`
		Limit      int    `json:"limit"`
	}
	if len(args) > 0 {
		json.Unmarshal(args, &params)
	}
	if params.Limit <= 0 {
		params.Limit = 10
	}

	query := `
		SELECT 
			summary->>'job_id' as job_id,
			summary->>'repository' as repository,
			summary->'detected_machine'->>'id' as current_machine,
			(summary->'detected_machine'->>'on_demand_price_per_hour')::numeric as current_price,
			(summary->'detected_machine'->>'vcpus')::int as current_vcpus,
			(summary->'detected_machine'->>'memory_gib')::numeric as current_memory,
			(summary->>'cpu_percent_peak')::numeric as cpu_peak,
			(summary->>'cpu_percent_avg')::numeric as cpu_avg,
			(summary->>'mem_used_gib_peak')::numeric as mem_peak,
			(summary->>'mem_total_gib')::numeric as mem_total,
			recommendations->0->'machine'->>'id' as recommended_machine,
			(recommendations->0->'machine'->>'on_demand_price_per_hour')::numeric as recommended_price,
			(recommendations->0->'machine'->>'vcpus')::int as recommended_vcpus,
			(recommendations->0->'machine'->>'memory_gib')::numeric as recommended_memory,
			(recommendations->0->>'current_monthly_usd')::numeric as current_monthly,
			(recommendations->0->>'estimated_monthly_usd')::numeric as estimated_monthly,
			(recommendations->0->>'cost_delta_percent')::numeric as cost_delta_pct,
			recommendations->0->>'reasoning' as reasoning
		FROM jobs
		WHERE recommendations IS NOT NULL 
		  AND jsonb_array_length(recommendations) > 0
		  AND (recommendations->0->>'cost_delta_percent')::numeric < -5
	`
	queryArgs := []interface{}{}
	argNum := 1

	if params.Repository != "" {
		query += fmt.Sprintf(" AND summary->>'repository' = $%d", argNum)
		queryArgs = append(queryArgs, params.Repository)
		argNum++
	}

	query += fmt.Sprintf(" ORDER BY ((recommendations->0->>'current_monthly_usd')::numeric - (recommendations->0->>'estimated_monthly_usd')::numeric) DESC LIMIT $%d", argNum)
	queryArgs = append(queryArgs, params.Limit)

	rows, err := a.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return "", fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("## 💡 RunRight Recommendations\n\n")
	if params.Repository != "" {
		sb.WriteString(fmt.Sprintf("**Repository:** %s\n\n", params.Repository))
	}

	sb.WriteString("| Job | Current Machine | Recommended | Savings | Reason |\n")
	sb.WriteString("|-----|-----------------|-------------|---------|--------|\n")

	var totalSavings float64
	count := 0

	for rows.Next() {
		var jobID, repo, currentMachine, recommendedMachine, reasoning sql.NullString
		var currentPrice, recommendedPrice, currentMonthly, estimatedMonthly, costDeltaPct sql.NullFloat64
		var currentVCPUs, recommendedVCPUs sql.NullInt64
		var currentMemory, recommendedMemory, cpuPeak, cpuAvg, memPeak, memTotal sql.NullFloat64

		if err := rows.Scan(
			&jobID, &repo, &currentMachine, &currentPrice, &currentVCPUs, &currentMemory,
			&cpuPeak, &cpuAvg, &memPeak, &memTotal,
			&recommendedMachine, &recommendedPrice, &recommendedVCPUs, &recommendedMemory,
			&currentMonthly, &estimatedMonthly, &costDeltaPct, &reasoning,
		); err != nil {
			continue
		}

		count++
		savings := currentMonthly.Float64 - estimatedMonthly.Float64
		totalSavings += savings

		// Build utilization info
		utilInfo := ""
		if cpuPeak.Valid && memPeak.Valid && memTotal.Valid && memTotal.Float64 > 0 {
			memPct := (memPeak.Float64 / memTotal.Float64) * 100
			utilInfo = fmt.Sprintf("CPU: %.0f%%, Mem: %.0f%%", cpuPeak.Float64, memPct)
		}

		// Build machine comparison
		currentDesc := currentMachine.String
		if currentVCPUs.Valid && currentMemory.Valid {
			currentDesc = fmt.Sprintf("%s (%dvCPU, %.0fGB, $%.2f/hr)", currentMachine.String, currentVCPUs.Int64, currentMemory.Float64, currentPrice.Float64)
		}

		recommendedDesc := recommendedMachine.String
		if recommendedVCPUs.Valid && recommendedMemory.Valid {
			recommendedDesc = fmt.Sprintf("%s (%dvCPU, %.0fGB, $%.2f/hr)", recommendedMachine.String, recommendedVCPUs.Int64, recommendedMemory.Float64, recommendedPrice.Float64)
		}

		reasonText := reasoning.String
		if reasonText == "" {
			reasonText = utilInfo
		}
		if len(reasonText) > 50 {
			reasonText = reasonText[:47] + "..."
		}

		sb.WriteString(fmt.Sprintf("| %s | %s | %s | **$%.2f/mo** | %s |\n",
			jobID.String, currentDesc, recommendedDesc, savings, reasonText))
	}

	if count == 0 {
		sb.WriteString("\nNo recommendations found")
		if params.Repository != "" {
			sb.WriteString(fmt.Sprintf(" for %s", params.Repository))
		}
		sb.WriteString(". Jobs may already be optimized or need more run data.\n")
	} else {
		sb.WriteString(fmt.Sprintf("\n**Total Potential Savings:** $%.2f/mo ($%.2f/yr)\n", totalSavings, totalSavings*12))
		sb.WriteString("\n### How to Apply:\n")
		sb.WriteString("1. Review the recommendations above\n")
		sb.WriteString("2. Update your CI workflow to use the recommended machine type\n")
		sb.WriteString("3. RunRight will continue monitoring to verify savings\n")
	}

	return sb.String(), nil
}

// ToolsSystemPromptAddition returns the addition to the system prompt that describes tool capabilities.
func (a *Assistant) ToolsSystemPromptAddition() string {
	return `

## You are an intelligent assistant with tools. Use them wisely.

You have function calling tools to manage alerts, policies, jobs, roles, API keys, labels, and more.

### CRITICAL RULES:

1. **ASK for missing required info** — If the user's request is vague or missing key details (like the NAME of an alert, or WHICH repository), ask a quick clarifying question first. Don't guess names.

2. **When user confirms, CALL THE TOOL** — When the user says "yes", "ok", "do it", etc. after you asked a clarifying question, you MUST call the appropriate tool immediately. Do NOT describe what you would do. Do NOT say "*calls ...*" or "[CALLS ...]". Actually invoke the tool.

3. **Report results briefly** — After a tool executes, give a short confirmation. Example: "Created alert 'high-cpu-warning' for runrightio/ml-platform."

4. **Don't ask permission** — When you have all the info you need, just call the tool. Don't ask "Would you like me to create this?"

5. **Never describe tool calls** — Never write text like "*calls create_alert*" or "I will call the function". Just call it silently and report the result.

### WHEN TO ASK FOR CLARIFICATION:

Ask when:
- Alert/policy/role NAME isn't specified: "What would you like to name this alert?"
- Repository is ambiguous: "Which repository? (e.g., runrightio/ml-platform)"
- Threshold/value isn't specified: "What threshold should trigger this alert?"

Don't ask when:
- You can use sensible defaults (e.g., enabled=true)
- The user gave enough info to proceed

### Available Tools:

**ALERTS:**
- create_alert_rule(name, condition_type, threshold, repository?, channel?, destination?)
- list_alerts(repository?)
- delete_alert(alert_id or alert_name)
- toggle_alert(alert_id or alert_name, enabled?)

**POLICIES:**
- create_policy(max_cost_per_hour, repository?, job_id?, enabled?)
- update_policy(repository, max_cost_per_hour?, enabled?)
- delete_policy(repository, job_id?)

**JOBS:**
- snooze_job(job_id, duration_days?, reason?)
- archive_job(job_id)
- get_job_details(job_id)
- list_high_waste_jobs(min_waste_percent?, limit?)
- generate_savings_report(repository?, time_range_days?)

**ROLE & USER MANAGEMENT:**
- create_role(name, description?, permissions[])
- list_roles()
- delete_role(role_id or role_name)
- list_users(role?)
- update_user_role(user_email, new_role)

**API KEY MANAGEMENT:**
- create_api_key(name, scopes[]?, expires_days?)
- list_api_keys()
- revoke_api_key(key_id or key_name)

**REPOSITORY OWNERSHIP:**
- set_repository_ownership(repository, team_name, notification_destinations[]?)
- list_repository_ownership(repository?)
- remove_repository_ownership(repository, team_name)

**LABELS (Runner Label Mappings):**
- create_label(label, vcpus, memory_gib, cost_per_hour, provider?, instance_type?, repository?, is_gpu?, gpu_type?, gpu_count?)
- list_labels(gpu_only?)
- delete_label(label)

**AUDIT LOGS:**
- search_audit_logs(action?, actor?, resource_type?, resource_name?, limit?)

**COST ANALYSIS:**
- get_repository_costs(limit?) — get cost ranking of repos. Use when user asks about "most expensive", "highest cost", or "top spending" repositories
- get_recommendations(repository?, limit?) — **CRITICAL: ALWAYS call this when user asks "how to save", "how to reduce costs", "what can we do", "fix it", or "recommendations"**. Returns specific machine switches with savings amounts.

**ANALYSIS:**
- analyze_page() — use when user asks about "this page", "what I'm looking at"

### IMPORTANT: When users ask about saving money or reducing costs:

1. **ALWAYS call get_recommendations()** first to get RunRight's specific machine recommendations
2. Present the actual machine switches (e.g., "switch gpu-inference from p3.2xlarge to p3.xlarge")
3. Show the specific savings amounts from the tool results
4. Explain WHY based on utilization data (CPU %, memory %)

### Example Conversations:

**User:** "What is the most expensive repo?"
**You:** *calls get_repository_costs* → "**runrightio/ml-platform** is your most expensive repo at **$1505.66 total**."

**User:** "What can we do to fix it or save on it?"
**You:** *calls get_recommendations(repository="runrightio/ml-platform")* → Shows specific recommendations:

"Here are RunRight's recommendations for **runrightio/ml-platform**:

| Job | Current | Recommended | Savings |
|-----|---------|-------------|---------|
| gpu-inference | p3.2xlarge ($3.06/hr) | p3.xlarge ($1.53/hr) | **$125.90/mo** |
| ml-training | m5.4xlarge ($0.77/hr) | m5.2xlarge ($0.38/hr) | **$48.20/mo** |

The gpu-inference job only uses 45% GPU and 32% memory, so a smaller instance is sufficient.
**Total potential savings: $174.10/mo ($2,089/yr)**"

**User:** "Make an alert for our most expensive repo"
**You:** *calls get_repository_costs* → finds runrightio/ml-platform is #1 at $1505
**You:** "Your most expensive repo is runrightio/ml-platform ($1505). What should I name this alert and what threshold should trigger it?"

**User:** "call it ml-cost-alert at $50/hr"
**You:** *calls create_alert_rule* → "Created alert 'ml-cost-alert' for runrightio/ml-platform — triggers when cost exceeds $50/hr."

**User:** "list all users"
**You:** *calls list_users* → Shows the formatted user list

**User:** "create a CI API key"
**You:** *calls create_api_key with name="CI"* → "Created API key 'CI'. Key prefix: rr_abc... (save the full key securely)"

**User:** "who deleted the high-cpu alert?"
**You:** *calls search_audit_logs* → "dev@runright.io deleted 'high-cpu-alert' on July 8 at 3:45 PM."
`
}
