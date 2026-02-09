package main

import (
	"fmt"

	"kubikles/pkg/debug"
	"kubikles/pkg/terminal"
)

// =============================================================================
// Terminal
// =============================================================================

// StartTerminalSession starts a new terminal session and returns the session ID
func (a *App) StartTerminalSession(opts terminal.SessionOptions) (string, error) {
	debug.LogTerminal("StartTerminalSession called", map[string]interface{}{"context": opts.Context, "ns": opts.Namespace, "pod": opts.Pod, "container": opts.Container, "cmd": opts.Command})
	if a.terminalManager == nil {
		return "", fmt.Errorf("terminal manager not initialized")
	}
	return a.terminalManager.StartSession(opts)
}

// SendTerminalInput sends input to a terminal session
func (a *App) SendTerminalInput(sessionID string, data string) error {
	if a.terminalManager == nil {
		return fmt.Errorf("terminal manager not initialized")
	}
	return a.terminalManager.SendInput(sessionID, []byte(data))
}

// ResizeTerminal resizes a terminal session
func (a *App) ResizeTerminal(sessionID string, cols, rows int) error {
	if a.terminalManager == nil {
		return fmt.Errorf("terminal manager not initialized")
	}
	return a.terminalManager.Resize(sessionID, cols, rows)
}

// CloseTerminalSession closes a terminal session
func (a *App) CloseTerminalSession(sessionID string) error {
	debug.LogTerminal("CloseTerminalSession called", map[string]interface{}{"sessionID": sessionID})
	if a.terminalManager == nil {
		return fmt.Errorf("terminal manager not initialized")
	}
	return a.terminalManager.CloseSession(sessionID)
}

// Watchers: see app_watchers.go

// StatefulSets: see app_statefulsets.go

// DaemonSets: see app_daemonsets.go

// ReplicaSets: see app_replicasets.go

// Dialogs: see app_dialogs.go

// File Transfer: see app_filetransfer.go

// Jobs: see app_jobs.go

// Storage: see app_storage.go

// Custom Resources: see app_customresources.go

// Port Forwarding: see app_portforward.go

// Ingress Forwarding: see app_ingressfwd.go

// Helm: see app_helm.go

// RBAC: see app_rbac.go

// Network: see app_network.go

// Webhooks: see app_webhooks.go

// Scheduling: see app_scheduling.go

// CSI: see app_csi.go

// Prometheus: see app_prometheus.go

// Certificates: see app_certificates.go

// Diagnostics: see app_diagnostics.go
