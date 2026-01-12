package helm

import (
	"testing"
)

// ============================================================================
// Manifest Parsing Tests
// ============================================================================

func TestParseManifestResources_SingleResource(t *testing.T) {
	manifest := `
apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: production
`
	resources, err := parseManifestResources(manifest, "default")
	if err != nil {
		t.Fatalf("parseManifestResources() error = %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("parseManifestResources() returned %d resources, want 1", len(resources))
	}
	if resources[0].Kind != "Service" {
		t.Errorf("Resource kind = %s, want Service", resources[0].Kind)
	}
	if resources[0].Name != "my-service" {
		t.Errorf("Resource name = %s, want my-service", resources[0].Name)
	}
	if resources[0].Namespace != "production" {
		t.Errorf("Resource namespace = %s, want production", resources[0].Namespace)
	}
}

func TestParseManifestResources_DefaultNamespace(t *testing.T) {
	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
`
	resources, err := parseManifestResources(manifest, "my-namespace")
	if err != nil {
		t.Fatalf("parseManifestResources() error = %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("parseManifestResources() returned %d resources, want 1", len(resources))
	}
	if resources[0].Namespace != "my-namespace" {
		t.Errorf("Resource namespace = %s, want my-namespace (default)", resources[0].Namespace)
	}
}

func TestParseManifestResources_MultipleResources(t *testing.T) {
	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-1
---
apiVersion: v1
kind: Secret
metadata:
  name: secret-1
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deployment-1
`
	resources, err := parseManifestResources(manifest, "default")
	if err != nil {
		t.Fatalf("parseManifestResources() error = %v", err)
	}
	if len(resources) != 3 {
		t.Errorf("parseManifestResources() returned %d resources, want 3", len(resources))
	}

	// Check resource types
	kinds := make(map[string]bool)
	for _, r := range resources {
		kinds[r.Kind] = true
	}
	if !kinds["ConfigMap"] {
		t.Error("Missing ConfigMap resource")
	}
	if !kinds["Secret"] {
		t.Error("Missing Secret resource")
	}
	if !kinds["Deployment"] {
		t.Error("Missing Deployment resource")
	}
}

func TestParseManifestResources_EmptyManifest(t *testing.T) {
	resources, err := parseManifestResources("", "default")
	if err != nil {
		t.Fatalf("parseManifestResources() error = %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("parseManifestResources() returned %d resources, want 0", len(resources))
	}
}

func TestParseManifestResources_OnlySeparators(t *testing.T) {
	manifest := `---
---
---`
	resources, err := parseManifestResources(manifest, "default")
	if err != nil {
		t.Fatalf("parseManifestResources() error = %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("parseManifestResources() returned %d resources, want 0", len(resources))
	}
}

func TestParseManifestResources_InvalidYAML(t *testing.T) {
	manifest := `
kind: Service
metadata:
  name: test
---
invalid yaml [[ that won't parse
---
kind: ConfigMap
metadata:
  name: valid-config
`
	// Should skip invalid YAML and continue
	resources, err := parseManifestResources(manifest, "default")
	if err != nil {
		t.Fatalf("parseManifestResources() error = %v", err)
	}
	// Should have 2 valid resources (Service and ConfigMap)
	if len(resources) != 2 {
		t.Errorf("parseManifestResources() returned %d resources, want 2", len(resources))
	}
}

func TestParseManifestResources_MissingKind(t *testing.T) {
	manifest := `
apiVersion: v1
metadata:
  name: no-kind
`
	resources, err := parseManifestResources(manifest, "default")
	if err != nil {
		t.Fatalf("parseManifestResources() error = %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("parseManifestResources() should skip resources without kind, got %d", len(resources))
	}
}

func TestParseManifestResources_MissingName(t *testing.T) {
	manifest := `
apiVersion: v1
kind: Service
metadata:
  namespace: test
`
	resources, err := parseManifestResources(manifest, "default")
	if err != nil {
		t.Fatalf("parseManifestResources() error = %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("parseManifestResources() should skip resources without name, got %d", len(resources))
	}
}

// ============================================================================
// YAML Document Splitting Tests
// ============================================================================

func TestSplitYAMLDocuments_Simple(t *testing.T) {
	data := []byte("doc1\n---\ndoc2")

	advance, token, err := splitYAMLDocuments(data, false)
	if err != nil {
		t.Fatalf("splitYAMLDocuments() error = %v", err)
	}
	if string(token) != "doc1" {
		t.Errorf("First token = %q, want 'doc1'", string(token))
	}
	if advance != 8 { // len("doc1\n---")
		t.Errorf("Advance = %d, want 8", advance)
	}
}

func TestSplitYAMLDocuments_AtEOF(t *testing.T) {
	data := []byte("last doc")

	advance, token, err := splitYAMLDocuments(data, true)
	if err != nil {
		t.Fatalf("splitYAMLDocuments() error = %v", err)
	}
	if string(token) != "last doc" {
		t.Errorf("Token = %q, want 'last doc'", string(token))
	}
	if advance != 8 {
		t.Errorf("Advance = %d, want 8", advance)
	}
}

func TestSplitYAMLDocuments_Empty(t *testing.T) {
	advance, token, err := splitYAMLDocuments([]byte{}, true)
	if err != nil {
		t.Fatalf("splitYAMLDocuments() error = %v", err)
	}
	if token != nil {
		t.Errorf("Token should be nil for empty data at EOF")
	}
	if advance != 0 {
		t.Errorf("Advance = %d, want 0", advance)
	}
}

func TestSplitYAMLDocuments_NeedMoreData(t *testing.T) {
	data := []byte("incomplete")

	advance, token, err := splitYAMLDocuments(data, false)
	if err != nil {
		t.Fatalf("splitYAMLDocuments() error = %v", err)
	}
	if token != nil {
		t.Errorf("Token should be nil when requesting more data")
	}
	if advance != 0 {
		t.Errorf("Advance = %d, want 0", advance)
	}
}

// ============================================================================
// Resource Reference Tests
// ============================================================================

func TestResourceReference_Fields(t *testing.T) {
	ref := ResourceReference{
		Kind:      "Deployment",
		Name:      "my-app",
		Namespace: "production",
	}

	if ref.Kind != "Deployment" {
		t.Errorf("Kind = %s, want Deployment", ref.Kind)
	}
	if ref.Name != "my-app" {
		t.Errorf("Name = %s, want my-app", ref.Name)
	}
	if ref.Namespace != "production" {
		t.Errorf("Namespace = %s, want production", ref.Namespace)
	}
}

// ============================================================================
// Release Model Tests
// ============================================================================

func TestRelease_Fields(t *testing.T) {
	rel := Release{
		Name:         "my-release",
		Namespace:    "default",
		Revision:     5,
		Status:       "deployed",
		Chart:        "nginx",
		ChartVersion: "1.2.3",
		AppVersion:   "1.21.0",
	}

	if rel.Name != "my-release" {
		t.Errorf("Name = %s, want my-release", rel.Name)
	}
	if rel.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", rel.Namespace)
	}
	if rel.Revision != 5 {
		t.Errorf("Revision = %d, want 5", rel.Revision)
	}
	if rel.Status != "deployed" {
		t.Errorf("Status = %s, want deployed", rel.Status)
	}
	if rel.ChartVersion != "1.2.3" {
		t.Errorf("ChartVersion = %s, want 1.2.3", rel.ChartVersion)
	}
}

// ============================================================================
// Upgrade Options Tests
// ============================================================================

func TestUpgradeOptions_Defaults(t *testing.T) {
	opts := UpgradeOptions{}

	// Check that zero values are as expected
	if opts.ReuseValues != false {
		t.Error("Default ReuseValues should be false")
	}
	if opts.Force != false {
		t.Error("Default Force should be false")
	}
	if opts.ResetValues != false {
		t.Error("Default ResetValues should be false")
	}
	if opts.Wait != false {
		t.Error("Default Wait should be false")
	}
	if opts.IsOCI != false {
		t.Error("Default IsOCI should be false")
	}
}

func TestUpgradeOptions_WithValues(t *testing.T) {
	opts := UpgradeOptions{
		RepoName:  "bitnami",
		RepoURL:   "https://charts.bitnami.com/bitnami",
		ChartName: "nginx",
		Version:   "1.0.0",
		Force:     true,
		Wait:      true,
		Timeout:   300,
		Values: map[string]interface{}{
			"replicaCount": 3,
			"service": map[string]interface{}{
				"type": "LoadBalancer",
			},
		},
	}

	if opts.RepoName != "bitnami" {
		t.Errorf("RepoName = %s, want bitnami", opts.RepoName)
	}
	if opts.ChartName != "nginx" {
		t.Errorf("ChartName = %s, want nginx", opts.ChartName)
	}
	if opts.Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", opts.Version)
	}
	if !opts.Force {
		t.Error("Force should be true")
	}
	if !opts.Wait {
		t.Error("Wait should be true")
	}
	if opts.Timeout != 300 {
		t.Errorf("Timeout = %d, want 300", opts.Timeout)
	}
	if opts.Values["replicaCount"] != 3 {
		t.Errorf("Values[replicaCount] = %v, want 3", opts.Values["replicaCount"])
	}
}

func TestUpgradeOptions_OCI(t *testing.T) {
	opts := UpgradeOptions{
		RepoURL:       "https://ghcr.io",
		ChartName:     "my-chart",
		Version:       "2.0.0",
		IsOCI:         true,
		OCIRepository: "myorg/charts/my-chart",
	}

	if !opts.IsOCI {
		t.Error("IsOCI should be true")
	}
	if opts.OCIRepository != "myorg/charts/my-chart" {
		t.Errorf("OCIRepository = %s, want myorg/charts/my-chart", opts.OCIRepository)
	}
}

// ============================================================================
// Release History Tests
// ============================================================================

func TestReleaseHistory_Fields(t *testing.T) {
	history := ReleaseHistory{
		Revision:    3,
		Status:      "deployed",
		Chart:       "nginx-1.2.3",
		AppVersion:  "1.21.0",
		Description: "Upgrade complete",
	}

	if history.Revision != 3 {
		t.Errorf("Revision = %d, want 3", history.Revision)
	}
	if history.Status != "deployed" {
		t.Errorf("Status = %s, want deployed", history.Status)
	}
	if history.Chart != "nginx-1.2.3" {
		t.Errorf("Chart = %s, want nginx-1.2.3", history.Chart)
	}
	if history.AppVersion != "1.21.0" {
		t.Errorf("AppVersion = %s, want 1.21.0", history.AppVersion)
	}
	if history.Description != "Upgrade complete" {
		t.Errorf("Description = %s, want 'Upgrade complete'", history.Description)
	}
}

// ============================================================================
// Release Detail Tests
// ============================================================================

func TestReleaseDetail_Fields(t *testing.T) {
	detail := ReleaseDetail{
		Release: Release{
			Name:         "my-release",
			Namespace:    "production",
			Revision:     2,
			Status:       "deployed",
			Chart:        "postgresql",
			ChartVersion: "12.0.0",
			AppVersion:   "15.0",
		},
		Notes: "Thank you for installing PostgreSQL!",
		Values: map[string]interface{}{
			"auth": map[string]interface{}{
				"username": "admin",
			},
		},
	}

	if detail.Name != "my-release" {
		t.Errorf("Name = %s, want my-release", detail.Name)
	}
	if detail.Notes != "Thank you for installing PostgreSQL!" {
		t.Errorf("Notes incorrect")
	}
	if detail.Values["auth"] == nil {
		t.Error("Values should contain auth")
	}
}

// ============================================================================
// Complex Manifest Tests
// ============================================================================

func TestParseManifestResources_RealWorldManifest(t *testing.T) {
	manifest := `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: nginx
  namespace: default
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-config
  namespace: default
data:
  nginx.conf: |
    server {
      listen 80;
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: default
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.21
---
apiVersion: v1
kind: Service
metadata:
  name: nginx
  namespace: default
spec:
  selector:
    app: nginx
  ports:
  - port: 80
    targetPort: 80
`
	resources, err := parseManifestResources(manifest, "default")
	if err != nil {
		t.Fatalf("parseManifestResources() error = %v", err)
	}
	if len(resources) != 4 {
		t.Errorf("parseManifestResources() returned %d resources, want 4", len(resources))
	}

	// Verify all expected resources
	expected := map[string]string{
		"ServiceAccount": "nginx",
		"ConfigMap":      "nginx-config",
		"Deployment":     "nginx",
		"Service":        "nginx",
	}

	for _, r := range resources {
		if expectedName, ok := expected[r.Kind]; ok {
			if r.Name != expectedName {
				t.Errorf("Resource %s name = %s, want %s", r.Kind, r.Name, expectedName)
			}
			delete(expected, r.Kind)
		}
	}

	if len(expected) > 0 {
		for kind := range expected {
			t.Errorf("Missing resource: %s", kind)
		}
	}
}
