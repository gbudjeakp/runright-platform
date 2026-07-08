package assistant

// memory.go — conversation memory compaction and token-budget-aware context trimming.
//
// Strategy:
//   - Conversations are compacted when they exceed compactThreshold messages.
//     The oldest messages (beyond recentKeepCount) are summarised by the LLM and
//     deleted; the summary is stored in assistant_conversations.summary.
//   - On every turn the summary is prepended as a compact "memory" system message
//     so the model always has full conversational context without ballooning token usage.
//   - Context data (jobs, savings, etc.) is trimmed to fit a per-provider token budget
//     so we never overflow context windows on small local models.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sgbudje/runright-platform/internal/types"
)

// ── Constants ────────────────────────────────────────────────────────────────

const (
	// compactThreshold: compact when a conversation has this many stored messages.
	compactThreshold = 24
	// recentKeepCount: verbatim messages to preserve after compaction.
	recentKeepCount = 10
	// charsPerToken: rough approximation — English averages ~4 chars/token.
	charsPerToken = 4
)

// providerTokenBudget holds token limits per provider.
type providerTokenBudget struct {
	// contextBudget is the max tokens to spend on RunRight data (jobs, savings, etc.)
	contextBudget int
	// historyBudget is the max tokens to spend on verbatim conversation history.
	historyBudget int
	// summaryBudget is max tokens for the compacted memory summary.
	summaryBudget int
}

func (a *Assistant) tokenBudgets() providerTokenBudget {
	switch a.cfg.Provider {
	case ProviderOllama:
		// Local models: conservative — most have ≤8k effective context.
		return providerTokenBudget{contextBudget: 2500, historyBudget: 1200, summaryBudget: 400}
	case ProviderAnthropic:
		return providerTokenBudget{contextBudget: 10000, historyBudget: 6000, summaryBudget: 800}
	default: // OpenAI
		return providerTokenBudget{contextBudget: 10000, historyBudget: 6000, summaryBudget: 800}
	}
}

// ── Token estimation ─────────────────────────────────────────────────────────

// estimateTokens returns a rough token count for arbitrary text.
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + charsPerToken - 1) / charsPerToken
}

// ── ConversationMemory ───────────────────────────────────────────────────────

// ConversationMemory holds the compacted state for one conversation.
type ConversationMemory struct {
	// Summary is a LLM-generated digest of messages older than RecentMessages.
	Summary string
	// RecentMessages are the last recentKeepCount messages verbatim.
	RecentMessages []types.ChatMessage
}

// buildMemory fetches the conversation's persisted summary and its most recent
// messages, returning them ready for injection into the LLM prompt.
func (a *Assistant) buildMemory(ctx context.Context, convID string) (*ConversationMemory, error) {
	var summary sql.NullString
	err := a.db.QueryRowContext(ctx,
		`SELECT summary FROM assistant_conversations WHERE id = $1`, convID,
	).Scan(&summary)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	recent, err := a.getRecentMessages(ctx, convID, recentKeepCount)
	if err != nil {
		return nil, err
	}

	mem := &ConversationMemory{RecentMessages: recent}
	if summary.Valid {
		mem.Summary = summary.String
	}
	return mem, nil
}

// getRecentMessages returns the last `limit` messages in chronological order.
func (a *Assistant) getRecentMessages(ctx context.Context, convID string, limit int) ([]types.ChatMessage, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, conversation_id, role, content, metadata, created_at
		FROM (
			SELECT id, conversation_id, role, content, metadata, created_at
			FROM assistant_messages
			WHERE conversation_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		) sub
		ORDER BY created_at ASC
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
			_ = json.Unmarshal([]byte(metaJSON.String), &msg.Metadata)
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

// ── Compaction ───────────────────────────────────────────────────────────────

// compactIfNeeded is called asynchronously after each assistant turn.
// When the conversation exceeds compactThreshold messages it summarises the
// oldest ones and deletes them, keeping only recentKeepCount verbatim.
func (a *Assistant) compactIfNeeded(ctx context.Context, convID string) {
	var count int
	if err := a.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM assistant_messages WHERE conversation_id = $1`, convID,
	).Scan(&count); err != nil || count <= compactThreshold {
		return
	}

	// Fetch all messages older than the recentKeepCount newest.
	type rawMsg struct {
		ID        string
		Role      string
		Content   string
		CreatedAt time.Time
	}

	rows, err := a.db.QueryContext(ctx, `
		SELECT id, role, content, created_at
		FROM (
			SELECT id, role, content, created_at,
			       ROW_NUMBER() OVER (ORDER BY created_at DESC) AS rn
			FROM assistant_messages
			WHERE conversation_id = $1
		) sub
		WHERE rn > $2
		ORDER BY created_at ASC
	`, convID, recentKeepCount)
	if err != nil {
		return
	}

	var toSummarise []rawMsg
	var ids []string
	for rows.Next() {
		var m rawMsg
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			rows.Close()
			return
		}
		toSummarise = append(toSummarise, m)
		ids = append(ids, m.ID)
	}
	rows.Close()

	if len(toSummarise) == 0 {
		return
	}

	// Fetch existing summary to roll into the new one.
	var existingSummary sql.NullString
	_ = a.db.QueryRowContext(ctx,
		`SELECT summary FROM assistant_conversations WHERE id = $1`, convID,
	).Scan(&existingSummary)

	// Build the summarisation prompt.
	var sb strings.Builder
	if existingSummary.Valid && existingSummary.String != "" {
		sb.WriteString("Prior summary (roll this into the new summary):\n")
		sb.WriteString(existingSummary.String)
		sb.WriteString("\n\nNew messages to incorporate:\n")
	} else {
		sb.WriteString("Summarise the following CI/CD cost-analysis conversation. " +
			"Preserve specific job names, numbers, findings, and recommendations. Be concise:\n\n")
	}
	for _, m := range toSummarise {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, m.Content))
	}

	summary, err := a.generateSummary(ctx, sb.String())
	if err != nil {
		// Non-fatal — skip compaction and try again next turn.
		return
	}

	// Persist summary and delete the summarised messages in a transaction.
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`UPDATE assistant_conversations SET summary = $1, last_compacted_at = NOW() WHERE id = $2`,
		summary, convID,
	); err != nil {
		return
	}

	// Build DELETE … IN ($2,$3,…) with proper placeholders.
	if len(ids) > 0 {
		placeholders := make([]string, len(ids))
		args := make([]any, len(ids)+1)
		args[0] = convID
		for i, id := range ids {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args[i+1] = id
		}
		q := fmt.Sprintf(
			`DELETE FROM assistant_messages WHERE conversation_id = $1 AND id IN (%s)`,
			strings.Join(placeholders, ","),
		)
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return
		}
	}

	_ = tx.Commit()
}

// generateSummary calls the configured LLM with a minimal prompt to produce a
// compact summary string. It does not include RunRight data context.
func (a *Assistant) generateSummary(ctx context.Context, prompt string) (string, error) {
	// Use a time-boxed context so a slow model doesn't block indefinitely.
	sCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	instruction := "You are a helpful summariser. " +
		"Produce a concise bullet-point summary of the conversation below, " +
		"preserving all specific metrics, job names, cost figures, and recommendations. " +
		"Output plain text, no markdown headers."

	switch a.cfg.Provider {
	case ProviderOllama:
		return a.callOllamaRaw(sCtx, instruction, prompt)
	case ProviderAnthropic:
		return a.callAnthropicRaw(sCtx, instruction, prompt)
	default:
		return a.callOpenAIRaw(sCtx, instruction, prompt)
	}
}

// ── Raw (no-history) LLM calls for internal tasks (summarisation) ───────────

func (a *Assistant) callOllamaRaw(ctx context.Context, system, user string) (string, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type reqBody struct {
		Model    string `json:"model"`
		Messages []msg  `json:"messages"`
		Stream   bool   `json:"stream"`
		Options  struct {
			Temperature float64 `json:"temperature"`
			NumPredict  int     `json:"num_predict"`
		} `json:"options"`
	}
	type respBody struct {
		Message msg    `json:"message"`
		Error   string `json:"error,omitempty"`
	}

	body, _ := json.Marshal(reqBody{
		Model:    a.cfg.Model,
		Messages: []msg{{Role: "system", Content: system}, {Role: "user", Content: user}},
		Stream:   false,
	})
	// Temperature and NumPredict set via zero values (Ollama defaults are fine for summaries).

	endpoint := strings.TrimSuffix(a.cfg.BaseURL, "/") + "/api/chat"
	return a.doRawPost(ctx, endpoint, body, "", func(b []byte) (string, error) {
		var r respBody
		if err := json.Unmarshal(b, &r); err != nil {
			return "", err
		}
		if r.Error != "" {
			return "", fmt.Errorf("ollama: %s", r.Error)
		}
		return r.Message.Content, nil
	})
}

func (a *Assistant) callOpenAIRaw(ctx context.Context, system, user string) (string, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type reqBody struct {
		Model       string  `json:"model"`
		Messages    []msg   `json:"messages"`
		MaxTokens   int     `json:"max_tokens"`
		Temperature float64 `json:"temperature"`
	}
	type choice struct {
		Message msg `json:"message"`
	}
	type respBody struct {
		Choices []choice `json:"choices"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	body, _ := json.Marshal(reqBody{
		Model:       a.cfg.Model,
		Messages:    []msg{{Role: "system", Content: system}, {Role: "user", Content: user}},
		MaxTokens:   512,
		Temperature: 0.2,
	})

	return a.doRawPost(ctx, "https://api.openai.com/v1/chat/completions", body,
		"Bearer "+a.cfg.APIKey, func(b []byte) (string, error) {
			var r respBody
			if err := json.Unmarshal(b, &r); err != nil {
				return "", err
			}
			if r.Error != nil {
				return "", fmt.Errorf("openai: %s", r.Error.Message)
			}
			if len(r.Choices) == 0 {
				return "", fmt.Errorf("openai: no choices")
			}
			return r.Choices[0].Message.Content, nil
		})
}

func (a *Assistant) callAnthropicRaw(ctx context.Context, system, user string) (string, error) {
	type content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type msg struct {
		Role    string    `json:"role"`
		Content []content `json:"content"`
	}
	type reqBody struct {
		Model     string `json:"model"`
		System    string `json:"system"`
		Messages  []msg  `json:"messages"`
		MaxTokens int    `json:"max_tokens"`
	}
	type respBody struct {
		Content []content `json:"content"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	body, _ := json.Marshal(reqBody{
		Model:     a.cfg.Model,
		System:    system,
		Messages:  []msg{{Role: "user", Content: []content{{Type: "text", Text: user}}}},
		MaxTokens: 512,
	})

	return a.doRawPost(ctx, "https://api.anthropic.com/v1/messages", body,
		"", func(b []byte) (string, error) {
			var r respBody
			if err := json.Unmarshal(b, &r); err != nil {
				return "", err
			}
			if r.Error != nil {
				return "", fmt.Errorf("anthropic: %s", r.Error.Message)
			}
			var out strings.Builder
			for _, c := range r.Content {
				if c.Type == "text" {
					out.WriteString(c.Text)
				}
			}
			return out.String(), nil
		})
}

// doRawPost is a shared helper for the raw LLM calls above.
// authHeader should be "Bearer <key>" for OpenAI, "" for Anthropic (uses x-api-key), "" for Ollama.
func (a *Assistant) doRawPost(ctx context.Context, url string, body []byte, authHeader string,
	parse func([]byte) (string, error)) (string, error) {

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if strings.Contains(url, "anthropic.com") {
		req.Header.Set("x-api-key", a.cfg.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return parse(raw)
}

// ── Context trimming ─────────────────────────────────────────────────────────

// trimContext prunes the AssistantContext to fit within budgetTokens.
// Priority order: semantic context → savings → recent jobs → policies → older jobs → repos.
func trimContext(assistantCtx *types.AssistantContext, budgetTokens int) *types.AssistantContext {
	if assistantCtx == nil {
		return assistantCtx
	}

	used := 0

	out := &types.AssistantContext{}

	// 1. Semantic context is highest priority (already filtered by RAG).
	if t := estimateTokens(assistantCtx.SemanticContext); used+t <= budgetTokens {
		out.SemanticContext = assistantCtx.SemanticContext
		used += t
	} else if budgetTokens-used > 200 {
		// Truncate rather than drop entirely.
		limit := (budgetTokens - used) * charsPerToken
		cutoff := min(limit, len(assistantCtx.SemanticContext))
		out.SemanticContext = assistantCtx.SemanticContext[:cutoff] + "\n[truncated]"
		used = budgetTokens
	}

	// 2. Savings summary (compact).
	if assistantCtx.Savings != nil {
		t := estimateTokens(mustJSON(assistantCtx.Savings))
		if used+t <= budgetTokens {
			out.Savings = assistantCtx.Savings
			used += t
		}
	}

	// 3. Jobs — add as many as budget allows, most-recent first.
	out.Recommendations = assistantCtx.Recommendations
	for _, job := range assistantCtx.Jobs {
		j := job
		t := estimateTokens(mustJSON(j))
		if used+t > budgetTokens {
			break
		}
		out.Jobs = append(out.Jobs, j)
		used += t
	}

	// 4. Policies.
	if used < budgetTokens {
		for _, p := range assistantCtx.Policies {
			t := estimateTokens(mustJSON(p))
			if used+t > budgetTokens {
				break
			}
			out.Policies = append(out.Policies, p)
			used += t
		}
	}

	// 5. Repositories (very small, just strings).
	if used < budgetTokens {
		out.Repositories = assistantCtx.Repositories
	}

	return out
}

// trimHistory reduces the history slice to fit within budgetTokens,
// keeping the most recent messages.
func trimHistory(history []types.ChatMessage, budgetTokens int) []types.ChatMessage {
	if len(history) == 0 {
		return history
	}
	// Walk from newest to oldest, collecting what fits.
	var kept []types.ChatMessage
	used := 0
	for i := len(history) - 1; i >= 0; i-- {
		t := estimateTokens(history[i].Content)
		if used+t > budgetTokens {
			break
		}
		kept = append([]types.ChatMessage{history[i]}, kept...)
		used += t
	}
	return kept
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
