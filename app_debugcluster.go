//go:build debugcluster

package main

import (
	"fmt"
	"log"

	"kubikles/pkg/k8s"
)

// IsDebugClusterEnabled returns true in debug cluster builds.
func (a *App) IsDebugClusterEnabled() bool {
	return true
}

// GetDebugClusterConfig returns the current debug cluster resource counts.
func (a *App) GetDebugClusterConfig() k8s.DebugClusterConfig {
	return k8s.GetDebugClusterConfig()
}

// SetDebugClusterConfig regenerates the debug cluster with new resource counts.
// If the debug cluster context is currently active, watchers are restarted.
func (a *App) SetDebugClusterConfig(config k8s.DebugClusterConfig) error {
	if err := k8s.RegenerateDebugCluster(config); err != nil {
		return fmt.Errorf("failed to regenerate debug cluster: %w", err)
	}
	log.Printf("[Debug Cluster] Config updated: %+v", config)

	// If we're currently on the debug cluster context, re-switch to pick up new data
	if a.k8sClient != nil && k8s.IsDebugClusterContext(a.k8sClient.GetCurrentContext()) {
		return a.SwitchContext(k8s.DebugClusterContextName)
	}
	return nil
}

// ResetDebugCluster regenerates the debug cluster with the current config.
func (a *App) ResetDebugCluster() error {
	config := k8s.GetDebugClusterConfig()
	return a.SetDebugClusterConfig(config)
}
