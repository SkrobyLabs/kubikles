package main

import (
	"context"
	"fmt"
	"kubikles/pkg/k8s"
	"kubikles/pkg/terminal"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

// App struct
type App struct {
	ctx             context.Context
	k8sClient       *k8s.Client
	terminalService *terminal.Service
}

// NewApp creates a new App application struct
func NewApp() *App {
	client, err := k8s.NewClient()
	if err != nil {
		fmt.Printf("Error initializing K8s client: %v\n", err)
	}
	return &App{
		k8sClient:       client,
		terminalService: terminal.NewService(),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.terminalService.Start(); err != nil {
		fmt.Printf("Failed to start terminal service: %v\n", err)
	}
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

// --- K8s Methods Exposed to Frontend ---

func (a *App) ListContexts() ([]string, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListContexts()
}

func (a *App) GetCurrentContext() string {
	if a.k8sClient == nil {
		return ""
	}
	return a.k8sClient.GetCurrentContext()
}

func (a *App) SwitchContext(name string) error {
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.SwitchContext(name)
}

func (a *App) ListPods(namespace string) ([]v1.Pod, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPods(namespace)
}

func (a *App) ListNodes() ([]v1.Node, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListNodes()
}

func (a *App) ListNamespaces() ([]v1.Namespace, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListNamespaces()
}

func (a *App) ListServices(namespace string) ([]v1.Service, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListServices(namespace)
}

func (a *App) ListConfigMaps(namespace string) ([]v1.ConfigMap, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListConfigMaps(namespace)
}

func (a *App) ListSecrets(namespace string) ([]v1.Secret, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListSecrets(namespace)
}

func (a *App) ListDeployments(namespace string) ([]appsv1.Deployment, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListDeployments(namespace)
}

func (a *App) GetPodLogs(namespace, podName string) (string, error) {
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodLogs(namespace, podName)
}

func (a *App) DeletePod(namespace, name string) error {
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePod(namespace, name)
}

func (a *App) ForceDeletePod(namespace, name string) error {
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ForceDeletePod(namespace, name)
}

func (a *App) GetPodYaml(namespace, name string) (string, error) {
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodYaml(namespace, name)
}

func (a *App) UpdatePodYaml(namespace, name, content string) error {
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePodYaml(namespace, name, content)
}

func (a *App) OpenTerminal(contextName, namespace, pod, container string) (string, error) {
	if a.terminalService == nil || a.terminalService.Port == 0 {
		return "", fmt.Errorf("terminal service not running")
	}

	url := fmt.Sprintf("ws://localhost:%d/terminal?context=%s&namespace=%s&pod=%s&container=%s",
		a.terminalService.Port, contextName, namespace, pod, container)

	return url, nil
}
