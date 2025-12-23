package helm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

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

// ChartSource represents a chart available from a repository
type ChartSource struct {
	RepoName   string         `json:"repoName"`
	RepoURL    string         `json:"repoUrl"`
	Priority   int            `json:"priority"`
	ChartName  string         `json:"chartName"`
	Versions   []ChartVersion `json:"versions"`
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

	return os.WriteFile(path, data, 0644)
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

	// Remove from priorities
	priorities, err := loadRepoPriorities()
	if err == nil {
		delete(priorities.Priorities, name)
		saveRepoPriorities(priorities)
	}

	// Remove cached index file
	cacheDir := c.settings.RepositoryCache
	indexFile := filepath.Join(cacheDir, fmt.Sprintf("%s-index.yaml", name))
	os.Remove(indexFile)

	return nil
}

// UpdateRepository updates the index for a repository
func (c *Client) UpdateRepository(name string) error {
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

// UpdateAllRepositories updates the index for all repositories
func (c *Client) UpdateAllRepositories() error {
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No repos to update
		}
		return fmt.Errorf("failed to load repository file: %w", err)
	}

	var errs []string
	for _, r := range f.Repositories {
		chartRepo, err := repo.NewChartRepository(r, getter.All(c.settings))
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.Name, err))
			continue
		}

		chartRepo.CachePath = c.settings.RepositoryCache
		if _, err := chartRepo.DownloadIndexFile(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.Name, err))
		}
	}

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
