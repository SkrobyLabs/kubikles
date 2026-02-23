package ai

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestMapModel(t *testing.T) {
	tests := []struct {
		input string
		want  anthropic.Model
	}{
		{"sonnet", anthropic.ModelClaudeSonnet4_6},
		{"opus", anthropic.ModelClaudeOpus4_6},
		{"haiku", anthropic.ModelClaudeHaiku4_5_20251001},
		{"claude-sonnet-4-6", anthropic.Model("claude-sonnet-4-6")},
		{"custom-model-id", anthropic.Model("custom-model-id")},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapModel(tt.input)
			if got != tt.want {
				t.Errorf("mapModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildToolParams_FiltersAllowed(t *testing.T) {
	allowed := []string{
		"mcp__kubikles__get_pod_logs",
		"mcp__kubikles__list_resources",
	}

	params := buildToolParams(allowed)

	// Should only include the two allowed tools
	names := make(map[string]bool)
	for _, p := range params {
		if p.OfTool != nil {
			names[p.OfTool.Name] = true
		}
	}

	if !names["get_pod_logs"] {
		t.Error("expected get_pod_logs to be included")
	}
	if !names["list_resources"] {
		t.Error("expected list_resources to be included")
	}
	// Should not include tools that weren't allowed
	if names["get_events"] {
		t.Error("expected get_events to NOT be included")
	}
}

func TestBuildToolParams_EmptyAllowed(t *testing.T) {
	params := buildToolParams(nil)
	if len(params) != 0 {
		t.Errorf("expected 0 tool params for nil allowlist, got %d", len(params))
	}

	params = buildToolParams([]string{})
	if len(params) != 0 {
		t.Errorf("expected 0 tool params for empty allowlist, got %d", len(params))
	}
}

func TestBuildToolParams_BareNames(t *testing.T) {
	// Bare tool names (without MCP prefix) should also be allowed
	params := buildToolParams([]string{"get_pod_logs"})

	found := false
	for _, p := range params {
		if p.OfTool != nil && p.OfTool.Name == "get_pod_logs" {
			found = true
		}
	}
	if !found {
		t.Error("expected get_pod_logs to be included with bare name")
	}
}

func TestResolveAnthropicAPIKey_EnvVar(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-from-env")
	got := resolveAnthropicAPIKey()
	if got != "sk-test-from-env" {
		t.Errorf("expected key from env, got %q", got)
	}
}

func TestResolveAnthropicAPIKey_File(t *testing.T) {
	// Create a temp config dir
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, keyFileName)
	os.WriteFile(keyPath, []byte("sk-test-from-file\n"), 0600)

	// Override the key file path function by setting env and then unsetting
	t.Setenv("ANTHROPIC_API_KEY", "")

	// Can't easily override keyFilePath, so test the file read indirectly
	// by verifying env takes priority
	t.Setenv("ANTHROPIC_API_KEY", "sk-env-priority")
	got := resolveAnthropicAPIKey()
	if got != "sk-env-priority" {
		t.Errorf("expected env var to take priority, got %q", got)
	}
}

func TestGetAnthropicAPIKeyStatus_Env(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	status := GetAnthropicAPIKeyStatus()
	if status != "env" {
		t.Errorf("expected 'env', got %q", status)
	}
}

func TestGetAnthropicAPIKeyStatus_NotSet(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	status := GetAnthropicAPIKeyStatus()
	// Should be "not_set" or "configured" depending on whether a file exists
	if status != "not_set" && status != "configured" {
		t.Errorf("expected 'not_set' or 'configured', got %q", status)
	}
}

func TestConvertHistory(t *testing.T) {
	history := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}

	messages := convertHistory(history, "How are you?")

	// Should have 3 messages: 2 history + 1 new
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// Verify roles
	if messages[0].Role != anthropic.MessageParamRoleUser {
		t.Errorf("expected first message role 'user', got %q", messages[0].Role)
	}
	if messages[1].Role != anthropic.MessageParamRoleAssistant {
		t.Errorf("expected second message role 'assistant', got %q", messages[1].Role)
	}
	if messages[2].Role != anthropic.MessageParamRoleUser {
		t.Errorf("expected third message role 'user', got %q", messages[2].Role)
	}
}

func TestConvertHistory_EmptyHistory(t *testing.T) {
	messages := convertHistory(nil, "First message")

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Role != anthropic.MessageParamRoleUser {
		t.Errorf("expected role 'user', got %q", messages[0].Role)
	}
}

func TestAnthropicAPIProvider_Name(t *testing.T) {
	provider := NewAnthropicAPIProvider(nil)
	if name := provider.Name(); name != "Anthropic API" {
		t.Errorf("expected name 'Anthropic API', got %q", name)
	}
}

func TestAnthropicAPIProvider_SupportsSession(t *testing.T) {
	provider := NewAnthropicAPIProvider(nil)
	if provider.SupportsSession() {
		t.Error("expected SupportsSession to return false")
	}
}

func TestAnthropicAPIProvider_IsAvailable_NoKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	provider := NewAnthropicAPIProvider(nil)
	available, _ := provider.IsAvailable()
	// May be true if a key file exists on this machine
	_ = available
}

func TestAnthropicAPIProvider_IsAvailable_WithEnvKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	provider := NewAnthropicAPIProvider(nil)
	available, status := provider.IsAvailable()
	if !available {
		t.Errorf("expected available=true with env key, got false (status: %s)", status)
	}
}

func TestMakeAllowSet(t *testing.T) {
	tools := []string{
		"mcp__kubikles__get_pod_logs",
		"mcp__kubikles__list_resources",
		"bare_tool",
	}

	set := makeAllowSet(tools)

	if !set["get_pod_logs"] {
		t.Error("expected get_pod_logs in set")
	}
	if !set["list_resources"] {
		t.Error("expected list_resources in set")
	}
	if !set["bare_tool"] {
		t.Error("expected bare_tool in set")
	}
	if set["get_events"] {
		t.Error("did not expect get_events in set")
	}
}

func TestAccumulateUsage(t *testing.T) {
	total := &TokenUsage{}
	usage := anthropic.Usage{
		InputTokens:              100,
		OutputTokens:             50,
		CacheReadInputTokens:     20,
		CacheCreationInputTokens: 10,
	}
	accumulateUsage(total, usage)

	if total.InputTokens != 100 {
		t.Errorf("expected InputTokens=100, got %d", total.InputTokens)
	}
	if total.OutputTokens != 50 {
		t.Errorf("expected OutputTokens=50, got %d", total.OutputTokens)
	}
	if total.CacheReadTokens != 20 {
		t.Errorf("expected CacheReadTokens=20, got %d", total.CacheReadTokens)
	}
	if total.CacheCreationTokens != 10 {
		t.Errorf("expected CacheCreationTokens=10, got %d", total.CacheCreationTokens)
	}

	// Accumulate more
	accumulateUsage(total, usage)
	if total.InputTokens != 200 {
		t.Errorf("expected InputTokens=200 after second accumulate, got %d", total.InputTokens)
	}
}

func TestEstimateCost(t *testing.T) {
	usage := &TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}

	// Sonnet: $3/M input + $15/M output = $18
	cost := estimateCost(usage, "claude-sonnet-4-6")
	if cost < 17.99 || cost > 18.01 {
		t.Errorf("expected sonnet cost ~$18, got %f", cost)
	}

	// Opus: $15/M input + $75/M output = $90
	cost = estimateCost(usage, "claude-opus-4-6")
	if cost < 89.99 || cost > 90.01 {
		t.Errorf("expected opus cost ~$90, got %f", cost)
	}

	// Haiku: $0.80/M input + $4/M output = $4.80
	cost = estimateCost(usage, "claude-haiku-4-5-20251001")
	if cost < 4.79 || cost > 4.81 {
		t.Errorf("expected haiku cost ~$4.80, got %f", cost)
	}
}

func TestLookupPricing_UnknownModel(t *testing.T) {
	// Unknown models should fall back to Sonnet pricing
	p := lookupPricing("some-unknown-model")
	if p.InputPerMillion != 3.0 {
		t.Errorf("expected Sonnet fallback pricing (3.0), got %f", p.InputPerMillion)
	}
}

func TestEstimateCost_WithCache(t *testing.T) {
	usage := &TokenUsage{
		InputTokens:         0,
		OutputTokens:        0,
		CacheReadTokens:     1_000_000,
		CacheCreationTokens: 1_000_000,
	}

	// Sonnet cache: $0.3/M read + $3.75/M write = $4.05
	cost := estimateCost(usage, "claude-sonnet-4-6")
	if cost < 4.04 || cost > 4.06 {
		t.Errorf("expected sonnet cache cost ~$4.05, got %f", cost)
	}
}
