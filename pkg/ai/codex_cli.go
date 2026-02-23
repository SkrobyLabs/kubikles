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

// CodexCLIProvider implements Provider using the OpenAI Codex CLI subprocess.
type CodexCLIProvider struct {
	cliPath  string // resolved path to codex binary
	initOnce sync.Once
	initErr  error
}

// NewCodexCLIProvider creates a new Codex CLI provider.
func NewCodexCLIProvider() *CodexCLIProvider {
	return &CodexCLIProvider{}
}

func (c *CodexCLIProvider) Name() string {
	return "Codex CLI"
}

func (c *CodexCLIProvider) resolveCLI() (string, error) {
	c.initOnce.Do(func() {
		path, err := findCodexCLI()
		if err != nil {
			c.initErr = err
			return
		}
		c.cliPath = path
	})
	return c.cliPath, c.initErr
}

func (c *CodexCLIProvider) IsAvailable() (bool, string) {
	path, err := c.resolveCLI()
	if err != nil {
		return false, "Codex CLI not found. Install it from https://github.com/openai/codex"
	}

	return true, fmt.Sprintf("Codex CLI found at %s", path)
}

// SupportsSession returns false — Codex CLI does not support persistent bidirectional sessions.
func (c *CodexCLIProvider) SupportsSession() bool {
	return false
}

// Capabilities returns the provider's capabilities.
func (c *CodexCLIProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		SupportsStreaming: true,
		SupportsSessions:  false,
		SupportedTools:    nil,
		MaxContextLength:  200000,
	}
}

// StartSession is not supported by Codex CLI. The manager uses sendMessageOneShot instead.
func (c *CodexCLIProvider) StartSession(_, _, _, _ string, _, _ []string, _ func(StreamEvent)) (Session, error) {
	return nil, fmt.Errorf("codex CLI does not support persistent sessions")
}

func (c *CodexCLIProvider) SendMessage(ctx context.Context, req Request, onChunk func(StreamEvent)) error {
	cliPath, err := c.resolveCLI()
	if err != nil {
		return fmt.Errorf("codex CLI not found: %w", err)
	}

	// Build the prompt with system prompt and history baked in (Codex exec is stateless)
	prompt := buildCodexPrompt(req)

	args := []string{
		"exec",
		"--json",
		"--full-auto",
		"--sandbox", "workspace-write",
	}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	// Generate MCP config and set CODEX_HOME to a temp dir containing config.toml
	var tempDir string
	mcpTempDir, err := writeCodexMCPConfig(req.K8sContext, req.AllowedTools, req.AllowedCommands)
	if err == nil {
		tempDir = mcpTempDir
		defer os.RemoveAll(tempDir)
	}
	// If MCP config fails, proceed without tools (no tempDir means no CODEX_HOME override)

	// The prompt is the final positional argument
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, cliPath, args...)
	env := os.Environ()
	if tempDir != "" {
		env = append(env, "CODEX_HOME="+tempDir)
	}
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start codex CLI: %w", err)
	}

	// Read stdout line by line and parse streaming JSONL
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	parser := &codexStreamParser{}
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
		onChunk(StreamEvent{Type: "error", Content: fmt.Sprintf("codex CLI exited with error: %v", err)})
	}

	// Ensure done is always sent
	if !parser.sentDone {
		onChunk(StreamEvent{Type: "done"})
	}
	return nil
}

// codexStreamEvent represents the JSONL structure from Codex CLI --json output.
type codexStreamEvent struct {
	Type string `json:"type"`
	// For item events
	Item *codexItem `json:"item,omitempty"`
	// For turn.completed
	Usage *codexUsage `json:"usage,omitempty"`
	// For error events
	Message string `json:"message,omitempty"`
}

type codexItem struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "agent_message", "command_execution", "reasoning", etc.
	Text string `json:"text"` // present for agent_message
}

type codexUsage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

// codexStreamParser tracks state across streaming JSONL lines from Codex CLI.
// Codex --json format emits:
//   - "thread.started", "turn.started" — lifecycle events (ignored)
//   - "item.started", "item.completed" — item events; we extract text from agent_message
//   - "turn.completed" — end of turn with usage stats
//   - "turn.failed" — turn error
//   - "error" — top-level error
type codexStreamParser struct {
	sentDone bool
}

func (p *codexStreamParser) parseLine(line string) StreamEvent {
	var msg codexStreamEvent
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return StreamEvent{}
	}

	switch msg.Type {
	case "item.completed":
		if msg.Item != nil && msg.Item.Type == "agent_message" && msg.Item.Text != "" {
			return StreamEvent{Type: "text", Content: msg.Item.Text}
		}
	case "turn.completed":
		var usage *TokenUsage
		if msg.Usage != nil {
			usage = &TokenUsage{
				InputTokens:     msg.Usage.InputTokens,
				OutputTokens:    msg.Usage.OutputTokens,
				CacheReadTokens: msg.Usage.CachedInputTokens,
			}
		}
		p.sentDone = true
		return StreamEvent{Type: "done", Usage: usage}
	case "turn.failed":
		errMsg := "turn failed"
		if msg.Message != "" {
			errMsg = msg.Message
		}
		return StreamEvent{Type: "error", Content: errMsg}
	case "error":
		errMsg := "unknown error"
		if msg.Message != "" {
			errMsg = msg.Message
		}
		return StreamEvent{Type: "error", Content: errMsg}
	}

	// Ignore: thread.started, turn.started, item.started, and other item types
	return StreamEvent{}
}

// buildCodexPrompt constructs a single prompt string from the request.
// Since Codex exec is stateless, we embed system prompt and conversation history.
func buildCodexPrompt(req Request) string {
	var sb strings.Builder

	if req.SystemPrompt != "" {
		sb.WriteString(req.SystemPrompt)
		sb.WriteString("\n\n")
	}

	if len(req.History) > 0 {
		sb.WriteString("<conversation_history>\n")
		for _, msg := range req.History {
			sb.WriteString(msg.Role)
			sb.WriteString(": ")
			sb.WriteString(msg.Content)
			sb.WriteString("\n")
		}
		sb.WriteString("</conversation_history>\n\n")
	}

	sb.WriteString(req.Message)
	return sb.String()
}

// writeCodexMCPConfig creates a temporary directory with a config.toml for Codex CLI.
// The directory is used as CODEX_HOME so Codex picks up the MCP server configuration.
// The caller must defer os.RemoveAll on the returned path.
func writeCodexMCPConfig(k8sContext string, allowedTools []string, allowedCommands []string) (string, error) {
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

	// Always pass command allowlist to MCP server so it doesn't fall back to defaults.
	mcpArgs = append(mcpArgs, "--allowed-commands", strings.Join(allowedCommands, "|"))

	// Create temp dir to serve as CODEX_HOME, preserving existing config (auth, etc.)
	tempDir, err := os.MkdirTemp("", "kubikles-codex-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Copy all files from real Codex home so auth tokens and other settings are preserved
	if realHome := codexHomeDir(); realHome != "" {
		copyCodexHome(realHome, tempDir)
	}

	// Read existing config.toml (if any) and append our MCP server section
	var toml strings.Builder
	configPath := filepath.Join(tempDir, "config.toml")
	if existing, err := os.ReadFile(configPath); err == nil {
		toml.Write(existing)
		if len(existing) > 0 && existing[len(existing)-1] != '\n' {
			toml.WriteString("\n")
		}
		toml.WriteString("\n")
	}

	toml.WriteString(fmt.Sprintf("[mcp_servers.%s]\n", cfg.MCPServerName))
	toml.WriteString(fmt.Sprintf("command = %q\n", exePath))

	// Write args array
	toml.WriteString("args = [")
	for i, arg := range mcpArgs {
		if i > 0 {
			toml.WriteString(", ")
		}
		toml.WriteString(fmt.Sprintf("%q", arg))
	}
	toml.WriteString("]\n")

	if err := os.WriteFile(configPath, []byte(toml.String()), 0600); err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to write config.toml: %w", err)
	}

	return tempDir, nil
}

// findCodexCLI searches for the codex CLI binary in PATH and common locations.
func findCodexCLI() (string, error) {
	// First check PATH
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}

	// Check common install locations
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/usr/local/bin/codex",
		filepath.Join(home, ".local", "bin", "codex"),
		filepath.Join(home, ".npm-global", "bin", "codex"),
	}

	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/opt/homebrew/bin/codex")
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("codex CLI binary not found in PATH or common locations")
}

// codexHomeDir returns the real Codex CLI home directory.
// Codex uses CODEX_HOME env var, or falls back to ~/.codex.
func codexHomeDir() string {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".codex")
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	return ""
}

// copyCodexHome copies files from src to dst (non-recursive, top-level files only).
// This preserves auth tokens and other settings from the real Codex home.
func copyCodexHome(src, dst string) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(dst, entry.Name()), data, 0600)
	}
}
