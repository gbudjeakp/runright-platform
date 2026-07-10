package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sgbudje/runright-platform/internal/types"
)

// Provider interface abstracts different AI providers (OpenAI, Anthropic, Ollama).
type Provider interface {
	Name() string
	Chat(ctx context.Context, req ChatCompletionRequest, executeTool ToolExecutor) (string, error)
}

// ToolExecutor is a function that executes a tool call and returns the result.
type ToolExecutor func(ctx context.Context, call ToolCall) (*ToolResult, error)

// ChatCompletionRequest is the unified request format for all providers.
type ChatCompletionRequest struct {
	SystemPrompt    string
	MemorySummary   string
	SemanticContext string
	DataContext     string
	History         []types.ChatMessage
	UserMessage     string
	Tools           []Tool
	MaxIterations   int
	ForceToolUse    bool // If true, model MUST call at least one tool on first turn
}

// ProviderConfig holds common configuration for providers.
type ProviderConfig struct {
	APIKey      string
	BaseURL     string
	Model       string
	MaxTokens   int
	Temperature float64
	HTTPClient  *http.Client
}

// NewProvider creates a provider based on the provider type.
func NewProvider(providerType LLMProvider, cfg ProviderConfig) Provider {
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 2048
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.3
	}

	switch providerType {
	case ProviderAnthropic:
		return &anthropicProvider{cfg: cfg}
	case ProviderOllama:
		return &ollamaProvider{cfg: cfg}
	default:
		return &openAIProvider{cfg: cfg}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// OpenAI Provider
// ═══════════════════════════════════════════════════════════════════════════════

type openAIProvider struct {
	cfg ProviderConfig
}

func (p *openAIProvider) Name() string { return "openai" }

func (p *openAIProvider) Chat(ctx context.Context, req ChatCompletionRequest, executeTool ToolExecutor) (string, error) {
	// Build initial messages
	messages := []map[string]interface{}{
		{"role": "system", "content": req.SystemPrompt},
	}

	if req.MemorySummary != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": "CONVERSATION MEMORY (summary of earlier turns):\n" + req.MemorySummary,
		})
	}
	if req.SemanticContext != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": "RELEVANT CONTEXT (semantic search):\n" + req.SemanticContext,
		})
	}
	if req.DataContext != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": "Current RunRight data:\n" + req.DataContext,
		})
	}

	for _, h := range req.History {
		if h.Role == "user" || h.Role == "assistant" {
			messages = append(messages, map[string]interface{}{"role": h.Role, "content": h.Content})
		}
	}
	messages = append(messages, map[string]interface{}{"role": "user", "content": req.UserMessage})

	// Convert tools
	var tools []map[string]interface{}
	for _, t := range req.Tools {
		tools = append(tools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}

	maxIter := req.MaxIterations
	if maxIter == 0 {
		maxIter = 5
	}

	endpoint := "https://api.openai.com/v1/chat/completions"
	if p.cfg.BaseURL != "" {
		endpoint = strings.TrimSuffix(p.cfg.BaseURL, "/") + "/v1/chat/completions"
	}

	// Tool execution loop
	for iteration := 0; iteration < maxIter; iteration++ {
		// Determine tool_choice: force on first iteration if requested, then auto
		toolChoice := "auto"
		if iteration == 0 && req.ForceToolUse && len(tools) > 0 {
			toolChoice = "required"
		}

		reqBody := map[string]interface{}{
			"model":       p.cfg.Model,
			"messages":    messages,
			"tools":       tools,
			"tool_choice": toolChoice,
			"max_tokens":  p.cfg.MaxTokens,
			"temperature": p.cfg.Temperature,
		}

		body, _ := json.Marshal(reqBody)
		httpReq, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

		resp, err := p.cfg.HTTPClient.Do(httpReq)
		if err != nil {
			return "", err
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result map[string]interface{}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return "", fmt.Errorf("parse response: %w", err)
		}

		if errObj, ok := result["error"].(map[string]interface{}); ok {
			return "", fmt.Errorf("OpenAI error: %v", errObj["message"])
		}

		choices, _ := result["choices"].([]interface{})
		if len(choices) == 0 {
			return "", fmt.Errorf("no response from OpenAI")
		}

		choice := choices[0].(map[string]interface{})
		message := choice["message"].(map[string]interface{})
		finishReason, _ := choice["finish_reason"].(string)

		toolCalls, hasToolCalls := message["tool_calls"].([]interface{})
		if !hasToolCalls || len(toolCalls) == 0 || finishReason == "stop" {
			content, _ := message["content"].(string)
			return content, nil
		}

		// Add assistant message with tool calls
		messages = append(messages, message)

		// Execute tools
		for _, tc := range toolCalls {
			tcMap := tc.(map[string]interface{})
			fn := tcMap["function"].(map[string]interface{})
			tcID, _ := tcMap["id"].(string)

			toolCall := ToolCall{
				ID:        tcID,
				Name:      fn["name"].(string),
				Arguments: json.RawMessage(fn["arguments"].(string)),
			}

			toolResult, err := executeTool(ctx, toolCall)
			resultContent := ""
			if err != nil {
				resultContent = fmt.Sprintf("Error: %s", err.Error())
			} else if !toolResult.Success {
				resultContent = "Error: " + toolResult.Error
			} else {
				resultContent = toolResult.Result
			}

			messages = append(messages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": tcID,
				"content":      resultContent,
			})
		}
	}

	return "", fmt.Errorf("max tool iterations exceeded")
}

// ═══════════════════════════════════════════════════════════════════════════════
// Anthropic Provider
// ═══════════════════════════════════════════════════════════════════════════════

type anthropicProvider struct {
	cfg ProviderConfig
}

func (p *anthropicProvider) Name() string { return "anthropic" }

func (p *anthropicProvider) Chat(ctx context.Context, req ChatCompletionRequest, executeTool ToolExecutor) (string, error) {
	// Build system prompt
	var systemParts []string
	systemParts = append(systemParts, req.SystemPrompt)
	if req.MemorySummary != "" {
		systemParts = append(systemParts, "CONVERSATION MEMORY:\n"+req.MemorySummary)
	}
	if req.SemanticContext != "" {
		systemParts = append(systemParts, "RELEVANT CONTEXT:\n"+req.SemanticContext)
	}
	if req.DataContext != "" {
		systemParts = append(systemParts, "Current RunRight data:\n"+req.DataContext)
	}
	systemPrompt := strings.Join(systemParts, "\n\n")

	// Build messages
	var messages []map[string]interface{}
	for _, h := range req.History {
		if h.Role == "user" || h.Role == "assistant" {
			messages = append(messages, map[string]interface{}{
				"role":    h.Role,
				"content": []map[string]interface{}{{"type": "text", "text": h.Content}},
			})
		}
	}
	messages = append(messages, map[string]interface{}{
		"role":    "user",
		"content": []map[string]interface{}{{"type": "text", "text": req.UserMessage}},
	})

	// Convert tools
	var tools []map[string]interface{}
	for _, t := range req.Tools {
		tools = append(tools, map[string]interface{}{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.Parameters,
		})
	}

	maxIter := req.MaxIterations
	if maxIter == 0 {
		maxIter = 5
	}

	endpoint := "https://api.anthropic.com/v1/messages"
	if p.cfg.BaseURL != "" {
		endpoint = strings.TrimSuffix(p.cfg.BaseURL, "/") + "/v1/messages"
	}

	// Tool execution loop
	for iteration := 0; iteration < maxIter; iteration++ {
		// Determine tool_choice: force on first iteration if requested, then auto
		toolChoice := map[string]string{"type": "auto"}
		if iteration == 0 && req.ForceToolUse && len(tools) > 0 {
			toolChoice = map[string]string{"type": "any"} // Anthropic uses "any" to require a tool call
		}

		reqBody := map[string]interface{}{
			"model":       p.cfg.Model,
			"system":      systemPrompt,
			"messages":    messages,
			"tools":       tools,
			"tool_choice": toolChoice,
			"max_tokens":  p.cfg.MaxTokens,
		}

		body, _ := json.Marshal(reqBody)
		httpReq, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", p.cfg.APIKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")

		resp, err := p.cfg.HTTPClient.Do(httpReq)
		if err != nil {
			return "", err
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result map[string]interface{}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return "", fmt.Errorf("parse response: %w", err)
		}

		if errObj, ok := result["error"].(map[string]interface{}); ok {
			return "", fmt.Errorf("Anthropic error: %v", errObj["message"])
		}

		content, _ := result["content"].([]interface{})
		stopReason, _ := result["stop_reason"].(string)

		// Separate tool uses and text
		var toolUses []map[string]interface{}
		var textParts []string
		for _, c := range content {
			cMap := c.(map[string]interface{})
			cType, _ := cMap["type"].(string)
			if cType == "tool_use" {
				toolUses = append(toolUses, cMap)
			} else if cType == "text" {
				text, _ := cMap["text"].(string)
				textParts = append(textParts, text)
			}
		}

		if len(toolUses) == 0 || stopReason == "end_turn" {
			return strings.Join(textParts, ""), nil
		}

		// Add assistant message
		messages = append(messages, map[string]interface{}{"role": "assistant", "content": content})

		// Execute tools
		var toolResults []map[string]interface{}
		for _, tu := range toolUses {
			tuID, _ := tu["id"].(string)
			tuName, _ := tu["name"].(string)
			tuInput, _ := json.Marshal(tu["input"])

			toolCall := ToolCall{ID: tuID, Name: tuName, Arguments: tuInput}
			toolResult, err := executeTool(ctx, toolCall)

			resultContent := ""
			if err != nil {
				resultContent = fmt.Sprintf("Error: %s", err.Error())
			} else if !toolResult.Success {
				resultContent = "Error: " + toolResult.Error
			} else {
				resultContent = toolResult.Result
			}

			toolResults = append(toolResults, map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": tuID,
				"content":     resultContent,
			})
		}
		messages = append(messages, map[string]interface{}{"role": "user", "content": toolResults})
	}

	return "", fmt.Errorf("max tool iterations exceeded")
}

// ═══════════════════════════════════════════════════════════════════════════════
// Ollama Provider
// ═══════════════════════════════════════════════════════════════════════════════

type ollamaProvider struct {
	cfg ProviderConfig
}

func (p *ollamaProvider) Name() string { return "ollama" }

func (p *ollamaProvider) Chat(ctx context.Context, req ChatCompletionRequest, executeTool ToolExecutor) (string, error) {
	// Build messages
	messages := []map[string]interface{}{
		{"role": "system", "content": req.SystemPrompt},
	}

	if req.MemorySummary != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": "CONVERSATION MEMORY:\n" + req.MemorySummary,
		})
	}
	if req.SemanticContext != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": "RELEVANT CONTEXT:\n" + req.SemanticContext,
		})
	}
	if req.DataContext != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": "Current RunRight data:\n" + req.DataContext,
		})
	}

	for _, h := range req.History {
		if h.Role == "user" || h.Role == "assistant" {
			messages = append(messages, map[string]interface{}{"role": h.Role, "content": h.Content})
		}
	}
	messages = append(messages, map[string]interface{}{"role": "user", "content": req.UserMessage})

	// Convert tools
	var tools []map[string]interface{}
	for _, t := range req.Tools {
		tools = append(tools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}

	maxIter := req.MaxIterations
	if maxIter == 0 {
		maxIter = 5
	}

	endpoint := strings.TrimSuffix(p.cfg.BaseURL, "/") + "/api/chat"

	// Tool execution loop
	for iteration := 0; iteration < maxIter; iteration++ {
		reqBody := map[string]interface{}{
			"model":    p.cfg.Model,
			"messages": messages,
			"tools":    tools,
			"stream":   false,
			"options": map[string]interface{}{
				"temperature": p.cfg.Temperature,
				"num_predict": p.cfg.MaxTokens,
			},
		}

		// Force tool use on first iteration if requested (Ollama 0.5.1+ supports tool_choice)
		if iteration == 0 && req.ForceToolUse && len(tools) > 0 {
			reqBody["tool_choice"] = "required"
		}

		body, _ := json.Marshal(reqBody)
		httpReq, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := p.cfg.HTTPClient.Do(httpReq)
		if err != nil {
			return "", fmt.Errorf("ollama request failed: %w (is Ollama running at %s?)", err, p.cfg.BaseURL)
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(respBody))
		}

		var result map[string]interface{}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return "", fmt.Errorf("parse ollama response: %w", err)
		}

		if errStr, ok := result["error"].(string); ok && errStr != "" {
			return "", fmt.Errorf("ollama error: %s", errStr)
		}

		message, _ := result["message"].(map[string]interface{})
		content, _ := message["content"].(string)
		toolCalls, hasToolCalls := message["tool_calls"].([]interface{})

		// Fallback: Some models output JSON-formatted function calls in content
		// instead of using the tool_calls field. Parse and convert them.
		if (!hasToolCalls || len(toolCalls) == 0) && strings.TrimSpace(content) != "" {
			if parsed := parseJSONFunctionCall(content); parsed != nil {
				toolCalls = []interface{}{parsed}
				hasToolCalls = true
			}
		}

		if !hasToolCalls || len(toolCalls) == 0 {
			return content, nil
		}

		// Add assistant message with tool calls
		messages = append(messages, message)

		// Execute tools
		for _, tc := range toolCalls {
			tcMap := tc.(map[string]interface{})
			fn, _ := tcMap["function"].(map[string]interface{})
			fnName, _ := fn["name"].(string)
			fnArgs, _ := json.Marshal(fn["arguments"])

			toolCall := ToolCall{
				ID:        fmt.Sprintf("ollama-%d-%s", iteration, fnName),
				Name:      fnName,
				Arguments: fnArgs,
			}

			toolResult, err := executeTool(ctx, toolCall)
			resultContent := ""
			if err != nil {
				resultContent = fmt.Sprintf("Error: %s", err.Error())
			} else if !toolResult.Success {
				resultContent = "Error: " + toolResult.Error
			} else {
				resultContent = toolResult.Result
			}

			messages = append(messages, map[string]interface{}{
				"role":    "tool",
				"content": resultContent,
			})
		}
	}

	return "", fmt.Errorf("max tool iterations exceeded")
}

// parseJSONFunctionCall attempts to parse a JSON-formatted function call from content.
// Some models output function calls as JSON text instead of using the tool_calls field.
// Supports formats like:
//   {"function": "name", "args": {...}}
//   {"name": "func_name", "arguments": {...}}
//   {"function": {"name": "...", "arguments": {...}}}
func parseJSONFunctionCall(content string) map[string]interface{} {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "{") {
		return nil
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil
	}

	// Format 1: {"function": "name", "args": {...}}
	if fnName, ok := parsed["function"].(string); ok {
		args := parsed["args"]
		if args == nil {
			args = parsed["arguments"]
		}
		if args == nil {
			args = map[string]interface{}{}
		}
		return map[string]interface{}{
			"function": map[string]interface{}{
				"name":      fnName,
				"arguments": args,
			},
		}
	}

	// Format 2: {"name": "func_name", "arguments": {...}}
	if fnName, ok := parsed["name"].(string); ok {
		args := parsed["arguments"]
		if args == nil {
			args = parsed["args"]
		}
		if args == nil {
			args = map[string]interface{}{}
		}
		return map[string]interface{}{
			"function": map[string]interface{}{
				"name":      fnName,
				"arguments": args,
			},
		}
	}

	// Format 3: {"function": {"name": "...", "arguments": {...}}}
	if fn, ok := parsed["function"].(map[string]interface{}); ok {
		if _, hasName := fn["name"]; hasName {
			return parsed // Already in correct format
		}
	}

	return nil
}
