package ai

import "sync"

// Config holds configuration values for the AI subsystem.
type Config struct {
	// MCPPrefix is the prefix used for MCP tool names (e.g., "mcp__kubikles__").
	MCPPrefix string

	// MCPServerName is the name of the MCP server in the config (e.g., "kubikles").
	MCPServerName string
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		MCPPrefix:     "mcp__kubikles__",
		MCPServerName: "kubikles",
	}
}

var (
	globalConfig     Config
	globalConfigOnce sync.Once
)

// GetConfig returns the global AI configuration.
// The configuration is initialized once with default values.
func GetConfig() Config {
	globalConfigOnce.Do(func() {
		globalConfig = DefaultConfig()
	})
	return globalConfig
}

// SetConfig sets the global AI configuration.
// This should be called early in application startup if custom values are needed.
// Note: This is not thread-safe during initialization; call before any concurrent access.
func SetConfig(cfg Config) {
	globalConfig = cfg
}
