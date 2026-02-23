package ai

import "context"

// Provider defines the interface for AI chat providers.
type Provider interface {
	Name() string
	IsAvailable() (bool, string) // available, status message
	SendMessage(ctx context.Context, req Request, onChunk func(StreamEvent)) error
	// SupportsSession returns true if provider supports persistent bidirectional sessions.
	SupportsSession() bool
	// StartSession creates a persistent session for bidirectional streaming.
	// Returns nil if SupportsSession() is false.
	StartSession(sessionID, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, onEvent func(StreamEvent)) (Session, error)
	// Capabilities returns the provider's capabilities for feature detection.
	Capabilities() ProviderCapabilities
}

// Session represents a persistent CLI session for bidirectional streaming.
type Session interface {
	SendMessage(message string) error
	Close()
	IsAlive() bool
}

// ProviderCapabilities describes what features a provider supports.
type ProviderCapabilities struct {
	// SupportsStreaming indicates if the provider can stream responses token-by-token.
	SupportsStreaming bool `json:"supportsStreaming"`
	// SupportsSessions indicates if the provider supports persistent bidirectional sessions.
	SupportsSessions bool `json:"supportsSessions"`
	// SupportedTools is a list of tool names this provider can use, or nil for all tools.
	SupportedTools []string `json:"supportedTools,omitempty"`
	// MaxContextLength is the maximum number of tokens the provider can handle.
	MaxContextLength int `json:"maxContextLength"`
}

// Message represents a single message in a conversation.
type Message struct {
	Role    string // "user", "assistant"
	Content string
}

// Request represents a message to send to the AI provider.
type Request struct {
	SessionID       string
	Message         string
	SystemPrompt    string
	Model           string
	IsResume        bool      // true for follow-up messages (Claude CLI uses --resume)
	History         []Message // full conversation history for stateless providers
	K8sContext      string    // current K8s context name for MCP server
	AllowedTools    []string  // fully-qualified tool names for --allowedTools
	AllowedCommands []string  // command prefixes for run_command tool allowlist
}

// StreamEvent represents a streaming response chunk from the AI provider.
type StreamEvent struct {
	Type    string // "text", "done", "error"
	Content string
	Usage   *TokenUsage // token usage stats (may be nil)
}

// TokenUsage contains token usage statistics from the AI provider.
type TokenUsage struct {
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	CostUSD             float64 `json:"costUSD"`
}
