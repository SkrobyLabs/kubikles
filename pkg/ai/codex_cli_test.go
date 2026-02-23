package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexCLIProvider_Name(t *testing.T) {
	provider := NewCodexCLIProvider()
	if name := provider.Name(); name != "Codex CLI" {
		t.Errorf("expected name 'Codex CLI', got %q", name)
	}
}

func TestCodexCLIProvider_SupportsSession(t *testing.T) {
	provider := NewCodexCLIProvider()
	if provider.SupportsSession() {
		t.Error("expected SupportsSession to return false")
	}
}

func TestCodexCLIProvider_Capabilities(t *testing.T) {
	provider := NewCodexCLIProvider()
	caps := provider.Capabilities()

	if !caps.SupportsStreaming {
		t.Error("expected SupportsStreaming to be true")
	}
	if caps.SupportsSessions {
		t.Error("expected SupportsSessions to be false")
	}
	if caps.SupportedTools != nil {
		t.Error("expected SupportedTools to be nil")
	}
	if caps.MaxContextLength != 200000 {
		t.Errorf("expected MaxContextLength 200000, got %d", caps.MaxContextLength)
	}
}

func TestCodexStreamParser_AgentMessage(t *testing.T) {
	parser := &codexStreamParser{}

	line := `{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"Hello world"}}`
	event := parser.parseLine(line)

	if event.Type != "text" {
		t.Errorf("expected type 'text', got %q", event.Type)
	}
	if event.Content != "Hello world" {
		t.Errorf("expected content 'Hello world', got %q", event.Content)
	}
}

func TestCodexStreamParser_AgentMessageEmpty(t *testing.T) {
	parser := &codexStreamParser{}

	line := `{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":""}}`
	event := parser.parseLine(line)

	if event.Type != "" {
		t.Errorf("expected empty type for empty text, got %q", event.Type)
	}
}

func TestCodexStreamParser_NonAgentItem(t *testing.T) {
	parser := &codexStreamParser{}

	line := `{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"ls"}}`
	event := parser.parseLine(line)

	if event.Type != "" {
		t.Errorf("expected empty type for non-agent item, got %q", event.Type)
	}
}

func TestCodexStreamParser_TurnCompleted(t *testing.T) {
	parser := &codexStreamParser{}

	line := `{"type":"turn.completed","usage":{"input_tokens":24763,"cached_input_tokens":24448,"output_tokens":122}}`
	event := parser.parseLine(line)

	if event.Type != "done" {
		t.Errorf("expected type 'done', got %q", event.Type)
	}
	if !parser.sentDone {
		t.Error("expected sentDone to be true")
	}
	if event.Usage == nil {
		t.Fatal("expected usage to be non-nil")
	}
	if event.Usage.InputTokens != 24763 {
		t.Errorf("expected InputTokens 24763, got %d", event.Usage.InputTokens)
	}
	if event.Usage.CacheReadTokens != 24448 {
		t.Errorf("expected CacheReadTokens 24448, got %d", event.Usage.CacheReadTokens)
	}
	if event.Usage.OutputTokens != 122 {
		t.Errorf("expected OutputTokens 122, got %d", event.Usage.OutputTokens)
	}
}

func TestCodexStreamParser_TurnCompletedNoUsage(t *testing.T) {
	parser := &codexStreamParser{}

	line := `{"type":"turn.completed"}`
	event := parser.parseLine(line)

	if event.Type != "done" {
		t.Errorf("expected type 'done', got %q", event.Type)
	}
	if event.Usage != nil {
		t.Error("expected usage to be nil")
	}
}

func TestCodexStreamParser_TurnFailed(t *testing.T) {
	parser := &codexStreamParser{}

	line := `{"type":"turn.failed","message":"rate limit exceeded"}`
	event := parser.parseLine(line)

	if event.Type != "error" {
		t.Errorf("expected type 'error', got %q", event.Type)
	}
	if event.Content != "rate limit exceeded" {
		t.Errorf("expected content 'rate limit exceeded', got %q", event.Content)
	}
}

func TestCodexStreamParser_TurnFailedNoMessage(t *testing.T) {
	parser := &codexStreamParser{}

	line := `{"type":"turn.failed"}`
	event := parser.parseLine(line)

	if event.Type != "error" {
		t.Errorf("expected type 'error', got %q", event.Type)
	}
	if event.Content != "turn failed" {
		t.Errorf("expected content 'turn failed', got %q", event.Content)
	}
}

func TestCodexStreamParser_Error(t *testing.T) {
	parser := &codexStreamParser{}

	line := `{"type":"error","message":"authentication failed"}`
	event := parser.parseLine(line)

	if event.Type != "error" {
		t.Errorf("expected type 'error', got %q", event.Type)
	}
	if event.Content != "authentication failed" {
		t.Errorf("expected content 'authentication failed', got %q", event.Content)
	}
}

func TestCodexStreamParser_ErrorNoMessage(t *testing.T) {
	parser := &codexStreamParser{}

	line := `{"type":"error"}`
	event := parser.parseLine(line)

	if event.Type != "error" {
		t.Errorf("expected type 'error', got %q", event.Type)
	}
	if event.Content != "unknown error" {
		t.Errorf("expected content 'unknown error', got %q", event.Content)
	}
}

func TestCodexStreamParser_IgnoredEvents(t *testing.T) {
	parser := &codexStreamParser{}

	ignored := []string{
		`{"type":"thread.started","thread_id":"abc123"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"id":"item_1","type":"command_execution"}}`,
	}

	for _, line := range ignored {
		event := parser.parseLine(line)
		if event.Type != "" {
			t.Errorf("expected empty type for %s, got %q", line, event.Type)
		}
	}
}

func TestCodexStreamParser_InvalidJSON(t *testing.T) {
	parser := &codexStreamParser{}

	event := parser.parseLine("not valid json")
	if event.Type != "" {
		t.Errorf("expected empty type for invalid JSON, got %q", event.Type)
	}
}

func TestBuildCodexPrompt_MessageOnly(t *testing.T) {
	req := Request{Message: "What pods are running?"}
	result := buildCodexPrompt(req)

	if result != "What pods are running?" {
		t.Errorf("expected just the message, got %q", result)
	}
}

func TestBuildCodexPrompt_WithSystemPrompt(t *testing.T) {
	req := Request{
		SystemPrompt: "You are a Kubernetes assistant.",
		Message:      "List deployments",
	}
	result := buildCodexPrompt(req)

	if !strings.HasPrefix(result, "You are a Kubernetes assistant.") {
		t.Errorf("expected system prompt at start, got %q", result)
	}
	if !strings.HasSuffix(result, "List deployments") {
		t.Errorf("expected message at end, got %q", result)
	}
}

func TestBuildCodexPrompt_WithHistory(t *testing.T) {
	req := Request{
		SystemPrompt: "System prompt",
		History: []Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		},
		Message: "What next?",
	}
	result := buildCodexPrompt(req)

	if !strings.Contains(result, "<conversation_history>") {
		t.Error("expected conversation_history tag")
	}
	if !strings.Contains(result, "user: Hello") {
		t.Error("expected user message in history")
	}
	if !strings.Contains(result, "assistant: Hi there") {
		t.Error("expected assistant message in history")
	}
	if !strings.Contains(result, "</conversation_history>") {
		t.Error("expected closing conversation_history tag")
	}
	if !strings.HasSuffix(result, "What next?") {
		t.Errorf("expected message at end, got %q", result)
	}
}

func TestBuildCodexPrompt_NoHistoryNoSystem(t *testing.T) {
	req := Request{Message: "Hello"}
	result := buildCodexPrompt(req)

	if strings.Contains(result, "<conversation_history>") {
		t.Error("should not include conversation_history when history is empty")
	}
	if result != "Hello" {
		t.Errorf("expected 'Hello', got %q", result)
	}
}

func TestWriteCodexMCPConfig_CreatesConfigTOML(t *testing.T) {
	// Ensure default config is used
	SetConfig(DefaultConfig())

	tempDir, err := writeCodexMCPConfig("my-context", []string{"mcp__kubikles__get_pods"}, []string{"kubectl"})
	if err != nil {
		t.Fatalf("writeCodexMCPConfig failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.toml: %v", err)
	}

	content := string(data)

	// Check TOML structure
	if !strings.Contains(content, "[mcp_servers.kubikles]") {
		t.Error("expected [mcp_servers.kubikles] section")
	}
	if !strings.Contains(content, "command = ") {
		t.Error("expected command field")
	}
	if !strings.Contains(content, "--mcp-server") {
		t.Error("expected --mcp-server in args")
	}
	if !strings.Contains(content, "--k8s-context") {
		t.Error("expected --k8s-context in args")
	}
	if !strings.Contains(content, "my-context") {
		t.Error("expected context name in args")
	}
	if !strings.Contains(content, "--allowed-tools") {
		t.Error("expected --allowed-tools in args")
	}
}

func TestWriteCodexMCPConfig_NoTools(t *testing.T) {
	SetConfig(DefaultConfig())

	tempDir, err := writeCodexMCPConfig("", nil, nil)
	if err != nil {
		t.Fatalf("writeCodexMCPConfig failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	data, err := os.ReadFile(filepath.Join(tempDir, "config.toml"))
	if err != nil {
		t.Fatalf("failed to read config.toml: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "--allowed-commands") {
		t.Error("expected --allowed-commands even with empty allowlist")
	}
}

func TestWriteCodexMCPConfig_Cleanup(t *testing.T) {
	SetConfig(DefaultConfig())

	tempDir, err := writeCodexMCPConfig("", nil, nil)
	if err != nil {
		t.Fatalf("writeCodexMCPConfig failed: %v", err)
	}

	// Verify dir exists
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Fatal("temp dir should exist before cleanup")
	}

	// Simulate cleanup
	os.RemoveAll(tempDir)

	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Error("temp dir should not exist after cleanup")
	}
}
