package k8s

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kubikles/pkg/debug"
	"kubikles/pkg/events"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type acceleratorDebugEvent struct {
	name string
	data []interface{}
}

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

func TestSnapshotCurrentContextLogsFailuresToDebug(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	kubeDir := filepath.Join(home, ".kube")
	if err := os.MkdirAll(kubeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(kubeDir, "config")
	var calls []acceleratorDebugEvent
	debug.Init(events.EmitterFunc(func(name string, data ...interface{}) {
		calls = append(calls, acceleratorDebugEvent{name: name, data: data})
	}))
	debug.SetEnabled(true)
	t.Cleanup(func() {
		debug.SetEnabled(false)
		debug.Init(&events.NoopEmitter{})
	})

	if err := os.WriteFile(path, []byte("invalid: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Client{currentContext: "selected"}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(c.getLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: "selected"})
	_, expected := loader.RawConfig()
	if expected == nil {
		t.Fatal("invalid kubeconfig unexpectedly loaded")
	}
	if _, err := c.SnapshotCurrentContext("selected"); err != ErrAcceleratorContextUnavailable {
		t.Fatalf("unexpected snapshot error: %v", err)
	}
	assertAcceleratorSnapshotDebugEvent(t, calls, "selected", "load_kubeconfig", expected.Error())

	calls = nil
	c.currentContext = "other"
	if _, err := c.SnapshotCurrentContext("selected"); err != ErrAcceleratorContextUnavailable {
		t.Fatalf("unexpected validation error: %v", err)
	}
	assertAcceleratorSnapshotDebugEvent(t, calls, "selected", "validate_context", "requested context is empty, malformed, reserved, or not current")

	calls = nil
	writeAcceleratorKubeconfig(t, path, "selected", "selected", "cluster", "user", "default", "https://selected.invalid", "token")
	c.currentContext = "selected"
	if _, err := c.SnapshotCurrentContext("selected"); err != nil {
		t.Fatalf("valid snapshot failed: %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("successful snapshot emitted debug events: %#v", calls)
	}
}

func assertAcceleratorSnapshotDebugEvent(t *testing.T, calls []acceleratorDebugEvent, contextName, stage, reason string) {
	t.Helper()
	if len(calls) != 1 {
		t.Fatalf("expected one debug event, got %#v", calls)
	}
	call := calls[0]
	if call.name != "debug:log" || len(call.data) != 3 {
		t.Fatalf("unexpected debug event: %#v", call)
	}
	if call.data[0] != debug.CategoryHelm || call.data[1] != "Accelerator context snapshot failed" {
		t.Fatalf("unexpected debug event metadata: %#v", call.data)
	}
	details, ok := call.data[2].(map[string]interface{})
	if !ok || details["context"] != contextName || details["stage"] != stage || details["error"] != reason {
		t.Fatalf("unexpected debug event details: %#v", call.data[2])
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
