package helm

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/yaml"
)

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
	Values        map[string]interface{} `json:"values"`
	ComputedValues map[string]interface{} `json:"computedValues"`
	Notes         string                 `json:"notes"`
	Manifest      string                 `json:"manifest"`
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

// Client provides Helm operations
type Client struct {
	settings *cli.EnvSettings
}

// NewClient creates a new Helm client
func NewClient() *Client {
	settings := cli.New()
	return &Client{
		settings: settings,
	}
}

// getActionConfig creates a Helm action configuration for the given namespace and context
func (c *Client) getActionConfig(namespace, contextName string) (*action.Configuration, error) {
	actionConfig := new(action.Configuration)

	// Get kubeconfig path
	kubeconfigPath := c.settings.KubeConfig
	if kubeconfigPath == "" {
		home := homedir.HomeDir()
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	// Create a getter that respects the context
	getter := &contextAwareGetter{
		kubeconfigPath: kubeconfigPath,
		context:        contextName,
	}

	if err := actionConfig.Init(getter, namespace, os.Getenv("HELM_DRIVER"), func(format string, v ...interface{}) {
		// Silent logging - can be changed to debug if needed
	}); err != nil {
		return nil, fmt.Errorf("failed to initialize helm action config: %w", err)
	}

	return actionConfig, nil
}

// ListReleases returns all releases across namespaces
func (c *Client) ListReleases(contextName string, namespaces []string) ([]Release, error) {
	var allReleases []Release

	// If no namespaces specified, list all
	if len(namespaces) == 0 {
		releases, err := c.listReleasesInNamespace(contextName, "")
		if err != nil {
			return nil, err
		}
		allReleases = releases
	} else {
		for _, ns := range namespaces {
			releases, err := c.listReleasesInNamespace(contextName, ns)
			if err != nil {
				return nil, fmt.Errorf("failed to list releases in namespace %s: %w", ns, err)
			}
			allReleases = append(allReleases, releases...)
		}
	}

	// Sort by updated time descending
	sort.Slice(allReleases, func(i, j int) bool {
		return allReleases[i].Updated.After(allReleases[j].Updated)
	})

	return allReleases, nil
}

func (c *Client) listReleasesInNamespace(contextName, namespace string) ([]Release, error) {
	actionConfig, err := c.getActionConfig(namespace, contextName)
	if err != nil {
		return nil, err
	}

	listAction := action.NewList(actionConfig)
	listAction.All = true
	listAction.AllNamespaces = namespace == ""
	listAction.SetStateMask()

	results, err := listAction.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	releases := make([]Release, 0, len(results))
	for _, r := range results {
		releases = append(releases, releaseToModel(r))
	}

	return releases, nil
}

// GetRelease returns detailed information about a specific release
func (c *Client) GetRelease(contextName, namespace, name string) (*ReleaseDetail, error) {
	actionConfig, err := c.getActionConfig(namespace, contextName)
	if err != nil {
		return nil, err
	}

	getAction := action.NewGet(actionConfig)
	r, err := getAction.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get release %s: %w", name, err)
	}

	detail := &ReleaseDetail{
		Release:        releaseToModel(r),
		Values:         r.Config,
		ComputedValues: r.Chart.Values,
		Notes:          r.Info.Notes,
		Manifest:       r.Manifest,
	}

	return detail, nil
}

// GetReleaseValues returns the values for a release (user-supplied values)
func (c *Client) GetReleaseValues(contextName, namespace, name string) (map[string]interface{}, error) {
	actionConfig, err := c.getActionConfig(namespace, contextName)
	if err != nil {
		return nil, err
	}

	getValuesAction := action.NewGetValues(actionConfig)
	getValuesAction.AllValues = false // Only user-supplied values

	values, err := getValuesAction.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get release values for %s: %w", name, err)
	}

	return values, nil
}

// GetReleaseAllValues returns all values for a release (computed + user-supplied)
func (c *Client) GetReleaseAllValues(contextName, namespace, name string) (map[string]interface{}, error) {
	actionConfig, err := c.getActionConfig(namespace, contextName)
	if err != nil {
		return nil, err
	}

	getValuesAction := action.NewGetValues(actionConfig)
	getValuesAction.AllValues = true

	values, err := getValuesAction.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get release values for %s: %w", name, err)
	}

	return values, nil
}

// GetReleaseHistory returns the revision history for a release
func (c *Client) GetReleaseHistory(contextName, namespace, name string) ([]ReleaseHistory, error) {
	actionConfig, err := c.getActionConfig(namespace, contextName)
	if err != nil {
		return nil, err
	}

	historyAction := action.NewHistory(actionConfig)
	historyAction.Max = 256 // Max number of revisions to fetch

	releases, err := historyAction.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get release history for %s: %w", name, err)
	}

	history := make([]ReleaseHistory, 0, len(releases))
	for _, r := range releases {
		history = append(history, ReleaseHistory{
			Revision:    r.Version,
			Status:      r.Info.Status.String(),
			Chart:       r.Chart.Metadata.Name,
			AppVersion:  r.Chart.Metadata.AppVersion,
			Updated:     r.Info.LastDeployed.Time,
			Description: r.Info.Description,
		})
	}

	// Sort by revision descending (newest first)
	sort.Slice(history, func(i, j int) bool {
		return history[i].Revision > history[j].Revision
	})

	return history, nil
}

// Uninstall removes a release
func (c *Client) Uninstall(contextName, namespace, name string) error {
	actionConfig, err := c.getActionConfig(namespace, contextName)
	if err != nil {
		return err
	}

	uninstallAction := action.NewUninstall(actionConfig)
	_, err = uninstallAction.Run(name)
	if err != nil {
		return fmt.Errorf("failed to uninstall release %s: %w", name, err)
	}

	return nil
}

// Rollback rolls back a release to a specific revision
func (c *Client) Rollback(contextName, namespace, name string, revision int) error {
	actionConfig, err := c.getActionConfig(namespace, contextName)
	if err != nil {
		return err
	}

	rollbackAction := action.NewRollback(actionConfig)
	rollbackAction.Version = revision

	if err := rollbackAction.Run(name); err != nil {
		return fmt.Errorf("failed to rollback release %s to revision %d: %w", name, revision, err)
	}

	return nil
}

// GetReleaseResources returns the Kubernetes resources managed by a Helm release
func (c *Client) GetReleaseResources(contextName, namespace, name string) ([]ResourceReference, error) {
	actionConfig, err := c.getActionConfig(namespace, contextName)
	if err != nil {
		return nil, err
	}

	getAction := action.NewGet(actionConfig)
	r, err := getAction.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get release %s: %w", name, err)
	}

	return parseManifestResources(r.Manifest, namespace)
}

// parseManifestResources extracts resource references from a Helm manifest
func parseManifestResources(manifest, defaultNamespace string) ([]ResourceReference, error) {
	var resources []ResourceReference

	// Split manifest into individual documents
	scanner := bufio.NewScanner(strings.NewReader(manifest))
	scanner.Split(splitYAMLDocuments)

	for scanner.Scan() {
		doc := strings.TrimSpace(scanner.Text())
		if doc == "" || doc == "---" {
			continue
		}

		// Parse the YAML document to extract metadata
		var obj struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}

		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			continue // Skip invalid documents
		}

		if obj.Kind == "" || obj.Metadata.Name == "" {
			continue
		}

		ns := obj.Metadata.Namespace
		if ns == "" {
			ns = defaultNamespace
		}

		resources = append(resources, ResourceReference{
			Kind:      obj.Kind,
			Name:      obj.Metadata.Name,
			Namespace: ns,
		})
	}

	return resources, nil
}

// splitYAMLDocuments is a split function for Scanner that splits on YAML document separators
func splitYAMLDocuments(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// Look for document separator
	sep := []byte("\n---")
	if i := strings.Index(string(data), string(sep)); i >= 0 {
		return i + len(sep), data[0:i], nil
	}

	// If at EOF, return what's left
	if atEOF {
		return len(data), data, nil
	}

	// Request more data
	return 0, nil, nil
}

// releaseToModel converts a Helm release to our model
func releaseToModel(r *release.Release) Release {
	return Release{
		Name:         r.Name,
		Namespace:    r.Namespace,
		Revision:     r.Version,
		Status:       r.Info.Status.String(),
		Chart:        r.Chart.Metadata.Name,
		ChartVersion: r.Chart.Metadata.Version,
		AppVersion:   r.Chart.Metadata.AppVersion,
		Updated:      r.Info.LastDeployed.Time,
		Description:  r.Info.Description,
	}
}

// contextAwareGetter implements RESTClientGetter for a specific context
type contextAwareGetter struct {
	kubeconfigPath string
	context        string
}

func (g *contextAwareGetter) ToRESTConfig() (*rest.Config, error) {
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: g.kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}
	if g.context != "" {
		configOverrides.CurrentContext = g.context
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	return clientConfig.ClientConfig()
}

func (g *contextAwareGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := g.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}

	return memory.NewMemCacheClient(discoveryClient), nil
}

func (g *contextAwareGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	return mapper, nil
}

func (g *contextAwareGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: g.kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}
	if g.context != "" {
		configOverrides.CurrentContext = g.context
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
}
