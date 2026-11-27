package k8s

import (
	"fmt"
	"net/http"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// The accessors below hand the shared store the same authenticated clients the
// rest of the application uses. They take a context name so a store shard is
// always bound to the context it was created for.

// KubeClient returns a typed clientset for the named context. An empty name
// means the client's current context.
func (c *Client) KubeClient(contextName string) (kubernetes.Interface, error) {
	return c.getClientForContext(contextName)
}

// DynamicClient returns a dynamic client for the named context.
func (c *Client) DynamicClient(contextName string) (dynamic.Interface, error) {
	return c.getDynamicClientForContext(contextName)
}

// MetadataClient returns a metadata-only client (PartialObjectMetadata list and
// watch, protobuf with JSON fallback) for the named context.
func (c *Client) MetadataClient(contextName string) (metadata.Interface, error) {
	if contextName == "" {
		c.mu.RLock()
		contextName = c.currentContext
		c.mu.RUnlock()
	}
	if IsDebugClusterContext(contextName) {
		return nil, fmt.Errorf("metadata client is not available on the debug cluster")
	}
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(c.getLoadingRules(), overrides)
	config, err := loader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config for context %s: %w", contextName, err)
	}
	config = metadata.ConfigFor(config)
	return newAuthenticatedClient(config, func(cfg *rest.Config, hc *http.Client) (metadata.Interface, error) {
		return metadata.NewForConfigAndClient(cfg, hc)
	})
}

// APIPodNodeSource returns the default API-server backed PodNodeSource, so a
// store-backed source can fall back to it while its shards are cold.
func APIPodNodeSource(c *Client) PodNodeSource { return apiPodNodeSource{c} }
