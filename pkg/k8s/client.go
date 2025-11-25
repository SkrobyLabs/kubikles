package k8s

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
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

func (c *Client) DeleteNamespace(name string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	return cs.CoreV1().Namespaces().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) GetNamespaceYAML(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}

	ns, err := cs.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Clean up fields that shouldn't be in the YAML
	ns.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(ns)
	if err != nil {
		return "", fmt.Errorf("failed to marshal namespace to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

func (c *Client) UpdateNamespaceYAML(name string, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	// Parse the YAML to a Namespace object
	var ns v1.Namespace
	if err := yaml.Unmarshal([]byte(yamlContent), &ns); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	// Ensure the name matches
	if ns.Name != name {
		return fmt.Errorf("namespace name in YAML (%s) does not match expected name (%s)", ns.Name, name)
	}

	_, err = cs.CoreV1().Namespaces().Update(context.TODO(), &ns, metav1.UpdateOptions{})
	return err
}

func (c *Client) ListEvents(namespace string) ([]v1.Event, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	events, err := cs.CoreV1().Events(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return events.Items, nil
}

func (c *Client) GetEventYAML(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}

	event, err := cs.CoreV1().Events(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	event.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

func (c *Client) UpdateEventYAML(namespace, name string, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	var event v1.Event
	if err := yaml.Unmarshal([]byte(yamlContent), &event); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	if event.Namespace != namespace || event.Name != name {
		return fmt.Errorf("namespace/name mismatch in yaml")
	}

	_, err = cs.CoreV1().Events(namespace).Update(context.TODO(), &event, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteEvent(namespace, name string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	return cs.CoreV1().Events(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
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

// GetSecretData returns the secret's data as a map of key -> base64-encoded value
func (c *Client) GetSecretData(namespace, name string) (map[string]string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	secret, err := cs.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for k, v := range secret.Data {
		result[k] = string(v)
	}
	return result, nil
}

// UpdateSecretData updates the secret's data from a map of key -> value (values are raw strings, will be stored as bytes)
func (c *Client) UpdateSecretData(namespace, name string, data map[string]string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	secret, err := cs.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	secret.Data = make(map[string][]byte)
	for k, v := range data {
		secret.Data[k] = []byte(v)
	}
	_, err = cs.CoreV1().Secrets(namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	return err
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

func (c *Client) GetPodLogs(namespace, podName, containerName string, timestamps bool) (string, error) {
	return c.getPodLogsWithOptions(namespace, podName, containerName, func(i int64) *int64 { return &i }(100), timestamps)
}

func (c *Client) GetAllPodLogs(namespace, podName, containerName string, timestamps bool) (string, error) {
	return c.getPodLogsWithOptions(namespace, podName, containerName, nil, timestamps)
}

func (c *Client) getPodLogsWithOptions(namespace, podName, containerName string, tailLines *int64, timestamps bool) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}

	opts := &v1.PodLogOptions{
		TailLines:  tailLines,
		Timestamps: timestamps,
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

// DaemonSet operations
func (c *Client) ListDaemonSets(contextName, namespace string) ([]appsv1.DaemonSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	daemonsets, err := cs.AppsV1().DaemonSets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return daemonsets.Items, nil
}

func (c *Client) GetDaemonSetYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	daemonset, err := cs.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	daemonset.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(daemonset)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateDaemonSetYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var daemonset appsv1.DaemonSet
	if err := yaml.Unmarshal([]byte(yamlContent), &daemonset); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AppsV1().DaemonSets(namespace).Update(context.TODO(), &daemonset, metav1.UpdateOptions{})
	return err
}

func (c *Client) RestartDaemonSet(contextName, namespace, name string) error {
	fmt.Printf("Restarting daemonset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Patch the daemonset to trigger a rollout
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, metav1.Now().String())
	_, err = cs.AppsV1().DaemonSets(namespace).Patch(context.TODO(), name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (c *Client) DeleteDaemonSet(contextName, namespace, name string) error {
	fmt.Printf("Deleting daemonset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.AppsV1().DaemonSets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// ReplicaSet operations
func (c *Client) ListReplicaSets(contextName, namespace string) ([]appsv1.ReplicaSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	replicasets, err := cs.AppsV1().ReplicaSets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return replicasets.Items, nil
}

func (c *Client) GetReplicaSetYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	replicaset, err := cs.AppsV1().ReplicaSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	replicaset.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(replicaset)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateReplicaSetYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var replicaset appsv1.ReplicaSet
	if err := yaml.Unmarshal([]byte(yamlContent), &replicaset); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AppsV1().ReplicaSets(namespace).Update(context.TODO(), &replicaset, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteReplicaSet(contextName, namespace, name string) error {
	fmt.Printf("Deleting replicaset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.AppsV1().ReplicaSets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// Job operations
func (c *Client) ListJobs(contextName, namespace string) ([]batchv1.Job, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	jobs, err := cs.BatchV1().Jobs(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return jobs.Items, nil
}

func (c *Client) GetJobYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	job, err := cs.BatchV1().Jobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(job)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateJobYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var job batchv1.Job
	if err := yaml.Unmarshal([]byte(yamlContent), &job); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.BatchV1().Jobs(namespace).Update(context.TODO(), &job, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteJob(contextName, namespace, name string) error {
	fmt.Printf("Deleting job: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.BatchV1().Jobs(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// CronJob operations
func (c *Client) ListCronJobs(contextName, namespace string) ([]batchv1.CronJob, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	cronJobs, err := cs.BatchV1().CronJobs(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return cronJobs.Items, nil
}

func (c *Client) GetCronJobYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	cronJob, err := cs.BatchV1().CronJobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(cronJob)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCronJobYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var cronJob batchv1.CronJob
	if err := yaml.Unmarshal([]byte(yamlContent), &cronJob); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.BatchV1().CronJobs(namespace).Update(context.TODO(), &cronJob, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCronJob(contextName, namespace, name string) error {
	fmt.Printf("Deleting cronjob: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.BatchV1().CronJobs(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) TriggerCronJob(contextName, namespace, cronJobName string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Get the CronJob to use as template
	cronJob, err := cs.BatchV1().CronJobs(namespace).Get(context.TODO(), cronJobName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get cronjob: %w", err)
	}

	// Create a Job from the CronJob spec
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cronJobName + "-manual-",
			Namespace:    namespace,
			Annotations: map[string]string{
				"cronjob.kubernetes.io/instantiate": "manual",
			},
		},
		Spec: cronJob.Spec.JobTemplate.Spec,
	}

	_, err = cs.BatchV1().Jobs(namespace).Create(context.TODO(), job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	return nil
}

func (c *Client) SuspendCronJob(contextName, namespace, name string, suspend bool) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Use JSON patch to update only the suspend field
	patchData := fmt.Sprintf(`{"spec":{"suspend":%t}}`, suspend)

	result, err := cs.BatchV1().CronJobs(namespace).Patch(
		context.TODO(),
		name,
		types.MergePatchType,
		[]byte(patchData),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch cronjob: %w", err)
	}

	_ = result
	return nil
}

// PersistentVolumeClaim operations
func (c *Client) ListPVCs(contextName, namespace string) ([]v1.PersistentVolumeClaim, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	pvcs, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvcs.Items, nil
}

func (c *Client) GetPVCYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	pvc.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(pvc)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePVCYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var pvc v1.PersistentVolumeClaim
	if err := yaml.Unmarshal([]byte(yamlContent), &pvc); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().PersistentVolumeClaims(namespace).Update(context.TODO(), &pvc, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePVC(contextName, namespace, name string) error {
	fmt.Printf("Deleting PVC: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().PersistentVolumeClaims(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// PersistentVolume operations (cluster-scoped)
func (c *Client) ListPVs(contextName string) ([]v1.PersistentVolume, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	pvs, err := cs.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvs.Items, nil
}

func (c *Client) GetPVYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	pv, err := cs.CoreV1().PersistentVolumes().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	pv.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(pv)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePVYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var pv v1.PersistentVolume
	if err := yaml.Unmarshal([]byte(yamlContent), &pv); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().PersistentVolumes().Update(context.TODO(), &pv, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePV(contextName, name string) error {
	fmt.Printf("Deleting PV: context=%s, name=%s\n", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().PersistentVolumes().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// StorageClass operations (cluster-scoped)
func (c *Client) ListStorageClasses(contextName string) ([]storagev1.StorageClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	scs, err := cs.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return scs.Items, nil
}

func (c *Client) GetStorageClassYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	sc, err := cs.StorageV1().StorageClasses().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	sc.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(sc)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateStorageClassYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var sc storagev1.StorageClass
	if err := yaml.Unmarshal([]byte(yamlContent), &sc); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.StorageV1().StorageClasses().Update(context.TODO(), &sc, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteStorageClass(contextName, name string) error {
	fmt.Printf("Deleting StorageClass: context=%s, name=%s\n", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.StorageV1().StorageClasses().Delete(context.TODO(), name, metav1.DeleteOptions{})
}
