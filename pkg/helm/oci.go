//go:build helm

package helm

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// ListOCIRegistries returns a list of known OCI registries with auth status
func (c *Client) ListOCIRegistries() ([]OCIRegistry, error) {
	return listOCIRegistriesFromConfig()
}

// SetOCIRegistryPriority sets the priority for an OCI registry
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

// RemoveOCIRegistry removes an OCI registry (logout, remove from Docker config, and remove priority)
func (c *Client) RemoveOCIRegistry(registry string) error {
	// First try logout via helm (might fail if not authenticated)
	_ = c.LogoutOCIRegistry(registry)

	// Also directly remove from Docker config (in case logout didn't remove it)
	configPath := getDockerConfigPath()
	data, err := os.ReadFile(configPath)
	if err == nil {
		var config DockerConfig
		if json.Unmarshal(data, &config) == nil {
			// Try different URL variants
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
					_ = os.WriteFile(configPath, newData, 0600) // Contains auth credentials
				}
			}
		}
	}

	// Remove from priorities
	priorities, err := loadOCIPriorities()
	if err == nil {
		// Also try variants for priorities
		delete(priorities.Priorities, registry)
		delete(priorities.Priorities, strings.TrimPrefix(registry, "https://"))
		delete(priorities.Priorities, strings.TrimPrefix(registry, "http://"))
		_ = saveOCIPriorities(priorities)
	}

	return nil
}

// LoginOCIRegistry authenticates to an OCI registry
func (c *Client) LoginOCIRegistry(registry, username, password string) error {
	// Use helm registry login command
	cmd := exec.Command("helm", "registry", "login", registry, "--username", username, "--password-stdin")
	cmd.Stdin = strings.NewReader(password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to login to registry %s: %s", registry, string(output))
	}
	return nil
}

// LoginACRWithAzureCLI logs into an ACR using Azure CLI credentials
func (c *Client) LoginACRWithAzureCLI(registry string) error {
	// Extract registry name from URL
	registryName := registry
	if strings.Contains(registry, ".azurecr.io") {
		// Extract just the registry name
		registryName = strings.TrimSuffix(registry, ".azurecr.io")
		registryName = strings.TrimPrefix(registryName, "https://")
		registryName = strings.TrimPrefix(registryName, "http://")
	}

	// First, try to get admin credentials
	cmd := exec.Command("az", "acr", "credential", "show", "-n", registryName,
		"--query", "{username:username, password:passwords[0].value}", "-o", "json")
	output, err := cmd.Output()
	if err == nil {
		var creds struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if json.Unmarshal(output, &creds) == nil && creds.Username != "" && creds.Password != "" {
			// Use admin credentials
			return c.LoginOCIRegistry(registry, creds.Username, creds.Password)
		}
	}

	// Fallback: Use az acr login which handles authentication automatically
	cmd = exec.Command("az", "acr", "login", "-n", registryName)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to login to ACR %s: %s", registryName, string(output))
	}

	return nil
}

// LogoutOCIRegistry logs out from an OCI registry
func (c *Client) LogoutOCIRegistry(registry string) error {
	cmd := exec.Command("helm", "registry", "logout", registry)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to logout from registry %s: %s", registry, string(output))
	}
	return nil
}

// SearchOCIChart searches for a chart across all OCI registries
func (c *Client) SearchOCIChart(chartName string) ([]OCIChartSource, error) {
	registries, err := c.ListOCIRegistries()
	if err != nil {
		return nil, err
	}

	var sources []OCIChartSource

	for _, reg := range registries {
		if !reg.Authenticated {
			continue // Skip unauthenticated registries
		}

		if reg.IsACR {
			// Search ACR using Azure CLI
			acrName := extractACRNameFromURL(reg.URL)
			if acrName == "" {
				continue
			}

			// List repositories in ACR
			cmd := exec.Command("az", "acr", "repository", "list", "-n", acrName, "-o", "json")
			output, err := cmd.Output()
			if err != nil {
				continue // Skip if we can't list repos
			}

			var repos []string
			if err := json.Unmarshal(output, &repos); err != nil {
				continue
			}

			// Find repos that match the chart name
			for _, repo := range repos {
				// Match if repo ends with the chart name or equals it
				repoLower := strings.ToLower(repo)
				chartLower := strings.ToLower(chartName)
				if repoLower == chartLower || strings.HasSuffix(repoLower, "/"+chartLower) {
					// Get tags for this repo
					cmd := exec.Command("az", "acr", "repository", "show-tags", "-n", acrName, "--repository", repo, "-o", "json", "--orderby", "time_desc")
					tagsOutput, err := cmd.Output()
					if err != nil {
						continue
					}

					var tags []string
					if err := json.Unmarshal(tagsOutput, &tags); err != nil {
						continue
					}

					versions := make([]OCIChartVersion, 0, len(tags))
					for _, tag := range tags {
						versions = append(versions, OCIChartVersion{Version: tag})
					}

					sources = append(sources, OCIChartSource{
						RegistryURL: reg.URL,
						Repository:  repo,
						ChartName:   chartName,
						Priority:    reg.Priority,
						IsACR:       true,
						Versions:    versions,
					})
				}
			}
		}
		// TODO: Add support for other OCI registries using OCI Distribution API
	}

	// Sort by priority
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	return sources, nil
}
