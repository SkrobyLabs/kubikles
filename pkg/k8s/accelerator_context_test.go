package k8s

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestSnapshotCurrentContextUsesMergedConfigAndNamespace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	kubeDir := filepath.Join(home, ".kube")
	if err := os.MkdirAll(kubeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	primary := filepath.Join(kubeDir, "config")
	extra := filepath.Join(home, "extra-kubeconfig")
	writeAcceleratorKubeconfig(t, primary, "other", "other", "other-cluster", "other-user", "", "https://other.invalid", "other-token")
	writeAcceleratorKubeconfig(t, extra, "selected", "selected", "selected-cluster", "selected-user", "team-a", "https://selected.invalid", "selected-token")

	c := &Client{currentContext: "selected", extraKubeconfigPaths: []string{extra}}
	snapshot, err := c.SnapshotCurrentContext("selected")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextName() != "selected" || snapshot.Namespace() != "team-a" {
		t.Fatalf("unexpected snapshot: %s/%s", snapshot.ContextName(), snapshot.Namespace())
	}
	if got := snapshot.RESTConfig(); got == nil || got.Host != "https://selected.invalid" || got.BearerToken != "selected-token" {
		t.Fatal("snapshot did not retain exact selected desktop REST configuration")
	}
	if snapshot.Clientset() == nil || len(snapshot.Identity()) != 64 {
		t.Fatal("snapshot missing client or safe identity")
	}

	writeAcceleratorKubeconfig(t, extra, "selected", "selected", "selected-cluster", "selected-user", "", "https://selected.invalid", "selected-token")
	snapshot, err = c.SnapshotCurrentContext("selected")
	if err != nil || snapshot.Namespace() != "default" {
		t.Fatalf("empty namespace did not become default: %v %#v", err, snapshot)
	}
}

func TestSnapshotContextIdentityIsSafeAndStable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	kubeDir := filepath.Join(home, ".kube")
	if err := os.MkdirAll(kubeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(kubeDir, "config")
	secretA, secretB := "bearer-secret-a", "bearer-secret-b"
	writeAcceleratorKubeconfig(t, path, "selected", "selected", "cluster", "user", "one", "https://selected.invalid", secretA)
	c := &Client{currentContext: "selected"}
	first, err := c.SnapshotCurrentContext("selected")
	if err != nil {
		t.Fatal(err)
	}
	writeAcceleratorKubeconfig(t, path, "selected", "selected", "cluster", "user", "two", "https://selected.invalid", secretB)
	second, err := c.SnapshotCurrentContext("selected")
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity() != second.Identity() {
		t.Fatal("identity changed with credential or namespace material")
	}
	for _, output := range []string{first.Identity(), fmt.Sprintf("%v", first), fmt.Sprintf("%#v", first), errString(err)} {
		if strings.Contains(output, secretA) || strings.Contains(output, secretB) || strings.Contains(output, path) {
			t.Fatalf("snapshot exposed secret corpus: %q", output)
		}
	}

	c.currentContext = "different"
	if _, err := c.SnapshotCurrentContext("selected"); err != ErrAcceleratorContextUnavailable {
		t.Fatal("non-current context did not fail closed")
	}
	c.currentContext = DebugClusterContextName
	if _, err := c.SnapshotCurrentContext(DebugClusterContextName); err != ErrAcceleratorContextUnavailable {
		t.Fatal("debug context did not fail closed")
	}
}

func TestSnapshotCurrentContextRejectsMalformedNamespaceAndMissingEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	kubeDir := filepath.Join(home, ".kube")
	if err := os.MkdirAll(kubeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(kubeDir, "config")
	c := &Client{currentContext: "selected"}
	for _, namespace := range []string{"UPPER", "white space", strings.Repeat("a", 64)} {
		writeAcceleratorKubeconfig(t, path, "selected", "selected", "cluster", "user", namespace, "https://selected.invalid", "token")
		if _, err := c.SnapshotCurrentContext("selected"); err != ErrAcceleratorContextUnavailable {
			t.Fatalf("accepted invalid namespace %q", namespace)
		}
	}
	writeAcceleratorKubeconfig(t, path, "selected", "selected", "missing", "user", "", "https://selected.invalid", "token")
	if _, err := c.SnapshotCurrentContext("selected"); err != ErrAcceleratorContextUnavailable {
		t.Fatal("accepted missing cluster")
	}
}

func writeAcceleratorKubeconfig(t *testing.T, path, current, contextName, clusterName, authName, namespace, server, token string) {
	t.Helper()
	config := clientcmdapi.NewConfig()
	config.CurrentContext = current
	config.Contexts[contextName] = &clientcmdapi.Context{Cluster: clusterName, AuthInfo: authName, Namespace: namespace}
	if clusterName != "missing" {
		config.Clusters[clusterName] = &clientcmdapi.Cluster{Server: server, InsecureSkipTLSVerify: true}
	}
	config.AuthInfos[authName] = &clientcmdapi.AuthInfo{Token: token}
	if err := clientcmd.WriteToFile(*config, path); err != nil {
		t.Fatal(err)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
