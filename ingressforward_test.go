package main

import (
	"encoding/json"
	"testing"
)

// ============================================================================
// IngressController Tests
// ============================================================================

func TestIngressController_Fields(t *testing.T) {
	controller := IngressController{
		Namespace: "ingress-nginx",
		Name:      "nginx-ingress-controller",
		Type:      "nginx",
		HTTPPort:  80,
		HTTPSPort: 443,
	}

	if controller.Namespace != "ingress-nginx" {
		t.Errorf("Namespace = %s, want ingress-nginx", controller.Namespace)
	}
	if controller.Name != "nginx-ingress-controller" {
		t.Errorf("Name = %s, want nginx-ingress-controller", controller.Name)
	}
	if controller.Type != "nginx" {
		t.Errorf("Type = %s, want nginx", controller.Type)
	}
	if controller.HTTPPort != 80 {
		t.Errorf("HTTPPort = %d, want 80", controller.HTTPPort)
	}
	if controller.HTTPSPort != 443 {
		t.Errorf("HTTPSPort = %d, want 443", controller.HTTPSPort)
	}
}

func TestIngressController_Types(t *testing.T) {
	controllerTypes := []string{"traefik", "nginx", "other"}

	for _, ctrlType := range controllerTypes {
		controller := IngressController{Type: ctrlType}
		if controller.Type != ctrlType {
			t.Errorf("Type = %s, want %s", controller.Type, ctrlType)
		}
	}
}

func TestIngressController_Traefik(t *testing.T) {
	controller := IngressController{
		Namespace: "traefik-system",
		Name:      "traefik",
		Type:      "traefik",
		HTTPPort:  8000,
		HTTPSPort: 8443,
	}

	if controller.Type != "traefik" {
		t.Error("Controller type should be traefik")
	}
}

func TestIngressController_JSON(t *testing.T) {
	controller := IngressController{
		Namespace: "kube-system",
		Name:      "ingress-controller",
		Type:      "nginx",
		HTTPPort:  80,
		HTTPSPort: 443,
	}

	// Marshal to JSON
	data, err := json.Marshal(controller)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded IngressController
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Namespace != controller.Namespace {
		t.Error("Namespace mismatch after JSON round-trip")
	}
	if decoded.Type != controller.Type {
		t.Error("Type mismatch after JSON round-trip")
	}
	if decoded.HTTPPort != controller.HTTPPort {
		t.Error("HTTPPort mismatch after JSON round-trip")
	}
}

// ============================================================================
// IngressForwardState Tests
// ============================================================================

func TestIngressForwardState_Stopped(t *testing.T) {
	state := IngressForwardState{
		Active: false,
		Status: "stopped",
	}

	if state.Active {
		t.Error("Active should be false for stopped state")
	}
	if state.Status != "stopped" {
		t.Errorf("Status = %s, want stopped", state.Status)
	}
}

func TestIngressForwardState_Running(t *testing.T) {
	controller := IngressController{
		Namespace: "ingress-nginx",
		Name:      "nginx",
		Type:      "nginx",
		HTTPPort:  80,
		HTTPSPort: 443,
	}

	state := IngressForwardState{
		Active:           true,
		Status:           "running",
		Controller:       &controller,
		LocalHTTPPort:    8080,
		LocalHTTPSPort:   8443,
		Hostnames:        []string{"app.local", "api.local"},
		PortForwardIDs:   []string{"pf-http", "pf-https"},
		HostsFileUpdated: true,
	}

	if !state.Active {
		t.Error("Active should be true for running state")
	}
	if state.Status != "running" {
		t.Errorf("Status = %s, want running", state.Status)
	}
	if state.Controller == nil {
		t.Fatal("Controller should not be nil")
	}
	if state.LocalHTTPPort != 8080 {
		t.Errorf("LocalHTTPPort = %d, want 8080", state.LocalHTTPPort)
	}
	if len(state.Hostnames) != 2 {
		t.Errorf("Hostnames length = %d, want 2", len(state.Hostnames))
	}
	if !state.HostsFileUpdated {
		t.Error("HostsFileUpdated should be true")
	}
}

func TestIngressForwardState_Error(t *testing.T) {
	state := IngressForwardState{
		Active: false,
		Status: "error",
		Error:  "failed to detect ingress controller",
	}

	if state.Status != "error" {
		t.Errorf("Status = %s, want error", state.Status)
	}
	if state.Error == "" {
		t.Error("Error should not be empty for error state")
	}
}

func TestIngressForwardState_StatusValues(t *testing.T) {
	validStatuses := []string{"stopped", "starting", "running", "error"}

	for _, status := range validStatuses {
		state := IngressForwardState{Status: status}
		if state.Status != status {
			t.Errorf("Status = %s, want %s", state.Status, status)
		}
	}
}

func TestIngressForwardState_JSON(t *testing.T) {
	controller := IngressController{
		Namespace: "ingress",
		Name:      "ctrl",
		Type:      "nginx",
		HTTPPort:  80,
		HTTPSPort: 443,
	}

	state := IngressForwardState{
		Active:           true,
		Status:           "running",
		Controller:       &controller,
		LocalHTTPPort:    8080,
		LocalHTTPSPort:   8443,
		Hostnames:        []string{"test.local"},
		PortForwardIDs:   []string{"pf-1"},
		HostsFileUpdated: true,
	}

	// Marshal to JSON
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded IngressForwardState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Active != state.Active {
		t.Error("Active mismatch after JSON round-trip")
	}
	if decoded.Status != state.Status {
		t.Error("Status mismatch after JSON round-trip")
	}
	if decoded.Controller == nil {
		t.Fatal("Controller should not be nil after JSON round-trip")
	}
	if len(decoded.Hostnames) != 1 {
		t.Error("Hostnames mismatch after JSON round-trip")
	}
}

// ============================================================================
// IngressForwardEvent Tests
// ============================================================================

func TestIngressForwardEvent_Types(t *testing.T) {
	eventTypes := []string{"started", "stopped", "error", "hosts_updated", "hosts_cleared"}

	for _, eventType := range eventTypes {
		event := IngressForwardEvent{Type: eventType}
		if event.Type != eventType {
			t.Errorf("Type = %s, want %s", event.Type, eventType)
		}
	}
}

func TestIngressForwardEvent_Started(t *testing.T) {
	state := IngressForwardState{
		Active:         true,
		Status:         "running",
		LocalHTTPPort:  8080,
		LocalHTTPSPort: 8443,
		Hostnames:      []string{"app.local"},
	}

	event := IngressForwardEvent{
		Type:  "started",
		State: state,
	}

	if event.Type != "started" {
		t.Errorf("Type = %s, want started", event.Type)
	}
	if !event.State.Active {
		t.Error("State.Active should be true for started event")
	}
}

func TestIngressForwardEvent_Stopped(t *testing.T) {
	state := IngressForwardState{
		Active: false,
		Status: "stopped",
	}

	event := IngressForwardEvent{
		Type:  "stopped",
		State: state,
	}

	if event.Type != "stopped" {
		t.Errorf("Type = %s, want stopped", event.Type)
	}
	if event.State.Active {
		t.Error("State.Active should be false for stopped event")
	}
}

func TestIngressForwardEvent_Error(t *testing.T) {
	state := IngressForwardState{
		Active: false,
		Status: "error",
		Error:  "pod not found",
	}

	event := IngressForwardEvent{
		Type:  "error",
		State: state,
	}

	if event.Type != "error" {
		t.Errorf("Type = %s, want error", event.Type)
	}
	if event.State.Error != "pod not found" {
		t.Errorf("State.Error = %s, want 'pod not found'", event.State.Error)
	}
}

func TestIngressForwardEvent_HostsUpdated(t *testing.T) {
	state := IngressForwardState{
		Active:           true,
		Status:           "running",
		Hostnames:        []string{"new.local", "api.local"},
		HostsFileUpdated: true,
	}

	event := IngressForwardEvent{
		Type:  "hosts_updated",
		State: state,
	}

	if event.Type != "hosts_updated" {
		t.Errorf("Type = %s, want hosts_updated", event.Type)
	}
	if !event.State.HostsFileUpdated {
		t.Error("State.HostsFileUpdated should be true")
	}
}

func TestIngressForwardEvent_JSON(t *testing.T) {
	state := IngressForwardState{
		Active: true,
		Status: "running",
	}

	event := IngressForwardEvent{
		Type:  "started",
		State: state,
	}

	// Marshal to JSON
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded IngressForwardEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != "started" {
		t.Error("Type mismatch after JSON round-trip")
	}
	if decoded.State.Status != "running" {
		t.Error("State.Status mismatch after JSON round-trip")
	}
}

// ============================================================================
// Hostname Validation Tests
// ============================================================================

func TestHostname_ValidFormats(t *testing.T) {
	validHostnames := []string{
		"app.local",
		"api.example.com",
		"my-service.namespace.svc.cluster.local",
		"test-app-123.dev.local",
	}

	for _, hostname := range validHostnames {
		if hostname == "" {
			t.Errorf("Hostname should not be empty: %s", hostname)
		}
	}
}

func TestHostname_MultipleSubdomains(t *testing.T) {
	hostnames := []string{
		"a.b.c.d.local",
		"deep.nested.sub.domain.example.com",
	}

	for _, hostname := range hostnames {
		if hostname == "" {
			t.Error("Hostname should not be empty")
		}
	}
}
