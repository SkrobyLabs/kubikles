package main

import (
	"kubikles/pkg/ai"
	"kubikles/pkg/tools"
)

// =============================================================================
// AI Assistant
// =============================================================================

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
func (a *App) SendAIMessage(sessionID, message, systemPrompt, model string, allowedTools []string, timeoutSeconds int) bool {
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
	return a.aiManager.SendMessage(sessionID, message, systemPrompt, model, k8sCtx, allowedTools, timeoutSeconds)
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
