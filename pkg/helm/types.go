package helm

import (
	"errors"
	"time"
)

// ErrHelmNotAvailable is returned by stub methods when Helm support is not compiled in.
var ErrHelmNotAvailable = errors.New("Helm support not included in this build")

// ErrOperationNotSupported is returned when an operation has no CLI equivalent.
var ErrOperationNotSupported = errors.New("this operation is not supported without the Helm SDK")

// Release represents a Helm release with relevant information for the UI
type Release struct {
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	Revision     int       `json:"revision"`
	Status       string    `json:"status"`
	Chart        string    `json:"chart"`
	ChartVersion string    `json:"chartVersion"`
	AppVersion   string    `json:"appVersion"`
	Updated      time.Time `json:"updated"`
	Description  string    `json:"description"`
}

// ReleaseDetail contains full release information including values
type ReleaseDetail struct {
	Release
	Values         map[string]interface{} `json:"values"`
	ComputedValues map[string]interface{} `json:"computedValues"`
	Notes          string                 `json:"notes"`
	Manifest       string                 `json:"manifest"`
}

// ReleaseHistory represents a single revision in release history
type ReleaseHistory struct {
	Revision    int       `json:"revision"`
	Status      string    `json:"status"`
	Chart       string    `json:"chart"`
	AppVersion  string    `json:"appVersion"`
	Updated     time.Time `json:"updated"`
	Description string    `json:"description"`
}

// ResourceReference represents a reference to a Kubernetes resource
type ResourceReference struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// UpgradeOptions contains options for upgrading a release
type UpgradeOptions struct {
	RepoName      string                 `json:"repoName"`      // Repository name (or oci://registry for OCI)
	RepoURL       string                 `json:"repoUrl"`       // Repository URL
	ChartName     string                 `json:"chartName"`     // Chart name
	Version       string                 `json:"version"`       // Target version (empty = latest)
	Values        map[string]interface{} `json:"values"`        // Override values
	ReuseValues   bool                   `json:"reuseValues"`   // Reuse values from current release
	ResetValues   bool                   `json:"resetValues"`   // Reset to chart defaults
	Force         bool                   `json:"force"`         // Force resource updates
	Wait          bool                   `json:"wait"`          // Wait for resources ready
	Timeout       int                    `json:"timeout"`       // Timeout in seconds
	IsOCI         bool                   `json:"isOci"`         // True if this is an OCI registry source
	OCIRepository string                 `json:"ociRepository"` // For OCI: the repository path within registry
}

// TemplateResult contains the rendered manifests from a template operation
type TemplateResult struct {
	Manifests string `json:"manifests"` // Rendered YAML manifests
	Notes     string `json:"notes"`     // Release notes
}

// DryRunResult contains the diff between current and proposed state
type DryRunResult struct {
	CurrentManifest  string `json:"currentManifest"`  // Current deployed manifest
	ProposedManifest string `json:"proposedManifest"` // Proposed manifest from dry-run
	Notes            string `json:"notes"`            // Release notes from dry-run
}

// ValidationError represents a single values validation error with path info
type ValidationError struct {
	Path    string `json:"path"`    // JSON path to the invalid field (e.g. ".service.type")
	Message string `json:"message"` // Error message describing the validation failure
}

// Repository represents a Helm chart repository with priority
type Repository struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Priority int    `json:"priority"` // Lower number = higher priority (0 is highest)
}

// ChartVersion represents an available version of a chart
type ChartVersion struct {
	Version     string    `json:"version"`
	AppVersion  string    `json:"appVersion"`
	Description string    `json:"description"`
	Created     time.Time `json:"created"`
	Deprecated  bool      `json:"deprecated"`
}

// ChartSource represents a chart available from a repository or OCI registry
type ChartSource struct {
	RepoName      string         `json:"repoName"`
	RepoURL       string         `json:"repoUrl"`
	Priority      int            `json:"priority"`
	ChartName     string         `json:"chartName"`
	Versions      []ChartVersion `json:"versions"`
	IsOCI         bool           `json:"isOci"`         // True if this is an OCI registry source
	OCIRepository string         `json:"ociRepository"` // For OCI: the full repository path within registry
}

// ChartSourceInfo provides basic info about a chart source for listing
type ChartSourceInfo struct {
	Name     string `json:"name"`     // Display name (repo name or oci://registry)
	URL      string `json:"url"`      // URL of the source
	IsOCI    bool   `json:"isOci"`    // True if OCI registry
	IsACR    bool   `json:"isAcr"`    // True if Azure Container Registry
	Priority int    `json:"priority"` // Priority (lower = higher priority)
}

// ChartSearchResult contains the result of searching a single source
type ChartSearchResult struct {
	Found    bool         `json:"found"`    // Whether chart was found
	Source   *ChartSource `json:"source"`   // The source details if found
	Log      string       `json:"log"`      // Log message describing what happened
	Duration int64        `json:"duration"` // Search duration in milliseconds
}

// OCIRegistry represents an OCI registry with authentication status
type OCIRegistry struct {
	URL           string `json:"url"`
	Username      string `json:"username"`
	Authenticated bool   `json:"authenticated"`
	IsACR         bool   `json:"isAcr"`    // Azure Container Registry
	Priority      int    `json:"priority"` // Lower number = higher priority
}

// OCIChartVersion represents a version of a chart from an OCI registry
type OCIChartVersion struct {
	Version string `json:"version"`
}

// OCIChartSource represents a chart available from an OCI registry
type OCIChartSource struct {
	RegistryURL string            `json:"registryUrl"`
	Repository  string            `json:"repository"` // Path within registry (e.g., "helm/mychart")
	ChartName   string            `json:"chartName"`
	Priority    int               `json:"priority"`
	IsACR       bool              `json:"isAcr"`
	Versions    []OCIChartVersion `json:"versions"`
}

// RepoPriorities stores priority settings for repositories
type RepoPriorities struct {
	Priorities map[string]int `json:"priorities"` // repo name -> priority
}

// OCIPriorities stores priority settings for OCI registries
type OCIPriorities struct {
	Priorities map[string]int `json:"priorities"` // registry URL -> priority
}

// DockerConfig represents the Docker config.json structure
type DockerConfig struct {
	Auths       map[string]DockerAuth `json:"auths"`
	CredsStore  string                `json:"credsStore,omitempty"`
	CredHelpers map[string]string     `json:"credHelpers,omitempty"`
}

// DockerAuth represents a single auth entry
type DockerAuth struct {
	Auth          string `json:"auth"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
}

// ACRCredentials holds username and password for ACR authentication
type ACRCredentials struct {
	Username string
	Password string
}
