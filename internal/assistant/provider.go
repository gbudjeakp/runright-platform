package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
			switch cType {
			case "tool_use":
				toolUses = append(toolUses, cMap)
			case "text":
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

		// Check if tool_calls have empty arguments (Ollama bug - sometimes returns
		// tool_calls with empty args but puts real args in content text)
		shouldFallback := !hasToolCalls || len(toolCalls) == 0
		if hasToolCalls && len(toolCalls) > 0 {
			// Check if arguments are empty
			tc := toolCalls[0].(map[string]interface{})
			if fn, ok := tc["function"].(map[string]interface{}); ok {
				if args, ok := fn["arguments"].(map[string]interface{}); ok && len(args) == 0 {
					shouldFallback = true
				}
			}
		}

		// Fallback: Some models output function calls in content as JSON or text
		// instead of using the tool_calls field. Parse and convert them.
		if shouldFallback && strings.TrimSpace(content) != "" {
			if parsed := parseFunctionCall(content); parsed != nil {
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
			fnArgsRaw := fn["arguments"]
			// Ollama sometimes wraps arguments in {"object": {...}} - unwrap if present
			if argsMap, ok := fnArgsRaw.(map[string]interface{}); ok {
				if obj, hasObj := argsMap["object"].(map[string]interface{}); hasObj {
					fnArgsRaw = obj
				}
			}
			fnArgs, _ := json.Marshal(fnArgsRaw)

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

// parseFunctionCall attempts to parse a function call from various content formats.
// Returns nil if no valid function call is found.
func parseFunctionCall(content string) map[string]interface{} {
	content = strings.TrimSpace(content)
	
	// Try JSON parsing first
	if result := parseJSONFunctionCall(content); result != nil {
		return result
	}
	
	// Try text format: [CALLS function_name with: key=value, key="value"]
	// or: [CALLS function_name immediately with: key=value]
	if result := parseTextFunctionCall(content); result != nil {
		return result
	}
	
	return nil
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

	// Format 1: {"function": "name", "args": {...}} or {"function": "name", "parameters": {...}}
	if fnName, ok := parsed["function"].(string); ok {
		args := parsed["args"]
		if args == nil {
			args = parsed["arguments"]
		}
		if args == nil {
			args = parsed["parameters"]
		}
		if args == nil {
			args = parsed["params"]
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

	// Format 2: {"name": "func_name", "arguments": {...}} or {"name": "func_name", "parameters": {...}}
	if fnName, ok := parsed["name"].(string); ok {
		args := parsed["arguments"]
		if args == nil {
			args = parsed["args"]
		}
		if args == nil {
			args = parsed["parameters"]
		}
		if args == nil {
			args = parsed["params"]
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

// parseTextFunctionCall parses text-formatted function calls like:
// [CALLS create_alert_rule with: name="test", threshold=90]
// [CALLS create_alert_rule immediately with: name="test"]
// *calls create_alert_rule*
// *calls create_alert_rule with name="test"*
func parseTextFunctionCall(content string) map[string]interface{} {
	content = strings.TrimSpace(content)
	
	// Try [CALLS ...] format first
	if result := parseBracketCalls(content); result != nil {
		return result
	}
	
	// Try *calls ...* format (asterisk-wrapped)
	if result := parseAsteriskCalls(content); result != nil {
		return result
	}
	
	return nil
}

// parseBracketCalls handles [CALLS function_name ...] format
func parseBracketCalls(content string) map[string]interface{} {
	callsIdx := strings.Index(strings.ToUpper(content), "[CALLS ")
	if callsIdx == -1 {
		return nil
	}
	
	closeIdx := strings.Index(content[callsIdx:], "]")
	if closeIdx == -1 {
		return nil
	}
	
	callContent := content[callsIdx+7 : callsIdx+closeIdx]
	return parseCallContent(callContent)
}

// parseAsteriskCalls handles *calls function_name* format
// It finds all *calls ...* patterns and returns the one with arguments (preferring calls with args)
func parseAsteriskCalls(content string) map[string]interface{} {
	lower := strings.ToLower(content)
	
	var bestResult map[string]interface{}
	var bestArgCount int
	
	// Find all "*calls " patterns
	searchStart := 0
	for {
		callsIdx := strings.Index(lower[searchStart:], "*calls ")
		if callsIdx == -1 {
			break
		}
		callsIdx += searchStart
		
		// Find the closing asterisk or end of line
		startContent := callsIdx + 7
		closeIdx := strings.Index(content[startContent:], "*")
		if closeIdx == -1 {
			closeIdx = strings.Index(content[startContent:], "\n")
			if closeIdx == -1 {
				closeIdx = len(content) - startContent
			}
		}
		
		callContent := content[startContent : startContent+closeIdx]
		result := parseCallContent(callContent)
		
		if result != nil {
			// Count arguments
			argCount := 0
			if fn, ok := result["function"].(map[string]interface{}); ok {
				if args, ok := fn["arguments"].(map[string]interface{}); ok {
					argCount = len(args)
				}
			}
			
			// Prefer the call with the most arguments
			if bestResult == nil || argCount > bestArgCount {
				bestResult = result
				bestArgCount = argCount
			}
		}
		
		searchStart = startContent + closeIdx + 1
		if searchStart >= len(content) {
			break
		}
	}
	
	return bestResult
}

// parseCallContent extracts function name and args from various formats:
// - "function_name with: key=value, key=value"
// - "function_name(key=value, key=value)"
// - "function_name(key=\"value\")"
func parseCallContent(callContent string) map[string]interface{} {
	callContent = strings.TrimSpace(callContent)
	
	// Remove "immediately" if present
	callContent = strings.Replace(callContent, " immediately", "", 1)
	
	var funcName string
	var argsStr string
	
	// Check for parentheses format first: function_name(args)
	if parenIdx := strings.Index(callContent, "("); parenIdx != -1 {
		funcName = strings.TrimSpace(callContent[:parenIdx])
		// Find matching closing paren
		closeIdx := strings.LastIndex(callContent, ")")
		if closeIdx > parenIdx {
			argsStr = callContent[parenIdx+1 : closeIdx]
		} else {
			argsStr = callContent[parenIdx+1:]
		}
	} else if idx := strings.Index(strings.ToLower(callContent), " with:"); idx != -1 {
		// "with:" format
		funcName = strings.TrimSpace(callContent[:idx])
		argsStr = strings.TrimSpace(callContent[idx+6:])
	} else if idx := strings.Index(strings.ToLower(callContent), " with "); idx != -1 {
		// "with " format  
		funcName = strings.TrimSpace(callContent[:idx])
		argsStr = strings.TrimSpace(callContent[idx+6:])
	} else {
		// Just function name, no args
		funcName = callContent
	}
	
	if funcName == "" {
		return nil
	}
	
	args := map[string]interface{}{}
	if argsStr != "" {
		args = parseKeyValueArgs(argsStr)
	}
	
	return map[string]interface{}{
		"function": map[string]interface{}{
			"name":      funcName,
			"arguments": args,
		},
	}
}

// parseKeyValueArgs parses "key=value, key=\"value\", key=123" format
func parseKeyValueArgs(s string) map[string]interface{} {
	args := map[string]interface{}{}
	
	// Simple regex-free parsing
	// Split by comma, but be careful with quoted strings
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)
	pairs := []string{}
	
	for _, ch := range s {
		if !inQuote && (ch == '"' || ch == '\'') {
			inQuote = true
			quoteChar = ch
			current.WriteRune(ch)
		} else if inQuote && ch == quoteChar {
			inQuote = false
			current.WriteRune(ch)
		} else if !inQuote && ch == ',' {
			pairs = append(pairs, current.String())
			current.Reset()
		} else {
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		pairs = append(pairs, current.String())
	}
	
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		eqIdx := strings.Index(pair, "=")
		if eqIdx == -1 {
			continue
		}
		
		key := strings.TrimSpace(pair[:eqIdx])
		val := strings.TrimSpace(pair[eqIdx+1:])
		
		// Remove quotes from value
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		
		// Try to parse as number
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			args[key] = i
		} else if f, err := strconv.ParseFloat(val, 64); err == nil {
			args[key] = f
		} else if val == "true" {
			args[key] = true
		} else if val == "false" {
			args[key] = false
		} else {
			args[key] = val
		}
	}
	
	return args
}
