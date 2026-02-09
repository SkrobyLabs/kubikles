package main

import (
	"fmt"

	"kubikles/pkg/debug"
)

// =============================================================================
// Port Forwarding
// =============================================================================

// GetPortForwardConfigs returns all port forward configurations, optionally filtered by context
func (a *App) GetPortForwardConfigs(contextFilter string) []PortForwardConfig {
	debug.LogPortforward("GetPortForwardConfigs called", map[string]interface{}{"contextFilter": contextFilter})
	if a.portForwardManager == nil {
		return []PortForwardConfig{}
	}
	return a.portForwardManager.GetConfigs(contextFilter)
}

// GetActivePortForwards returns all active port forwards
func (a *App) GetActivePortForwards() []ActivePortForward {
	debug.LogPortforward("GetActivePortForwards called", nil)
	if a.portForwardManager == nil {
		return []ActivePortForward{}
	}
	return a.portForwardManager.GetActiveForwards()
}

// AddPortForwardConfig adds a new port forward configuration
func (a *App) AddPortForwardConfig(cfg PortForwardConfig) (*PortForwardConfig, error) {
	debug.LogPortforward("AddPortForwardConfig called", map[string]interface{}{"context": cfg.Context, "namespace": cfg.Namespace, "type": cfg.ResourceType, "name": cfg.ResourceName, "localPort": cfg.LocalPort, "remotePort": cfg.RemotePort})
	if a.portForwardManager == nil {
		return nil, fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.AddConfig(cfg)
}

// UpdatePortForwardConfig updates an existing port forward configuration
func (a *App) UpdatePortForwardConfig(cfg PortForwardConfig) error {
	debug.LogPortforward("UpdatePortForwardConfig called", map[string]interface{}{"id": cfg.ID})
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.UpdateConfig(cfg)
}

// DeletePortForwardConfig deletes a port forward configuration
func (a *App) DeletePortForwardConfig(configID string) error {
	debug.LogPortforward("DeletePortForwardConfig called", map[string]interface{}{"id": configID})
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.DeleteConfig(configID)
}

// StartPortForward starts a port forward
func (a *App) StartPortForward(configID string) error {
	debug.LogPortforward("StartPortForward called", map[string]interface{}{"id": configID})
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.Start(configID)
}

// StopPortForward stops a port forward
func (a *App) StopPortForward(configID string) error {
	debug.LogPortforward("StopPortForward called", map[string]interface{}{"id": configID})
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.Stop(configID)
}

// StopAllPortForwards stops all active port forwards
func (a *App) StopAllPortForwards() {
	debug.LogPortforward("StopAllPortForwards called", nil)
	if a.portForwardManager == nil {
		return
	}
	a.portForwardManager.StopAll()
}

// GetAvailablePort finds an available local port
func (a *App) GetAvailablePort(preferred int) int {
	debug.LogPortforward("GetAvailablePort called", map[string]interface{}{"preferred": preferred})
	if a.portForwardManager == nil {
		return 0
	}
	return a.portForwardManager.GetAvailablePort(preferred)
}

// GetRandomAvailablePort gets a random available port avoiding well-known and configured ports
func (a *App) GetRandomAvailablePort() int {
	debug.LogPortforward("GetRandomAvailablePort called", nil)
	if a.portForwardManager == nil {
		return 0
	}
	return a.portForwardManager.GetRandomAvailablePort()
}

// StartFavoritePortForwards starts all favorite port forwards for a context
func (a *App) StartFavoritePortForwards(contextName string) {
	debug.LogPortforward("StartFavoritePortForwards called", map[string]interface{}{"context": contextName})
	if a.portForwardManager == nil {
		return
	}
	a.portForwardManager.StartFavorites(contextName)
}

// StartPortForwardsWithMode starts port forwards based on the specified mode
// mode can be: "all", "favorites", "none"
// Only starts forwards that were running when the app was closed
func (a *App) StartPortForwardsWithMode(contextName, mode string) {
	debug.LogPortforward("StartPortForwardsWithMode called", map[string]interface{}{"context": contextName, "mode": mode})
	if a.portForwardManager == nil {
		return
	}
	a.portForwardManager.StartWithMode(contextName, mode)
}
