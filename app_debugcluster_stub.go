//go:build !debugcluster

package main

import "kubikles/pkg/k8s"

// IsDebugClusterEnabled returns false in production builds.
func (a *App) IsDebugClusterEnabled() bool {
	return false
}

// GetDebugClusterConfig returns an empty config in production builds.
func (a *App) GetDebugClusterConfig() k8s.DebugClusterConfig {
	return k8s.DebugClusterConfig{}
}

// SetDebugClusterConfig is a no-op in production builds.
func (a *App) SetDebugClusterConfig(_ k8s.DebugClusterConfig) error {
	return nil
}

// ResetDebugCluster is a no-op in production builds.
func (a *App) ResetDebugCluster() error {
	return nil
}
