package helm

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

// Note: chart package is imported for ChartVersion tests below

// ============================================================================
// ACR URL Detection Tests
// ============================================================================

func TestIsACRURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "standard ACR URL",
			url:      "https://myregistry.azurecr.io",
			expected: true,
		},
		{
			name:     "ACR URL with path",
			url:      "https://myregistry.azurecr.io/helm/charts",
			expected: true,
		},
		{
			name:     "ACR URL without https",
			url:      "http://myregistry.azurecr.io",
			expected: true,
		},
		{
			name:     "Docker Hub",
			url:      "https://registry-1.docker.io",
			expected: false,
		},
		{
			name:     "GitHub Container Registry",
			url:      "https://ghcr.io",
			expected: false,
		},
		{
			name:     "Empty URL",
			url:      "",
			expected: false,
		},
		{
			name:     "Standard helm repo",
			url:      "https://charts.helm.sh/stable",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isACRURL(tt.url)
			if result != tt.expected {
				t.Errorf("isACRURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// ACR Name Extraction Tests
// ============================================================================

func TestExtractACRName(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "standard ACR URL",
			url:      "https://myregistry.azurecr.io",
			expected: "myregistry",
		},
		{
			name:     "ACR URL with path",
			url:      "https://testacr.azurecr.io/helm/charts",
			expected: "testacr",
		},
		{
			name:     "ACR URL with http",
			url:      "http://myreg.azurecr.io",
			expected: "myreg",
		},
		{
			name:     "non-ACR URL",
			url:      "https://charts.helm.sh/stable",
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
			result := extractACRName(tt.url)
			if result != tt.expected {
				t.Errorf("extractACRName(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// JWT Expiration Tests
// ============================================================================

func TestIsJWTExpired(t *testing.T) {
	// Helper to create a valid JWT token with specified expiration
	createJWT := func(exp int64) string {
		header := base64.URLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
		claims := map[string]interface{}{"exp": exp}
		claimsJSON, _ := json.Marshal(claims)
		payload := base64.URLEncoding.EncodeToString(claimsJSON)
		// Signature doesn't matter for this test
		sig := base64.URLEncoding.EncodeToString([]byte("signature"))
		return header + "." + payload + "." + sig
	}

	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{
			name:     "empty token",
			token:    "",
			expected: true,
		},
		{
			name:     "not a JWT (static password)",
			token:    "my-static-password",
			expected: false,
		},
		{
			name:     "expired token",
			token:    createJWT(time.Now().Unix() - 3600), // 1 hour ago
			expected: true,
		},
		{
			name:     "valid token (expires in 1 hour)",
			token:    createJWT(time.Now().Unix() + 3600),
			expected: false,
		},
		{
			name:     "token expiring soon (within 5 min buffer)",
			token:    createJWT(time.Now().Unix() + 200), // 3.3 minutes
			expected: true,
		},
		{
			name:     "token expiring just outside buffer",
			token:    createJWT(time.Now().Unix() + 400), // 6.6 minutes
			expected: false,
		},
		{
			name:     "invalid JWT format (2 parts)",
			token:    "header.payload",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isJWTExpired(tt.token)
			if result != tt.expected {
				t.Errorf("isJWTExpired() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ============================================================================
// Repository Management Tests (with temp directories)
// ============================================================================

// setupTestClient creates a Client with a temp directory for repo config
func setupTestClient(t *testing.T) (*Client, string) {
	tmpDir, err := os.MkdirTemp("", "helm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	repoFile := filepath.Join(tmpDir, "repositories.yaml")
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	// Create empty repo file
	emptyRepo := &repo.File{
		APIVersion:   "v1",
		Generated:    time.Now(),
		Repositories: []*repo.Entry{},
	}
	data, _ := yaml.Marshal(emptyRepo)
	if err := os.WriteFile(repoFile, data, 0644); err != nil {
		t.Fatalf("failed to write repo file: %v", err)
	}

	settings := cli.New()
	settings.RepositoryConfig = repoFile
	settings.RepositoryCache = cacheDir

	return &Client{settings: settings}, tmpDir
}

func TestListRepositories_Empty(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	repos, err := client.ListRepositories()
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("ListRepositories() returned %d repos, want 0", len(repos))
	}
}

func TestListRepositories_WithRepos(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	// Add some repositories to the file
	repoFile := client.settings.RepositoryConfig
	f := &repo.File{
		APIVersion: "v1",
		Generated:  time.Now(),
		Repositories: []*repo.Entry{
			{Name: "bitnami", URL: "https://charts.bitnami.com/bitnami"},
			{Name: "stable", URL: "https://charts.helm.sh/stable"},
		},
	}
	data, _ := yaml.Marshal(f)
	if err := os.WriteFile(repoFile, data, 0644); err != nil {
		t.Fatalf("failed to write repo file: %v", err)
	}

	repos, err := client.ListRepositories()
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}
	if len(repos) != 2 {
		t.Errorf("ListRepositories() returned %d repos, want 2", len(repos))
	}

	// Check that repos are present
	names := make(map[string]bool)
	for _, r := range repos {
		names[r.Name] = true
	}
	if !names["bitnami"] {
		t.Error("ListRepositories() missing 'bitnami' repo")
	}
	if !names["stable"] {
		t.Error("ListRepositories() missing 'stable' repo")
	}
}

func TestRemoveRepository(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	// Add a repository first
	repoFile := client.settings.RepositoryConfig
	f := &repo.File{
		APIVersion: "v1",
		Generated:  time.Now(),
		Repositories: []*repo.Entry{
			{Name: "to-remove", URL: "https://example.com/charts"},
			{Name: "keep-this", URL: "https://example.com/other"},
		},
	}
	data, _ := yaml.Marshal(f)
	if err := os.WriteFile(repoFile, data, 0644); err != nil {
		t.Fatalf("failed to write repo file: %v", err)
	}

	// Remove the repo
	err := client.RemoveRepository("to-remove")
	if err != nil {
		t.Fatalf("RemoveRepository() error = %v", err)
	}

	// Verify it's gone
	repos, _ := client.ListRepositories()
	if len(repos) != 1 {
		t.Errorf("After RemoveRepository(), got %d repos, want 1", len(repos))
	}
	if repos[0].Name != "keep-this" {
		t.Errorf("Wrong repo remained: got %s, want keep-this", repos[0].Name)
	}
}

func TestRemoveRepository_NotFound(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	err := client.RemoveRepository("nonexistent")
	if err == nil {
		t.Error("RemoveRepository() should error for nonexistent repo")
	}
}

func TestSetRepositoryPriority(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	// Add a repository first
	repoFile := client.settings.RepositoryConfig
	f := &repo.File{
		APIVersion: "v1",
		Generated:  time.Now(),
		Repositories: []*repo.Entry{
			{Name: "test-repo", URL: "https://example.com/charts"},
		},
	}
	data, _ := yaml.Marshal(f)
	if err := os.WriteFile(repoFile, data, 0644); err != nil {
		t.Fatalf("failed to write repo file: %v", err)
	}

	// Set priority
	err := client.SetRepositoryPriority("test-repo", 100)
	if err != nil {
		t.Fatalf("SetRepositoryPriority() error = %v", err)
	}

	// Verify priority is set
	repos, _ := client.ListRepositories()
	if len(repos) != 1 {
		t.Fatalf("Expected 1 repo, got %d", len(repos))
	}
	if repos[0].Priority != 100 {
		t.Errorf("Priority = %d, want 100", repos[0].Priority)
	}
}

func TestSetRepositoryPriority_NotFound(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	err := client.SetRepositoryPriority("nonexistent", 50)
	if err == nil {
		t.Error("SetRepositoryPriority() should error for nonexistent repo")
	}
}

// Note: SearchChart tests are skipped because they make network calls to OCI registries
// which makes them slow and unreliable as unit tests.
// These would be better as integration tests.

// ============================================================================
// Chart Version Tests
// ============================================================================

func TestGetChartVersions_NotFound(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	_, err := client.GetChartVersions("nonexistent-repo", "nginx")
	if err == nil {
		t.Error("GetChartVersions() should error for nonexistent repo")
	}
}

func TestGetChartVersions_WithVersions(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	// Create a repository with an index file
	repoFile := client.settings.RepositoryConfig
	f := &repo.File{
		APIVersion: "v1",
		Generated:  time.Now(),
		Repositories: []*repo.Entry{
			{Name: "test-repo", URL: "https://example.com/charts"},
		},
	}
	data, _ := yaml.Marshal(f)
	if err := os.WriteFile(repoFile, data, 0644); err != nil {
		t.Fatalf("failed to write repo file: %v", err)
	}

	// Create an index file with multiple versions
	indexPath := filepath.Join(client.settings.RepositoryCache, "test-repo-index.yaml")
	index := &repo.IndexFile{
		APIVersion: "v1",
		Generated:  time.Now(),
		Entries: map[string]repo.ChartVersions{
			"nginx": {
				{Metadata: &chart.Metadata{Name: "nginx", Version: "3.0.0", AppVersion: "1.23.0"}},
				{Metadata: &chart.Metadata{Name: "nginx", Version: "2.0.0", AppVersion: "1.22.0"}},
				{Metadata: &chart.Metadata{Name: "nginx", Version: "1.0.0", AppVersion: "1.21.0"}},
			},
		},
	}
	indexData, _ := yaml.Marshal(index)
	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		t.Fatalf("failed to write index file: %v", err)
	}

	versions, err := client.GetChartVersions("test-repo", "nginx")
	if err != nil {
		t.Fatalf("GetChartVersions() error = %v", err)
	}
	if len(versions) != 3 {
		t.Errorf("GetChartVersions() returned %d versions, want 3", len(versions))
	}

	// Check versions are present
	versionMap := make(map[string]bool)
	for _, v := range versions {
		versionMap[v.Version] = true
	}
	for _, expected := range []string{"1.0.0", "2.0.0", "3.0.0"} {
		if !versionMap[expected] {
			t.Errorf("GetChartVersions() missing version %s", expected)
		}
	}
}

func TestGetChartVersions_ChartNotInRepo(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	// Create a repository with an index file
	repoFile := client.settings.RepositoryConfig
	f := &repo.File{
		APIVersion: "v1",
		Generated:  time.Now(),
		Repositories: []*repo.Entry{
			{Name: "test-repo", URL: "https://example.com/charts"},
		},
	}
	data, _ := yaml.Marshal(f)
	if err := os.WriteFile(repoFile, data, 0644); err != nil {
		t.Fatalf("failed to write repo file: %v", err)
	}

	// Create an empty index file
	indexPath := filepath.Join(client.settings.RepositoryCache, "test-repo-index.yaml")
	index := &repo.IndexFile{
		APIVersion: "v1",
		Generated:  time.Now(),
		Entries:    map[string]repo.ChartVersions{},
	}
	indexData, _ := yaml.Marshal(index)
	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		t.Fatalf("failed to write index file: %v", err)
	}

	_, err := client.GetChartVersions("test-repo", "nonexistent-chart")
	if err == nil {
		t.Error("GetChartVersions() should error for chart not in repo")
	}
}

// ============================================================================
// Client Initialization Tests
// ============================================================================

func TestNewClient(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.settings == nil {
		t.Error("NewClient() settings is nil")
	}
}

func TestGetSettings(t *testing.T) {
	client := NewClient()
	settings := client.getSettings()
	if settings == nil {
		t.Error("getSettings() returned nil")
	}
}

// ============================================================================
// Repository URL Types Tests
// ============================================================================

func TestRepository_URLTypes(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	// Add repositories with different URL formats
	repoFile := client.settings.RepositoryConfig
	f := &repo.File{
		APIVersion: "v1",
		Generated:  time.Now(),
		Repositories: []*repo.Entry{
			{Name: "http-repo", URL: "https://charts.example.com"},
			{Name: "oci-repo", URL: "oci://registry.example.com/charts"},
			{Name: "acr-repo", URL: "https://myacr.azurecr.io/helm/v1/repo"},
		},
	}
	data, _ := yaml.Marshal(f)
	if err := os.WriteFile(repoFile, data, 0644); err != nil {
		t.Fatalf("failed to write repo file: %v", err)
	}

	repos, err := client.ListRepositories()
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}

	// Verify all repos are listed
	if len(repos) != 3 {
		t.Errorf("ListRepositories() returned %d repos, want 3", len(repos))
	}

	// Find each repo and check URL is preserved
	repoURLs := make(map[string]string)
	for _, r := range repos {
		repoURLs[r.Name] = r.URL
	}

	if repoURLs["http-repo"] != "https://charts.example.com" {
		t.Errorf("http-repo URL = %s, want https://charts.example.com", repoURLs["http-repo"])
	}
	if repoURLs["oci-repo"] != "oci://registry.example.com/charts" {
		t.Errorf("oci-repo URL = %s, want oci://registry.example.com/charts", repoURLs["oci-repo"])
	}
	if repoURLs["acr-repo"] != "https://myacr.azurecr.io/helm/v1/repo" {
		t.Errorf("acr-repo URL = %s, want https://myacr.azurecr.io/helm/v1/repo", repoURLs["acr-repo"])
	}
}
