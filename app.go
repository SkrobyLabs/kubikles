package main

import (
	"archive/zip"
	"context"
	"fmt"
	"kubikles/pkg/k8s"
	"kubikles/pkg/terminal"
	"os"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
)

// App struct
type App struct {
	ctx              context.Context
	k8sClient        *k8s.Client
	terminalService  *terminal.Service
	podWatcherCancel context.CancelFunc
	podWatcherMutex  sync.Mutex
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

// TestEmit emits a test debug log event
func (a *App) TestEmit() {
	a.LogDebug("TestEmit called from frontend")
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

// ConfigMap YAML operations
func (a *App) GetConfigMapYaml(namespace, name string) (string, error) {
	a.LogDebug("GetConfigMapYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetConfigMapYaml(namespace, name)
}

func (a *App) UpdateConfigMapYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateConfigMapYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateConfigMapYaml(namespace, name, yamlContent)
}

func (a *App) DeleteConfigMap(namespace, name string) error {
	a.LogDebug("DeleteConfigMap called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteConfigMap(namespace, name)
}

// Secret YAML operations
func (a *App) GetSecretYaml(namespace, name string) (string, error) {
	a.LogDebug("GetSecretYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetSecretYaml(namespace, name)
}

func (a *App) UpdateSecretYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateSecretYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateSecretYaml(namespace, name, yamlContent)
}

func (a *App) DeleteSecret(namespace, name string) error {
	a.LogDebug("DeleteSecret called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteSecret(namespace, name)
}

func (a *App) ListDeployments(namespace string) ([]appsv1.Deployment, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListDeployments(namespace)
}

func (a *App) GetPodLogs(namespace, podName, containerName string, timestamps bool) (string, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetPodLogs called: context=%s, ns=%s, pod=%s, container=%s, timestamps=%v", currentContext, namespace, podName, containerName, timestamps)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodLogs(namespace, podName, containerName, timestamps)
}

func (a *App) GetAllPodLogs(namespace, podName, containerName string, timestamps bool) (string, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetAllPodLogs called: context=%s, ns=%s, pod=%s, container=%s, timestamps=%v", currentContext, namespace, podName, containerName, timestamps)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetAllPodLogs(namespace, podName, containerName, timestamps)
}

// LogDebug sends a debug message to the frontend
func (a *App) LogDebug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println("DEBUG:", msg)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "debug-log", msg)
	}
}

func (a *App) DeletePod(contextName, namespace, name string) error {
	a.LogDebug("DeletePod called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeletePod(contextName, namespace, name)
	if err != nil {
		a.LogDebug("DeletePod error: %v", err)
	} else {
		a.LogDebug("DeletePod success")
	}
	return err
}

func (a *App) ForceDeletePod(contextName, namespace, name string) error {
	a.LogDebug("ForceDeletePod called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.ForceDeletePod(contextName, namespace, name)
	if err != nil {
		a.LogDebug("ForceDeletePod error: %v", err)
	} else {
		a.LogDebug("ForceDeletePod success")
	}
	return err
}

func (a *App) GetPodYaml(namespace, name string) (string, error) {
	a.LogDebug("GetPodYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodYaml(namespace, name)
}

func (a *App) UpdatePodYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdatePodYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePodYaml(namespace, name, yamlContent)
}

func (a *App) OpenTerminal(contextName, namespace, podName, containerName string) (string, error) {
	a.LogDebug("OpenTerminal called: context=%s, ns=%s, pod=%s, container=%s", contextName, namespace, podName, containerName)
	if a.terminalService == nil {
		return "", fmt.Errorf("terminal service not initialized")
	}

	// Generate a unique ID for the terminal session (unused for now, but good for future)
	// terminalID := fmt.Sprintf("%s-%s-%s", namespace, podName, containerName)
	url := fmt.Sprintf("ws://localhost:%d/terminal?context=%s&namespace=%s&pod=%s&container=%s",
		a.terminalService.Port, contextName, namespace, podName, containerName)

	return url, nil
}

// --- Watcher ---

type PodEvent struct {
	Type string  `json:"type"`
	Pod  *v1.Pod `json:"pod"`
}

func (a *App) StartPodWatcher(namespace string) {
	a.LogDebug("Starting pod watcher for namespace: %s", namespace)

	// Cancel existing watcher if any
	a.podWatcherMutex.Lock()
	if a.podWatcherCancel != nil {
		a.podWatcherCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.podWatcherCancel = cancel
	a.podWatcherMutex.Unlock()

	go a.watchPodsLoop(ctx, namespace)
}

func (a *App) watchPodsLoop(ctx context.Context, namespace string) {
	defer func() {
		a.LogDebug("Pod watcher stopped for namespace: %s", namespace)
	}()

	watcher, err := a.k8sClient.WatchPods(ctx, namespace)
	if err != nil {
		a.LogDebug("Failed to start pod watcher: %v", err)
		return
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.ResultChan():
			if !ok {
				a.LogDebug("Watcher channel closed")
				return
			}

			// Cast to Pod
			pod, ok := event.Object.(*v1.Pod)
			if !ok {
				continue
			}

			// Emit event to frontend
			// We only care about ADDED, MODIFIED, DELETED
			if event.Type == "ADDED" || event.Type == "MODIFIED" || event.Type == "DELETED" {
				runtime.EventsEmit(a.ctx, "pod-event", PodEvent{
					Type: string(event.Type),
					Pod:  pod,
				})
			}
		}
	}
}

func (a *App) GetDeploymentYaml(namespace, name string) (string, error) {
	a.LogDebug("GetDeploymentYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetDeploymentYaml(namespace, name)
}

func (a *App) UpdateDeploymentYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateDeploymentYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateDeploymentYaml(namespace, name, yamlContent)
}

func (a *App) DeleteDeployment(contextName, namespace, name string) error {
	a.LogDebug("DeleteDeployment called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeleteDeployment(contextName, namespace, name)
	if err != nil {
		a.LogDebug("DeleteDeployment error: %v", err)
	} else {
		a.LogDebug("DeleteDeployment success")
	}
	return err
}

func (a *App) RestartDeployment(contextName, namespace, name string) error {
	a.LogDebug("RestartDeployment called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.RestartDeployment(contextName, namespace, name)
	if err != nil {
		a.LogDebug("RestartDeployment error: %v", err)
	} else {
		a.LogDebug("RestartDeployment success")
	}
	return err
}

// StatefulSet operations
func (a *App) ListStatefulSets(contextName, namespace string) ([]appsv1.StatefulSet, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListStatefulSets(contextName, namespace)
}

func (a *App) GetStatefulSetYaml(namespace, name string) (string, error) {
	a.LogDebug("GetStatefulSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetStatefulSetYaml(namespace, name)
}

func (a *App) UpdateStatefulSetYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateStatefulSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateStatefulSetYaml(namespace, name, yamlContent)
}

// DaemonSet wrappers
func (a *App) ListDaemonSets(namespace string) ([]appsv1.DaemonSet, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListDaemonSets called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListDaemonSets(currentContext, namespace)
}

func (a *App) GetDaemonSetYaml(namespace, name string) (string, error) {
	a.LogDebug("GetDaemonSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetDaemonSetYaml(namespace, name)
}

func (a *App) UpdateDaemonSetYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateDaemonSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateDaemonSetYaml(namespace, name, yamlContent)
}

func (a *App) RestartDaemonSet(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("RestartDaemonSet called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.RestartDaemonSet(currentContext, namespace, name)
}

func (a *App) DeleteDaemonSet(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteDaemonSet called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteDaemonSet(currentContext, namespace, name)
}

// ReplicaSet wrappers
func (a *App) ListReplicaSets(namespace string) ([]appsv1.ReplicaSet, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListReplicaSets called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListReplicaSets(currentContext, namespace)
}

func (a *App) GetReplicaSetYaml(namespace, name string) (string, error) {
	a.LogDebug("GetReplicaSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetReplicaSetYaml(namespace, name)
}

func (a *App) UpdateReplicaSetYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateReplicaSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateReplicaSetYaml(namespace, name, yamlContent)
}

func (a *App) DeleteReplicaSet(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteReplicaSet called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteReplicaSet(currentContext, namespace, name)
}

func (a *App) RestartStatefulSet(contextName, namespace, name string) error {
	a.LogDebug("RestartStatefulSet called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.RestartStatefulSet(contextName, namespace, name)
	if err != nil {
		a.LogDebug("RestartStatefulSet error: %v", err)
	} else {
		a.LogDebug("RestartStatefulSet success")
	}
	return err
}

func (a *App) DeleteStatefulSet(contextName, namespace, name string) error {
	a.LogDebug("DeleteStatefulSet called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeleteStatefulSet(contextName, namespace, name)
	if err != nil {
		a.LogDebug("DeleteStatefulSet error: %v", err)
	} else {
		a.LogDebug("DeleteStatefulSet success")
	}
	return err
}

func (a *App) SaveLogFile(content string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "kubikles-debug-logs.txt",
		Title:           "Save Debug Logs",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Text Files (*.txt)",
				Pattern:     "*.txt",
			},
		},
	})

	if err != nil {
		return err
	}

	if filePath == "" {
		return nil // User cancelled
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

func (a *App) SavePodLogs(content string, defaultFilename string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Title:           "Save Pod Logs",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Log Files (*.log)",
				Pattern:     "*.log",
			},
			{
				DisplayName: "Text Files (*.txt)",
				Pattern:     "*.txt",
			},
		},
	})

	if err != nil {
		return err
	}

	if filePath == "" {
		return nil // User cancelled
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

// PodLogEntry represents a single container's logs for the bundle
type PodLogEntry struct {
	PodName       string `json:"podName"`
	ContainerName string `json:"containerName"`
	Logs          string `json:"logs"`
}

// SaveLogsBundle saves multiple pod logs as a zip file
func (a *App) SaveLogsBundle(entries []PodLogEntry, defaultFilename string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Title:           "Save Logs Bundle",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Zip Files (*.zip)",
				Pattern:     "*.zip",
			},
		},
	})

	if err != nil {
		return err
	}

	if filePath == "" {
		return nil // User cancelled
	}

	// Create the zip file
	zipFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, entry := range entries {
		// Create path: podName/containerName.log
		logPath := fmt.Sprintf("%s/%s.log", entry.PodName, entry.ContainerName)
		writer, err := zipWriter.Create(logPath)
		if err != nil {
			return fmt.Errorf("failed to create zip entry %s: %w", logPath, err)
		}
		_, err = writer.Write([]byte(entry.Logs))
		if err != nil {
			return fmt.Errorf("failed to write logs for %s: %w", logPath, err)
		}
	}

	return nil
}

// Job operations
func (a *App) ListJobs(namespace string) ([]batchv1.Job, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListJobs called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListJobs(currentContext, namespace)
}

func (a *App) GetJobYaml(namespace, name string) (string, error) {
	a.LogDebug("GetJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetJobYaml(namespace, name)
}

func (a *App) UpdateJobYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateJobYaml(namespace, name, yamlContent)
}

func (a *App) DeleteJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteJob called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteJob(currentContext, namespace, name)
}

// CronJob operations
func (a *App) ListCronJobs(namespace string) ([]batchv1.CronJob, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListCronJobs called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCronJobs(currentContext, namespace)
}

func (a *App) GetCronJobYaml(namespace, name string) (string, error) {
	a.LogDebug("GetCronJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCronJobYaml(namespace, name)
}

func (a *App) UpdateCronJobYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateCronJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCronJobYaml(namespace, name, yamlContent)
}

func (a *App) DeleteCronJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteCronJob called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCronJob(currentContext, namespace, name)
}

func (a *App) TriggerCronJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("TriggerCronJob called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.TriggerCronJob(currentContext, namespace, name)
}

func (a *App) SuspendCronJob(namespace, name string, suspend bool) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("SuspendCronJob called: context=%s, ns=%s, name=%s, suspend=%v", currentContext, namespace, name, suspend)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.SuspendCronJob(currentContext, namespace, name, suspend)
}
