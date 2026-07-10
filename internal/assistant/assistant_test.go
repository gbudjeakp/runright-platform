package assistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sgbudje/runright-platform/internal/types"
)

// ─── Config Tests ────────────────────────────────────────────────────────────

func TestNew_Defaults(t *testing.T) {
	tests := []struct {
		name          string
		cfg           Config
		wantModel     string
		wantMaxJobs   int
		wantBaseURL   string
	}{
		{
			name:        "OpenAI defaults",
			cfg:         Config{Provider: ProviderOpenAI, APIKey: "sk-test"},
			wantModel:   "gpt-4o",
			wantMaxJobs: 100,
		},
		{
			name:        "Anthropic defaults",
			cfg:         Config{Provider: ProviderAnthropic, APIKey: "sk-ant-test"},
			wantModel:   "claude-sonnet-4-20250514",
			wantMaxJobs: 100,
		},
		{
			name:        "Ollama defaults",
			cfg:         Config{Provider: ProviderOllama},
			wantModel:   "llama3.2",
			wantMaxJobs: 100,
			wantBaseURL: "http://localhost:11434",
		},
		{
			name:        "Custom model override",
			cfg:         Config{Provider: ProviderOpenAI, APIKey: "sk-test", Model: "gpt-4-turbo"},
			wantModel:   "gpt-4-turbo",
			wantMaxJobs: 100,
		},
		{
			name:        "Custom MaxContextJobs",
			cfg:         Config{Provider: ProviderOpenAI, APIKey: "sk-test", MaxContextJobs: 50},
			wantModel:   "gpt-4o",
			wantMaxJobs: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(nil, tt.cfg)
			if a.cfg.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", a.cfg.Model, tt.wantModel)
			}
			if a.cfg.MaxContextJobs != tt.wantMaxJobs {
				t.Errorf("MaxContextJobs = %d, want %d", a.cfg.MaxContextJobs, tt.wantMaxJobs)
			}
			if tt.wantBaseURL != "" && a.cfg.BaseURL != tt.wantBaseURL {
				t.Errorf("BaseURL = %q, want %q", a.cfg.BaseURL, tt.wantBaseURL)
			}
		})
	}
}

func TestIsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "OpenAI with API key",
			cfg:  Config{Provider: ProviderOpenAI, APIKey: "sk-test"},
			want: true,
		},
		{
			name: "OpenAI without API key",
			cfg:  Config{Provider: ProviderOpenAI},
			want: false,
		},
		{
			name: "Anthropic with API key",
			cfg:  Config{Provider: ProviderAnthropic, APIKey: "sk-ant-test"},
			want: true,
		},
		{
			name: "Anthropic without API key",
			cfg:  Config{Provider: ProviderAnthropic},
			want: false,
		},
		{
			name: "Ollama with default URL",
			cfg:  Config{Provider: ProviderOllama},
			want: true, // BaseURL defaults to localhost
		},
		{
			name: "Ollama with custom URL",
			cfg:  Config{Provider: ProviderOllama, BaseURL: "http://remote:11434"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(nil, tt.cfg)
			if got := a.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetProviderInfo(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		wantProvider string
		wantModel    string
		wantBaseURL  bool
	}{
		{
			name:         "OpenAI",
			cfg:          Config{Provider: ProviderOpenAI, APIKey: "test"},
			wantProvider: "openai",
			wantModel:    "gpt-4o",
			wantBaseURL:  false,
		},
		{
			name:         "Ollama includes base_url",
			cfg:          Config{Provider: ProviderOllama, BaseURL: "http://test:11434"},
			wantProvider: "ollama",
			wantModel:    "llama3.2",
			wantBaseURL:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(nil, tt.cfg)
			info := a.GetProviderInfo()
			if info["provider"] != tt.wantProvider {
				t.Errorf("provider = %q, want %q", info["provider"], tt.wantProvider)
			}
			if info["model"] != tt.wantModel {
				t.Errorf("model = %q, want %q", info["model"], tt.wantModel)
			}
			_, hasBaseURL := info["base_url"]
			if hasBaseURL != tt.wantBaseURL {
				t.Errorf("has base_url = %v, want %v", hasBaseURL, tt.wantBaseURL)
			}
		})
	}
}

// ─── Helper Function Tests ──────────────────────────────────────────────────

func TestSummarizeQuestion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int  // Max expected length
		wantDots bool // Should end with "..."
	}{
		{"short", "What repos do we have?", 23, false},
		{"very short", "Short", 5, false},
		{"empty", "", 0, false},
		{"long truncation", "This is a very long question that should be truncated because it exceeds fifty characters", 50, true},
		{"trimmed", "  trimmed whitespace  ", 20, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeQuestion(tt.input)
			if len(got) > tt.wantLen+3 { // +3 for "..."
				t.Errorf("summarizeQuestion(%q) len = %d, want <= %d", tt.input, len(got), tt.wantLen+3)
			}
			if tt.wantDots && len(got) > 3 {
				if got[len(got)-3:] != "..." {
					t.Errorf("summarizeQuestion(%q) = %q, want to end with '...'", tt.input, got)
				}
			}
		})
	}
}

// ─── Ollama Integration Tests ───────────────────────────────────────────────

func TestCallOllama_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", 405)
			return
		}

		resp := map[string]any{
			"message": map[string]string{
				"role":    "assistant",
				"content": "You have 5 repositories in your system.",
			},
			"done": true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	a := New(nil, Config{
		Provider: ProviderOllama,
		BaseURL:  server.URL,
		Model:    "llama3.2",
	})

	ctx := context.Background()
	history := []types.ChatMessage{}
	assistantCtx := &types.AssistantContext{
		Repositories: []string{"repo1", "repo2", "repo3", "repo4", "repo5"},
	}

	response, err := a.callLLM(ctx, "", history, assistantCtx, "How many repos?", "")
	if err != nil {
		t.Fatalf("callOllama failed: %v", err)
	}
	if response != "You have 5 repositories in your system." {
		t.Errorf("unexpected response: %q", response)
	}
}

func TestCallOllama_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"error": "model not found",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	a := New(nil, Config{
		Provider: ProviderOllama,
		BaseURL:  server.URL,
		Model:    "nonexistent",
	})

	ctx := context.Background()
	_, err := a.callLLM(ctx, "", nil, &types.AssistantContext{}, "test", "")
	if err == nil {
		t.Error("expected error for model not found")
	}
}

func TestCallOllama_ConnectionError(t *testing.T) {
	a := New(nil, Config{
		Provider: ProviderOllama,
		BaseURL:  "http://localhost:99999", // Invalid port
		Model:    "llama3.2",
	})

	ctx := context.Background()
	_, err := a.callLLM(ctx, "", nil, &types.AssistantContext{}, "test", "")
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestCallOllama_ContextTruncation(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		resp := map[string]any{
			"message": map[string]string{"role": "assistant", "content": "ok"},
			"done":    true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	a := New(nil, Config{
		Provider: ProviderOllama,
		BaseURL:  server.URL,
		Model:    "llama3.2",
	})

	// Create very large context
	largeContext := &types.AssistantContext{
		Repositories: make([]string, 1000),
	}
	for i := range largeContext.Repositories {
		largeContext.Repositories[i] = "very-long-repository-name-that-takes-up-space-" + string(rune('a'+i%26))
	}

	_, err := a.callLLM(context.Background(), "", nil, largeContext, "test", "")
	if err != nil {
		t.Fatalf("callOllama failed: %v", err)
	}

	if receivedBody == nil {
		t.Error("no request body received")
	}
}

func TestCallOllama_History(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		resp := map[string]any{
			"message": map[string]string{"role": "assistant", "content": "ok"},
			"done":    true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	a := New(nil, Config{
		Provider: ProviderOllama,
		BaseURL:  server.URL,
		Model:    "llama3.2",
	})

	history := []types.ChatMessage{
		{Role: "user", Content: "First question"},
		{Role: "assistant", Content: "First answer"},
		{Role: "user", Content: "Second question"},
		{Role: "assistant", Content: "Second answer"},
	}

	_, err := a.callLLM(context.Background(), "", history, &types.AssistantContext{}, "Third question", "")
	if err != nil {
		t.Fatalf("callOllama failed: %v", err)
	}

	messages := receivedBody["messages"].([]any)
	if len(messages) < 4 {
		t.Errorf("expected at least 4 messages, got %d", len(messages))
	}

	lastMsg := messages[len(messages)-1].(map[string]any)
	if lastMsg["content"] != "Third question" {
		t.Errorf("last message content = %q, want 'Third question'", lastMsg["content"])
	}
}

// ─── Edge Cases ─────────────────────────────────────────────────────────────

func TestChat_NotConfigured(t *testing.T) {
	a := New(nil, Config{Provider: ProviderOpenAI}) // No API key

	_, err := a.Chat(context.Background(), types.ChatRequest{Message: "test"}, "")
	if err == nil {
		t.Error("expected error for unconfigured assistant")
	}
}

func TestSystemPrompt(t *testing.T) {
	a := New(nil, Config{Provider: ProviderOllama})
	prompt := a.systemPrompt()

	if len(prompt) < 100 {
		t.Errorf("system prompt too short: %d characters", len(prompt))
	}

	if !strings.Contains(prompt, "RunRight") {
		t.Error("system prompt should mention RunRight")
	}

	if !strings.Contains(prompt, "CI/CD") {
		t.Error("system prompt should mention CI/CD")
	}
}

// ─── Benchmark ──────────────────────────────────────────────────────────────

func BenchmarkSummarizeQuestion(b *testing.B) {
	questions := []string{
		"Short question",
		"This is a much longer question that will need to be truncated at some point",
		"A question with no obvious break points at the fifty character boundary location",
	}
	for i := 0; i < b.N; i++ {
		for _, q := range questions {
			summarizeQuestion(q)
		}
	}
}

func BenchmarkBuildContextJSON(b *testing.B) {
	ctx := &types.AssistantContext{
		Jobs: make([]types.MetricsSummary, 100),
		Repositories: []string{
			"org/repo1", "org/repo2", "org/repo3",
		},
	}
	for i := range ctx.Jobs {
		ctx.Jobs[i] = types.MetricsSummary{
			JobID:         "job-" + string(rune('a'+i%26)),
			Repository:    "org/repo1",
			CPUPercentP95: float64(50 + i%50),
			MemUsedGiBP95: float64(2 + i%6),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(ctx)
	}
}

// ─── Memory / Token Tests ─────────────────────────────────────────────────────

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		wantMin  int
		wantMax  int
	}{
		{"", 0, 0},
		{"hello", 1, 3},
		{strings.Repeat("a", 400), 99, 101}, // ~100 tokens
		{strings.Repeat("word ", 100), 100, 130}, // ~120 tokens
	}
	for _, tt := range tests {
		got := estimateTokens(tt.input)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("estimateTokens(%d chars) = %d, want %d-%d", len(tt.input), got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestTrimContext_BudgetRespected(t *testing.T) {
	// Build a large context with many jobs.
	ctx := &types.AssistantContext{
		Jobs: make([]types.MetricsSummary, 200),
		Savings: &types.SavingsSnapshot{
			TotalPotentialSavingsUSD: 1000,
			SavingsPercent:           25,
		},
	}
	for i := range ctx.Jobs {
		ctx.Jobs[i] = types.MetricsSummary{
			JobID:      "job-" + strings.Repeat("x", 50),
			Repository: "org/repo",
		}
	}

	budget := 1000 // 1000 tokens
	trimmed := trimContext(ctx, budget)

	// Marshalling the trimmed context should not exceed budget significantly.
	b, _ := json.Marshal(trimmed)
	tokens := estimateTokens(string(b))
	if tokens > budget*2 { // allow some overhead for JSON structure
		t.Errorf("trimmed context used %d tokens, expected ≤%d", tokens, budget*2)
	}

	// Should still have savings (small, high priority).
	if trimmed.Savings == nil {
		t.Error("savings should be preserved (small, high priority)")
	}

	// Should have fewer jobs than original.
	if len(trimmed.Jobs) >= len(ctx.Jobs) {
		t.Errorf("expected fewer jobs after trimming, got %d", len(trimmed.Jobs))
	}
}

func TestTrimContext_SemanticContextPriority(t *testing.T) {
	semanticCtx := strings.Repeat("semantic result ", 20) // ~320 chars / ~80 tokens
	ctx := &types.AssistantContext{
		SemanticContext: semanticCtx,
		Jobs: func() []types.MetricsSummary {
			jobs := make([]types.MetricsSummary, 100)
			for i := range jobs {
				jobs[i] = types.MetricsSummary{JobID: "job-" + strings.Repeat("y", 40)}
			}
			return jobs
		}(),
	}

	// With a very tight budget the semantic context should be preserved over jobs.
	trimmed := trimContext(ctx, 100)
	if trimmed.SemanticContext == "" {
		t.Error("semantic context should survive tight budget (highest priority)")
	}
}

func TestTrimHistory_BudgetRespected(t *testing.T) {
	history := make([]types.ChatMessage, 20)
	for i := range history {
		history[i] = types.ChatMessage{
			Role:    "user",
			Content: strings.Repeat("word ", 100), // ~500 chars / ~125 tokens each
		}
	}

	budget := 500 // 500 tokens
	trimmed := trimHistory(history, budget)

	total := 0
	for _, m := range trimmed {
		total += estimateTokens(m.Content)
	}
	if total > budget {
		t.Errorf("trimmed history used %d tokens, exceeds budget %d", total, budget)
	}

	// Should keep most recent messages (not oldest).
	if len(trimmed) > 0 && len(history) > 0 {
		last := history[len(history)-1]
		trimmedLast := trimmed[len(trimmed)-1]
		if last.Content != trimmedLast.Content {
			t.Error("trimHistory should preserve the most recent messages")
		}
	}
}

func TestTrimHistory_Empty(t *testing.T) {
	result := trimHistory(nil, 1000)
	if len(result) != 0 {
		t.Errorf("expected empty result for nil input, got %d messages", len(result))
	}
}

func TestCallOllama_MemorySummaryInjected(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		resp := map[string]any{
			"message": map[string]string{"role": "assistant", "content": "ok"},
			"done":    true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	a := New(nil, Config{Provider: ProviderOllama, BaseURL: server.URL, Model: "llama3.1"})

	summary := "User asked about repo1 costs; assistant said $120/month."
	_, err := a.callLLM(context.Background(), summary, nil, &types.AssistantContext{}, "Follow-up question", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	messages, ok := receivedBody["messages"].([]any)
	if !ok {
		t.Fatal("no messages in request body")
	}

	// Check that the memory summary appears in one of the system messages.
	found := false
	for _, m := range messages {
		msg := m.(map[string]any)
		if msg["role"] == "system" && strings.Contains(msg["content"].(string), "CONVERSATION MEMORY") {
			found = true
			break
		}
	}
	if !found {
		t.Error("memory summary should be injected as a system message")
	}
}

func TestTokenBudgets(t *testing.T) {
	providers := []LLMProvider{ProviderOllama, ProviderOpenAI, ProviderAnthropic}
	for _, p := range providers {
		a := New(nil, Config{Provider: p})
		b := a.tokenBudgets()
		if b.contextBudget <= 0 {
			t.Errorf("provider %s: contextBudget must be positive", p)
		}
		if b.historyBudget <= 0 {
			t.Errorf("provider %s: historyBudget must be positive", p)
		}
		// Ollama should have tighter budgets than cloud providers.
		if p == ProviderOllama {
			openai := New(nil, Config{Provider: ProviderOpenAI}).tokenBudgets()
			if b.contextBudget >= openai.contextBudget {
				t.Error("Ollama should have a smaller context budget than OpenAI")
			}
		}
	}
}

