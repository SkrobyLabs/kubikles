package k8s

import (
	"errors"
	"fmt"
	"slices"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const InClusterContextName = "in-cluster"

var (
	ErrNilRESTConfig           = errors.New("nil REST config")
	ErrFixedContextUnavailable = errors.New("fixed context unavailable")
	ErrFixedContextImmutable   = errors.New("fixed context is immutable")
)

// NewClientForRESTConfig creates a typed Kubernetes client for one fixed context.
func NewClientForRESTConfig(config *rest.Config) (*Client, error) {
	if config == nil {
		return nil, ErrNilRESTConfig
	}
	clientConfig := deepCopyRESTConfig(config)
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset from REST config: %w", err)
	}
	return &Client{
		clientset:      clientset,
		currentContext: InClusterContextName,
		fixedContext:   true,
		baseConfig:     deepCopyRESTConfig(config),
	}, nil
}

// NewInClusterClient creates a fixed Kubernetes client from service-account configuration.
func NewInClusterClient() (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load in-cluster REST config: %w", err)
	}
	return NewClientForRESTConfig(config)
}

func (c *Client) isFixedContext() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fixedContext
}

func (c *Client) acceptsFixedContext(contextName string, allowEmpty bool) bool {
	return contextName == InClusterContextName || (allowEmpty && contextName == "")
}

func (c *Client) copyBaseConfig() *rest.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.baseConfig == nil {
		return nil
	}
	return deepCopyRESTConfig(c.baseConfig)
}

// deepCopyRESTConfig extends rest.CopyConfig with copies of the mutable nested
// values it aliases in client-go v0.34.2. Function and interface values retain
// their identity because they cannot be safely copied generically.
func deepCopyRESTConfig(config *rest.Config) *rest.Config {
	if config == nil {
		return nil
	}

	// rest.CopyConfig aliases ExecProvider before assigning a deep copy to its
	// Config field. Detach that struct first so the assignment cannot mutate the
	// caller (or a fixed client's stored baseConfig when called by a getter).
	detached := *config
	if config.ExecProvider != nil {
		execProvider := *config.ExecProvider
		detached.ExecProvider = &execProvider
	}
	copy := rest.CopyConfig(&detached)
	copy.TLSClientConfig.CertData = slices.Clone(config.TLSClientConfig.CertData)
	copy.TLSClientConfig.KeyData = slices.Clone(config.TLSClientConfig.KeyData)
	copy.TLSClientConfig.CAData = slices.Clone(config.TLSClientConfig.CAData)
	copy.TLSClientConfig.NextProtos = slices.Clone(config.TLSClientConfig.NextProtos)
	copy.Impersonate.Groups = slices.Clone(config.Impersonate.Groups)
	if config.Impersonate.Extra != nil {
		copy.Impersonate.Extra = make(map[string][]string, len(config.Impersonate.Extra))
		for key, values := range config.Impersonate.Extra {
			copy.Impersonate.Extra[key] = slices.Clone(values)
		}
	}
	copy.AuthProvider = config.AuthProvider.DeepCopy()
	copy.ExecProvider = config.ExecProvider.DeepCopy()
	if config.GroupVersion != nil {
		groupVersion := *config.GroupVersion
		copy.GroupVersion = &groupVersion
	}
	return copy
}
