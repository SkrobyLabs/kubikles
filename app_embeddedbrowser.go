package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"kubikles/pkg/debug"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	browserPodName          = "kubikles-browser"
	browserPodLabel         = "kubikles.io/component"
	browserPodLabelVal      = "embedded-browser"
	browserImage            = "lscr.io/linuxserver/chromium:latest"
	browserContainerPort    = 3000
	browserPortFwdLabel     = "Browser: Chromium"
	browserStartTimeout     = 3 * time.Minute
)

// EmbeddedBrowserSession is the public state returned to the frontend.
type EmbeddedBrowserSession struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	LocalPort int    `json:"localPort"`
	Status    string `json:"status"` // "stopped", "starting", "running", "error"
	Error     string `json:"error,omitempty"`
}

// embeddedBrowserState is the private in-memory state held by the App.
type embeddedBrowserState struct {
	mu        sync.Mutex
	session   *EmbeddedBrowserSession
	portFwdID string
}

// =============================================================================
// Embedded Browser API
// =============================================================================

// StartEmbeddedBrowser creates a Chromium pod in the given namespace and
// port-forwards its noVNC interface to a random local port.
func (a *App) StartEmbeddedBrowser(namespace string) (EmbeddedBrowserSession, error) {
	a.embeddedBrowser.mu.Lock()
	defer a.embeddedBrowser.mu.Unlock()

	// Return existing running session
	if a.embeddedBrowser.session != nil && a.embeddedBrowser.session.Status == "running" {
		return *a.embeddedBrowser.session, nil
	}

	if a.k8sClient == nil {
		return EmbeddedBrowserSession{}, fmt.Errorf("k8s client not initialized")
	}
	if a.portForwardManager == nil {
		return EmbeddedBrowserSession{}, fmt.Errorf("port forward manager not initialized")
	}

	debug.LogPortforward("StartEmbeddedBrowser", map[string]interface{}{"namespace": namespace})

	sess := &EmbeddedBrowserSession{
		PodName:   browserPodName,
		Namespace: namespace,
		Status:    "starting",
	}
	a.embeddedBrowser.session = sess

	// Best-effort delete any leftover pod from a previous session
	_ = a.k8sClient.ForceDeletePod("", namespace, browserPodName)
	time.Sleep(500 * time.Millisecond)

	// Create the pod
	pod := buildBrowserPod(namespace)
	if _, err := a.k8sClient.CreatePod("", namespace, pod); err != nil {
		sess.Status = "error"
		sess.Error = err.Error()
		a.embeddedBrowser.session = nil
		return EmbeddedBrowserSession{Status: "error", Error: err.Error()}, fmt.Errorf("create pod: %w", err)
	}

	// Wait for Ready
	if err := a.k8sClient.WaitForPodRunning("", namespace, browserPodName, browserStartTimeout); err != nil {
		_ = a.k8sClient.ForceDeletePod("", namespace, browserPodName)
		sess.Status = "error"
		sess.Error = err.Error()
		a.embeddedBrowser.session = nil
		return EmbeddedBrowserSession{Status: "error", Error: err.Error()}, err
	}

	// Pick a free local port
	localPort := a.portForwardManager.GetRandomAvailablePort()
	if localPort == 0 {
		_ = a.k8sClient.ForceDeletePod("", namespace, browserPodName)
		a.embeddedBrowser.session = nil
		return EmbeddedBrowserSession{Status: "error", Error: "no available local port"}, fmt.Errorf("no available local port")
	}

	// Register port-forward config (ephemeral, AutoStart=false)
	currentCtx := a.k8sClient.GetCurrentContext()
	cfg, err := a.portForwardManager.AddConfig(PortForwardConfig{
		Context:      currentCtx,
		Namespace:    namespace,
		ResourceType: "pod",
		ResourceName: browserPodName,
		LocalPort:    localPort,
		RemotePort:   browserContainerPort,
		Label:        browserPortFwdLabel,
		AutoStart:    false,
		KeepAlive:    true,
	})
	if err != nil {
		_ = a.k8sClient.ForceDeletePod("", namespace, browserPodName)
		a.embeddedBrowser.session = nil
		return EmbeddedBrowserSession{Status: "error", Error: err.Error()}, err
	}

	// Start the actual port forward
	if err := a.portForwardManager.Start(cfg.ID); err != nil {
		_ = a.portForwardManager.DeleteConfig(cfg.ID)
		_ = a.k8sClient.ForceDeletePod("", namespace, browserPodName)
		a.embeddedBrowser.session = nil
		return EmbeddedBrowserSession{Status: "error", Error: err.Error()}, err
	}

	sess.LocalPort = localPort
	sess.Status = "running"
	a.embeddedBrowser.portFwdID = cfg.ID

	debug.LogPortforward("StartEmbeddedBrowser: ready", map[string]interface{}{
		"namespace": namespace,
		"localPort": localPort,
	})
	return *sess, nil
}

// StopEmbeddedBrowser tears down the port forward and deletes the pod.
func (a *App) StopEmbeddedBrowser() error {
	a.embeddedBrowser.mu.Lock()
	defer a.embeddedBrowser.mu.Unlock()

	sess := a.embeddedBrowser.session
	if sess == nil {
		return nil
	}

	debug.LogPortforward("StopEmbeddedBrowser", map[string]interface{}{"namespace": sess.Namespace})

	if a.embeddedBrowser.portFwdID != "" && a.portForwardManager != nil {
		_ = a.portForwardManager.Stop(a.embeddedBrowser.portFwdID)
		_ = a.portForwardManager.DeleteConfig(a.embeddedBrowser.portFwdID)
		a.embeddedBrowser.portFwdID = ""
	}

	if a.k8sClient != nil {
		_ = a.k8sClient.ForceDeletePod("", sess.Namespace, browserPodName)
	}

	a.embeddedBrowser.session = nil
	return nil
}

// SendTextToEmbeddedBrowser injects text into the remote X11 session via xdotool,
// making it available to paste (Ctrl+V) in the remote Chromium.
// It first sets the X11 clipboard with xclip, falling back to xdotool type if xclip
// is not available.
func (a *App) SendTextToEmbeddedBrowser(text string) error {
	a.embeddedBrowser.mu.Lock()
	sess := a.embeddedBrowser.session
	a.embeddedBrowser.mu.Unlock()

	if sess == nil || sess.Status != "running" {
		return fmt.Errorf("browser not running")
	}
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	// Try xclip first (sets X11 clipboard — user can then Ctrl+V)
	_, err := a.k8sClient.ExecCommandInPod(sess.Namespace, browserPodName, "chromium",
		[]string{"/bin/sh", "-c", fmt.Sprintf("printf '%%s' %s | xclip -selection clipboard -i", shellescape(text))},
	)
	if err == nil {
		return nil
	}

	// Fallback: xdotool type (types the text into whatever is focused)
	_, err = a.k8sClient.ExecCommandInPod(sess.Namespace, browserPodName, "chromium",
		[]string{"xdotool", "type", "--clearmodifiers", "--", text},
	)
	return err
}

// shellescape wraps a string in single quotes for safe shell interpolation.
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// GetEmbeddedBrowserStatus returns the current session state.
func (a *App) GetEmbeddedBrowserStatus() EmbeddedBrowserSession {
	a.embeddedBrowser.mu.Lock()
	defer a.embeddedBrowser.mu.Unlock()
	if a.embeddedBrowser.session == nil {
		return EmbeddedBrowserSession{Status: "stopped"}
	}
	return *a.embeddedBrowser.session
}

// =============================================================================
// Helpers
// =============================================================================

// buildBrowserPod constructs the Pod spec for the Chromium container.
func buildBrowserPod(namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      browserPodName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":           browserPodName,
				browserPodLabel: browserPodLabelVal,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "chromium",
					Image: browserImage,
					Ports: []corev1.ContainerPort{
						{ContainerPort: browserContainerPort, Protocol: corev1.ProtocolTCP},
					},
					Env: []corev1.EnvVar{
						{Name: "TZ", Value: "UTC"},
						{Name: "PUID", Value: "1000"},
						{Name: "PGID", Value: "1000"},
						{Name: "CUSTOM_PORT", Value: fmt.Sprintf("%d", browserContainerPort)},
						// Lower resolution = smaller frames = less encoding/transfer lag
						{Name: "DISPLAY_WIDTH", Value: "1280"},
						{Name: "DISPLAY_HEIGHT", Value: "800"},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeUnconfined,
						},
					},
				},
			},
		},
	}
}
