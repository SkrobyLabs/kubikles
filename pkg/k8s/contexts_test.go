package k8s

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestFixedClientContextIsImmutable(t *testing.T) {
	client, err := NewClientForRESTConfig(testRESTConfig(t))
	if err != nil {
		t.Fatalf("NewClientForRESTConfig() error = %v", err)
	}
	if got := client.GetCurrentContext(); got != InClusterContextName {
		t.Fatalf("GetCurrentContext() = %q, want %q", got, InClusterContextName)
	}
	contexts, err := client.ListContexts()
	if err != nil || !reflect.DeepEqual(contexts, []string{InClusterContextName}) {
		t.Fatalf("ListContexts() = %v, %v", contexts, err)
	}
	details, err := client.GetContextDetails()
	if err != nil || !reflect.DeepEqual(details, []ContextDetail{{Name: InClusterContextName, Cluster: InClusterContextName, Server: testRESTConfig(t).Host, AuthInfo: "service-account", IsActive: true}}) {
		t.Fatalf("GetContextDetails() = %#v, %v", details, err)
	}
	if err := client.SwitchContext(InClusterContextName); err != nil {
		t.Fatalf("SwitchContext(in-cluster) error = %v", err)
	}
	for _, name := range []string{"", "other"} {
		if err := client.SwitchContext(name); !errors.Is(err, ErrFixedContextUnavailable) {
			t.Errorf("SwitchContext(%q) error = %v, want ErrFixedContextUnavailable", name, err)
		}
	}
	if _, err := client.getClientForContext(InClusterContextName); err != nil {
		t.Fatalf("getClientForContext(in-cluster) error = %v", err)
	}
	if _, err := client.getClientForContext(""); err != nil {
		t.Fatalf("getClientForContext(\"\") error = %v", err)
	}
	if _, err := client.getClientForContext("other"); !errors.Is(err, ErrFixedContextUnavailable) {
		t.Errorf("getClientForContext(other) error = %v", err)
	}
	if _, err := client.GetFullContextDetail(InClusterContextName); !errors.Is(err, ErrFixedContextImmutable) {
		t.Errorf("GetFullContextDetail() error = %v", err)
	}
	for _, err := range []error{
		client.DeleteContext(InClusterContextName),
		client.RenameContext(InClusterContextName, "other"),
		client.UpdateContextDetail(InClusterContextName, ContextUpdateRequest{}),
	} {
		if !errors.Is(err, ErrFixedContextImmutable) {
			t.Errorf("mutation error = %v, want ErrFixedContextImmutable", err)
		}
	}
}

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
