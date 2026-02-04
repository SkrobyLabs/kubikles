package helm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

// ACR token refresh functionality

// Pre-compiled regex for ACR URL parsing (avoid recompiling on every call)
var acrURLRegex = regexp.MustCompile(`https?://([^.]+)\.azurecr\.io`)

// isACRURL checks if a URL is an Azure Container Registry URL
func isACRURL(url string) bool {
	return strings.Contains(url, ".azurecr.io")
}

// extractACRName extracts the registry name from an ACR URL
func extractACRName(url string) string {
	matches := acrURLRegex.FindStringSubmatch(url)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// isJWTExpired checks if a JWT token is expired (with 5 minute buffer)
func isJWTExpired(token string) bool {
	if token == "" {
		return true
	}

	// JWT has 3 parts separated by dots
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		// Not a valid JWT - might be a static password, treat as not expired
		return false
	}

	// Decode the payload (second part) - JWT uses base64url encoding
	// Add padding if needed
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Try standard encoding as fallback
		decoded, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return false // Can't decode, assume not expired
		}
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return false // Can't parse, assume not expired
	}

	now := time.Now().Unix()
	return now > (claims.Exp - 300)
}

// ACRCredentials holds username and password for ACR authentication
type ACRCredentials struct {
	Username string
	Password string
}

// refreshACRCredentials attempts to get ACR admin credentials using Azure CLI
func refreshACRCredentials(registryName string) (*ACRCredentials, error) {
	// Try to get admin credentials (more reliable than refresh tokens)
	cmd := exec.Command("az", "acr", "credential", "show", "-n", registryName,
		"--query", "{username:username, password:passwords[0].value}", "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		// Admin might be disabled, try token approach as fallback
		return refreshACRToken(registryName)
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(output, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse ACR credentials: %w", err)
	}

	if creds.Username == "" || creds.Password == "" {
		return refreshACRToken(registryName)
	}

	return &ACRCredentials{Username: creds.Username, Password: creds.Password}, nil
}

// refreshACRToken attempts to get a fresh ACR token using Azure CLI (fallback)
func refreshACRToken(registryName string) (*ACRCredentials, error) {
	// Try to get token using az acr login --expose-token
	cmd := exec.Command("az", "acr", "login", "-n", registryName, "--expose-token", "--query", "accessToken", "-o", "tsv")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ACR credentials (is Azure CLI installed and logged in?): %w", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return nil, fmt.Errorf("empty token returned from Azure CLI")
	}

	// Token auth uses special username
	return &ACRCredentials{
		Username: "00000000-0000-0000-0000-000000000000",
		Password: token,
	}, nil
}

// RefreshACRTokenIfNeeded checks if an ACR repo needs credential refresh and does it
func (c *Client) RefreshACRTokenIfNeeded(repoName string) error {
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	var entry *repo.Entry
	for _, r := range f.Repositories {
		if r.Name == repoName {
			entry = r
			break
		}
	}

	if entry == nil {
		return fmt.Errorf("repository %q not found", repoName)
	}

	// Check if this is an ACR URL
	if !isACRURL(entry.URL) {
		return nil // Not an ACR repo, nothing to do
	}

	// Check if using JWT token that's expired, or if using admin creds (don't refresh those)
	isJWT := strings.Contains(entry.Password, ".") && len(strings.Split(entry.Password, ".")) == 3
	if isJWT && !isJWTExpired(entry.Password) {
		return nil // JWT token is still valid
	}

	// If it's admin credentials (not JWT), don't refresh
	if !isJWT && entry.Password != "" {
		return nil // Using admin credentials, no refresh needed
	}

	// Extract registry name and refresh
	registryName := extractACRName(entry.URL)
	if registryName == "" {
		return fmt.Errorf("could not extract registry name from URL: %s", entry.URL)
	}

	creds, err := refreshACRCredentials(registryName)
	if err != nil {
		return fmt.Errorf("failed to refresh ACR credentials for %s: %w", registryName, err)
	}

	// Update the entry with new credentials
	entry.Username = creds.Username
	entry.Password = creds.Password

	// Save the updated repo file
	if err := f.WriteFile(repoFile, 0644); err != nil {
		return fmt.Errorf("failed to save updated credentials: %w", err)
	}

	return nil
}

// ForceRefreshACRCredentials forces a credential refresh for an ACR repo, ignoring expiry check
func (c *Client) ForceRefreshACRCredentials(repoName string) error {
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	var entry *repo.Entry
	for _, r := range f.Repositories {
		if r.Name == repoName {
			entry = r
			break
		}
	}

	if entry == nil {
		return fmt.Errorf("repository %q not found", repoName)
	}

	if !isACRURL(entry.URL) {
		return fmt.Errorf("repository %q is not an ACR repository", repoName)
	}

	registryName := extractACRName(entry.URL)
	if registryName == "" {
		return fmt.Errorf("could not extract registry name from URL: %s", entry.URL)
	}

	creds, err := refreshACRCredentials(registryName)
	if err != nil {
		return fmt.Errorf("failed to refresh ACR credentials: %w", err)
	}

	entry.Username = creds.Username
	entry.Password = creds.Password

	if err := f.WriteFile(repoFile, 0644); err != nil {
		return fmt.Errorf("failed to save updated credentials: %w", err)
	}

	return nil
}

// RefreshAllACRTokens refreshes credentials for all ACR repos that need it
func (c *Client) RefreshAllACRTokens() error {
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	var errs []string
	modified := false

	for _, entry := range f.Repositories {
		if !isACRURL(entry.URL) {
			continue
		}

		// Check if using JWT token
		isJWT := strings.Contains(entry.Password, ".") && len(strings.Split(entry.Password, ".")) == 3

		// If using admin credentials (not JWT), skip refresh
		if !isJWT && entry.Password != "" {
			continue
		}

		// If JWT and not expired, skip
		if isJWT && !isJWTExpired(entry.Password) {
			continue
		}

		registryName := extractACRName(entry.URL)
		if registryName == "" {
			errs = append(errs, fmt.Sprintf("%s: could not extract registry name", entry.Name))
			continue
		}

		creds, err := refreshACRCredentials(registryName)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", entry.Name, err))
			continue
		}

		entry.Username = creds.Username
		entry.Password = creds.Password
		modified = true
	}

	if modified {
		if err := f.WriteFile(repoFile, 0644); err != nil {
			return fmt.Errorf("failed to save updated credentials: %w", err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to refresh some ACR credentials: %s", strings.Join(errs, "; "))
	}

	return nil
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

// RepoPriorities stores priority settings for repositories
type RepoPriorities struct {
	Priorities map[string]int `json:"priorities"` // repo name -> priority
}

// getRepoPrioritiesPath returns the path to the priorities config file
func getRepoPrioritiesPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE") // Windows
	}
	return filepath.Join(home, ".config", "kubikles", "helm-repo-priorities.json")
}

// loadRepoPriorities loads repository priorities from config
func loadRepoPriorities() (*RepoPriorities, error) {
	path := getRepoPrioritiesPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &RepoPriorities{Priorities: make(map[string]int)}, nil
		}
		return nil, err
	}

	var priorities RepoPriorities
	if err := json.Unmarshal(data, &priorities); err != nil {
		return nil, err
	}

	if priorities.Priorities == nil {
		priorities.Priorities = make(map[string]int)
	}

	return &priorities, nil
}

// saveRepoPriorities saves repository priorities to config
func saveRepoPriorities(priorities *RepoPriorities) error {
	path := getRepoPrioritiesPath()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(priorities, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// ListRepositories returns all configured Helm repositories with priorities
func (c *Client) ListRepositories() ([]Repository, error) {
	// Load Helm's repository file
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []Repository{}, nil
		}
		return nil, fmt.Errorf("failed to load repository file: %w", err)
	}

	// Load our priorities
	priorities, err := loadRepoPriorities()
	if err != nil {
		return nil, fmt.Errorf("failed to load priorities: %w", err)
	}

	repos := make([]Repository, 0, len(f.Repositories))
	for _, r := range f.Repositories {
		priority := 100 // Default priority
		if p, ok := priorities.Priorities[r.Name]; ok {
			priority = p
		}
		repos = append(repos, Repository{
			Name:     r.Name,
			URL:      r.URL,
			Priority: priority,
		})
	}

	// Sort by priority (lower first), then by name
	sort.Slice(repos, func(i, j int) bool {
		if repos[i].Priority != repos[j].Priority {
			return repos[i].Priority < repos[j].Priority
		}
		return repos[i].Name < repos[j].Name
	})

	return repos, nil
}

// AddRepository adds a new Helm repository
func (c *Client) AddRepository(name, url string, priority int) error {
	// Validate inputs
	if name == "" || url == "" {
		return fmt.Errorf("repository name and URL are required")
	}

	// Load existing repository file
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load repository file: %w", err)
	}
	if f == nil {
		f = repo.NewFile()
	}

	// Check if repo already exists
	if f.Has(name) {
		return fmt.Errorf("repository %q already exists", name)
	}

	// Create repository entry
	entry := &repo.Entry{
		Name: name,
		URL:  url,
	}

	// Download and verify the repository index
	chartRepo, err := repo.NewChartRepository(entry, getter.All(c.settings))
	if err != nil {
		return fmt.Errorf("failed to create chart repository: %w", err)
	}

	// Download the index file to verify the repo is valid
	if _, err := chartRepo.DownloadIndexFile(); err != nil {
		return fmt.Errorf("failed to fetch repository index: %w", err)
	}

	// Add to file and save
	f.Update(entry)
	if err := f.WriteFile(repoFile, 0644); err != nil {
		return fmt.Errorf("failed to write repository file: %w", err)
	}

	// Save priority
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

// RemoveRepository removes a Helm repository
func (c *Client) RemoveRepository(name string) error {
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	if !f.Remove(name) {
		return fmt.Errorf("repository %q not found", name)
	}

	if err := f.WriteFile(repoFile, 0644); err != nil {
		return fmt.Errorf("failed to write repository file: %w", err)
	}

	// Remove from priorities (best-effort)
	priorities, err := loadRepoPriorities()
	if err == nil {
		delete(priorities.Priorities, name)
		_ = saveRepoPriorities(priorities)
	}

	// Remove cached index file (best-effort cleanup)
	cacheDir := c.settings.RepositoryCache
	indexFile := filepath.Join(cacheDir, fmt.Sprintf("%s-index.yaml", name))
	_ = os.Remove(indexFile)

	return nil
}

// UpdateRepository updates the index for a repository
func (c *Client) UpdateRepository(name string) error {
	// Try to refresh ACR token if needed (ignore errors, will fail on actual update if token issue)
	_ = c.RefreshACRTokenIfNeeded(name)

	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	var entry *repo.Entry
	for _, r := range f.Repositories {
		if r.Name == name {
			entry = r
			break
		}
	}

	if entry == nil {
		return fmt.Errorf("repository %q not found", name)
	}

	chartRepo, err := repo.NewChartRepository(entry, getter.All(c.settings))
	if err != nil {
		return fmt.Errorf("failed to create chart repository: %w", err)
	}

	chartRepo.CachePath = c.settings.RepositoryCache
	if _, err := chartRepo.DownloadIndexFile(); err != nil {
		return fmt.Errorf("failed to download index: %w", err)
	}

	return nil
}

// UpdateAllRepositories updates the index for all repositories in parallel
func (c *Client) UpdateAllRepositories() error {
	// First, try to refresh any expired ACR tokens
	_ = c.RefreshAllACRTokens()

	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No repos to update
		}
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	if len(f.Repositories) == 0 {
		return nil
	}

	// Parallel update with bounded concurrency
	const maxConcurrency = 4
	sem := make(chan struct{}, maxConcurrency)

	var mu sync.Mutex
	var errs []string

	var wg sync.WaitGroup
	for _, r := range f.Repositories {
		wg.Add(1)
		go func(entry *repo.Entry) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// For ACR repos, ensure credentials are passed
			if isACRURL(entry.URL) && !entry.PassCredentialsAll {
				entry.PassCredentialsAll = true
			}

			chartRepo, err := repo.NewChartRepository(entry, getter.All(c.settings))
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Sprintf("%s: %v", entry.Name, err))
				mu.Unlock()
				return
			}

			chartRepo.CachePath = c.settings.RepositoryCache
			if _, err := chartRepo.DownloadIndexFile(); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Sprintf("%s: %v", entry.Name, err))
				mu.Unlock()
			}
		}(r)
	}
	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("failed to update some repositories: %s", strings.Join(errs, "; "))
	}

	return nil
}

// SetRepositoryPriority sets the priority for a repository
func (c *Client) SetRepositoryPriority(name string, priority int) error {
	// Verify repo exists
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	if !f.Has(name) {
		return fmt.Errorf("repository %q not found", name)
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

// SearchChart searches for a chart across all repositories
func (c *Client) SearchChart(chartName string) ([]ChartSource, error) {
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []ChartSource{}, nil
		}
		return nil, fmt.Errorf("failed to load repository file: %w", err)
	}

	priorities, err := loadRepoPriorities()
	if err != nil {
		return nil, fmt.Errorf("failed to load priorities: %w", err)
	}

	var sources []ChartSource

	for _, r := range f.Repositories {
		// Load the index file for this repo
		indexPath := filepath.Join(c.settings.RepositoryCache, fmt.Sprintf("%s-index.yaml", r.Name))
		indexFile, err := repo.LoadIndexFile(indexPath)
		if err != nil {
			continue // Skip repos without index
		}

		// Search for the chart
		// First try exact match, then prefix match
		var chartVersions repo.ChartVersions
		if cv, ok := indexFile.Entries[chartName]; ok {
			chartVersions = cv
		} else {
			// Try to find charts that match
			for name, cv := range indexFile.Entries {
				if strings.EqualFold(name, chartName) || strings.HasSuffix(strings.ToLower(name), "/"+strings.ToLower(chartName)) {
					chartVersions = cv
					chartName = name // Use the actual name
					break
				}
			}
		}

		if len(chartVersions) == 0 {
			continue
		}

		priority := 100
		if p, ok := priorities.Priorities[r.Name]; ok {
			priority = p
		}

		versions := make([]ChartVersion, 0, len(chartVersions))
		for _, cv := range chartVersions {
			versions = append(versions, ChartVersion{
				Version:     cv.Version,
				AppVersion:  cv.AppVersion,
				Description: cv.Description,
				Created:     cv.Created,
				Deprecated:  cv.Deprecated,
			})
		}

		// Sort versions (newest first)
		sort.Slice(versions, func(i, j int) bool {
			return versions[i].Created.After(versions[j].Created)
		})

		sources = append(sources, ChartSource{
			RepoName:  r.Name,
			RepoURL:   r.URL,
			Priority:  priority,
			ChartName: chartName,
			Versions:  versions,
		})
	}

	// Also search OCI registries
	ociSources, err := c.SearchOCIChart(chartName)
	if err == nil && len(ociSources) > 0 {
		for _, oci := range ociSources {
			// Convert OCIChartVersion to ChartVersion
			versions := make([]ChartVersion, 0, len(oci.Versions))
			for _, v := range oci.Versions {
				versions = append(versions, ChartVersion{
					Version: v.Version,
				})
			}

			// Create a display name for the OCI source
			registryName := strings.TrimPrefix(oci.RegistryURL, "https://")
			registryName = strings.TrimPrefix(registryName, "http://")

			sources = append(sources, ChartSource{
				RepoName:      fmt.Sprintf("oci://%s", registryName),
				RepoURL:       oci.RegistryURL,
				Priority:      oci.Priority,
				ChartName:     oci.ChartName,
				Versions:      versions,
				IsOCI:         true,
				OCIRepository: oci.Repository,
			})
		}
	}

	// Sort sources by priority (lower first)
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	return sources, nil
}

// GetChartVersions returns available versions for a chart from a specific repo
func (c *Client) GetChartVersions(repoName, chartName string) ([]ChartVersion, error) {
	indexPath := filepath.Join(c.settings.RepositoryCache, fmt.Sprintf("%s-index.yaml", repoName))
	indexFile, err := repo.LoadIndexFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load index for %s: %w", repoName, err)
	}

	chartVersions, ok := indexFile.Entries[chartName]
	if !ok {
		return nil, fmt.Errorf("chart %q not found in repository %q", chartName, repoName)
	}

	versions := make([]ChartVersion, 0, len(chartVersions))
	for _, cv := range chartVersions {
		versions = append(versions, ChartVersion{
			Version:     cv.Version,
			AppVersion:  cv.AppVersion,
			Description: cv.Description,
			Created:     cv.Created,
			Deprecated:  cv.Deprecated,
		})
	}

	// Sort versions (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Created.After(versions[j].Created)
	})

	return versions, nil
}

// getSettings returns the Helm CLI settings (for use by repo functions)
func (c *Client) getSettings() *cli.EnvSettings {
	return c.settings
}

// ListChartSources returns all available chart sources (HTTP repos + OCI registries)
func (c *Client) ListChartSources() ([]ChartSourceInfo, error) {
	var sources []ChartSourceInfo

	// Load HTTP repositories
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load repository file: %w", err)
	}

	priorities, _ := loadRepoPriorities()
	if priorities == nil {
		priorities = &RepoPriorities{Priorities: make(map[string]int)}
	}

	if f != nil {
		for _, r := range f.Repositories {
			priority := 100
			if p, ok := priorities.Priorities[r.Name]; ok {
				priority = p
			}
			sources = append(sources, ChartSourceInfo{
				Name:     r.Name,
				URL:      r.URL,
				IsOCI:    false,
				IsACR:    false,
				Priority: priority,
			})
		}
	}

	// Load OCI registries
	ociRegistries, err := c.ListOCIRegistries()
	if err == nil {
		for _, reg := range ociRegistries {
			if !reg.Authenticated {
				continue // Skip unauthenticated registries
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

	// Sort by priority
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	return sources, nil
}

// SearchChartInSource searches for a chart in a specific source
func (c *Client) SearchChartInSource(sourceName, chartName string) (*ChartSearchResult, error) {
	start := time.Now()

	// Check if it's an OCI source
	if strings.HasPrefix(sourceName, "oci://") {
		return c.searchChartInOCISource(sourceName, chartName, start)
	}

	// HTTP repository search
	return c.searchChartInHTTPRepo(sourceName, chartName, start)
}

// searchChartInHTTPRepo searches for a chart in an HTTP repository
func (c *Client) searchChartInHTTPRepo(repoName, chartName string, start time.Time) (*ChartSearchResult, error) {
	result := &ChartSearchResult{Found: false}

	// Load index file
	indexPath := filepath.Join(c.settings.RepositoryCache, fmt.Sprintf("%s-index.yaml", repoName))
	indexFile, err := repo.LoadIndexFile(indexPath)
	if err != nil {
		result.Log = fmt.Sprintf("[%s] Failed to load index: %v", repoName, err)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	// Search for chart
	var chartVersions repo.ChartVersions
	var foundName string
	if cv, ok := indexFile.Entries[chartName]; ok {
		chartVersions = cv
		foundName = chartName
	} else {
		// Try case-insensitive and suffix match
		for name, cv := range indexFile.Entries {
			if strings.EqualFold(name, chartName) || strings.HasSuffix(strings.ToLower(name), "/"+strings.ToLower(chartName)) {
				chartVersions = cv
				foundName = name
				break
			}
		}
	}

	if len(chartVersions) == 0 {
		result.Log = fmt.Sprintf("[%s] Chart '%s' not found in index (%d charts available)", repoName, chartName, len(indexFile.Entries))
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	// Get repo info
	repoFile := c.settings.RepositoryConfig
	f, _ := repo.LoadFile(repoFile)
	var repoURL string
	if f != nil {
		for _, r := range f.Repositories {
			if r.Name == repoName {
				repoURL = r.URL
				break
			}
		}
	}

	priorities, _ := loadRepoPriorities()
	priority := 100
	if priorities != nil {
		if p, ok := priorities.Priorities[repoName]; ok {
			priority = p
		}
	}

	// Build versions list
	versions := make([]ChartVersion, 0, len(chartVersions))
	for _, cv := range chartVersions {
		versions = append(versions, ChartVersion{
			Version:     cv.Version,
			AppVersion:  cv.AppVersion,
			Description: cv.Description,
			Created:     cv.Created,
			Deprecated:  cv.Deprecated,
		})
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Created.After(versions[j].Created)
	})

	result.Found = true
	result.Source = &ChartSource{
		RepoName:  repoName,
		RepoURL:   repoURL,
		Priority:  priority,
		ChartName: foundName,
		Versions:  versions,
		IsOCI:     false,
	}
	result.Log = fmt.Sprintf("[%s] Found chart '%s' with %d versions", repoName, foundName, len(versions))
	result.Duration = time.Since(start).Milliseconds()

	return result, nil
}

// searchChartInOCISource searches for a chart in an OCI registry
func (c *Client) searchChartInOCISource(sourceName, chartName string, start time.Time) (*ChartSearchResult, error) {
	result := &ChartSearchResult{Found: false}

	// Extract registry from oci://registry format
	registry := strings.TrimPrefix(sourceName, "oci://")

	// Find the registry in our list
	ociRegistries, err := c.ListOCIRegistries()
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
		result.Log = fmt.Sprintf("[%s] Registry not found in configured registries", sourceName)
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

	// Search ACR
	acrName := extractACRNameFromURL(targetRegistry.URL)
	if acrName == "" {
		result.Log = fmt.Sprintf("[%s] Could not extract ACR name from URL", sourceName)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	// List repositories in ACR
	cmd := exec.Command("az", "acr", "repository", "list", "-n", acrName, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		result.Log = fmt.Sprintf("[%s] Failed to list ACR repositories: %v", sourceName, err)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	var repos []string
	if err := json.Unmarshal(output, &repos); err != nil {
		result.Log = fmt.Sprintf("[%s] Failed to parse ACR repositories: %v", sourceName, err)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	// Find matching repository
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

	// Get tags for the matching repo
	cmd = exec.Command("az", "acr", "repository", "show-tags", "-n", acrName, "--repository", matchingRepo, "-o", "json", "--orderby", "time_desc")
	tagsOutput, err := cmd.Output()
	if err != nil {
		result.Log = fmt.Sprintf("[%s] Found chart '%s' but failed to get tags: %v", sourceName, matchingRepo, err)
		result.Duration = time.Since(start).Milliseconds()
		return result, nil
	}

	var tags []string
	if err := json.Unmarshal(tagsOutput, &tags); err != nil {
		result.Log = fmt.Sprintf("[%s] Found chart '%s' but failed to parse tags: %v", sourceName, matchingRepo, err)
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
