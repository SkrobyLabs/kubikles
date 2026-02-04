package main

import (
	"fmt"
)

// =============================================================================
// Port Forwarding
// =============================================================================

// GetPortForwardConfigs returns all port forward configurations, optionally filtered by context
func (a *App) GetPortForwardConfigs(contextFilter string) []PortForwardConfig {
	a.logDebug("GetPortForwardConfigs called: contextFilter=%s", contextFilter)
	if a.portForwardManager == nil {
		return []PortForwardConfig{}
	}
	return a.portForwardManager.GetConfigs(contextFilter)
}

// GetActivePortForwards returns all active port forwards
func (a *App) GetActivePortForwards() []ActivePortForward {
	a.logDebug("GetActivePortForwards called")
	if a.portForwardManager == nil {
		return []ActivePortForward{}
	}
	return a.portForwardManager.GetActiveForwards()
}

// AddPortForwardConfig adds a new port forward configuration
func (a *App) AddPortForwardConfig(cfg PortForwardConfig) (*PortForwardConfig, error) {
	a.logDebug("AddPortForwardConfig called: context=%s, ns=%s, type=%s, name=%s, ports=%d:%d",
		cfg.Context, cfg.Namespace, cfg.ResourceType, cfg.ResourceName, cfg.LocalPort, cfg.RemotePort)
	if a.portForwardManager == nil {
		return nil, fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.AddConfig(cfg)
}

// UpdatePortForwardConfig updates an existing port forward configuration
func (a *App) UpdatePortForwardConfig(cfg PortForwardConfig) error {
	a.logDebug("UpdatePortForwardConfig called: id=%s", cfg.ID)
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.UpdateConfig(cfg)
}

// DeletePortForwardConfig deletes a port forward configuration
func (a *App) DeletePortForwardConfig(configID string) error {
	a.logDebug("DeletePortForwardConfig called: id=%s", configID)
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.DeleteConfig(configID)
}

// StartPortForward starts a port forward
func (a *App) StartPortForward(configID string) error {
	a.logDebug("StartPortForward called: id=%s", configID)
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.Start(configID)
}

// StopPortForward stops a port forward
func (a *App) StopPortForward(configID string) error {
	a.logDebug("StopPortForward called: id=%s", configID)
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.Stop(configID)
}

// StopAllPortForwards stops all active port forwards
func (a *App) StopAllPortForwards() {
	a.logDebug("StopAllPortForwards called")
	if a.portForwardManager == nil {
		return
	}
	a.portForwardManager.StopAll()
}

// GetAvailablePort finds an available local port
func (a *App) GetAvailablePort(preferred int) int {
	a.logDebug("GetAvailablePort called: preferred=%d", preferred)
	if a.portForwardManager == nil {
		return 0
	}
	return a.portForwardManager.GetAvailablePort(preferred)
}

// GetRandomAvailablePort gets a random available port avoiding well-known and configured ports
func (a *App) GetRandomAvailablePort() int {
	a.logDebug("GetRandomAvailablePort called")
	if a.portForwardManager == nil {
		return 0
	}
	return a.portForwardManager.GetRandomAvailablePort()
}

// StartFavoritePortForwards starts all favorite port forwards for a context
func (a *App) StartFavoritePortForwards(contextName string) {
	a.logDebug("StartFavoritePortForwards called: context=%s", contextName)
	if a.portForwardManager == nil {
		return
	}
	a.portForwardManager.StartFavorites(contextName)
}

// StartPortForwardsWithMode starts port forwards based on the specified mode
// mode can be: "all", "favorites", "none"
// Only starts forwards that were running when the app was closed
func (a *App) StartPortForwardsWithMode(contextName, mode string) {
	a.logDebug("StartPortForwardsWithMode called: context=%s, mode=%s", contextName, mode)
	if a.portForwardManager == nil {
		return
	}
	a.portForwardManager.StartWithMode(contextName, mode)
}
