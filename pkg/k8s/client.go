package k8s

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Client struct {
	clientset      *kubernetes.Clientset
	configLoading  clientcmd.ClientConfig
	currentContext string
	mu             sync.RWMutex
}

func NewClient() (*Client, error) {
	c := &Client{}
	if err := c.loadConfig(""); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) loadConfig(contextName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}

	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	c.configLoading = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := c.configLoading.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to load client config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	c.clientset = clientset

	// Update current context
	rawConfig, err := c.configLoading.RawConfig()
	if err == nil {
		if contextName != "" {
			c.currentContext = contextName
		} else {
			c.currentContext = rawConfig.CurrentContext
		}
	}

	return nil
}

func (c *Client) SwitchContext(contextName string) error {
	return c.loadConfig(contextName)
}

func (c *Client) GetCurrentContext() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentContext
}

func (c *Client) ListContexts() ([]string, error) {
	rawConfig, err := c.configLoading.RawConfig()
	if err != nil {
		return nil, err
	}
	var contexts []string
	for name := range rawConfig.Contexts {
		contexts = append(contexts, name)
	}
	return contexts, nil
}

// --- Resources ---

func (c *Client) ListPods(namespace string) ([]v1.Pod, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	pods, err := c.clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func (c *Client) ListNodes() ([]v1.Node, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	nodes, err := c.clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

func (c *Client) ListNamespaces() ([]v1.Namespace, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	namespaces, err := c.clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return namespaces.Items, nil
}

func (c *Client) ListServices(namespace string) ([]v1.Service, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	services, err := c.clientset.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return services.Items, nil
}

func (c *Client) ListConfigMaps(namespace string) ([]v1.ConfigMap, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cms, err := c.clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return cms.Items, nil
}

func (c *Client) ListSecrets(namespace string) ([]v1.Secret, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	secrets, err := c.clientset.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	// Sanitize secrets? For now, we return them as is, UI should handle masking.
	return secrets.Items, nil
}

func (c *Client) ListDeployments(namespace string) ([]appsv1.Deployment, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return deployments.Items, nil
}

func (c *Client) GetPodLogs(namespace, podName string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, &v1.PodLogOptions{
		TailLines: func(i int64) *int64 { return &i }(100), // Default to last 100 lines
	})

	podLogs, err := req.Stream(context.TODO())
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buf := new(strings.Builder)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
