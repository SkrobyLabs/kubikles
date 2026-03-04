package helm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// getDockerConfigPath returns the path to Docker's config.json
func getDockerConfigPath() string {
	// Check DOCKER_CONFIG env var first
	if dockerConfig := os.Getenv("DOCKER_CONFIG"); dockerConfig != "" {
		return filepath.Join(dockerConfig, "config.json")
	}

	// Default location
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE") // Windows
	}
	return filepath.Join(home, ".docker", "config.json")
}

// listOCIRegistriesFromConfig reads Docker config and returns OCI registries with auth status
func listOCIRegistriesFromConfig() ([]OCIRegistry, error) {
	configPath := getDockerConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []OCIRegistry{}, nil
		}
		return nil, fmt.Errorf("failed to read docker config: %w", err)
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse docker config: %w", err)
	}

	// Load priorities
	priorities, err := loadOCIPriorities()
	if err != nil {
		return nil, fmt.Errorf("failed to load OCI priorities: %w", err)
	}

	// Check if a global credential store is configured
	hasCredStore := config.CredsStore != ""

	var registries []OCIRegistry
	for url, auth := range config.Auths {
		// Skip Docker Hub and non-registry entries
		if url == "https://index.docker.io/v1/" || url == "docker.io" {
			continue
		}

		username := auth.Username
		if username == "" && auth.Auth != "" {
			// Decode base64 auth (username:password)
			decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
			if err == nil {
				if u, _, found := strings.Cut(string(decoded), ":"); found {
					username = u
				} else if len(decoded) > 0 {
					username = string(decoded)
				}
			}
		}

		priority := 100 // Default priority
		if p, ok := priorities.Priorities[url]; ok {
			priority = p
		}

		// Determine authentication status:
		// - Has auth token or password directly in config
		// - Has identity token (used by some registries)
		// - Has a credential helper configured for this registry
		// - Has a global credential store AND an entry exists (even if empty)
		hasDirectAuth := auth.Auth != "" || auth.Password != "" || auth.IdentityToken != ""
		hasCredHelper := config.CredHelpers[url] != ""
		// If entry exists and there's a global cred store, assume authenticated
		usesCredStore := hasCredStore && !hasDirectAuth && !hasCredHelper

		registries = append(registries, OCIRegistry{
			URL:           url,
			Username:      username,
			Authenticated: hasDirectAuth || hasCredHelper || usesCredStore,
			IsACR:         strings.Contains(url, ".azurecr.io"),
			Priority:      priority,
		})
	}

	// Sort by priority (lower first), then by URL
	sort.Slice(registries, func(i, j int) bool {
		if registries[i].Priority != registries[j].Priority {
			return registries[i].Priority < registries[j].Priority
		}
		return registries[i].URL < registries[j].URL
	})

	return registries, nil
}
