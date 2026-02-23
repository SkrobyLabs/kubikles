package main

import (
	"fmt"
	"sort"

	"kubikles/pkg/ai"
	"kubikles/pkg/k8s"
	"kubikles/pkg/tools"
)

// =============================================================================
// AI Assistant
// =============================================================================

// newAIManager creates an AI manager using a registry with all providers.
// k8sClient is needed by the Anthropic API provider for tool execution.
func newAIManager(k8sClient *k8s.Client) *ai.Manager {
	registry := ai.NewRegistry()
	registry.Register("anthropic-api", func() ai.Provider {
		return ai.NewAnthropicAPIProvider(k8sClient)
	}, ai.AnthropicModels)
	registry.Register("codex-cli", func() ai.Provider {
		return ai.NewCodexCLIProvider()
	}, ai.CodexModels)

	mgr, err := ai.NewManagerWithRegistry(registry)
	if err != nil {
		fmt.Printf("AI registry lookup failed (%v), falling back to Anthropic API\n", err)
		return ai.NewManager(ai.NewAnthropicAPIProvider(k8sClient))
	}
	return mgr
}

// AIProviderStatus represents the availability status of the AI provider
type AIProviderStatus struct {
	Available bool   `json:"available"`
	Status    string `json:"status"`
	Provider  string `json:"provider"`
}

// CheckAIProvider checks if an AI provider (Claude CLI) is available
func (a *App) CheckAIProvider() AIProviderStatus {
	if a.aiManager == nil {
		return AIProviderStatus{Available: false, Status: "AI manager not initialized"}
	}
	available, status := a.aiManager.CheckProvider()
	return AIProviderStatus{Available: available, Status: status, Provider: a.aiManager.ProviderName()}
}

// AIProviderInfo describes a registered AI provider and its availability.
type AIProviderInfo struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	Available bool   `json:"available"`
	Status    string `json:"status"`
}

// GetAIProviders returns all registered AI providers and their availability status.
func (a *App) GetAIProviders() []AIProviderInfo {
	registry := a.aiManager.Registry()
	var providers []AIProviderInfo
	for _, id := range registry.List() {
		factory := registry.Get(id)
		if factory == nil {
			continue
		}
		p := factory()
		available, status := p.IsAvailable()
		providers = append(providers, AIProviderInfo{
			Name:      p.Name(),
			ID:        id,
			Available: available,
			Status:    status,
		})
	}
	return providers
}

// AIModelOption describes a single model choice for the frontend dropdown.
type AIModelOption struct {
	Value         string `json:"value"`         // compound "provider-id/model-name"
	Label         string `json:"label"`         // display label for the model
	Provider      string `json:"provider"`      // provider registry key
	ProviderLabel string `json:"providerLabel"` // human-readable provider name
	Available     bool   `json:"available"`     // whether the provider is currently available
}

// GetAIModels returns available AI model options grouped by provider.
func (a *App) GetAIModels() []AIModelOption {
	registry := a.aiManager.Registry()
	models := registry.GetModels()

	// Build availability map
	providerAvail := make(map[string]bool)
	providerLabel := make(map[string]string)
	for _, id := range registry.List() {
		factory := registry.Get(id)
		if factory == nil {
			continue
		}
		p := factory()
		avail, _ := p.IsAvailable()
		providerAvail[id] = avail
		providerLabel[id] = p.Name()
	}

	var options []AIModelOption
	for provID, modelList := range models {
		for _, m := range modelList {
			options = append(options, AIModelOption{
				Value:         provID + "/" + m.Name,
				Label:         m.Label,
				Provider:      provID,
				ProviderLabel: providerLabel[provID],
				Available:     providerAvail[provID],
			})
		}
	}

	// Sort: available first, then by provider, then by value
	sort.Slice(options, func(i, j int) bool {
		if options[i].Available != options[j].Available {
			return options[i].Available
		}
		if options[i].Provider != options[j].Provider {
			return options[i].Provider < options[j].Provider
		}
		return options[i].Value < options[j].Value
	})

	return options
}

// GetToolDiscovery returns tool definitions and view/action mappings for the frontend.
// This allows the frontend to dynamically determine which tools are relevant for each view.
func (a *App) GetToolDiscovery() tools.ToolDiscoveryResponse {
	return tools.DefaultToolRegistry.GetDiscoveryResponse()
}

// StartAISession creates a new AI chat session and returns the session ID.
// clientID is the WebSocket client ID for server mode (pass empty string for desktop mode).
func (a *App) StartAISession(clientID string) string {
	if a.aiManager == nil {
		return ""
	}
	return a.aiManager.StartSession(clientID)
}

// SendAIMessage sends a message in an AI session. Streams response via ai:response events.
// Returns true if the request was successfully initiated, false if it failed immediately.
func (a *App) SendAIMessage(sessionID, message, systemPrompt, model string, allowedTools []string, allowedCommands []string, timeoutSeconds int) bool {
	if a.aiManager == nil {
		a.emitEvent("ai:response", ai.AIResponseEvent{
			SessionID: sessionID, Error: "AI is not configured", Done: true,
		})
		return false
	}
	k8sCtx := ""
	if a.k8sClient != nil {
		k8sCtx = a.k8sClient.GetCurrentContext()
	}
	return a.aiManager.SendMessage(sessionID, message, systemPrompt, model, k8sCtx, allowedTools, allowedCommands, timeoutSeconds)
}

// CancelAIRequest cancels the in-progress AI request for a session
func (a *App) CancelAIRequest(sessionID string) {
	if a.aiManager == nil {
		return
	}
	a.aiManager.CancelRequest(sessionID)
}

// ClearAISession resets an AI session (new session for fresh conversation). Returns new session ID.
func (a *App) ClearAISession(sessionID string) string {
	if a.aiManager == nil {
		return ""
	}
	return a.aiManager.ClearSession(sessionID)
}

// CloseAISession closes and cleans up an AI session
func (a *App) CloseAISession(sessionID string) {
	if a.aiManager == nil {
		return
	}
	a.aiManager.CloseSession(sessionID)
}

// SetAnthropicAPIKey stores the Anthropic API key securely.
func (a *App) SetAnthropicAPIKey(key string) error {
	return ai.SaveAnthropicAPIKey(key)
}

// GetAnthropicAPIKeyStatus returns "env", "configured", or "not_set".
func (a *App) GetAnthropicAPIKeyStatus() string {
	return ai.GetAnthropicAPIKeyStatus()
}

// ClearAnthropicAPIKey removes the stored API key.
func (a *App) ClearAnthropicAPIKey() error {
	return ai.ClearAnthropicAPIKey()
}
