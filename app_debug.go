package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"time"

	"runtime/debug"

	"kubikles/pkg/crashlog"
)

// =============================================================================
// Debug & Logging
// =============================================================================

// logDebug is an internal helper for formatted debug logging (not exported to frontend)
func (a *App) logDebug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05.000")
	fmt.Printf("DEBUG [%s]: %s\n", timestamp, msg)
	if a.ctx != nil {
		a.emitEvent("debug-log", msg)
	}
}

// LogDebug sends a debug message to the frontend (exported as single-arg for Wails binding compatibility)
func (a *App) LogDebug(msg string) {
	a.logDebug("%s", msg)
}

// LogMessage is an alias for LogDebug for frontend compatibility
func (a *App) LogMessage(message string) {
	a.logDebug("%s", message)
}

// GetCrashLogPath returns the path to the crash log file
func (a *App) GetCrashLogPath() string {
	return crashlog.GetLogPath()
}

// TestCrash triggers a panic for testing crash logging.
// Set inGoroutine=true to test goroutine panic recovery.
func (a *App) TestCrash(inGoroutine bool) {
	crashlog.Log("TestCrash called: inGoroutine=%v", inGoroutine)
	if inGoroutine {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					crashlog.LogError("TEST GOROUTINE PANIC RECOVERED: %v\nStack: %s", r, string(debug.Stack()))
				}
			}()
			panic("TEST PANIC IN GOROUTINE")
		}()
	} else {
		panic("TEST PANIC IN MAIN CALL")
	}
}

// OpenCrashLogDir opens the directory containing the crash log
func (a *App) OpenCrashLogDir() error {
	logPath := crashlog.GetLogPath()
	if logPath == "" {
		return fmt.Errorf("crash log path not available")
	}
	logDir := filepath.Dir(logPath)

	// Use native file manager based on OS
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", logDir)
	case "windows":
		// Windows explorer handles paths with spaces when passed directly
		cmd = exec.Command("explorer", logDir)
	default: // Linux and others
		cmd = exec.Command("xdg-open", logDir)
	}

	return cmd.Start()
}

func (a *App) DeletePod(namespace, name string) error {
	contextName := a.GetCurrentContext()
	a.logDebug("DeletePod called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeletePod(contextName, namespace, name)
	if err != nil {
		a.logDebug("DeletePod error: %v", err)
	} else {
		a.logDebug("DeletePod success")
	}
	return err
}

func (a *App) ForceDeletePod(namespace, name string) error {
	contextName := a.GetCurrentContext()
	a.logDebug("ForceDeletePod called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.ForceDeletePod(contextName, namespace, name)
	if err != nil {
		a.logDebug("ForceDeletePod error: %v", err)
	} else {
		a.logDebug("ForceDeletePod success")
	}
	return err
}

func (a *App) GetPodYaml(namespace, name string) (string, error) {
	a.logDebug("GetPodYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodYaml(namespace, name)
}

func (a *App) UpdatePodYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdatePodYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePodYaml(namespace, name, yamlContent)
}
