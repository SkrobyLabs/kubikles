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
