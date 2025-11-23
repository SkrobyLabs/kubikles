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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/yaml"
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

func (c *Client) getClientset() (*kubernetes.Clientset, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.clientset == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return c.clientset, nil
}

func (c *Client) ListPods(namespace string) ([]v1.Pod, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	pods, err := cs.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func (c *Client) WatchPods(ctx context.Context, namespace string) (watch.Interface, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	return cs.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{})
}

func (c *Client) ListNodes() ([]v1.Node, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	nodes, err := cs.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

func (c *Client) ListNamespaces() ([]v1.Namespace, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	namespaces, err := cs.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return namespaces.Items, nil
}

func (c *Client) ListServices(namespace string) ([]v1.Service, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	services, err := cs.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return services.Items, nil
}

func (c *Client) ListConfigMaps(namespace string) ([]v1.ConfigMap, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	cms, err := cs.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return cms.Items, nil
}

func (c *Client) ListSecrets(namespace string) ([]v1.Secret, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	secrets, err := cs.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	// Sanitize secrets? For now, we return them as is, UI should handle masking.
	return secrets.Items, nil
}

// ConfigMap YAML operations
func (c *Client) GetConfigMapYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	configMap, err := cs.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	configMap.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(configMap)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateConfigMapYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var configMap v1.ConfigMap
	if err := yaml.Unmarshal([]byte(yamlContent), &configMap); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().ConfigMaps(namespace).Update(context.TODO(), &configMap, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteConfigMap(namespace, name string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	return cs.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// Secret YAML operations
func (c *Client) GetSecretYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	secret, err := cs.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	secret.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(secret)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateSecretYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var secret v1.Secret
	if err := yaml.Unmarshal([]byte(yamlContent), &secret); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().Secrets(namespace).Update(context.TODO(), &secret, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteSecret(namespace, name string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	return cs.CoreV1().Secrets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) ListDeployments(namespace string) ([]appsv1.Deployment, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	deployments, err := cs.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return deployments.Items, nil
}

func (c *Client) GetPodLogs(namespace, podName, containerName string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}

	opts := &v1.PodLogOptions{
		TailLines: func(i int64) *int64 { return &i }(100), // Default to last 100 lines
	}
	if containerName != "" {
		opts.Container = containerName
	}

	req := cs.CoreV1().Pods(namespace).GetLogs(podName, opts)

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

func (c *Client) getClientForContext(contextName string) (*kubernetes.Clientset, error) {
	c.mu.RLock()
	if contextName == "" || contextName == c.currentContext {
		defer c.mu.RUnlock()
		if c.clientset == nil {
			return nil, fmt.Errorf("k8s client not initialized")
		}
		return c.clientset, nil
	}
	c.mu.RUnlock()

	// Create a temporary config for the requested context
	// We don't want to lock here as loadConfig does, but we are creating a new config
	// entirely separate from the struct's state.
	// Actually, we can reuse the loading logic but we need to be careful not to modify c.configLoading
	// if it's shared.
	// Simplest way: create a new loader.

	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}

	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := configLoader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config for context %s: %w", contextName, err)
	}

	return kubernetes.NewForConfig(config)
}

func (c *Client) DeletePod(contextName, namespace, name string) error {
	fmt.Printf("Deleting pod: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) ForceDeletePod(contextName, namespace, name string) error {
	fmt.Printf("Force deleting pod: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	gracePeriod := int64(0)
	return cs.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	})
}

func (c *Client) GetPodYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	pod, err := cs.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields to make it cleaner for editing
	pod.ManagedFields = nil

	y, err := yaml.Marshal(pod)
	if err != nil {
		return "", err
	}
	return string(y), nil
}

func (c *Client) UpdatePodYaml(namespace, name, content string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	// Parse the YAML to a Pod object
	var pod v1.Pod
	if err := yaml.Unmarshal([]byte(content), &pod); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	// Ensure namespace and name match
	if pod.Namespace != namespace || pod.Name != name {
		return fmt.Errorf("namespace/name mismatch in yaml")
	}

	_, err = cs.CoreV1().Pods(namespace).Update(context.TODO(), &pod, metav1.UpdateOptions{})
	return err
}

func (c *Client) GetDeploymentYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	deployment, err := cs.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	deployment.ManagedFields = nil

	y, err := yaml.Marshal(deployment)
	if err != nil {
		return "", err
	}
	return string(y), nil
}

func (c *Client) UpdateDeploymentYaml(namespace, name, content string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	var deployment appsv1.Deployment
	if err := yaml.Unmarshal([]byte(content), &deployment); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	if deployment.Namespace != namespace || deployment.Name != name {
		return fmt.Errorf("namespace/name mismatch in yaml")
	}

	_, err = cs.AppsV1().Deployments(namespace).Update(context.TODO(), &deployment, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteDeployment(contextName, namespace, name string) error {
	fmt.Printf("Deleting deployment: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.AppsV1().Deployments(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) RestartDeployment(contextName, namespace, name string) error {
	fmt.Printf("Restarting deployment: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Patch the deployment to trigger a rollout
	// We update the spec.template.metadata.annotations with a timestamp
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, metav1.Now().String())
	_, err = cs.AppsV1().Deployments(namespace).Patch(context.TODO(), name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

// StatefulSet operations
func (c *Client) ListStatefulSets(contextName, namespace string) ([]appsv1.StatefulSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	statefulsets, err := cs.AppsV1().StatefulSets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return statefulsets.Items, nil
}

func (c *Client) GetStatefulSetYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	statefulset, err := cs.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	statefulset.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(statefulset)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateStatefulSetYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var statefulset appsv1.StatefulSet
	if err := yaml.Unmarshal([]byte(yamlContent), &statefulset); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AppsV1().StatefulSets(namespace).Update(context.TODO(), &statefulset, metav1.UpdateOptions{})
	return err
}

func (c *Client) RestartStatefulSet(contextName, namespace, name string) error {
	fmt.Printf("Restarting statefulset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Patch the statefulset to trigger a rollout
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, metav1.Now().String())
	_, err = cs.AppsV1().StatefulSets(namespace).Patch(context.TODO(), name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (c *Client) DeleteStatefulSet(contextName, namespace, name string) error {
	fmt.Printf("Deleting statefulset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.AppsV1().StatefulSets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}
