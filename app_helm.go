package main

import (
	"fmt"

	"kubikles/pkg/debug"
	"kubikles/pkg/helm"
)

// =============================================================================
// Helm Release Management
// =============================================================================

// ListHelmReleases returns all Helm releases across the specified namespaces
func (a *App) ListHelmReleases(namespaces []string) ([]helm.Release, error) {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("ListHelmReleases", map[string]interface{}{"context": currentContext, "namespaces": namespaces})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListReleases(currentContext, namespaces)
}

// GetHelmRelease returns detailed information about a specific release
func (a *App) GetHelmRelease(namespace, name string) (*helm.ReleaseDetail, error) {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("GetHelmRelease", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetRelease(currentContext, namespace, name)
}

// GetHelmReleaseValues returns the user-supplied values for a release
func (a *App) GetHelmReleaseValues(namespace, name string) (map[string]interface{}, error) {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("GetHelmReleaseValues", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseValues(currentContext, namespace, name)
}

// GetHelmReleaseAllValues returns all computed values for a release
func (a *App) GetHelmReleaseAllValues(namespace, name string) (map[string]interface{}, error) {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("GetHelmReleaseAllValues", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseAllValues(currentContext, namespace, name)
}

// GetHelmReleaseHistory returns the revision history for a release
func (a *App) GetHelmReleaseHistory(namespace, name string) ([]helm.ReleaseHistory, error) {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("GetHelmReleaseHistory", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseHistory(currentContext, namespace, name)
}

// UninstallHelmRelease removes a Helm release
func (a *App) UninstallHelmRelease(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("UninstallHelmRelease", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.Uninstall(currentContext, namespace, name)
}

// RollbackHelmRelease rolls back a release to a specific revision
func (a *App) RollbackHelmRelease(namespace, name string, revision int) error {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("RollbackHelmRelease", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name, "revision": revision})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.Rollback(currentContext, namespace, name, revision)
}

// GetHelmReleaseResources returns the Kubernetes resources managed by a Helm release
func (a *App) GetHelmReleaseResources(namespace, name string) ([]helm.ResourceReference, error) {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("GetHelmReleaseResources", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
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
	debug.LogHelm("ListHelmRepositories", nil)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListRepositories()
}

// AddHelmRepository adds a new Helm repository
func (a *App) AddHelmRepository(name, url string, priority int) error {
	debug.LogHelm("AddHelmRepository", map[string]interface{}{"name": name, "url": url, "priority": priority})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.AddRepository(name, url, priority)
}

// RemoveHelmRepository removes a Helm repository
func (a *App) RemoveHelmRepository(name string) error {
	debug.LogHelm("RemoveHelmRepository", map[string]interface{}{"name": name})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.RemoveRepository(name)
}

// UpdateHelmRepository updates the index for a repository
func (a *App) UpdateHelmRepository(name string) error {
	debug.LogHelm("UpdateHelmRepository", map[string]interface{}{"name": name})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.UpdateRepository(name)
}

// UpdateAllHelmRepositories updates the index for all repositories
func (a *App) UpdateAllHelmRepositories() error {
	fmt.Println(">>> UpdateAllHelmRepositories called <<<")
	debug.LogHelm("UpdateAllHelmRepositories", nil)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.UpdateAllRepositories()
}

// SetHelmRepositoryPriority sets the priority for a repository
func (a *App) SetHelmRepositoryPriority(name string, priority int) error {
	debug.LogHelm("SetHelmRepositoryPriority", map[string]interface{}{"name": name, "priority": priority})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SetRepositoryPriority(name, priority)
}

// SearchHelmChart searches for a chart across all repositories
func (a *App) SearchHelmChart(chartName string) ([]helm.ChartSource, error) {
	debug.LogHelm("SearchHelmChart", map[string]interface{}{"chartName": chartName})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SearchChart(chartName)
}

// GetHelmChartVersions returns available versions for a chart from a specific repo
func (a *App) GetHelmChartVersions(repoName, chartName string) ([]helm.ChartVersion, error) {
	debug.LogHelm("GetHelmChartVersions", map[string]interface{}{"repo": repoName, "chart": chartName})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetChartVersions(repoName, chartName)
}

// UpgradeHelmRelease upgrades or reinstalls a release
func (a *App) UpgradeHelmRelease(namespace, name string, opts helm.UpgradeOptions) error {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("UpgradeHelmRelease", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name, "repo": opts.RepoName, "chart": opts.ChartName, "version": opts.Version})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.UpgradeRelease(currentContext, namespace, name, opts)
}

// ForceHelmReleaseStatus forces a release to a specific status (e.g., "deployed")
func (a *App) ForceHelmReleaseStatus(namespace, name, status string) error {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("ForceHelmReleaseStatus", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name, "status": status})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ForceReleaseStatus(currentContext, namespace, name, status)
}

// ListOCIRegistries returns a list of OCI registries with authentication status
func (a *App) ListOCIRegistries() ([]helm.OCIRegistry, error) {
	debug.LogHelm("ListOCIRegistries", nil)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListOCIRegistries()
}

// LoginOCIRegistry authenticates to an OCI registry with username/password
func (a *App) LoginOCIRegistry(registry, username, password string) error {
	debug.LogHelm("LoginOCIRegistry", map[string]interface{}{"registry": registry, "username": username})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.LoginOCIRegistry(registry, username, password)
}

// LoginACRWithAzureCLI logs into an Azure Container Registry using Azure CLI
func (a *App) LoginACRWithAzureCLI(registry string) error {
	debug.LogHelm("LoginACRWithAzureCLI", map[string]interface{}{"registry": registry})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.LoginACRWithAzureCLI(registry)
}

// LogoutOCIRegistry logs out from an OCI registry
func (a *App) LogoutOCIRegistry(registry string) error {
	debug.LogHelm("LogoutOCIRegistry", map[string]interface{}{"registry": registry})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.LogoutOCIRegistry(registry)
}

// SetOCIRegistryPriority sets the priority for an OCI registry
func (a *App) SetOCIRegistryPriority(registryURL string, priority int) error {
	debug.LogHelm("SetOCIRegistryPriority", map[string]interface{}{"registry": registryURL, "priority": priority})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SetOCIRegistryPriority(registryURL, priority)
}

// RemoveOCIRegistry removes an OCI registry (logout and remove priority)
func (a *App) RemoveOCIRegistry(registry string) error {
	debug.LogHelm("RemoveOCIRegistry", map[string]interface{}{"registry": registry})
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.RemoveOCIRegistry(registry)
}

// ListChartSources returns all available chart sources (HTTP repos + OCI registries)
func (a *App) ListChartSources() ([]helm.ChartSourceInfo, error) {
	debug.LogHelm("ListChartSources", nil)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListChartSources()
}

// SearchChartInSource searches for a chart in a specific source
func (a *App) SearchChartInSource(sourceName, chartName string) (*helm.ChartSearchResult, error) {
	debug.LogHelm("SearchChartInSource", map[string]interface{}{"source": sourceName, "chart": chartName})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SearchChartInSource(sourceName, chartName)
}

// =============================================================================
// Helm Template Preview / Dry-Run / Validation
// =============================================================================

// HelmTemplateRelease renders templates locally without contacting the cluster
func (a *App) HelmTemplateRelease(releaseName, namespace string, opts helm.UpgradeOptions) (*helm.TemplateResult, error) {
	debug.LogHelm("HelmTemplateRelease", map[string]interface{}{"name": releaseName, "ns": namespace, "chart": opts.ChartName, "version": opts.Version})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.TemplateRelease(releaseName, namespace, opts)
}

// HelmDryRunUpgrade performs a server-side dry-run upgrade and returns current vs proposed manifests
func (a *App) HelmDryRunUpgrade(namespace, releaseName string, opts helm.UpgradeOptions) (*helm.DryRunResult, error) {
	currentContext := a.GetCurrentContext()
	debug.LogHelm("HelmDryRunUpgrade", map[string]interface{}{"context": currentContext, "ns": namespace, "name": releaseName, "chart": opts.ChartName, "version": opts.Version})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.DryRunUpgrade(currentContext, namespace, releaseName, opts)
}

// HelmValidateValues validates values against the chart's JSON schema
func (a *App) HelmValidateValues(opts helm.UpgradeOptions) ([]helm.ValidationError, error) {
	debug.LogHelm("HelmValidateValues", map[string]interface{}{"chart": opts.ChartName, "version": opts.Version})
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ValidateValues(opts)
}
