package ai

import (
	"fmt"
	"sync"
)

// ProviderFactory is a function that creates a new Provider instance.
type ProviderFactory func() Provider

// Registry manages AI provider registrations and lookups.
type Registry struct {
	factories map[string]ProviderFactory
	mu        sync.RWMutex
}

// NewRegistry creates a new empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]ProviderFactory),
	}
}

// Register adds a provider factory to the registry.
// The name should be a unique identifier for the provider (e.g., "claude-cli").
func (r *Registry) Register(name string, factory ProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Get returns a provider factory by name, or nil if not found.
func (r *Registry) Get(name string) ProviderFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.factories[name]
}

// List returns the names of all registered providers.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// GetFirstAvailable returns the first registered provider that reports as available.
// If no provider is available, returns nil and an error.
func (r *Registry) GetFirstAvailable() (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, factory := range r.factories {
		provider := factory()
		if available, _ := provider.IsAvailable(); available {
			return provider, nil
		}
		// Log that this provider isn't available
		_ = name // Could add logging here
	}

	return nil, fmt.Errorf("no AI provider available")
}

// CreateProvider creates a new provider instance by name.
// Returns an error if the provider is not registered.
func (r *Registry) CreateProvider(name string) (Provider, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}

	return factory(), nil
}

// DefaultRegistry is the global default provider registry.
// Providers are registered in init() functions.
var DefaultRegistry = NewRegistry()

func init() {
	// Register the built-in Claude CLI provider
	DefaultRegistry.Register("claude-cli", func() Provider {
		return NewClaudeCLIProvider()
	})
}
