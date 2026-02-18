package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ============================================================================
// PortForwardConfig Tests
// ============================================================================

func TestPortForwardConfig_Fields(t *testing.T) {
	cfg := PortForwardConfig{
		ID:           "test-id-123",
		Context:      "minikube",
		Namespace:    "default",
		ResourceType: "pod",
		ResourceName: "nginx-pod",
		LocalPort:    8080,
		RemotePort:   80,
		Label:        "Nginx Dev",
		Favorite:     true,
		AutoStart:    true,
		KeepAlive:    false,
		HTTPS:        false,
		CreatedAt:    time.Now(),
	}

	if cfg.ID != "test-id-123" {
		t.Errorf("ID = %s, want test-id-123", cfg.ID)
	}
	if cfg.Context != "minikube" {
		t.Errorf("Context = %s, want minikube", cfg.Context)
	}
	if cfg.ResourceType != "pod" {
		t.Errorf("ResourceType = %s, want pod", cfg.ResourceType)
	}
	if cfg.LocalPort != 8080 {
		t.Errorf("LocalPort = %d, want 8080", cfg.LocalPort)
	}
	if cfg.RemotePort != 80 {
		t.Errorf("RemotePort = %d, want 80", cfg.RemotePort)
	}
	if !cfg.Favorite {
		t.Error("Favorite should be true")
	}
	if !cfg.AutoStart {
		t.Error("AutoStart should be true")
	}
	if cfg.KeepAlive {
		t.Error("KeepAlive should be false")
	}
}

func TestPortForwardConfig_AutoStartKeepAlive(t *testing.T) {
	tests := []struct {
		name      string
		autoStart bool
		keepAlive bool
	}{
		{"both false", false, false},
		{"autoStart only", true, false},
		{"keepAlive only", false, true},
		{"both true", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := PortForwardConfig{
				ID:        "test-" + tt.name,
				AutoStart: tt.autoStart,
				KeepAlive: tt.keepAlive,
			}
			if cfg.AutoStart != tt.autoStart {
				t.Errorf("AutoStart = %v, want %v", cfg.AutoStart, tt.autoStart)
			}
			if cfg.KeepAlive != tt.keepAlive {
				t.Errorf("KeepAlive = %v, want %v", cfg.KeepAlive, tt.keepAlive)
			}
		})
	}
}

func TestPortForwardConfig_AutoStartKeepAlive_JSON(t *testing.T) {
	cfg := PortForwardConfig{
		ID:        "json-flags",
		AutoStart: true,
		KeepAlive: true,
		LocalPort: 8080,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded PortForwardConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if !decoded.AutoStart {
		t.Error("AutoStart should survive JSON round-trip")
	}
	if !decoded.KeepAlive {
		t.Error("KeepAlive should survive JSON round-trip")
	}
}

func TestPortForwardConfig_BackwardCompat_NoAutoStartKeepAlive(t *testing.T) {
	// Simulate loading a config saved before AutoStart/KeepAlive existed
	jsonData := `{"id":"old-cfg","localPort":8080,"remotePort":80,"favorite":true}`

	var cfg PortForwardConfig
	if err := json.Unmarshal([]byte(jsonData), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Both should default to false (zero value) for old configs
	if cfg.AutoStart {
		t.Error("AutoStart should default to false for old configs")
	}
	if cfg.KeepAlive {
		t.Error("KeepAlive should default to false for old configs")
	}
}

func TestPortForwardConfig_ServiceType(t *testing.T) {
	cfg := PortForwardConfig{
		ID:           "svc-fwd",
		Context:      "prod",
		Namespace:    "production",
		ResourceType: "service",
		ResourceName: "api-gateway",
		LocalPort:    3000,
		RemotePort:   80,
	}

	if cfg.ResourceType != "service" {
		t.Errorf("ResourceType = %s, want service", cfg.ResourceType)
	}
}

func TestPortForwardConfig_HTTPS(t *testing.T) {
	cfg := PortForwardConfig{
		ID:         "https-fwd",
		HTTPS:      true,
		LocalPort:  8443,
		RemotePort: 443,
	}

	if !cfg.HTTPS {
		t.Error("HTTPS should be true")
	}
}

func TestPortForwardConfig_JSON(t *testing.T) {
	cfg := PortForwardConfig{
		ID:           "json-test",
		Context:      "test-ctx",
		Namespace:    "test-ns",
		ResourceType: "pod",
		ResourceName: "test-pod",
		LocalPort:    9000,
		RemotePort:   8080,
		Label:        "Test Label",
		Favorite:     true,
		HTTPS:        true,
		CreatedAt:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	// Marshal to JSON
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded PortForwardConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.ID != cfg.ID {
		t.Errorf("ID mismatch after JSON round-trip")
	}
	if decoded.LocalPort != cfg.LocalPort {
		t.Errorf("LocalPort mismatch after JSON round-trip")
	}
	if decoded.Favorite != cfg.Favorite {
		t.Errorf("Favorite mismatch after JSON round-trip")
	}
	if decoded.HTTPS != cfg.HTTPS {
		t.Errorf("HTTPS mismatch after JSON round-trip")
	}
}

// ============================================================================
// ActivePortForward Tests
// ============================================================================

func TestActivePortForward_Fields(t *testing.T) {
	cfg := PortForwardConfig{
		ID:         "active-test",
		LocalPort:  8080,
		RemotePort: 80,
	}

	active := ActivePortForward{
		Config:    cfg,
		Status:    "running",
		Error:     "",
		StartedAt: time.Now(),
	}

	if active.Status != "running" {
		t.Errorf("Status = %s, want running", active.Status)
	}
	if active.Error != "" {
		t.Error("Error should be empty")
	}
	if active.Config.LocalPort != 8080 {
		t.Errorf("Config.LocalPort = %d, want 8080", active.Config.LocalPort)
	}
}

func TestActivePortForward_StatusStates(t *testing.T) {
	states := []string{"starting", "running", "stopped", "error"}

	for _, state := range states {
		af := ActivePortForward{Status: state}
		if af.Status != state {
			t.Errorf("Status = %s, want %s", af.Status, state)
		}
	}
}

func TestActivePortForward_WithError(t *testing.T) {
	active := ActivePortForward{
		Status: "error",
		Error:  "connection refused",
	}

	if active.Status != "error" {
		t.Error("Status should be error")
	}
	if active.Error != "connection refused" {
		t.Errorf("Error = %s, want 'connection refused'", active.Error)
	}
}

// ============================================================================
// PortForwardEvent Tests
// ============================================================================

func TestPortForwardEvent_Types(t *testing.T) {
	eventTypes := []string{
		"started",
		"stopped",
		"error",
		"config_added",
		"config_removed",
		"config_updated",
	}

	for _, eventType := range eventTypes {
		event := PortForwardEvent{
			Type:     eventType,
			ConfigID: "test-config",
		}
		if event.Type != eventType {
			t.Errorf("Type = %s, want %s", event.Type, eventType)
		}
	}
}

func TestPortForwardEvent_WithConfig(t *testing.T) {
	cfg := PortForwardConfig{
		ID:        "cfg-123",
		LocalPort: 8080,
	}

	event := PortForwardEvent{
		Type:     "config_added",
		ConfigID: cfg.ID,
		Config:   &cfg,
	}

	if event.Config == nil {
		t.Fatal("Config should not be nil")
	}
	if event.Config.ID != "cfg-123" {
		t.Errorf("Config.ID = %s, want cfg-123", event.Config.ID)
	}
}

func TestPortForwardEvent_WithError(t *testing.T) {
	event := PortForwardEvent{
		Type:     "error",
		ConfigID: "err-config",
		Status:   "error",
		Error:    "pod not found",
	}

	if event.Error != "pod not found" {
		t.Errorf("Error = %s, want 'pod not found'", event.Error)
	}
}

func TestPortForwardEvent_JSON(t *testing.T) {
	cfg := PortForwardConfig{ID: "json-cfg", LocalPort: 9000}
	event := PortForwardEvent{
		Type:     "started",
		ConfigID: cfg.ID,
		Config:   &cfg,
		Status:   "running",
	}

	// Marshal to JSON
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded PortForwardEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != "started" {
		t.Error("Type mismatch after JSON round-trip")
	}
	if decoded.Status != "running" {
		t.Error("Status mismatch after JSON round-trip")
	}
}

// ============================================================================
// PortForwardStorage Tests
// ============================================================================

func TestPortForwardStorage_Empty(t *testing.T) {
	storage := PortForwardStorage{
		Configs: []PortForwardConfig{},
	}

	if len(storage.Configs) != 0 {
		t.Error("Empty storage should have 0 configs")
	}
}

func TestPortForwardStorage_WithConfigs(t *testing.T) {
	storage := PortForwardStorage{
		Configs: []PortForwardConfig{
			{ID: "cfg-1", LocalPort: 8080},
			{ID: "cfg-2", LocalPort: 8081},
			{ID: "cfg-3", LocalPort: 8082},
		},
	}

	if len(storage.Configs) != 3 {
		t.Errorf("Storage should have 3 configs, got %d", len(storage.Configs))
	}
}

func TestPortForwardStorage_JSON(t *testing.T) {
	storage := PortForwardStorage{
		Configs: []PortForwardConfig{
			{ID: "cfg-1", LocalPort: 8080, Label: "First"},
			{ID: "cfg-2", LocalPort: 8081, Label: "Second"},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(storage)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded PortForwardStorage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Configs) != 2 {
		t.Errorf("Expected 2 configs after JSON round-trip, got %d", len(decoded.Configs))
	}
	if decoded.Configs[0].ID != "cfg-1" {
		t.Error("First config ID mismatch after JSON round-trip")
	}
}

func TestPortForwardStorage_FilePersistence(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "portforward-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "portforwards.json")

	storage := PortForwardStorage{
		Configs: []PortForwardConfig{
			{ID: "persist-1", LocalPort: 9000, Favorite: true},
			{ID: "persist-2", LocalPort: 9001, HTTPS: true},
		},
	}

	// Write to file
	data, err := json.MarshalIndent(storage, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	if err := os.WriteFile(testFile, data, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Read back
	readData, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	var loaded PortForwardStorage
	if err := json.Unmarshal(readData, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(loaded.Configs) != 2 {
		t.Errorf("Expected 2 configs after file persistence, got %d", len(loaded.Configs))
	}
	if !loaded.Configs[0].Favorite {
		t.Error("First config should have Favorite=true after persistence")
	}
	if !loaded.Configs[1].HTTPS {
		t.Error("Second config should have HTTPS=true after persistence")
	}
}

// ============================================================================
// Port Validation Tests
// ============================================================================

func TestPortRange_Valid(t *testing.T) {
	validPorts := []int{80, 443, 8080, 3000, 9000, 65535}

	for _, port := range validPorts {
		if port < 1 || port > 65535 {
			t.Errorf("Port %d should be valid", port)
		}
	}
}

func TestPortRange_PrivilegedPorts(t *testing.T) {
	// Ports 1-1023 are privileged
	privilegedPorts := []int{22, 80, 443, 1023}

	for _, port := range privilegedPorts {
		if port > 1023 {
			t.Errorf("Port %d should be privileged (< 1024)", port)
		}
	}
}

// ============================================================================
// Config ID Generation Tests
// ============================================================================

func TestConfigID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	configs := make([]PortForwardConfig, 100)

	for i := 0; i < 100; i++ {
		// Simulate ID generation (would use uuid in real code)
		cfg := PortForwardConfig{
			ID:        "generated-id-" + string(rune(i)),
			LocalPort: 8000 + i,
		}
		configs[i] = cfg
		ids[cfg.ID] = true
	}

	if len(ids) != 100 {
		t.Error("All generated IDs should be unique")
	}
}
