package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ClaudeCLIProvider implements Provider using the Claude CLI subprocess.
type ClaudeCLIProvider struct {
	cliPath  string // resolved path to claude binary
	initOnce sync.Once
	initErr  error
}

// NewClaudeCLIProvider creates a new Claude CLI provider.
func NewClaudeCLIProvider() *ClaudeCLIProvider {
	return &ClaudeCLIProvider{}
}

func (c *ClaudeCLIProvider) Name() string {
	return "Claude CLI"
}

func (c *ClaudeCLIProvider) resolveCLI() (string, error) {
	c.initOnce.Do(func() {
		path, err := findClaudeCLI()
		if err != nil {
			c.initErr = err
			return
		}
		c.cliPath = path
	})
	return c.cliPath, c.initErr
}

func (c *ClaudeCLIProvider) IsAvailable() (bool, string) {
	path, err := c.resolveCLI()
	if err != nil {
		return false, "Claude CLI not found. Install it from https://docs.anthropic.com/en/docs/claude-cli"
	}
	return true, fmt.Sprintf("Claude CLI found at %s", path)
}

// SupportsSession returns true as Claude CLI supports persistent bidirectional sessions.
func (c *ClaudeCLIProvider) SupportsSession() bool {
	return true
}

// Capabilities returns the provider's capabilities.
func (c *ClaudeCLIProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		SupportsStreaming: true,
		SupportsSessions:  true,
		SupportedTools:    nil,    // nil means all tools supported
		MaxContextLength:  200000, // Claude models support large context
	}
}

// StartSession creates a persistent Claude CLI session for bidirectional streaming.
// This avoids the startup overhead of spawning a new process per message.
func (c *ClaudeCLIProvider) StartSession(sessionID, systemPrompt, model, k8sContext string, allowedTools []string, onEvent func(StreamEvent)) (Session, error) {
	cliPath, err := c.resolveCLI()
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found: %w", err)
	}

	session := newClaudeCLISession(sessionID, onEvent)
	if err := session.Start(cliPath, sessionID, systemPrompt, model, k8sContext, allowedTools); err != nil {
		return nil, err
	}

	return session, nil
}

func (c *ClaudeCLIProvider) SendMessage(ctx context.Context, req Request, onChunk func(StreamEvent)) error {
	cliPath, err := c.resolveCLI()
	if err != nil {
		return fmt.Errorf("claude CLI not found: %w", err)
	}

	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
	}

	// Generate MCP config for K8s tools and clean up after CLI exits
	mcpConfigPath, err := writeMCPConfig(req.K8sContext, req.AllowedTools)
	if err != nil {
		// Fall back to no-tools mode if MCP config fails
		args = append(args, "--allowedTools", "")
	} else {
		defer os.Remove(mcpConfigPath)
		args = append(args, "--mcp-config", mcpConfigPath)
		// Use dynamic allowedTools from request
		if len(req.AllowedTools) > 0 {
			for _, tool := range req.AllowedTools {
				args = append(args, "--allowedTools", tool)
			}
		} else {
			args = append(args, "--allowedTools", "")
		}
	}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	if req.IsResume {
		args = append(args, "--resume", req.SessionID)
	} else {
		args = append(args, "--session-id", req.SessionID)
		if req.SystemPrompt != "" {
			args = append(args, "--system-prompt", req.SystemPrompt)
		}
	}

	// The message is the final positional argument
	args = append(args, req.Message)

	cmd := exec.CommandContext(ctx, cliPath, args...)
	cmd.Env = append(os.Environ(), "TERM=dumb")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start claude CLI: %w", err)
	}

	// Read stdout line by line and parse streaming JSON
	scanner := bufio.NewScanner(stdout)
	// Allow larger lines (Claude can return large JSON)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	parser := &streamParser{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		event := parser.parseLine(line)
		if event.Type != "" {
			onChunk(event)
		}
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		onChunk(StreamEvent{Type: "error", Content: fmt.Sprintf("stream read error: %v", err)})
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		onChunk(StreamEvent{Type: "error", Content: fmt.Sprintf("claude CLI exited with error: %v", err)})
	}

	// Ensure done is always sent (result event usually sends it, but guard against missing it)
	if !parser.sentDone {
		onChunk(StreamEvent{Type: "done"})
	}
	return nil
}

// cliStreamMessage represents the JSON structure from Claude CLI --output-format stream-json
type cliStreamMessage struct {
	Type   string `json:"type"`
	Result string `json:"result"`
	// For assistant messages
	Message struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage *cliUsage `json:"usage,omitempty"`
	} `json:"message"`
	// For content_block_delta
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	// For result messages
	TotalCostUSD float64   `json:"total_cost_usd,omitempty"`
	Usage        *cliUsage `json:"usage,omitempty"`
}

// cliUsage represents token usage from Claude CLI JSON output
type cliUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// streamParser tracks state across streaming JSON lines to avoid duplicate content.
// Claude CLI stream-json format emits:
//   - "system" events (hooks, init) — ignored
//   - "assistant" event with full message content — this is the primary text source
//   - "result" event with the same text duplicated — we only use this for "done"
type streamParser struct {
	receivedText bool
	sentDone     bool
	lastUsage    *TokenUsage // track latest usage for done event
}

func (p *streamParser) parseLine(line string) StreamEvent {
	var msg cliStreamMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return StreamEvent{}
	}

	switch msg.Type {
	case "assistant":
		// Reset for each new assistant turn so text after tool calls is captured
		p.receivedText = false
		// Extract usage from assistant message
		var usage *TokenUsage
		if msg.Message.Usage != nil {
			usage = &TokenUsage{
				InputTokens:         msg.Message.Usage.InputTokens,
				OutputTokens:        msg.Message.Usage.OutputTokens,
				CacheReadTokens:     msg.Message.Usage.CacheReadInputTokens,
				CacheCreationTokens: msg.Message.Usage.CacheCreationInputTokens,
			}
			p.lastUsage = usage
		}
		// Full assistant message — primary content source
		var texts []string
		for _, c := range msg.Message.Content {
			if c.Type == "text" && c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
		if len(texts) > 0 {
			p.receivedText = true
			return StreamEvent{Type: "text", Content: strings.Join(texts, ""), Usage: usage}
		}
	case "content_block_delta":
		// Incremental streaming (may be used in future CLI versions)
		if msg.Delta.Type == "text_delta" && msg.Delta.Text != "" {
			p.receivedText = true
			return StreamEvent{Type: "text", Content: msg.Delta.Text}
		}
	case "result":
		// Extract final usage from result (includes cost)
		var usage *TokenUsage
		if msg.Usage != nil {
			usage = &TokenUsage{
				InputTokens:         msg.Usage.InputTokens,
				OutputTokens:        msg.Usage.OutputTokens,
				CacheReadTokens:     msg.Usage.CacheReadInputTokens,
				CacheCreationTokens: msg.Usage.CacheCreationInputTokens,
				CostUSD:             msg.TotalCostUSD,
			}
			p.lastUsage = usage
		}
		// Result duplicates the full text — only use as fallback if we got nothing else
		if !p.receivedText && msg.Result != "" {
			return StreamEvent{Type: "text", Content: msg.Result, Usage: usage}
		}
		p.sentDone = true
		return StreamEvent{Type: "done", Usage: usage}
	}

	return StreamEvent{}
}

// findClaudeCLI searches for the claude CLI binary in PATH and common locations.
func findClaudeCLI() (string, error) {
	// First check PATH
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}

	// Check common install locations
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/usr/local/bin/claude",
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, ".npm-global", "bin", "claude"),
		filepath.Join(home, ".claude", "bin", "claude"),
	}

	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/opt/homebrew/bin/claude",
			filepath.Join(home, "Library", "Application Support", "Claude", "bin", "claude"),
		)
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("claude CLI binary not found in PATH or common locations")
}

// writeMCPConfig writes a temporary MCP server config JSON file for Claude CLI.
// The caller must defer os.Remove on the returned path.
// allowedTools is the list of fully-qualified tool names (e.g. mcp__kubikles__get_pod_logs);
// short names are extracted and passed to the MCP server for server-side enforcement.
func writeMCPConfig(k8sContext string, allowedTools []string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	mcpArgs := []string{"--mcp-server"}
	if k8sContext != "" {
		mcpArgs = append(mcpArgs, "--k8s-context", k8sContext)
	}

	// Extract short MCP tool names for server-side enforcement
	cfg := GetConfig()
	var shortNames []string
	for _, t := range allowedTools {
		if strings.HasPrefix(t, cfg.MCPPrefix) {
			shortNames = append(shortNames, strings.TrimPrefix(t, cfg.MCPPrefix))
		}
	}
	if len(shortNames) > 0 {
		mcpArgs = append(mcpArgs, "--allowed-tools", strings.Join(shortNames, ","))
	}

	config := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			cfg.MCPServerName: map[string]interface{}{
				"command": exePath,
				"args":    mcpArgs,
			},
		},
	}

	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal MCP config: %w", err)
	}

	f, err := os.CreateTemp("", "kubikles-mcp-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp MCP config: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("failed to write MCP config: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}
