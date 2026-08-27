package k8s

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestUpdateContextDetailReloadsActiveClientConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	kubeconfigPath := filepath.Join(home, ".kube", "config")
	if err := os.MkdirAll(filepath.Dir(kubeconfigPath), 0o755); err != nil {
		t.Fatalf("create kubeconfig directory: %v", err)
	}

	config := clientcmdapi.NewConfig()
	config.CurrentContext = "test"
	config.Contexts["test"] = &clientcmdapi.Context{
		Cluster:  "test-cluster",
		AuthInfo: "test-user",
	}
	config.Clusters["test-cluster"] = &clientcmdapi.Cluster{
		Server: "https://example.invalid",
	}
	config.AuthInfos["test-user"] = &clientcmdapi.AuthInfo{
		Token: "old-token",
	}
	if err := clientcmd.WriteToFile(*config, kubeconfigPath); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	client, err := NewClient()
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	client.mu.RLock()
	originalClientset := client.clientset
	client.mu.RUnlock()

	skipTLS := true
	token := "new-token"
	if err := client.UpdateContextDetail("test", ContextUpdateRequest{
		InsecureSkipTLSVerify: &skipTLS,
		Token:                 &token,
	}); err != nil {
		t.Fatalf("update active context: %v", err)
	}

	client.mu.RLock()
	reloadedClientset := client.clientset
	reloadedConfig, err := client.configLoading.ClientConfig()
	client.mu.RUnlock()
	if err != nil {
		t.Fatalf("load refreshed client config: %v", err)
	}

	if originalClientset == reloadedClientset {
		t.Fatal("active clientset was not rebuilt")
	}
	if !reloadedConfig.Insecure {
		t.Fatal("refreshed client config did not enable insecure TLS")
	}
	if reloadedConfig.BearerToken != token {
		t.Fatalf("refreshed client config token = %q, want %q", reloadedConfig.BearerToken, token)
	}
}

func TestExtraKubeconfigContextIsUsableForAllClientBuilders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	primaryPath := filepath.Join(home, ".kube", "config")
	extraPath := filepath.Join(t.TempDir(), "extra-config")
	writeTestKubeconfig(t, primaryPath, "primary", "https://primary.invalid", "primary-token")
	writeTestKubeconfig(t, extraPath, "extra", "https://extra.invalid", "extra-token")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	client.SetExtraKubeconfigPaths([]string{extraPath})

	contexts, err := client.ListContexts()
	if err != nil {
		t.Fatalf("list contexts: %v", err)
	}
	if !containsContext(contexts, "extra") {
		t.Fatalf("contexts %v do not include extra context", contexts)
	}

	if err := client.SwitchContext("extra"); err != nil {
		t.Fatalf("switch to extra context: %v", err)
	}
	if got := client.GetCurrentContext(); got != "extra" {
		t.Fatalf("current context = %q, want extra", got)
	}

	restConfig, err := client.GetRestConfigForContext("extra")
	if err != nil {
		t.Fatalf("get REST config for extra context: %v", err)
	}
	if restConfig.Host != "https://extra.invalid" || restConfig.BearerToken != "extra-token" {
		t.Fatalf("extra REST config = host %q token %q", restConfig.Host, restConfig.BearerToken)
	}
	if _, err := client.getClientForContext("extra"); err != nil {
		t.Fatalf("get client for extra context: %v", err)
	}
	if _, err := client.getClientsetForContext("primary"); err != nil {
		t.Fatalf("get clientset for primary context: %v", err)
	}
	if _, err := client.getDynamicClientForContext("extra"); err != nil {
		t.Fatalf("get dynamic client for extra context: %v", err)
	}
	if _, err := client.getApiExtensionsClientForContext("extra"); err != nil {
		t.Fatalf("get API extensions client for extra context: %v", err)
	}
}

func TestFailedContextSwitchPreservesPublishedClientState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	kubeconfigPath := filepath.Join(home, ".kube", "config")
	writeTestKubeconfig(t, kubeconfigPath, "primary", "https://primary.invalid", "primary-token")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	client.SetClientPoolSize(1)
	if err := client.SwitchContext("primary"); err != nil {
		t.Fatalf("rebuild client with pool: %v", err)
	}

	client.mu.RLock()
	beforeLoader := client.configLoading
	beforeClientset := client.clientset
	beforeMetrics := client.metricsClient
	beforePool := client.clientPool
	beforeContext := client.currentContext
	client.mu.RUnlock()

	if err := client.SwitchContext("missing"); err == nil {
		t.Fatal("switching to a missing context succeeded")
	}

	client.mu.RLock()
	defer client.mu.RUnlock()
	if client.configLoading != beforeLoader || client.clientset != beforeClientset || client.metricsClient != beforeMetrics || client.currentContext != beforeContext {
		t.Fatal("failed switch published partial client state")
	}
	if len(client.clientPool) != len(beforePool) || client.clientPool[0] != beforePool[0] {
		t.Fatal("failed switch replaced client pool")
	}
}

func writeTestKubeconfig(t *testing.T, path, contextName, server, token string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create kubeconfig directory: %v", err)
	}
	config := clientcmdapi.NewConfig()
	config.CurrentContext = contextName
	config.Contexts[contextName] = &clientcmdapi.Context{Cluster: contextName + "-cluster", AuthInfo: contextName + "-user"}
	config.Clusters[contextName+"-cluster"] = &clientcmdapi.Cluster{Server: server}
	config.AuthInfos[contextName+"-user"] = &clientcmdapi.AuthInfo{Token: token}
	if err := clientcmd.WriteToFile(*config, path); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
}

func containsContext(contexts []string, target string) bool {
	for _, context := range contexts {
		if context == target {
			return true
		}
	}
	return false
}
