package main

import (
	"fmt"

	"kubikles/pkg/helm"
)

// =============================================================================
// Helm Release Management
// =============================================================================

// ListHelmReleases returns all Helm releases across the specified namespaces
func (a *App) ListHelmReleases(namespaces []string) ([]helm.Release, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListHelmReleases called: context=%s, namespaces=%v", currentContext, namespaces)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListReleases(currentContext, namespaces)
}

// GetHelmRelease returns detailed information about a specific release
func (a *App) GetHelmRelease(namespace, name string) (*helm.ReleaseDetail, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetHelmRelease called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetRelease(currentContext, namespace, name)
}

// GetHelmReleaseValues returns the user-supplied values for a release
func (a *App) GetHelmReleaseValues(namespace, name string) (map[string]interface{}, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetHelmReleaseValues called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseValues(currentContext, namespace, name)
}

// GetHelmReleaseAllValues returns all computed values for a release
func (a *App) GetHelmReleaseAllValues(namespace, name string) (map[string]interface{}, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetHelmReleaseAllValues called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseAllValues(currentContext, namespace, name)
}

// GetHelmReleaseHistory returns the revision history for a release
func (a *App) GetHelmReleaseHistory(namespace, name string) ([]helm.ReleaseHistory, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetHelmReleaseHistory called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseHistory(currentContext, namespace, name)
}

// UninstallHelmRelease removes a Helm release
func (a *App) UninstallHelmRelease(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("UninstallHelmRelease called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.Uninstall(currentContext, namespace, name)
}

// RollbackHelmRelease rolls back a release to a specific revision
func (a *App) RollbackHelmRelease(namespace, name string, revision int) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("RollbackHelmRelease called: context=%s, ns=%s, name=%s, revision=%d", currentContext, namespace, name, revision)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.Rollback(currentContext, namespace, name, revision)
}

// GetHelmReleaseResources returns the Kubernetes resources managed by a Helm release
func (a *App) GetHelmReleaseResources(namespace, name string) ([]helm.ResourceReference, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetHelmReleaseResources called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseResources(currentContext, namespace, name)
}

// =============================================================================
// Helm Repository Management
// =============================================================================

// ListHelmRepositories returns all configured Helm repositories with priorities
func (a *App) ListHelmRepositories() ([]helm.Repository, error) {
	a.logDebug("ListHelmRepositories called")
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListRepositories()
}

// AddHelmRepository adds a new Helm repository
func (a *App) AddHelmRepository(name, url string, priority int) error {
	a.logDebug("AddHelmRepository called: name=%s, url=%s, priority=%d", name, url, priority)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.AddRepository(name, url, priority)
}

// RemoveHelmRepository removes a Helm repository
func (a *App) RemoveHelmRepository(name string) error {
	a.logDebug("RemoveHelmRepository called: name=%s", name)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.RemoveRepository(name)
}

// UpdateHelmRepository updates the index for a repository
func (a *App) UpdateHelmRepository(name string) error {
	a.logDebug("UpdateHelmRepository called: name=%s", name)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.UpdateRepository(name)
}

// UpdateAllHelmRepositories updates the index for all repositories
func (a *App) UpdateAllHelmRepositories() error {
	fmt.Println(">>> UpdateAllHelmRepositories called <<<")
	a.logDebug("UpdateAllHelmRepositories called")
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.UpdateAllRepositories()
}

// SetHelmRepositoryPriority sets the priority for a repository
func (a *App) SetHelmRepositoryPriority(name string, priority int) error {
	a.logDebug("SetHelmRepositoryPriority called: name=%s, priority=%d", name, priority)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SetRepositoryPriority(name, priority)
}

// SearchHelmChart searches for a chart across all repositories
func (a *App) SearchHelmChart(chartName string) ([]helm.ChartSource, error) {
	a.logDebug("SearchHelmChart called: chartName=%s", chartName)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SearchChart(chartName)
}

// GetHelmChartVersions returns available versions for a chart from a specific repo
func (a *App) GetHelmChartVersions(repoName, chartName string) ([]helm.ChartVersion, error) {
	a.logDebug("GetHelmChartVersions called: repo=%s, chart=%s", repoName, chartName)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetChartVersions(repoName, chartName)
}

// UpgradeHelmRelease upgrades or reinstalls a release
func (a *App) UpgradeHelmRelease(namespace, name string, opts helm.UpgradeOptions) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("UpgradeHelmRelease called: context=%s, ns=%s, name=%s, repo=%s, chart=%s, version=%s",
		currentContext, namespace, name, opts.RepoName, opts.ChartName, opts.Version)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.UpgradeRelease(currentContext, namespace, name, opts)
}

// ForceHelmReleaseStatus forces a release to a specific status (e.g., "deployed")
func (a *App) ForceHelmReleaseStatus(namespace, name, status string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("ForceHelmReleaseStatus called: context=%s, ns=%s, name=%s, status=%s",
		currentContext, namespace, name, status)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ForceReleaseStatus(currentContext, namespace, name, status)
}

// ListOCIRegistries returns a list of OCI registries with authentication status
func (a *App) ListOCIRegistries() ([]helm.OCIRegistry, error) {
	a.logDebug("ListOCIRegistries called")
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListOCIRegistries()
}

// LoginOCIRegistry authenticates to an OCI registry with username/password
func (a *App) LoginOCIRegistry(registry, username, password string) error {
	a.logDebug("LoginOCIRegistry called: registry=%s, username=%s", registry, username)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.LoginOCIRegistry(registry, username, password)
}

// LoginACRWithAzureCLI logs into an Azure Container Registry using Azure CLI
func (a *App) LoginACRWithAzureCLI(registry string) error {
	a.logDebug("LoginACRWithAzureCLI called: registry=%s", registry)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.LoginACRWithAzureCLI(registry)
}

// LogoutOCIRegistry logs out from an OCI registry
func (a *App) LogoutOCIRegistry(registry string) error {
	a.logDebug("LogoutOCIRegistry called: registry=%s", registry)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.LogoutOCIRegistry(registry)
}

// SetOCIRegistryPriority sets the priority for an OCI registry
func (a *App) SetOCIRegistryPriority(registryURL string, priority int) error {
	a.logDebug("SetOCIRegistryPriority called: registry=%s, priority=%d", registryURL, priority)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SetOCIRegistryPriority(registryURL, priority)
}

// RemoveOCIRegistry removes an OCI registry (logout and remove priority)
func (a *App) RemoveOCIRegistry(registry string) error {
	a.logDebug("RemoveOCIRegistry called: registry=%s", registry)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.RemoveOCIRegistry(registry)
}

// ListChartSources returns all available chart sources (HTTP repos + OCI registries)
func (a *App) ListChartSources() ([]helm.ChartSourceInfo, error) {
	a.logDebug("ListChartSources called")
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListChartSources()
}

// SearchChartInSource searches for a chart in a specific source
func (a *App) SearchChartInSource(sourceName, chartName string) (*helm.ChartSearchResult, error) {
	a.logDebug("SearchChartInSource called: source=%s, chart=%s", sourceName, chartName)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SearchChartInSource(sourceName, chartName)
}
