package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kubikles/pkg/k8s"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestFailedSwitchPreservesWatchers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	if err := os.MkdirAll(filepath.Dir(kubeconfigPath), 0o755); err != nil {
		t.Fatalf("create kubeconfig directory: %v", err)
	}
	config := clientcmdapi.NewConfig()
	config.CurrentContext = "active"
	config.Contexts["active"] = &clientcmdapi.Context{Cluster: "cluster", AuthInfo: "user"}
	config.Clusters["cluster"] = &clientcmdapi.Cluster{Server: "https://active.invalid"}
	config.AuthInfos["user"] = &clientcmdapi.AuthInfo{Token: "token"}
	if err := clientcmd.WriteToFile(*config, kubeconfigPath); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	client, err := k8s.NewClient()
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	app := &App{k8sClient: client}
	app.watcherManager = NewResourceWatcherManager(context.Background(), app)
	watchContext, cancel := context.WithCancel(context.Background())
	app.watcherManager.watchers["pods:default"] = &ResourceWatcher{
		Key: "pods:default", ResourceType: "pods", Namespace: "default", RefCount: 1, Cancel: cancel,
	}

	if err := app.SwitchContext("missing"); err == nil {
		t.Fatal("switching to a missing context succeeded")
	}
	if got := client.GetCurrentContext(); got != "active" {
		t.Fatalf("current context = %q, want active", got)
	}
	if _, ok := app.watcherManager.watchers["pods:default"]; !ok {
		t.Fatal("failed switch removed active watcher")
	}
	select {
	case <-watchContext.Done():
		t.Fatal("failed switch canceled active watcher")
	default:
	}
}
