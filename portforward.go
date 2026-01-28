package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForwardConfig represents a saved port forward configuration
type PortForwardConfig struct {
	ID           string    `json:"id"`
	Context      string    `json:"context"`      // K8s context name
	Namespace    string    `json:"namespace"`
	ResourceType string    `json:"resourceType"` // "pod" or "service"
	ResourceName string    `json:"resourceName"`
	LocalPort    int       `json:"localPort"`
	RemotePort   int       `json:"remotePort"`
	Label        string    `json:"label"`      // User-friendly name
	Favorite     bool      `json:"favorite"`   // Marks as favorite (for auto-start mode "favorites")
	WasRunning   bool      `json:"wasRunning"` // Was running when app was closed (for auto-start)
	HTTPS        bool      `json:"https"`      // Use HTTPS when opening in browser
	CreatedAt    time.Time `json:"createdAt"`
}

// ActivePortForward represents a running port forward
type ActivePortForward struct {
	Config    PortForwardConfig `json:"config"`
	Status    string            `json:"status"` // "starting", "running", "stopped", "error"
	Error     string            `json:"error"`
	StartedAt time.Time         `json:"startedAt"`
	// Internal fields (not serialized)
	stopChan chan struct{}
	doneChan chan struct{}
	stopOnce sync.Once // Ensures stopChan is only closed once
}

// PortForwardEvent is emitted when port forward status changes
type PortForwardEvent struct {
	Type     string            `json:"type"` // "started", "stopped", "error", "config_added", "config_removed", "config_updated"
	ConfigID string            `json:"configId"`
	Config   *PortForwardConfig `json:"config,omitempty"`
	Status   string            `json:"status,omitempty"`
	Error    string            `json:"error,omitempty"`
}

// PortForwardManager manages port forward configurations and active forwards
type PortForwardManager struct {
	app        *App
	configs    map[string]*PortForwardConfig
	active     map[string]*ActivePortForward
	usedPorts  map[int]string // localPort -> configID
	mutex      sync.RWMutex
	configPath string
}

// PortForwardStorage is the JSON structure for persisting configs
type PortForwardStorage struct {
	Configs []PortForwardConfig `json:"configs"`
}

// NewPortForwardManager creates a new port forward manager
func NewPortForwardManager(app *App) *PortForwardManager {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.TempDir()
	}

	pfDir := filepath.Join(configDir, "kubikles")
	os.MkdirAll(pfDir, 0755)

	m := &PortForwardManager{
		app:        app,
		configs:    make(map[string]*PortForwardConfig),
		active:     make(map[string]*ActivePortForward),
		usedPorts:  make(map[int]string),
		configPath: filepath.Join(pfDir, "port_forwards.json"),
	}

	m.loadConfigs()
	return m
}

// loadConfigs loads saved configurations from disk
func (m *PortForwardManager) loadConfigs() {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			m.app.LogDebug("PortForward: Failed to read config file: %v", err)
		}
		return
	}

	var storage PortForwardStorage
	if err := json.Unmarshal(data, &storage); err != nil {
		m.app.LogDebug("PortForward: Failed to parse config file: %v", err)
		return
	}

	for _, cfg := range storage.Configs {
		cfgCopy := cfg
		m.configs[cfg.ID] = &cfgCopy
		m.usedPorts[cfg.LocalPort] = cfg.ID
	}

	m.app.LogDebug("PortForward: Loaded %d configurations", len(m.configs))
}

// saveConfigs persists configurations to disk
func (m *PortForwardManager) saveConfigs() error {
	storage := PortForwardStorage{
		Configs: make([]PortForwardConfig, 0, len(m.configs)),
	}

	for _, cfg := range m.configs {
		storage.Configs = append(storage.Configs, *cfg)
	}

	data, err := json.MarshalIndent(storage, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configs: %w", err)
	}

	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetConfigs returns all configurations, optionally filtered by context
func (m *PortForwardManager) GetConfigs(contextFilter string) []PortForwardConfig {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make([]PortForwardConfig, 0)
	for _, cfg := range m.configs {
		if contextFilter == "" || cfg.Context == contextFilter {
			result = append(result, *cfg)
		}
	}
	return result
}

// GetActiveForwards returns all active port forwards
func (m *PortForwardManager) GetActiveForwards() []ActivePortForward {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make([]ActivePortForward, 0, len(m.active))
	for _, af := range m.active {
		result = append(result, ActivePortForward{
			Config:    af.Config,
			Status:    af.Status,
			Error:     af.Error,
			StartedAt: af.StartedAt,
		})
	}
	return result
}

// AddConfig adds a new port forward configuration
func (m *PortForwardManager) AddConfig(cfg PortForwardConfig) (*PortForwardConfig, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Generate ID if not provided
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}
	cfg.CreatedAt = time.Now()

	// Check if local port is already used by another config
	if existingID, exists := m.usedPorts[cfg.LocalPort]; exists && existingID != cfg.ID {
		return nil, fmt.Errorf("local port %d is already used by another port forward", cfg.LocalPort)
	}

	m.configs[cfg.ID] = &cfg
	m.usedPorts[cfg.LocalPort] = cfg.ID

	if err := m.saveConfigs(); err != nil {
		m.app.LogDebug("PortForward: Failed to save configs: %v", err)
	}

	m.emitEvent(PortForwardEvent{
		Type:     "config_added",
		ConfigID: cfg.ID,
		Config:   &cfg,
	})

	return &cfg, nil
}

// UpdateConfig updates an existing configuration
func (m *PortForwardManager) UpdateConfig(cfg PortForwardConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	existing, exists := m.configs[cfg.ID]
	if !exists {
		return fmt.Errorf("config not found: %s", cfg.ID)
	}

	// If port changed, check if new port is available
	if existing.LocalPort != cfg.LocalPort {
		if existingID, used := m.usedPorts[cfg.LocalPort]; used && existingID != cfg.ID {
			return fmt.Errorf("local port %d is already used by another port forward", cfg.LocalPort)
		}
		delete(m.usedPorts, existing.LocalPort)
		m.usedPorts[cfg.LocalPort] = cfg.ID
	}

	// Preserve creation time
	cfg.CreatedAt = existing.CreatedAt
	m.configs[cfg.ID] = &cfg

	if err := m.saveConfigs(); err != nil {
		m.app.LogDebug("PortForward: Failed to save configs: %v", err)
	}

	m.emitEvent(PortForwardEvent{
		Type:     "config_updated",
		ConfigID: cfg.ID,
		Config:   &cfg,
	})

	return nil
}

// DeleteConfig removes a configuration and stops any active forward
func (m *PortForwardManager) DeleteConfig(configID string) error {
	m.mutex.Lock()

	cfg, exists := m.configs[configID]
	if !exists {
		m.mutex.Unlock()
		return fmt.Errorf("config not found: %s", configID)
	}

	// Stop if active
	if af, isActive := m.active[configID]; isActive {
		m.mutex.Unlock()
		m.Stop(configID)
		m.mutex.Lock()
		_ = af // Silence unused warning
	}

	delete(m.usedPorts, cfg.LocalPort)
	delete(m.configs, configID)

	if err := m.saveConfigs(); err != nil {
		m.app.LogDebug("PortForward: Failed to save configs: %v", err)
	}

	m.mutex.Unlock()

	m.emitEvent(PortForwardEvent{
		Type:     "config_removed",
		ConfigID: configID,
	})

	return nil
}

// Start starts a port forward for the given config ID
func (m *PortForwardManager) Start(configID string) error {
	m.mutex.Lock()

	cfg, exists := m.configs[configID]
	if !exists {
		m.mutex.Unlock()
		return fmt.Errorf("config not found: %s", configID)
	}

	// Check if already active
	if _, isActive := m.active[configID]; isActive {
		m.mutex.Unlock()
		return fmt.Errorf("port forward already running for %s", configID)
	}

	// Check if port is available
	if !m.isPortAvailable(cfg.LocalPort) {
		m.mutex.Unlock()
		return fmt.Errorf("local port %d is not available", cfg.LocalPort)
	}

	// Mark as running and persist (so state survives unexpected shutdown)
	cfg.WasRunning = true
	if err := m.saveConfigs(); err != nil {
		m.app.LogDebug("PortForward: Failed to save running state on start: %v", err)
	}

	af := &ActivePortForward{
		Config:    *cfg,
		Status:    "starting",
		StartedAt: time.Now(),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}
	m.active[configID] = af

	m.mutex.Unlock()

	m.emitEvent(PortForwardEvent{
		Type:     "started",
		ConfigID: configID,
		Config:   cfg,
		Status:   "starting",
	})

	// Start the port forward in a goroutine
	go m.runPortForward(af)

	return nil
}

// Stop stops an active port forward
func (m *PortForwardManager) Stop(configID string) error {
	return m.stopInternal(configID, true)
}

// stopInternal stops an active port forward, optionally updating wasRunning state
func (m *PortForwardManager) stopInternal(configID string, updateWasRunning bool) error {
	m.mutex.Lock()

	af, exists := m.active[configID]
	if !exists {
		m.mutex.Unlock()
		return fmt.Errorf("no active port forward for %s", configID)
	}

	// Mark as not running and persist (so state survives unexpected shutdown)
	// Skip this when shutting down - SaveRunningState already captured the state
	if updateWasRunning {
		if cfg, cfgExists := m.configs[configID]; cfgExists {
			cfg.WasRunning = false
			if err := m.saveConfigs(); err != nil {
				m.app.LogDebug("PortForward: Failed to save running state on stop: %v", err)
			}
		}
	}

	m.mutex.Unlock()

	// Signal stop (only once to prevent panic on double-close)
	af.stopOnce.Do(func() {
		close(af.stopChan)
	})

	// Wait for goroutine to finish (with timeout)
	select {
	case <-af.doneChan:
	case <-time.After(5 * time.Second):
		m.app.LogDebug("PortForward: Timeout waiting for port forward to stop: %s", configID)
	}

	m.mutex.Lock()
	delete(m.active, configID)
	m.mutex.Unlock()

	m.emitEvent(PortForwardEvent{
		Type:     "stopped",
		ConfigID: configID,
		Status:   "stopped",
	})

	return nil
}

// StopAll stops all active port forwards, optionally saving running state first
func (m *PortForwardManager) StopAll() {
	m.mutex.RLock()
	ids := make([]string, 0, len(m.active))
	for id := range m.active {
		ids = append(ids, id)
	}
	m.mutex.RUnlock()

	for _, id := range ids {
		m.Stop(id)
	}
}

// CleanupIngressConfigs removes port forward configs that were created by ingress forwarding
// This handles the case where configs persist after a crash/force-quit
func (m *PortForwardManager) CleanupIngressConfigs(contextName string) {
	m.mutex.Lock()
	var toDelete []string
	for id, cfg := range m.configs {
		// Match ingress-managed configs by label pattern and context
		if cfg.Context == contextName && strings.HasPrefix(cfg.Label, "Ingress ") {
			toDelete = append(toDelete, id)
		}
	}
	m.mutex.Unlock()

	// Delete outside lock to avoid deadlock (DeleteConfig acquires lock)
	for _, id := range toDelete {
		m.app.LogDebug("PortForward: Cleaning up orphaned ingress config: %s", id)
		m.DeleteConfig(id)
	}
}

// StopAllAndSaveState saves running state and then stops all active port forwards
// This should be called on app shutdown
func (m *PortForwardManager) StopAllAndSaveState() {
	m.SaveRunningState()

	// Stop all active forwards WITHOUT updating wasRunning state
	// (SaveRunningState already captured the correct state)
	m.mutex.RLock()
	ids := make([]string, 0, len(m.active))
	for id := range m.active {
		ids = append(ids, id)
	}
	m.mutex.RUnlock()

	for _, id := range ids {
		m.stopInternal(id, false) // false = don't update wasRunning
	}
}

// Well-known ports to avoid for random/automatic selection
var wellKnownPorts = map[int]bool{
	20: true, 21: true, 22: true, 23: true, 25: true, 53: true,
	80: true, 110: true, 143: true, 443: true, 465: true, 587: true,
	993: true, 995: true, 1024: true, 1433: true, 1521: true, 3000: true, 3306: true,
	3389: true, 5000: true, 5432: true, 5672: true, 6379: true, 8000: true, 8080: true,
	8443: true, 8888: true, 9000: true, 9090: true, 27017: true,
}

// GetAvailablePort finds an available local port, starting from preferred
func (m *PortForwardManager) GetAvailablePort(preferred int) int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Try preferred port first if it's not a well-known port
	if preferred > 0 && !wellKnownPorts[preferred] && m.isPortAvailable(preferred) {
		if _, used := m.usedPorts[preferred]; !used {
			if !m.isPortUsedByConfig(preferred) {
				return preferred
			}
		}
	}

	// Find next available port starting from 10000
	for port := 10000; port < 65535; port++ {
		if !wellKnownPorts[port] {
			if _, used := m.usedPorts[port]; !used && m.isPortAvailable(port) && !m.isPortUsedByConfig(port) {
				return port
			}
		}
	}

	return 0
}

// GetRandomAvailablePort returns a random available port avoiding well-known ports and configured ports
func (m *PortForwardManager) GetRandomAvailablePort() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Seed the random number generator
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Try up to 100 random ports in the range 10000-60000
	for i := 0; i < 100; i++ {
		port := 10000 + rng.Intn(50000)

		if wellKnownPorts[port] {
			continue
		}
		if _, used := m.usedPorts[port]; used {
			continue
		}
		if m.isPortUsedByConfig(port) {
			continue
		}
		if !m.isPortAvailable(port) {
			continue
		}
		return port
	}

	// Fallback to sequential search
	for port := 10000; port < 65535; port++ {
		if !wellKnownPorts[port] {
			if _, used := m.usedPorts[port]; !used && m.isPortAvailable(port) && !m.isPortUsedByConfig(port) {
				return port
			}
		}
	}

	return 0
}

// isPortUsedByConfig checks if any port forward config (active or inactive) uses this port.
// Uses O(1) map lookup instead of O(n) linear search.
func (m *PortForwardManager) isPortUsedByConfig(port int) bool {
	_, used := m.usedPorts[port]
	return used
}

// isPortAvailable checks if a port is available on the system
func (m *PortForwardManager) isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// runPortForward runs the actual port forward
func (m *PortForwardManager) runPortForward(af *ActivePortForward) {
	defer close(af.doneChan)

	cfg := af.Config

	// Get the pod name (for services, find a backing pod)
	podName := cfg.ResourceName
	podNamespace := cfg.Namespace

	if cfg.ResourceType == "service" {
		var err error
		podName, err = m.findServiceBackingPod(cfg.Context, cfg.Namespace, cfg.ResourceName)
		if err != nil {
			m.updateStatus(cfg.ID, "error", fmt.Sprintf("Failed to find pod for service: %v", err))
			return
		}
		m.app.LogDebug("PortForward: Service %s resolved to pod %s", cfg.ResourceName, podName)
	}

	// Get REST config for the specific context
	restConfig, err := m.app.k8sClient.GetRestConfigForContext(cfg.Context)
	if err != nil {
		m.updateStatus(cfg.ID, "error", fmt.Sprintf("Failed to get REST config: %v", err))
		return
	}

	// Build the port-forward URL
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", podNamespace, podName)
	hostIP := restConfig.Host

	u, err := url.Parse(hostIP)
	if err != nil {
		m.updateStatus(cfg.ID, "error", fmt.Sprintf("Failed to parse host URL: %v", err))
		return
	}
	u.Path = path

	// Create SPDY transport
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		m.updateStatus(cfg.ID, "error", fmt.Sprintf("Failed to create transport: %v", err))
		return
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", u)

	// Set up port forward
	ports := []string{fmt.Sprintf("%d:%d", cfg.LocalPort, cfg.RemotePort)}
	readyChan := make(chan struct{})

	// Create a context that cancels when stopChan closes
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-af.stopChan
		cancel()
	}()

	fw, err := portforward.New(dialer, ports, ctx.Done(), readyChan, nil, nil)
	if err != nil {
		m.updateStatus(cfg.ID, "error", fmt.Sprintf("Failed to create port forwarder: %v", err))
		return
	}

	// Run in goroutine and wait for ready or error
	errChan := make(chan error, 1)
	go func() {
		errChan <- fw.ForwardPorts()
	}()

	// Wait for ready or error
	select {
	case <-readyChan:
		m.updateStatus(cfg.ID, "running", "")
		m.app.LogDebug("PortForward: Started %s localhost:%d -> %s:%d", cfg.ID, cfg.LocalPort, podName, cfg.RemotePort)
	case err := <-errChan:
		m.updateStatus(cfg.ID, "error", fmt.Sprintf("Port forward failed: %v", err))
		return
	case <-af.stopChan:
		return
	}

	// Wait for completion or stop
	select {
	case err := <-errChan:
		if err != nil {
			m.updateStatus(cfg.ID, "error", fmt.Sprintf("Port forward error: %v", err))
		}
	case <-af.stopChan:
		// Stopped by user
	}
}

// findServiceBackingPod finds a running pod that backs a service
func (m *PortForwardManager) findServiceBackingPod(contextName, namespace, serviceName string) (string, error) {
	pods, err := m.app.k8sClient.GetServiceBackingPods(contextName, namespace, serviceName)
	if err != nil {
		return "", err
	}
	if len(pods) == 0 {
		return "", fmt.Errorf("no running pods found for service %s", serviceName)
	}
	return pods[0], nil
}

// updateStatus updates the status of an active port forward
func (m *PortForwardManager) updateStatus(configID, status, errMsg string) {
	m.mutex.Lock()
	af, exists := m.active[configID]
	if exists {
		af.Status = status
		af.Error = errMsg
	}
	m.mutex.Unlock()

	// Log errors to debug
	if status == "error" && errMsg != "" {
		m.app.LogDebug("PortForward: Error for %s: %s", configID, errMsg)
	}

	eventType := status
	if status == "running" {
		eventType = "started"
	}

	m.emitEvent(PortForwardEvent{
		Type:     eventType,
		ConfigID: configID,
		Status:   status,
		Error:    errMsg,
	})
}

// emitEvent emits a port forward event to the frontend
func (m *PortForwardManager) emitEvent(event PortForwardEvent) {
	m.app.emitEvent("port-forward-event", event)
}

// SaveRunningState saves which port forwards are currently running (called before shutdown)
func (m *PortForwardManager) SaveRunningState() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Mark all configs with their current running state
	for id, cfg := range m.configs {
		_, isActive := m.active[id]
		cfg.WasRunning = isActive
	}

	if err := m.saveConfigs(); err != nil {
		m.app.LogDebug("PortForward: Failed to save running state: %v", err)
	} else {
		m.app.LogDebug("PortForward: Saved running state for %d configs", len(m.configs))
	}
}

// StartWithMode starts port forwards based on the specified mode
// mode can be: "all", "favorites", "none"
// Only starts forwards that were running when the app was closed (wasRunning=true)
func (m *PortForwardManager) StartWithMode(contextName, mode string) {
	if mode == "none" {
		m.app.LogDebug("PortForward: Auto-start mode is 'none', not starting any forwards")
		return
	}

	m.mutex.RLock()
	var toStart []string
	var skippedNotRunning, skippedNotFavorite, skippedWrongContext, skippedAlreadyActive int
	for id, cfg := range m.configs {
		if cfg.Context != contextName {
			skippedWrongContext++
			continue
		}
		if _, isActive := m.active[id]; isActive {
			skippedAlreadyActive++
			continue // Already running
		}
		if !cfg.WasRunning {
			skippedNotRunning++
			continue // Wasn't running when app was closed
		}

		// Check mode-specific criteria
		switch mode {
		case "all":
			toStart = append(toStart, id)
		case "favorites":
			if cfg.Favorite {
				toStart = append(toStart, id)
			} else {
				skippedNotFavorite++
			}
		}
	}
	m.mutex.RUnlock()

	m.app.LogDebug("PortForward: Starting %d port forwards with mode '%s' for context '%s' (skipped: %d not running, %d not favorite, %d wrong context, %d already active)",
		len(toStart), mode, contextName, skippedNotRunning, skippedNotFavorite, skippedWrongContext, skippedAlreadyActive)

	for _, id := range toStart {
		if err := m.Start(id); err != nil {
			m.app.LogDebug("PortForward: Failed to start %s: %v", id, err)
		}
	}
}

// StartFavorites starts all favorite port forwards for the current context (legacy method)
func (m *PortForwardManager) StartFavorites(contextName string) {
	m.mutex.RLock()
	var toStart []string
	for id, cfg := range m.configs {
		if cfg.Favorite && cfg.Context == contextName {
			if _, isActive := m.active[id]; !isActive {
				toStart = append(toStart, id)
			}
		}
	}
	m.mutex.RUnlock()

	for _, id := range toStart {
		if err := m.Start(id); err != nil {
			m.app.LogDebug("PortForward: Failed to start favorite %s: %v", id, err)
		}
	}
}
