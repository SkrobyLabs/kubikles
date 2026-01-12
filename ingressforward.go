package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"kubikles/pkg/hosts"
)

// IngressController represents a detected ingress controller
type IngressController struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Type      string `json:"type"` // "traefik", "nginx", "other"
	HTTPPort  int32  `json:"httpPort"`
	HTTPSPort int32  `json:"httpsPort"`
}

// IngressForwardState represents the current state of ingress forwarding
type IngressForwardState struct {
	Active           bool               `json:"active"`
	Status           string             `json:"status"` // "stopped", "starting", "running", "error"
	Error            string             `json:"error,omitempty"`
	Controller       *IngressController `json:"controller,omitempty"`
	LocalHTTPPort    int                `json:"localHttpPort"`
	LocalHTTPSPort   int                `json:"localHttpsPort"`
	Hostnames        []string           `json:"hostnames"`
	PortForwardIDs   []string           `json:"portForwardIds"`   // IDs in PortForwardManager
	HostsFileUpdated bool               `json:"hostsFileUpdated"` // Whether hosts file was modified
}

// IngressForwardEvent is emitted when ingress forward status changes
type IngressForwardEvent struct {
	Type  string              `json:"type"` // "started", "stopped", "error", "hosts_updated", "hosts_cleared"
	State IngressForwardState `json:"state"`
}

// IngressForwardManager manages ingress forwarding with hosts file updates
type IngressForwardManager struct {
	app          *App
	state        IngressForwardState
	hostsManager *hosts.Manager
	mutex        sync.RWMutex
}

// NewIngressForwardManager creates a new ingress forward manager
func NewIngressForwardManager(app *App) *IngressForwardManager {
	return &IngressForwardManager{
		app:          app,
		hostsManager: hosts.NewManager(),
		state: IngressForwardState{
			Active: false,
			Status: "stopped",
		},
	}
}

// GetState returns the current ingress forward state
func (m *IngressForwardManager) GetState() IngressForwardState {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.state
}

// DetectIngressController finds the ingress controller service in the cluster
func (m *IngressForwardManager) DetectIngressController() (*IngressController, error) {
	// Common ingress controller patterns
	patterns := []struct {
		namespace   string
		serviceName string
		controlType string
	}{
		// Traefik patterns (including k3s variations)
		{"traefik", "traefik", "traefik"},
		{"traefik-system", "traefik", "traefik"},
		{"kube-system", "traefik", "traefik"},
		{"kube-system", "traefik-ingress-service", "traefik"},
		{"kube-system", "traefik-ingress-controller", "traefik"},
		{"default", "traefik", "traefik"},
		// nginx-ingress patterns
		{"ingress-nginx", "ingress-nginx-controller", "nginx"},
		{"nginx-ingress", "nginx-ingress-controller", "nginx"},
		{"kube-system", "nginx-ingress-controller", "nginx"},
		// Contour patterns
		{"projectcontour", "envoy", "contour"},
		// HAProxy patterns
		{"haproxy-controller", "haproxy-ingress", "haproxy"},
	}

	for _, p := range patterns {
		services, err := m.app.k8sClient.ListServices(p.namespace)
		if err != nil {
			m.app.LogDebug("IngressForward: Failed to list services in namespace %s: %v", p.namespace, err)
			continue // Namespace might not exist
		}

		for _, svc := range services {
			if svc.Name == p.serviceName {
				controller := &IngressController{
					Namespace: svc.Namespace,
					Name:      svc.Name,
					Type:      p.controlType,
				}

				// Find HTTP and HTTPS ports
				for _, port := range svc.Spec.Ports {
					portName := strings.ToLower(port.Name)
					switch {
					case port.Port == 80 || portName == "http" || portName == "web":
						controller.HTTPPort = port.Port
					case port.Port == 443 || portName == "https" || portName == "websecure":
						controller.HTTPSPort = port.Port
					}
				}

				// Default ports if not found
				if controller.HTTPPort == 0 {
					controller.HTTPPort = 80
				}
				if controller.HTTPSPort == 0 {
					controller.HTTPSPort = 443
				}

				m.app.LogDebug("IngressForward: Detected %s controller at %s/%s (HTTP:%d, HTTPS:%d)",
					controller.Type, controller.Namespace, controller.Name, controller.HTTPPort, controller.HTTPSPort)
				return controller, nil
			}
		}
	}

	// Fallback: search all namespaces for services with traefik/ingress in name or labels
	m.app.LogDebug("IngressForward: Pattern matching failed, trying fallback search")
	allServices, err := m.app.k8sClient.ListServices("")
	if err == nil {
		for _, svc := range allServices {
			name := strings.ToLower(svc.Name)
			appLabel := strings.ToLower(svc.Labels["app"])
			appK8sName := strings.ToLower(svc.Labels["app.kubernetes.io/name"])

			var controllerType string
			if strings.Contains(name, "traefik") || strings.Contains(appLabel, "traefik") || strings.Contains(appK8sName, "traefik") {
				controllerType = "traefik"
			} else if strings.Contains(name, "nginx") || strings.Contains(appLabel, "nginx") || strings.Contains(appK8sName, "nginx") {
				controllerType = "nginx"
			} else if strings.Contains(name, "ingress") && (strings.Contains(appLabel, "ingress") || strings.Contains(appK8sName, "ingress")) {
				controllerType = "ingress"
			}

			if controllerType != "" {
				// Check if service has HTTP-like ports
				hasHTTPPort := false
				for _, port := range svc.Spec.Ports {
					if port.Port == 80 || port.Port == 443 || port.Port == 8080 || port.Port == 8443 {
						hasHTTPPort = true
						break
					}
				}
				if !hasHTTPPort {
					continue
				}

				controller := &IngressController{
					Namespace: svc.Namespace,
					Name:      svc.Name,
					Type:      controllerType,
				}

				for _, port := range svc.Spec.Ports {
					portName := strings.ToLower(port.Name)
					switch {
					case port.Port == 80 || portName == "http" || portName == "web":
						controller.HTTPPort = port.Port
					case port.Port == 443 || portName == "https" || portName == "websecure":
						controller.HTTPSPort = port.Port
					}
				}
				if controller.HTTPPort == 0 {
					controller.HTTPPort = 80
				}
				if controller.HTTPSPort == 0 {
					controller.HTTPSPort = 443
				}

				m.app.LogDebug("IngressForward: Fallback detected %s controller at %s/%s (HTTP:%d, HTTPS:%d)",
					controller.Type, controller.Namespace, controller.Name, controller.HTTPPort, controller.HTTPSPort)
				return controller, nil
			}
		}
	}

	return nil, fmt.Errorf("no ingress controller found in cluster")
}

// CollectIngressHostnames collects all unique hostnames from ingresses in the cluster
func (m *IngressForwardManager) CollectIngressHostnames(namespaces []string) ([]string, error) {
	hostnameSet := make(map[string]bool)

	// If no namespaces specified, get all ingresses
	if len(namespaces) == 0 {
		namespaces = []string{""}
	}

	for _, ns := range namespaces {
		ingresses, err := m.app.k8sClient.ListIngresses(ns)
		if err != nil {
			m.app.LogDebug("IngressForward: Failed to list ingresses in namespace %s: %v", ns, err)
			continue
		}

		for _, ing := range ingresses {
			for _, rule := range ing.Spec.Rules {
				if rule.Host != "" && !strings.Contains(rule.Host, "*") {
					hostnameSet[rule.Host] = true
				}
			}
			// Also check TLS hosts
			for _, tls := range ing.Spec.TLS {
				for _, host := range tls.Hosts {
					if host != "" && !strings.Contains(host, "*") {
						hostnameSet[host] = true
					}
				}
			}
		}
	}

	hostnames := make([]string, 0, len(hostnameSet))
	for h := range hostnameSet {
		hostnames = append(hostnames, h)
	}
	sort.Strings(hostnames)

	return hostnames, nil
}

// Start begins ingress forwarding: port forwards to controller and updates hosts file
func (m *IngressForwardManager) Start(controller *IngressController, namespaces []string) error {
	m.mutex.Lock()
	if m.state.Active {
		m.mutex.Unlock()
		return fmt.Errorf("ingress forwarding is already active")
	}
	m.state.Status = "starting"
	m.state.Active = true
	m.state.Error = ""
	m.mutex.Unlock()

	m.emitEvent("starting")

	// Clean up any orphaned ingress configs from previous crash/force-quit
	currentContext := m.app.k8sClient.GetCurrentContext()
	m.app.portForwardManager.CleanupIngressConfigs(currentContext)

	// Collect hostnames
	hostnames, err := m.CollectIngressHostnames(namespaces)
	if err != nil {
		m.setError(fmt.Sprintf("failed to collect hostnames: %v", err))
		return err
	}

	if len(hostnames) == 0 {
		m.setError("no ingress hostnames found in cluster")
		return fmt.Errorf("no ingress hostnames found")
	}

	m.mutex.Lock()
	m.state.Hostnames = hostnames
	m.state.Controller = controller
	m.mutex.Unlock()

	// Determine local ports - use non-privileged ports (no root needed)
	// Privileged ports (< 1024) like 80/443 require root to bind
	localHTTPPort := 0
	localHTTPSPort := 8443

	// HTTPS port - use 8443 (or find available)
	if !hosts.CheckPortAvailable(8443) {
		localHTTPSPort = m.app.portForwardManager.GetAvailablePort(8443)
		m.app.LogDebug("IngressForward: Port 8443 not available, using %d", localHTTPSPort)
	}

	// HTTP port - use 8080 if available (optional)
	if hosts.CheckPortAvailable(8080) {
		localHTTPPort = 8080
	} else {
		m.app.LogDebug("IngressForward: Port 8080 not available, skipping HTTP forwarding")
	}

	m.mutex.Lock()
	m.state.LocalHTTPPort = localHTTPPort
	m.state.LocalHTTPSPort = localHTTPSPort
	m.mutex.Unlock()

	// Create port forward configs
	var portForwardIDs []string

	// HTTPS port forward (primary - required)
	if controller.HTTPSPort > 0 {
		httpsConfig := PortForwardConfig{
			ID:           uuid.New().String(),
			Context:      m.app.k8sClient.GetCurrentContext(),
			Namespace:    controller.Namespace,
			ResourceType: "service",
			ResourceName: controller.Name,
			LocalPort:    localHTTPSPort,
			RemotePort:   int(controller.HTTPSPort),
			Label:        fmt.Sprintf("Ingress HTTPS (%s)", controller.Type),
			HTTPS:        true,
		}

		addedHTTPSConfig, err := m.app.portForwardManager.AddConfig(httpsConfig)
		if err != nil {
			m.setError(fmt.Sprintf("failed to create HTTPS port forward config: %v", err))
			return err
		}
		portForwardIDs = append(portForwardIDs, addedHTTPSConfig.ID)
	}

	// HTTP port forward (optional - only if port 80 is available)
	if localHTTPPort > 0 && controller.HTTPPort > 0 {
		httpConfig := PortForwardConfig{
			ID:           uuid.New().String(),
			Context:      m.app.k8sClient.GetCurrentContext(),
			Namespace:    controller.Namespace,
			ResourceType: "service",
			ResourceName: controller.Name,
			LocalPort:    localHTTPPort,
			RemotePort:   int(controller.HTTPPort),
			Label:        fmt.Sprintf("Ingress HTTP (%s)", controller.Type),
		}

		addedHTTPConfig, err := m.app.portForwardManager.AddConfig(httpConfig)
		if err != nil {
			m.app.LogDebug("IngressForward: Failed to create HTTP port forward (non-fatal): %v", err)
			// Don't fail - HTTP is optional
		} else {
			portForwardIDs = append(portForwardIDs, addedHTTPConfig.ID)
		}
	}

	// Must have at least HTTPS
	if len(portForwardIDs) == 0 {
		m.setError("failed to create any port forwards")
		return fmt.Errorf("failed to create any port forwards")
	}

	m.mutex.Lock()
	m.state.PortForwardIDs = portForwardIDs
	m.mutex.Unlock()

	// Start port forwards
	for _, id := range portForwardIDs {
		if err := m.app.portForwardManager.Start(id); err != nil {
			m.Stop() // Cleanup
			m.setError(fmt.Sprintf("failed to start port forward: %v", err))
			return err
		}
	}

	// Update hosts file and set up port redirection (443->8443, 80->8080)
	// Uses pfctl on macOS, iptables on Linux
	entries := make([]hosts.Entry, len(hostnames))
	for i, hostname := range hostnames {
		entries[i] = hosts.Entry{
			IP:       "127.0.0.1",
			Hostname: hostname,
		}
	}

	if err := m.hostsManager.AddEntriesWithPortRedirect(entries, localHTTPSPort, localHTTPPort); err != nil {
		m.app.LogDebug("IngressForward: Failed to update hosts file: %v", err)
		// Don't fail completely - port forwarding is still useful
		m.mutex.Lock()
		m.state.HostsFileUpdated = false
		m.state.Error = fmt.Sprintf("Port forwarding active but hosts file update failed: %v", err)
		m.mutex.Unlock()
	} else {
		m.mutex.Lock()
		m.state.HostsFileUpdated = true
		// Port redirection active (pfctl on macOS, iptables on Linux), show standard ports
		m.state.LocalHTTPSPort = 443
		if localHTTPPort > 0 {
			m.state.LocalHTTPPort = 80
		}
		m.mutex.Unlock()
		m.app.LogDebug("IngressForward: Added %d hostnames to hosts file with port redirection", len(hostnames))
	}

	m.mutex.Lock()
	m.state.Status = "running"
	m.mutex.Unlock()

	m.emitEvent("started")

	m.app.LogDebug("IngressForward: Started forwarding to %s/%s with %d hostnames",
		controller.Namespace, controller.Name, len(hostnames))

	return nil
}

// Stop stops ingress forwarding and cleans up hosts file
func (m *IngressForwardManager) Stop() error {
	m.mutex.Lock()
	if !m.state.Active {
		m.mutex.Unlock()
		return nil
	}

	portForwardIDs := m.state.PortForwardIDs
	hostsFileUpdated := m.state.HostsFileUpdated
	m.mutex.Unlock()

	// Stop port forwards
	for _, id := range portForwardIDs {
		if err := m.app.portForwardManager.Stop(id); err != nil {
			m.app.LogDebug("IngressForward: Failed to stop port forward %s: %v", id, err)
		}
		// Delete the config too
		m.app.portForwardManager.DeleteConfig(id)
	}

	// Clean up hosts file
	if hostsFileUpdated {
		if err := m.hostsManager.RemoveEntries(); err != nil {
			m.app.LogDebug("IngressForward: Failed to clean hosts file: %v", err)
		} else {
			m.app.LogDebug("IngressForward: Cleaned up hosts file entries")
		}
	}

	// Reset state
	m.mutex.Lock()
	m.state = IngressForwardState{
		Active: false,
		Status: "stopped",
	}
	m.mutex.Unlock()

	m.emitEvent("stopped")

	return nil
}

// RefreshHostnames re-collects hostnames and updates the hosts file
func (m *IngressForwardManager) RefreshHostnames(namespaces []string) error {
	m.mutex.RLock()
	if !m.state.Active {
		m.mutex.RUnlock()
		return fmt.Errorf("ingress forwarding is not active")
	}
	m.mutex.RUnlock()

	hostnames, err := m.CollectIngressHostnames(namespaces)
	if err != nil {
		return err
	}

	entries := make([]hosts.Entry, len(hostnames))
	for i, hostname := range hostnames {
		entries[i] = hosts.Entry{
			IP:       "127.0.0.1",
			Hostname: hostname,
		}
	}

	if err := m.hostsManager.AddEntries(entries); err != nil {
		return err
	}

	m.mutex.Lock()
	m.state.Hostnames = hostnames
	m.state.HostsFileUpdated = true
	m.mutex.Unlock()

	m.emitEvent("hosts_updated")

	return nil
}

// GetManagedHosts returns the currently managed hosts file entries
func (m *IngressForwardManager) GetManagedHosts() ([]hosts.Entry, error) {
	return m.hostsManager.GetManagedEntries()
}

// setError sets error state
func (m *IngressForwardManager) setError(errMsg string) {
	m.mutex.Lock()
	m.state.Status = "error"
	m.state.Error = errMsg
	m.state.Active = false
	m.mutex.Unlock()

	m.emitEvent("error")
}

// emitEvent emits an ingress forward event to the frontend
func (m *IngressForwardManager) emitEvent(eventType string) {
	if m.app.ctx != nil {
		m.mutex.RLock()
		state := m.state
		m.mutex.RUnlock()

		runtime.EventsEmit(m.app.ctx, "ingress-forward-event", IngressForwardEvent{
			Type:  eventType,
			State: state,
		})
	}
}

// Cleanup should be called on app shutdown
func (m *IngressForwardManager) Cleanup() {
	m.mutex.RLock()
	active := m.state.Active
	m.mutex.RUnlock()

	if active {
		m.Stop()
	}
}
