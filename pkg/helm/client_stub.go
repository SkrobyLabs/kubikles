//go:build !helm

package helm

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

// Client shells out to the helm CLI when the Helm SDK is not compiled in.
type Client struct {
	helmPath string
}

// NewClient creates a CLI-backed Helm client. If the helm binary is not found
// in PATH, the client is created but IsAvailable() returns false.
func NewClient() *Client {
	c := &Client{}
	c.helmPath, _ = exec.LookPath("helm")
	return c
}

// IsAvailable returns true when a helm binary was found in PATH.
func (c *Client) IsAvailable() bool {
	return c.helmPath != ""
}

// =============================================================================
// Internal helpers
// =============================================================================

// runHelm executes the helm CLI with the given arguments and returns stdout.
func (c *Client) runHelm(args ...string) ([]byte, error) {
	if c.helmPath == "" {
		return nil, ErrHelmNotAvailable
	}
	cmd := exec.Command(c.helmPath, args...) //nolint:gosec
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("helm %s: %s", args[0], strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("helm %s: %w", args[0], err)
	}
	return out, nil
}

// contextArgs returns --kube-context CTX if contextName is non-empty.
func contextArgs(contextName string) []string {
	if contextName == "" {
		return nil
	}
	return []string{"--kube-context", contextName}
}

// nsArgs returns -n NS if namespace is non-empty.
func nsArgs(namespace string) []string {
	if namespace == "" {
		return nil
	}
	return []string{"-n", namespace}
}

// writeValuesToTempFile marshals values to a temporary YAML file, returning
// its path. Caller must os.Remove the file when done.
func writeValuesToTempFile(values map[string]interface{}) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	data, err := yaml.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("failed to marshal values: %w", err)
	}
	f, err := os.CreateTemp("", "helm-values-*.yaml")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// buildChartRef builds the chart reference string for upgrade/template CLI calls.
func buildChartRef(opts UpgradeOptions) string {
	if opts.IsOCI {
		registry := strings.TrimPrefix(opts.RepoURL, "https://")
		registry = strings.TrimPrefix(registry, "http://")
		return fmt.Sprintf("oci://%s/%s", registry, opts.OCIRepository)
	}
	return fmt.Sprintf("%s/%s", opts.RepoName, opts.ChartName)
}

// =============================================================================
// CLI JSON intermediate types
// =============================================================================

// cliRelease is the JSON output from `helm list -o json`.
type cliRelease struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Revision   string `json:"revision"`
	Updated    string `json:"updated"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"app_version"`
}

func (r *cliRelease) toRelease() Release {
	name, version := splitChartNameVersion(r.Chart)
	rev, _ := strconv.Atoi(r.Revision)
	return Release{
		Name:         r.Name,
		Namespace:    r.Namespace,
		Revision:     rev,
		Status:       r.Status,
		Chart:        name,
		ChartVersion: version,
		AppVersion:   r.AppVersion,
		Updated:      parseHelmTime(r.Updated),
	}
}

// cliHistoryEntry is the JSON output from `helm history -o json`.
type cliHistoryEntry struct {
	Revision    int    `json:"revision"`
	Updated     string `json:"updated"`
	Status      string `json:"status"`
	Chart       string `json:"chart"`
	AppVersion  string `json:"app_version"`
	Description string `json:"description"`
}

// cliRepoEntry is the JSON output from `helm repo list -o json`.
type cliRepoEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// cliSearchEntry is the JSON output from `helm search repo -o json`.
type cliSearchEntry struct {
	Name         string `json:"name"`
	ChartVersion string `json:"chart_version"`
	AppVersion   string `json:"app_version"`
	Description  string `json:"description"`
}

// parseHelmTime parses the non-standard time format emitted by helm CLI.
// Example: "2024-01-15 10:30:00.123456789 +0000 UTC"
func parseHelmTime(s string) time.Time {
	s = strings.TrimSpace(s)
	layouts := []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 +0000 UTC",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// splitChartNameVersion splits "nginx-1.2.3" into ("nginx", "1.2.3").
// It splits on the last hyphen that is followed by a digit.
func splitChartNameVersion(chart string) (string, string) {
	for i := len(chart) - 1; i >= 0; i-- {
		if chart[i] == '-' && i+1 < len(chart) && chart[i+1] >= '0' && chart[i+1] <= '9' {
			return chart[:i], chart[i+1:]
		}
	}
	return chart, ""
}

// =============================================================================
// Tier 1 — Core release operations
// =============================================================================

// ListReleases returns all releases, optionally filtered by namespaces.
func (c *Client) ListReleases(contextName string, namespaces []string) ([]Release, error) {
	if len(namespaces) == 0 {
		// List all namespaces
		args := []string{"list", "-A", "-o", "json", "--all"}
		args = append(args, contextArgs(contextName)...)
		out, err := c.runHelm(args...)
		if err != nil {
			return nil, err
		}
		return parseCLIReleases(out)
	}

	var allReleases []Release
	for _, ns := range namespaces {
		args := []string{"list", "-o", "json", "--all"}
		args = append(args, nsArgs(ns)...)
		args = append(args, contextArgs(contextName)...)
		out, err := c.runHelm(args...)
		if err != nil {
			return nil, fmt.Errorf("failed to list releases in namespace %s: %w", ns, err)
		}
		releases, err := parseCLIReleases(out)
		if err != nil {
			return nil, err
		}
		allReleases = append(allReleases, releases...)
	}

	sort.Slice(allReleases, func(i, j int) bool {
		return allReleases[i].Updated.After(allReleases[j].Updated)
	})
	return allReleases, nil
}

func parseCLIReleases(data []byte) ([]Release, error) {
	var cli []cliRelease
	if err := json.Unmarshal(data, &cli); err != nil {
		return nil, fmt.Errorf("failed to parse helm list output: %w", err)
	}
	releases := make([]Release, 0, len(cli))
	for _, r := range cli {
		releases = append(releases, r.toRelease())
	}
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Updated.After(releases[j].Updated)
	})
	return releases, nil
}

// GetRelease returns detailed information about a specific release.
func (c *Client) GetRelease(contextName, namespace, name string) (*ReleaseDetail, error) {
	ctx := contextArgs(contextName)
	ns := nsArgs(namespace)

	// Get basic release info
	args := []string{"list", "-f", "^" + name + "$", "-o", "json"}
	args = append(args, ns...)
	args = append(args, ctx...)
	listOut, err := c.runHelm(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get release %s: %w", name, err)
	}

	var cliReleases []cliRelease
	if err := json.Unmarshal(listOut, &cliReleases); err != nil || len(cliReleases) == 0 {
		return nil, fmt.Errorf("release %s not found", name)
	}
	rel := cliReleases[0].toRelease()

	// Get user-supplied values
	values, _ := c.GetReleaseValues(contextName, namespace, name)

	// Get all computed values
	computedValues, _ := c.GetReleaseAllValues(contextName, namespace, name)

	// Get manifest
	manifestArgs := []string{"get", "manifest", name}
	manifestArgs = append(manifestArgs, ns...)
	manifestArgs = append(manifestArgs, ctx...)
	manifest, _ := c.runHelm(manifestArgs...)

	// Get notes
	notesArgs := []string{"get", "notes", name}
	notesArgs = append(notesArgs, ns...)
	notesArgs = append(notesArgs, ctx...)
	notes, _ := c.runHelm(notesArgs...)

	return &ReleaseDetail{
		Release:        rel,
		Values:         values,
		ComputedValues: computedValues,
		Notes:          string(notes),
		Manifest:       string(manifest),
	}, nil
}

// GetReleaseValues returns user-supplied values for a release.
func (c *Client) GetReleaseValues(contextName, namespace, name string) (map[string]interface{}, error) {
	args := []string{"get", "values", name, "-o", "json"}
	args = append(args, nsArgs(namespace)...)
	args = append(args, contextArgs(contextName)...)
	out, err := c.runHelm(args...)
	if err != nil {
		return nil, err
	}

	var values map[string]interface{}
	if err := json.Unmarshal(out, &values); err != nil {
		return nil, fmt.Errorf("failed to parse release values: %w", err)
	}
	return values, nil
}

// GetReleaseAllValues returns all values for a release (computed + user-supplied).
func (c *Client) GetReleaseAllValues(contextName, namespace, name string) (map[string]interface{}, error) {
	args := []string{"get", "values", name, "-o", "json", "-a"}
	args = append(args, nsArgs(namespace)...)
	args = append(args, contextArgs(contextName)...)
	out, err := c.runHelm(args...)
	if err != nil {
		return nil, err
	}

	var values map[string]interface{}
	if err := json.Unmarshal(out, &values); err != nil {
		return nil, fmt.Errorf("failed to parse release values: %w", err)
	}
	return values, nil
}

// GetReleaseHistory returns the revision history for a release.
func (c *Client) GetReleaseHistory(contextName, namespace, name string) ([]ReleaseHistory, error) {
	args := []string{"history", name, "-o", "json", "--max", "256"}
	args = append(args, nsArgs(namespace)...)
	args = append(args, contextArgs(contextName)...)
	out, err := c.runHelm(args...)
	if err != nil {
		return nil, err
	}

	var entries []cliHistoryEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse release history: %w", err)
	}

	history := make([]ReleaseHistory, 0, len(entries))
	for _, e := range entries {
		chartName, _ := splitChartNameVersion(e.Chart)
		history = append(history, ReleaseHistory{
			Revision:    e.Revision,
			Status:      e.Status,
			Chart:       chartName,
			AppVersion:  e.AppVersion,
			Updated:     parseHelmTime(e.Updated),
			Description: e.Description,
		})
	}

	sort.Slice(history, func(i, j int) bool {
		return history[i].Revision > history[j].Revision
	})
	return history, nil
}

// Uninstall removes a release.
func (c *Client) Uninstall(contextName, namespace, name string) error {
	args := []string{"uninstall", name}
	args = append(args, nsArgs(namespace)...)
	args = append(args, contextArgs(contextName)...)
	_, err := c.runHelm(args...)
	return err
}

// Rollback rolls back a release to a specific revision.
func (c *Client) Rollback(contextName, namespace, name string, revision int) error {
	args := []string{"rollback", name, strconv.Itoa(revision)}
	args = append(args, nsArgs(namespace)...)
	args = append(args, contextArgs(contextName)...)
	_, err := c.runHelm(args...)
	return err
}

// GetReleaseResources returns the Kubernetes resources managed by a Helm release.
func (c *Client) GetReleaseResources(contextName, namespace, name string) ([]ResourceReference, error) {
	args := []string{"get", "manifest", name}
	args = append(args, nsArgs(namespace)...)
	args = append(args, contextArgs(contextName)...)
	out, err := c.runHelm(args...)
	if err != nil {
		return nil, err
	}
	return parseManifestResources(string(out), namespace)
}

// =============================================================================
// Tier 2 — Repository & chart operations
// =============================================================================

// ListRepositories returns all configured Helm repositories with priorities.
func (c *Client) ListRepositories() ([]Repository, error) {
	out, err := c.runHelm("repo", "list", "-o", "json")
	if err != nil {
		// helm repo list fails if no repos configured — treat as empty
		if strings.Contains(err.Error(), "no repositories") {
			return []Repository{}, nil
		}
		return nil, err
	}

	var entries []cliRepoEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse repo list: %w", err)
	}

	priorities, err := loadRepoPriorities()
	if err != nil {
		return nil, fmt.Errorf("failed to load priorities: %w", err)
	}

	repos := make([]Repository, 0, len(entries))
	for _, e := range entries {
		priority := 100
		if p, ok := priorities.Priorities[e.Name]; ok {
			priority = p
		}
		repos = append(repos, Repository{
			Name:     e.Name,
			URL:      e.URL,
			Priority: priority,
		})
	}

	sort.Slice(repos, func(i, j int) bool {
		if repos[i].Priority != repos[j].Priority {
			return repos[i].Priority < repos[j].Priority
		}
		return repos[i].Name < repos[j].Name
	})
	return repos, nil
}

// AddRepository adds a new Helm repository.
func (c *Client) AddRepository(name, url string, priority int) error {
	if name == "" || url == "" {
		return fmt.Errorf("repository name and URL are required")
	}
	_, err := c.runHelm("repo", "add", name, url)
	if err != nil {
		return err
	}

	priorities, err := loadRepoPriorities()
	if err != nil {
		return fmt.Errorf("failed to load priorities: %w", err)
	}
	priorities.Priorities[name] = priority
	if err := saveRepoPriorities(priorities); err != nil {
		return fmt.Errorf("failed to save priority: %w", err)
	}
	return nil
}

// RemoveRepository removes a Helm repository.
func (c *Client) RemoveRepository(name string) error {
	_, err := c.runHelm("repo", "remove", name)
	if err != nil {
		return err
	}

	// Remove from priorities (best-effort)
	priorities, err := loadRepoPriorities()
	if err == nil {
		delete(priorities.Priorities, name)
		_ = saveRepoPriorities(priorities)
	}
	return nil
}

// UpdateRepository updates the index for a repository.
func (c *Client) UpdateRepository(name string) error {
	_, err := c.runHelm("repo", "update", name)
	return err
}

// UpdateAllRepositories updates the index for all repositories.
func (c *Client) UpdateAllRepositories() error {
	_, err := c.runHelm("repo", "update")
	return err
}

// SetRepositoryPriority sets the priority for a repository (pure Go, no CLI).
func (c *Client) SetRepositoryPriority(name string, priority int) error {
	priorities, err := loadRepoPriorities()
	if err != nil {
		return fmt.Errorf("failed to load priorities: %w", err)
	}
	priorities.Priorities[name] = priority
	if err := saveRepoPriorities(priorities); err != nil {
		return fmt.Errorf("failed to save priority: %w", err)
	}
	return nil
}

// SearchChart searches for a chart across all repositories.
func (c *Client) SearchChart(chartName string) ([]ChartSource, error) {
	out, err := c.runHelm("search", "repo", chartName, "-o", "json")
	if err != nil {
		// No results is not an error
		if strings.Contains(err.Error(), "no results found") {
			return []ChartSource{}, nil
		}
		return nil, err
	}

	var entries []cliSearchEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	priorities, _ := loadRepoPriorities()
	if priorities == nil {
		priorities = &RepoPriorities{Priorities: make(map[string]int)}
	}

	// Group entries by repo name
	grouped := make(map[string]*ChartSource)
	for _, e := range entries {
		repoName, chart, _ := strings.Cut(e.Name, "/")
		if chart == "" {
			continue
		}

		key := repoName
		cs, ok := grouped[key]
		if !ok {
			priority := 100
			if p, exists := priorities.Priorities[repoName]; exists {
				priority = p
			}
			cs = &ChartSource{
				RepoName:  repoName,
				Priority:  priority,
				ChartName: chart,
			}
			grouped[key] = cs
		}
		cs.Versions = append(cs.Versions, ChartVersion{
			Version:     e.ChartVersion,
			AppVersion:  e.AppVersion,
			Description: e.Description,
		})
	}

	sources := make([]ChartSource, 0, len(grouped))
	for _, cs := range grouped {
		sources = append(sources, *cs)
	}

	// Also search OCI registries (via ACR CLI)
	ociSources := c.searchOCICharts(chartName)
	sources = append(sources, ociSources...)

	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})
	return sources, nil
}

// searchOCICharts searches for a chart in authenticated OCI registries.
func (c *Client) searchOCICharts(chartName string) []ChartSource {
	registries, err := c.ListOCIRegistries()
	if err != nil {
		return nil
	}

	var sources []ChartSource
	for _, reg := range registries {
		if !reg.Authenticated || !reg.IsACR {
			continue
		}

		acrName := extractACRNameFromURL(reg.URL)
		if acrName == "" {
			continue
		}

		// List repositories in ACR
		cmd := exec.Command("az", "acr", "repository", "list", "-n", acrName, "-o", "json") //nolint:gosec
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		var repos []string
		if json.Unmarshal(output, &repos) != nil {
			continue
		}

		chartLower := strings.ToLower(chartName)
		for _, repo := range repos {
			repoLower := strings.ToLower(repo)
			if repoLower != chartLower && !strings.HasSuffix(repoLower, "/"+chartLower) {
				continue
			}

			// Get tags
			cmd := exec.Command("az", "acr", "repository", "show-tags", "-n", acrName, //nolint:gosec
				"--repository", repo, "-o", "json", "--orderby", "time_desc")
			tagsOut, err := cmd.Output()
			if err != nil {
				continue
			}

			var tags []string
			if json.Unmarshal(tagsOut, &tags) != nil {
				continue
			}

			versions := make([]ChartVersion, 0, len(tags))
			for _, tag := range tags {
				versions = append(versions, ChartVersion{Version: tag})
			}

			registryName := strings.TrimPrefix(reg.URL, "https://")
			registryName = strings.TrimPrefix(registryName, "http://")

			sources = append(sources, ChartSource{
				RepoName:      fmt.Sprintf("oci://%s", registryName),
				RepoURL:       reg.URL,
				Priority:      reg.Priority,
				ChartName:     chartName,
				Versions:      versions,
				IsOCI:         true,
				OCIRepository: repo,
			})
		}
	}
	return sources
}

// GetChartVersions returns available versions for a chart from a specific repo.
func (c *Client) GetChartVersions(repoName, chartName string) ([]ChartVersion, error) {
	query := fmt.Sprintf("%s/%s", repoName, chartName)
	out, err := c.runHelm("search", "repo", query, "--versions", "-o", "json")
	if err != nil {
		return nil, err
	}

	var entries []cliSearchEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse chart versions: %w", err)
	}

	versions := make([]ChartVersion, 0, len(entries))
	for _, e := range entries {
		versions = append(versions, ChartVersion{
			Version:     e.ChartVersion,
			AppVersion:  e.AppVersion,
			Description: e.Description,
		})
	}
	return versions, nil
}

// UpgradeRelease upgrades or reinstalls a release via CLI.
func (c *Client) UpgradeRelease(contextName, namespace, name string, opts UpgradeOptions) error {
	chartRef := buildChartRef(opts)

	args := []string{"upgrade", name, chartRef}
	args = append(args, nsArgs(namespace)...)
	args = append(args, contextArgs(contextName)...)

	if opts.Version != "" {
		args = append(args, "--version", opts.Version)
	}
	if opts.ReuseValues {
		args = append(args, "--reuse-values")
	}
	if opts.ResetValues {
		args = append(args, "--reset-values")
	}
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Wait {
		args = append(args, "--wait")
	}
	if opts.Timeout > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%ds", opts.Timeout))
	}

	// Write values to temp file
	valuesFile, err := writeValuesToTempFile(opts.Values)
	if err != nil {
		return fmt.Errorf("failed to write values: %w", err)
	}
	if valuesFile != "" {
		defer os.Remove(valuesFile)
		args = append(args, "--values", valuesFile)
	}

	_, err = c.runHelm(args...)
	return err
}

// ForceReleaseStatus is not supported without the Helm SDK.
func (c *Client) ForceReleaseStatus(_, _, _, _ string) error {
	return ErrOperationNotSupported
}

// =============================================================================
// Tier 3 — OCI operations (mostly pure Go, CLI-backed)
// =============================================================================

// ListOCIRegistries returns a list of OCI registries from Docker config (pure Go).
func (c *Client) ListOCIRegistries() ([]OCIRegistry, error) {
	return listOCIRegistriesFromConfig()
}

// SetOCIRegistryPriority sets the priority for an OCI registry (pure Go).
func (c *Client) SetOCIRegistryPriority(registryURL string, priority int) error {
	priorities, err := loadOCIPriorities()
	if err != nil {
		return fmt.Errorf("failed to load priorities: %w", err)
	}
	priorities.Priorities[registryURL] = priority
	if err := saveOCIPriorities(priorities); err != nil {
		return fmt.Errorf("failed to save priority: %w", err)
	}
	return nil
}

// LoginOCIRegistry authenticates to an OCI registry.
func (c *Client) LoginOCIRegistry(registry, username, password string) error {
	if c.helmPath == "" {
		return ErrHelmNotAvailable
	}
	cmd := exec.Command(c.helmPath, "registry", "login", registry, "--username", username, "--password-stdin") //nolint:gosec
	cmd.Stdin = strings.NewReader(password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to login to registry %s: %s", registry, strings.TrimSpace(string(output)))
	}
	return nil
}

// LoginACRWithAzureCLI logs into an ACR using Azure CLI credentials.
func (c *Client) LoginACRWithAzureCLI(registry string) error {
	registryName := registry
	if strings.Contains(registry, ".azurecr.io") {
		registryName = strings.TrimSuffix(registry, ".azurecr.io")
		registryName = strings.TrimPrefix(registryName, "https://")
		registryName = strings.TrimPrefix(registryName, "http://")
	}

	// First, try to get admin credentials
	cmd := exec.Command("az", "acr", "credential", "show", "-n", registryName, //nolint:gosec
		"--query", "{username:username, password:passwords[0].value}", "-o", "json")
	output, err := cmd.Output()
	if err == nil {
		var creds struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if json.Unmarshal(output, &creds) == nil && creds.Username != "" && creds.Password != "" {
			return c.LoginOCIRegistry(registry, creds.Username, creds.Password)
		}
	}

	// Fallback: Use az acr login
	cmd = exec.Command("az", "acr", "login", "-n", registryName) //nolint:gosec
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to login to ACR %s: %s", registryName, strings.TrimSpace(string(output)))
	}
	return nil
}

// LogoutOCIRegistry logs out from an OCI registry.
func (c *Client) LogoutOCIRegistry(registry string) error {
	if c.helmPath == "" {
		return ErrHelmNotAvailable
	}
	cmd := exec.Command(c.helmPath, "registry", "logout", registry) //nolint:gosec
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to logout from registry %s: %s", registry, strings.TrimSpace(string(output)))
	}
	return nil
}

// RemoveOCIRegistry removes an OCI registry (logout, remove from Docker config, remove priority).
func (c *Client) RemoveOCIRegistry(registry string) error {
	// Try logout via helm (might fail if not authenticated)
	_ = c.LogoutOCIRegistry(registry)

	// Also directly remove from Docker config
	configPath := getDockerConfigPath()
	data, err := os.ReadFile(configPath)
	if err == nil {
		var config DockerConfig
		if json.Unmarshal(data, &config) == nil {
			variants := []string{
				registry,
				strings.TrimPrefix(registry, "https://"),
				strings.TrimPrefix(registry, "http://"),
			}
			modified := false
			for _, variant := range variants {
				if _, ok := config.Auths[variant]; ok {
					delete(config.Auths, variant)
					modified = true
				}
			}
			if modified {
				if newData, err := json.MarshalIndent(config, "", "  "); err == nil {
					_ = os.WriteFile(configPath, newData, 0600)
				}
			}
		}
	}

	// Remove from priorities
	priorities, err := loadOCIPriorities()
	if err == nil {
		delete(priorities.Priorities, registry)
		delete(priorities.Priorities, strings.TrimPrefix(registry, "https://"))
		delete(priorities.Priorities, strings.TrimPrefix(registry, "http://"))
		_ = saveOCIPriorities(priorities)
	}
	return nil
}

// ListChartSources returns all available chart sources (HTTP repos + OCI registries).
func (c *Client) ListChartSources() ([]ChartSourceInfo, error) {
	var sources []ChartSourceInfo

	// Load HTTP repositories via CLI
	repos, err := c.ListRepositories()
	if err == nil {
		for _, r := range repos {
			sources = append(sources, ChartSourceInfo{
				Name:     r.Name,
				URL:      r.URL,
				IsOCI:    false,
				IsACR:    false,
				Priority: r.Priority,
			})
		}
	}

	// Load OCI registries from Docker config
	ociRegistries, err := listOCIRegistriesFromConfig()
	if err == nil {
		for _, reg := range ociRegistries {
			if !reg.Authenticated {
				continue
			}
			registryName := strings.TrimPrefix(reg.URL, "https://")
			registryName = strings.TrimPrefix(registryName, "http://")
			sources = append(sources, ChartSourceInfo{
				Name:     fmt.Sprintf("oci://%s", registryName),
				URL:      reg.URL,
				IsOCI:    true,
				IsACR:    reg.IsACR,
				Priority: reg.Priority,
			})
		}
	}

	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})
	return sources, nil
}

// SearchChartInSource searches for a chart in a specific source.
func (c *Client) SearchChartInSource(sourceName, chartName string) (*ChartSearchResult, error) {
	start := time.Now()

	if strings.HasPrefix(sourceName, "oci://") {
		return c.searchChartInOCISource(sourceName, chartName, start)
	}
	return c.searchChartInHTTPRepo(sourceName, chartName, start)
}

func (c *Client) searchChartInHTTPRepo(repoName, chartName string, start time.Time) (*ChartSearchResult, error) {
	result := &ChartSearchResult{Found: false}

	query := fmt.Sprintf("%s/%s", repoName, chartName)
	out, err := c.runHelm("search", "repo", query, "--versions", "-o", "json")
	if err != nil {
		result.Log = fmt.Sprintf("[%s] Search failed: %v", repoName, err)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	var entries []cliSearchEntry
	if err := json.Unmarshal(out, &entries); err != nil || len(entries) == 0 {
		result.Log = fmt.Sprintf("[%s] Chart '%s' not found", repoName, chartName)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	priorities, _ := loadRepoPriorities()
	priority := 100
	if priorities != nil {
		if p, ok := priorities.Priorities[repoName]; ok {
			priority = p
		}
	}

	versions := make([]ChartVersion, 0, len(entries))
	for _, e := range entries {
		versions = append(versions, ChartVersion{
			Version:     e.ChartVersion,
			AppVersion:  e.AppVersion,
			Description: e.Description,
		})
	}

	// Determine the actual chart name from the first entry
	foundName := chartName
	if len(entries) > 0 {
		if _, chart, found := strings.Cut(entries[0].Name, "/"); found {
			foundName = chart
		}
	}

	result.Found = true
	result.Source = &ChartSource{
		RepoName:  repoName,
		Priority:  priority,
		ChartName: foundName,
		Versions:  versions,
		IsOCI:     false,
	}
	result.Log = fmt.Sprintf("[%s] Found chart '%s' with %d versions", repoName, foundName, len(versions))
	result.Duration = time.Since(start).Milliseconds()
	return result, nil
}

func (c *Client) searchChartInOCISource(sourceName, chartName string, start time.Time) (*ChartSearchResult, error) {
	result := &ChartSearchResult{Found: false}

	registry := strings.TrimPrefix(sourceName, "oci://")

	ociRegistries, err := listOCIRegistriesFromConfig()
	if err != nil {
		result.Log = fmt.Sprintf("[%s] Failed to list OCI registries: %v", sourceName, err)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	var targetRegistry *OCIRegistry
	for i, reg := range ociRegistries {
		regName := strings.TrimPrefix(reg.URL, "https://")
		regName = strings.TrimPrefix(regName, "http://")
		if regName == registry {
			targetRegistry = &ociRegistries[i]
			break
		}
	}

	if targetRegistry == nil {
		result.Log = fmt.Sprintf("[%s] Registry not found", sourceName)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	if !targetRegistry.Authenticated {
		result.Log = fmt.Sprintf("[%s] Registry not authenticated", sourceName)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	if !targetRegistry.IsACR {
		result.Log = fmt.Sprintf("[%s] Non-ACR OCI registries not yet supported for search", sourceName)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	acrName := extractACRNameFromURL(targetRegistry.URL)
	if acrName == "" {
		result.Log = fmt.Sprintf("[%s] Could not extract ACR name from URL", sourceName)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	cmd := exec.Command("az", "acr", "repository", "list", "-n", acrName, "-o", "json") //nolint:gosec
	output, err := cmd.Output()
	if err != nil {
		result.Log = fmt.Sprintf("[%s] Failed to list ACR repositories: %v", sourceName, err)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	var repos []string
	if json.Unmarshal(output, &repos) != nil {
		result.Log = fmt.Sprintf("[%s] Failed to parse ACR repositories", sourceName)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	var matchingRepo string
	chartLower := strings.ToLower(chartName)
	for _, repoPath := range repos {
		repoLower := strings.ToLower(repoPath)
		if repoLower == chartLower || strings.HasSuffix(repoLower, "/"+chartLower) {
			matchingRepo = repoPath
			break
		}
	}

	if matchingRepo == "" {
		result.Log = fmt.Sprintf("[%s] Chart '%s' not found in %d repositories", sourceName, chartName, len(repos))
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	cmd = exec.Command("az", "acr", "repository", "show-tags", "-n", acrName, //nolint:gosec
		"--repository", matchingRepo, "-o", "json", "--orderby", "time_desc")
	tagsOutput, err := cmd.Output()
	if err != nil {
		result.Log = fmt.Sprintf("[%s] Found chart '%s' but failed to get tags: %v", sourceName, matchingRepo, err)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	var tags []string
	if json.Unmarshal(tagsOutput, &tags) != nil {
		result.Log = fmt.Sprintf("[%s] Found chart '%s' but failed to parse tags", sourceName, matchingRepo)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	versions := make([]ChartVersion, 0, len(tags))
	for _, tag := range tags {
		versions = append(versions, ChartVersion{Version: tag})
	}

	result.Found = true
	result.Source = &ChartSource{
		RepoName:      sourceName,
		RepoURL:       targetRegistry.URL,
		Priority:      targetRegistry.Priority,
		ChartName:     chartName,
		Versions:      versions,
		IsOCI:         true,
		OCIRepository: matchingRepo,
	}
	result.Log = fmt.Sprintf("[%s] Found chart '%s' with %d versions", sourceName, matchingRepo, len(versions))
	result.Duration = time.Since(start).Milliseconds()
	return result, nil
}

// =============================================================================
// Template / Dry-Run / Validation
// =============================================================================

// TemplateRelease renders templates locally using helm CLI.
func (c *Client) TemplateRelease(releaseName, namespace string, opts UpgradeOptions) (*TemplateResult, error) {
	chartRef := buildChartRef(opts)

	args := []string{"template", releaseName, chartRef}
	args = append(args, nsArgs(namespace)...)

	if opts.Version != "" {
		args = append(args, "--version", opts.Version)
	}

	valuesFile, err := writeValuesToTempFile(opts.Values)
	if err != nil {
		return nil, fmt.Errorf("failed to write values: %w", err)
	}
	if valuesFile != "" {
		defer os.Remove(valuesFile)
		args = append(args, "--values", valuesFile)
	}

	out, err := c.runHelm(args...)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	return &TemplateResult{
		Manifests: string(out),
	}, nil
}

// DryRunUpgrade performs a dry-run upgrade and returns current vs proposed manifests.
func (c *Client) DryRunUpgrade(contextName, namespace, releaseName string, opts UpgradeOptions) (*DryRunResult, error) {
	ctx := contextArgs(contextName)
	ns := nsArgs(namespace)

	// Get current manifest
	currentArgs := []string{"get", "manifest", releaseName}
	currentArgs = append(currentArgs, ns...)
	currentArgs = append(currentArgs, ctx...)
	currentManifest, err := c.runHelm(currentArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get current manifest: %w", err)
	}

	// Dry-run upgrade
	chartRef := buildChartRef(opts)
	upgradeArgs := []string{"upgrade", releaseName, chartRef, "--dry-run"}
	upgradeArgs = append(upgradeArgs, ns...)
	upgradeArgs = append(upgradeArgs, ctx...)

	if opts.Version != "" {
		upgradeArgs = append(upgradeArgs, "--version", opts.Version)
	}
	if opts.ReuseValues {
		upgradeArgs = append(upgradeArgs, "--reuse-values")
	}
	if opts.ResetValues {
		upgradeArgs = append(upgradeArgs, "--reset-values")
	}

	valuesFile, err := writeValuesToTempFile(opts.Values)
	if err != nil {
		return nil, fmt.Errorf("failed to write values: %w", err)
	}
	if valuesFile != "" {
		defer os.Remove(valuesFile)
		upgradeArgs = append(upgradeArgs, "--values", valuesFile)
	}

	proposedOutput, err := c.runHelm(upgradeArgs...)
	if err != nil {
		return nil, fmt.Errorf("dry-run upgrade failed: %w", err)
	}

	return &DryRunResult{
		CurrentManifest:  string(currentManifest),
		ProposedManifest: string(proposedOutput),
	}, nil
}

// ValidateValues is not supported without the Helm SDK.
func (c *Client) ValidateValues(_ UpgradeOptions) ([]ValidationError, error) {
	return nil, ErrOperationNotSupported
}
