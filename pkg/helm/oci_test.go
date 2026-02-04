package helm

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ============================================================================
// ACR URL Extraction Tests
// ============================================================================

func TestExtractACRNameFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "standard ACR URL with https",
			url:      "https://myregistry.azurecr.io",
			expected: "myregistry",
		},
		{
			name:     "ACR URL without https",
			url:      "http://testacr.azurecr.io",
			expected: "testacr",
		},
		{
			name:     "ACR URL with path",
			url:      "https://myacr.azurecr.io/helm/charts",
			expected: "myacr",
		},
		{
			name:     "ACR URL already stripped",
			url:      "myregistry.azurecr.io",
			expected: "myregistry",
		},
		{
			name:     "non-ACR registry",
			url:      "https://ghcr.io/myorg",
			expected: "",
		},
		{
			name:     "Docker Hub",
			url:      "https://registry-1.docker.io",
			expected: "",
		},
		{
			name:     "empty URL",
			url:      "",
			expected: "",
		},
		{
			name:     "ACR with numbers",
			url:      "https://registry123.azurecr.io",
			expected: "registry123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractACRNameFromURL(tt.url)
			if result != tt.expected {
				t.Errorf("extractACRNameFromURL(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// OCIRegistry Struct Tests
// ============================================================================

func TestOCIRegistry_Fields(t *testing.T) {
	reg := OCIRegistry{
		URL:           "https://ghcr.io",
		Username:      "myuser",
		Authenticated: true,
		IsACR:         false,
		Priority:      10,
	}

	if reg.URL != "https://ghcr.io" {
		t.Errorf("URL = %s, want https://ghcr.io", reg.URL)
	}
	if reg.Username != "myuser" {
		t.Errorf("Username = %s, want myuser", reg.Username)
	}
	if !reg.Authenticated {
		t.Error("Authenticated should be true")
	}
	if reg.IsACR {
		t.Error("IsACR should be false")
	}
	if reg.Priority != 10 {
		t.Errorf("Priority = %d, want 10", reg.Priority)
	}
}

func TestOCIRegistry_ACR(t *testing.T) {
	reg := OCIRegistry{
		URL:           "https://myacr.azurecr.io",
		Username:      "00000000-0000-0000-0000-000000000000",
		Authenticated: true,
		IsACR:         true,
		Priority:      5,
	}

	if !reg.IsACR {
		t.Error("IsACR should be true for Azure Container Registry")
	}
}

// ============================================================================
// OCIPriorities Tests
// ============================================================================

func TestOCIPriorities_Empty(t *testing.T) {
	priorities := OCIPriorities{
		Priorities: make(map[string]int),
	}

	if len(priorities.Priorities) != 0 {
		t.Error("New priorities should be empty")
	}
}

func TestOCIPriorities_SetGet(t *testing.T) {
	priorities := OCIPriorities{
		Priorities: make(map[string]int),
	}

	priorities.Priorities["https://ghcr.io"] = 10
	priorities.Priorities["https://myacr.azurecr.io"] = 5

	if priorities.Priorities["https://ghcr.io"] != 10 {
		t.Errorf("Priority for ghcr.io = %d, want 10", priorities.Priorities["https://ghcr.io"])
	}
	if priorities.Priorities["https://myacr.azurecr.io"] != 5 {
		t.Errorf("Priority for ACR = %d, want 5", priorities.Priorities["https://myacr.azurecr.io"])
	}
}

func TestOCIPriorities_JSON(t *testing.T) {
	priorities := OCIPriorities{
		Priorities: map[string]int{
			"https://ghcr.io":          10,
			"https://myacr.azurecr.io": 5,
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(priorities)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded OCIPriorities
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Priorities["https://ghcr.io"] != 10 {
		t.Error("JSON round-trip failed for ghcr.io priority")
	}
}

// ============================================================================
// DockerConfig Tests
// ============================================================================

func TestDockerConfig_Empty(t *testing.T) {
	config := DockerConfig{
		Auths: make(map[string]DockerAuth),
	}

	if len(config.Auths) != 0 {
		t.Error("New DockerConfig should have empty Auths")
	}
}

func TestDockerConfig_WithAuth(t *testing.T) {
	// Create base64 encoded auth string (username:password)
	authStr := base64.StdEncoding.EncodeToString([]byte("myuser:mypassword"))

	config := DockerConfig{
		Auths: map[string]DockerAuth{
			"https://ghcr.io": {
				Auth:     authStr,
				Username: "myuser",
			},
		},
	}

	if auth, ok := config.Auths["https://ghcr.io"]; !ok {
		t.Error("Auth entry for ghcr.io not found")
	} else {
		if auth.Username != "myuser" {
			t.Errorf("Username = %s, want myuser", auth.Username)
		}
	}
}

func TestDockerConfig_CredsStore(t *testing.T) {
	config := DockerConfig{
		Auths:      make(map[string]DockerAuth),
		CredsStore: "desktop",
	}

	if config.CredsStore != "desktop" {
		t.Errorf("CredsStore = %s, want desktop", config.CredsStore)
	}
}

func TestDockerConfig_CredHelpers(t *testing.T) {
	config := DockerConfig{
		Auths: make(map[string]DockerAuth),
		CredHelpers: map[string]string{
			"myacr.azurecr.io": "acr-helper",
			"gcr.io":           "gcloud",
		},
	}

	if config.CredHelpers["myacr.azurecr.io"] != "acr-helper" {
		t.Error("CredHelper for ACR not set correctly")
	}
	if config.CredHelpers["gcr.io"] != "gcloud" {
		t.Error("CredHelper for GCR not set correctly")
	}
}

func TestDockerConfig_JSON(t *testing.T) {
	authStr := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	config := DockerConfig{
		Auths: map[string]DockerAuth{
			"https://ghcr.io": {
				Auth: authStr,
			},
		},
		CredsStore: "desktop",
	}

	// Marshal to JSON
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal DockerConfig: %v", err)
	}

	// Unmarshal back
	var decoded DockerConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal DockerConfig: %v", err)
	}

	if decoded.CredsStore != "desktop" {
		t.Error("JSON round-trip failed for CredsStore")
	}
	if _, ok := decoded.Auths["https://ghcr.io"]; !ok {
		t.Error("JSON round-trip failed for Auths")
	}
}

// ============================================================================
// DockerAuth Tests
// ============================================================================

func TestDockerAuth_BasicAuth(t *testing.T) {
	authStr := base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))
	auth := DockerAuth{
		Auth: authStr,
	}

	// Decode and verify
	decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
	if err != nil {
		t.Fatalf("Failed to decode auth: %v", err)
	}
	if string(decoded) != "testuser:testpass" {
		t.Errorf("Decoded auth = %s, want testuser:testpass", string(decoded))
	}
}

func TestDockerAuth_IdentityToken(t *testing.T) {
	auth := DockerAuth{
		IdentityToken: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
	}

	if auth.IdentityToken == "" {
		t.Error("IdentityToken should not be empty")
	}
}

// ============================================================================
// OCIChartVersion Tests
// ============================================================================

func TestOCIChartVersion_Fields(t *testing.T) {
	version := OCIChartVersion{
		Version: "1.2.3",
	}

	if version.Version != "1.2.3" {
		t.Errorf("Version = %s, want 1.2.3", version.Version)
	}
}

// ============================================================================
// OCIChartSource Tests
// ============================================================================

func TestOCIChartSource_Fields(t *testing.T) {
	source := OCIChartSource{
		RegistryURL: "https://ghcr.io",
		Repository:  "myorg/charts/nginx",
		ChartName:   "nginx",
		Priority:    10,
		IsACR:       false,
		Versions: []OCIChartVersion{
			{Version: "1.0.0"},
			{Version: "2.0.0"},
		},
	}

	if source.RegistryURL != "https://ghcr.io" {
		t.Errorf("RegistryURL = %s, want https://ghcr.io", source.RegistryURL)
	}
	if source.Repository != "myorg/charts/nginx" {
		t.Errorf("Repository = %s, want myorg/charts/nginx", source.Repository)
	}
	if source.ChartName != "nginx" {
		t.Errorf("ChartName = %s, want nginx", source.ChartName)
	}
	if len(source.Versions) != 2 {
		t.Errorf("Versions length = %d, want 2", len(source.Versions))
	}
}

func TestOCIChartSource_ACR(t *testing.T) {
	source := OCIChartSource{
		RegistryURL: "https://myacr.azurecr.io",
		Repository:  "helm/nginx",
		ChartName:   "nginx",
		Priority:    5,
		IsACR:       true,
	}

	if !source.IsACR {
		t.Error("IsACR should be true for Azure Container Registry source")
	}
}

// ============================================================================
// Path Helper Tests (using temp env)
// ============================================================================

func TestGetOCIPrioritiesPath(t *testing.T) {
	// This tests that the function returns a valid path
	path := getOCIPrioritiesPath()

	if path == "" {
		t.Error("getOCIPrioritiesPath() returned empty string")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("Path should be absolute: %s", path)
	}
	if filepath.Ext(path) != ".json" {
		t.Error("Path should have .json extension")
	}
}

func TestGetDockerConfigPath_Default(t *testing.T) {
	// Unset DOCKER_CONFIG to test default behavior
	orig := os.Getenv("DOCKER_CONFIG")
	os.Unsetenv("DOCKER_CONFIG")
	defer func() {
		if orig != "" {
			os.Setenv("DOCKER_CONFIG", orig)
		}
	}()

	path := getDockerConfigPath()
	if path == "" {
		t.Error("getDockerConfigPath() returned empty string")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("Path should be absolute: %s", path)
	}
}

func TestGetDockerConfigPath_EnvOverride(t *testing.T) {
	// Set DOCKER_CONFIG
	tmpDir, err := os.MkdirTemp("", "docker-config-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	orig := os.Getenv("DOCKER_CONFIG")
	os.Setenv("DOCKER_CONFIG", tmpDir)
	defer func() {
		if orig != "" {
			os.Setenv("DOCKER_CONFIG", orig)
		} else {
			os.Unsetenv("DOCKER_CONFIG")
		}
	}()

	path := getDockerConfigPath()
	expected := filepath.Join(tmpDir, "config.json")
	if path != expected {
		t.Errorf("getDockerConfigPath() = %s, want %s", path, expected)
	}
}

// ============================================================================
// OCI Priorities File Tests (with temp directory)
// ============================================================================

func TestLoadSaveOCIPriorities(t *testing.T) {
	// Create a temporary home directory
	tmpDir, err := os.MkdirTemp("", "oci-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override HOME for this test
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Save priorities
	priorities := &OCIPriorities{
		Priorities: map[string]int{
			"https://ghcr.io":          10,
			"https://myacr.azurecr.io": 5,
		},
	}

	err = saveOCIPriorities(priorities)
	if err != nil {
		t.Fatalf("saveOCIPriorities() error = %v", err)
	}

	// Load priorities back
	loaded, err := loadOCIPriorities()
	if err != nil {
		t.Fatalf("loadOCIPriorities() error = %v", err)
	}

	if loaded.Priorities["https://ghcr.io"] != 10 {
		t.Errorf("Loaded priority for ghcr.io = %d, want 10", loaded.Priorities["https://ghcr.io"])
	}
	if loaded.Priorities["https://myacr.azurecr.io"] != 5 {
		t.Errorf("Loaded priority for ACR = %d, want 5", loaded.Priorities["https://myacr.azurecr.io"])
	}
}

func TestLoadOCIPriorities_NotExists(t *testing.T) {
	// Create a temporary home directory with no config
	tmpDir, err := os.MkdirTemp("", "oci-test-empty-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override HOME for this test
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Load priorities from non-existent file
	priorities, err := loadOCIPriorities()
	if err != nil {
		t.Fatalf("loadOCIPriorities() should not error for missing file: %v", err)
	}

	if priorities == nil {
		t.Fatal("loadOCIPriorities() returned nil")
	}
	if priorities.Priorities == nil {
		t.Error("Priorities map should be initialized")
	}
	if len(priorities.Priorities) != 0 {
		t.Errorf("Priorities should be empty for new config, got %d entries", len(priorities.Priorities))
	}
}
