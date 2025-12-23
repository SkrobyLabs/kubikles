package helm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// OCIRegistry represents an OCI registry with authentication status
type OCIRegistry struct {
	URL           string `json:"url"`
	Username      string `json:"username"`
	Authenticated bool   `json:"authenticated"`
	IsACR         bool   `json:"isAcr"`    // Azure Container Registry
	Priority      int    `json:"priority"` // Lower number = higher priority
}

// OCIPriorities stores priority settings for OCI registries
type OCIPriorities struct {
	Priorities map[string]int `json:"priorities"` // registry URL -> priority
}

// getOCIPrioritiesPath returns the path to the OCI priorities config file
func getOCIPrioritiesPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE") // Windows
	}
	return filepath.Join(home, ".config", "kubikles", "oci-registry-priorities.json")
}

// loadOCIPriorities loads OCI registry priorities from config
func loadOCIPriorities() (*OCIPriorities, error) {
	path := getOCIPrioritiesPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &OCIPriorities{Priorities: make(map[string]int)}, nil
		}
		return nil, err
	}

	var priorities OCIPriorities
	if err := json.Unmarshal(data, &priorities); err != nil {
		return nil, err
	}

	if priorities.Priorities == nil {
		priorities.Priorities = make(map[string]int)
	}

	return &priorities, nil
}

// saveOCIPriorities saves OCI registry priorities to config
func saveOCIPriorities(priorities *OCIPriorities) error {
	path := getOCIPrioritiesPath()

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

// ListOCIRegistries returns a list of known OCI registries with auth status
func (c *Client) ListOCIRegistries() ([]OCIRegistry, error) {
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
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) >= 1 {
					username = parts[0]
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
					_ = os.WriteFile(configPath, newData, 0644)
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

// IsOCIRegistryAuthenticated checks if we're authenticated to a registry
func (c *Client) IsOCIRegistryAuthenticated(registry string) bool {
	configPath := getDockerConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}

	// Check various forms of the registry URL
	registryVariants := []string{
		registry,
		strings.TrimPrefix(registry, "https://"),
		strings.TrimPrefix(registry, "http://"),
	}

	for _, variant := range registryVariants {
		if auth, ok := config.Auths[variant]; ok {
			return auth.Auth != "" || auth.Password != ""
		}
	}

	return false
}
