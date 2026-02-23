package ai

import (
	"fmt"
	"sync"
)

// ProviderFactory is a function that creates a new Provider instance.
type ProviderFactory func() Provider

// ModelInfo describes a suggested model for a provider.
type ModelInfo struct {
	Name  string `json:"name"`  // e.g. "sonnet"
	Label string `json:"label"` // e.g. "Sonnet (Fast)"
}

// providerRegistration bundles a factory with its suggested models.
type providerRegistration struct {
	Factory ProviderFactory
	Models  []ModelInfo
}

// Registry manages AI provider registrations and lookups.
type Registry struct {
	registrations map[string]providerRegistration
	mu            sync.RWMutex
}

// NewRegistry creates a new empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		registrations: make(map[string]providerRegistration),
	}
}

// Register adds a provider factory to the registry with its suggested models.
// The name should be a unique identifier for the provider (e.g., "claude-cli").
func (r *Registry) Register(name string, factory ProviderFactory, models []ModelInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registrations[name] = providerRegistration{Factory: factory, Models: models}
}

// Get returns a provider factory by name, or nil if not found.
func (r *Registry) Get(name string) ProviderFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if reg, ok := r.registrations[name]; ok {
		return reg.Factory
	}
	return nil
}

// List returns the names of all registered providers.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.registrations))
	for name := range r.registrations {
		names = append(names, name)
	}
	return names
}

// GetFirstAvailable returns the first registered provider that reports as available.
// If no provider is available, returns nil and an error.
func (r *Registry) GetFirstAvailable() (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, reg := range r.registrations {
		provider := reg.Factory()
		if available, _ := provider.IsAvailable(); available {
			return provider, nil
		}
	}

	return nil, fmt.Errorf("no AI provider available")
}

// GetFirstAvailableWithID returns the first available provider and its registry key.
func (r *Registry) GetFirstAvailableWithID() (Provider, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, reg := range r.registrations {
		provider := reg.Factory()
		if available, _ := provider.IsAvailable(); available {
			return provider, name, nil
		}
	}

	return nil, "", fmt.Errorf("no AI provider available")
}

// CreateProvider creates a new provider instance by name.
// Returns an error if the provider is not registered.
func (r *Registry) CreateProvider(name string) (Provider, error) {
	r.mu.RLock()
	reg, ok := r.registrations[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}

	return reg.Factory(), nil
}

// GetModels returns the suggested models for each registered provider.
func (r *Registry) GetModels() map[string][]ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string][]ModelInfo, len(r.registrations))
	for name, reg := range r.registrations {
		result[name] = reg.Models
	}
	return result
}

// DefaultRegistry is the global default provider registry.
// Providers are registered in init() functions.
var DefaultRegistry = NewRegistry()

// AnthropicModels are the suggested models for the Anthropic API provider.
var AnthropicModels = []ModelInfo{
	{Name: "sonnet", Label: "Sonnet (Fast)"},
	{Name: "opus", Label: "Opus (Smart)"},
	{Name: "haiku", Label: "Haiku (Fastest)"},
}

// CodexModels are the suggested models for the Codex CLI provider.
var CodexModels = []ModelInfo{
	{Name: "gpt-5.3-codex", Label: "GPT-5.3 Codex"},
	{Name: "gpt-5.2-codex", Label: "GPT-5.2 Codex"},
	{Name: "gpt-5.1-codex-max", Label: "GPT-5.1 Codex Max"},
	{Name: "gpt-5.2", Label: "GPT-5.2"},
	{Name: "gpt-5.1-codex-mini", Label: "GPT-5.1 Codex Mini"},
}

func init() {
	// Only register codex-cli in DefaultRegistry (no dependencies).
	// anthropic-api is registered by newAIManager which has *k8s.Client.
	DefaultRegistry.Register("codex-cli", func() Provider {
		return NewCodexCLIProvider()
	}, CodexModels)
}
