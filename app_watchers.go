package main

import (
	"context"
	"fmt"
	rtdebug "runtime/debug"
	"strings"
	"time"

	"kubikles/pkg/crashlog"
	"kubikles/pkg/debug"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubikles/pkg/k8s"
)

// =============================================================================
// Resource Watchers
// =============================================================================

// ResourceEvent is the generic event emitted for any resource type change
type ResourceEvent struct {
	Type         string                 `json:"type"`         // ADDED, MODIFIED, DELETED
	ResourceType string                 `json:"resourceType"` // pods, namespaces, deployments, etc.
	Namespace    string                 `json:"namespace"`    // Resource's namespace (empty for cluster-scoped)
	Resource     map[string]interface{} `json:"resource"`     // Unstructured resource data
}

// WatcherErrorEvent is emitted when a watcher encounters an error
type WatcherErrorEvent struct {
	ResourceType string `json:"resourceType"` // Resource type that failed
	Namespace    string `json:"namespace"`    // Namespace being watched
	Error        string `json:"error"`        // Error message
	Recoverable  bool   `json:"recoverable"`  // Whether the watcher will retry
	Context      string `json:"context"`      // K8s context this error belongs to
}

// WatcherStatusEvent is emitted when watcher status changes
type WatcherStatusEvent struct {
	ResourceType string `json:"resourceType"` // Resource type
	Namespace    string `json:"namespace"`    // Namespace being watched
	Status       string `json:"status"`       // "connected", "reconnecting", "stopped"
	Context      string `json:"context"`      // K8s context this status belongs to
}

// SubscribeResourceWatcher subscribes to a resource watcher, returning the watcher key
func (a *App) SubscribeResourceWatcher(resourceType, namespace string) string {
	if a.watcherManager == nil {
		debug.LogWatcher("SubscribeResourceWatcher: watcher manager not initialized", nil)
		return ""
	}
	return a.watcherManager.Subscribe(resourceType, namespace)
}

// SubscribeCRDWatcher subscribes to a CRD watcher using GVR, returning the watcher key
func (a *App) SubscribeCRDWatcher(group, version, resource, namespace string) string {
	if a.watcherManager == nil {
		debug.LogWatcher("SubscribeCRDWatcher: watcher manager not initialized", nil)
		return ""
	}
	return a.watcherManager.SubscribeCRD(group, version, resource, namespace)
}

// UnsubscribeWatcher unsubscribes from a watcher by key
func (a *App) UnsubscribeWatcher(watcherKey string) {
	if a.watcherManager == nil {
		debug.LogWatcher("UnsubscribeWatcher: watcher manager not initialized", nil)
		return
	}
	a.watcherManager.Unsubscribe(watcherKey)
}

// StopAllWatchers stops all active watchers (called on context switch)
func (a *App) StopAllWatchers() {
	if a.watcherManager == nil {
		debug.LogWatcher("StopAllWatchers: watcher manager not initialized", nil)
		return
	}
	a.watcherManager.StopAll()
}

// watchResourceLoop is the generic watch loop for standard Kubernetes resources.
// It includes reconnection logic with exponential backoff and resourceVersion tracking
// for resumable watches that avoid duplicate ADDED events on reconnection.
func (a *App) watchResourceLoop(ctx context.Context, resourceType, namespace string) {
	watcherKey := resourceType + ":" + namespace

	defer func() {
		if r := recover(); r != nil {
			crashlog.LogError("PANIC in watchResourceLoop: type=%s, namespace=%s, panic=%v\nStack: %s",
				resourceType, namespace, r, string(rtdebug.Stack()))
		}
		debug.LogWatcher("Resource watcher stopped", map[string]interface{}{"type": resourceType, "namespace": namespace})
		a.emitEvent("watcher-status", WatcherStatusEvent{
			ResourceType: resourceType,
			Namespace:    namespace,
			Status:       "stopped",
			Context:      a.GetCurrentContext(),
		})
	}()

	if a.k8sClient == nil {
		debug.LogWatcher("watchResourceLoop: k8s client not initialized", nil)
		return
	}

	// Get watcher for resourceVersion tracking
	var rw *ResourceWatcher
	if a.watcherManager != nil {
		a.watcherManager.mutex.RLock()
		rw = a.watcherManager.watchers[watcherKey]
		a.watcherManager.mutex.RUnlock()
	}

	// Helper to get current resourceVersion
	getResourceVersion := func() string {
		if rw == nil {
			return ""
		}
		rw.mu.RLock()
		defer rw.mu.RUnlock()
		return rw.ResourceVersion
	}

	// Helper to update resourceVersion
	setResourceVersion := func(rv string) {
		if rw != nil && rv != "" {
			rw.mu.Lock()
			rw.ResourceVersion = rv
			rw.mu.Unlock()
		}
	}

	// Helper to clear resourceVersion (for expired/gone errors)
	clearResourceVersion := func() {
		if rw != nil {
			rw.mu.Lock()
			rw.ResourceVersion = ""
			rw.mu.Unlock()
		}
	}

	// Reconnection parameters - more resilient for high-latency environments
	// Uses infinite retries with exponential backoff capped at 2 minutes
	consecutiveFailures := 0
	baseDelay := 1 * time.Second
	maxDelay := 2 * time.Minute

	for {
		// Check if context is canceled before starting/reconnecting
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Get last known resourceVersion for resumable watch
		resourceVersion := getResourceVersion()

		watcher, err := a.k8sClient.WatchResource(ctx, resourceType, namespace, resourceVersion)
		if err != nil {
			consecutiveFailures++
			debug.LogWatcher("Failed to start resource watcher", map[string]interface{}{"type": resourceType, "error": err.Error(), "failures": consecutiveFailures})

			// If we have a stale resourceVersion, clear it and retry fresh
			if resourceVersion != "" && (strings.Contains(err.Error(), "too old") || strings.Contains(err.Error(), "expired")) {
				debug.LogWatcher("ResourceVersion too old, resetting", map[string]interface{}{"type": resourceType})
				clearResourceVersion()
			}

			// Emit error event (always recoverable with infinite retries)
			a.emitEvent("watcher-error", WatcherErrorEvent{
				ResourceType: resourceType,
				Namespace:    namespace,
				Error:        err.Error(),
				Recoverable:  true,
				Context:      a.GetCurrentContext(),
			})

			// Exponential backoff with jitter
			delay := baseDelay * time.Duration(1<<uint(min(consecutiveFailures, 7))) //nolint:gosec // capped at 7, safe for uint
			if delay > maxDelay {
				delay = maxDelay
			}

			a.emitEvent("watcher-status", WatcherStatusEvent{
				ResourceType: resourceType,
				Namespace:    namespace,
				Status:       "reconnecting",
				Context:      a.GetCurrentContext(),
			})

			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				continue
			}
		}

		// Successfully connected
		consecutiveFailures = 0
		a.emitEvent("watcher-status", WatcherStatusEvent{
			ResourceType: resourceType,
			Namespace:    namespace,
			Status:       "connected",
			Context:      a.GetCurrentContext(),
		})

		// Process events from this watcher
		watcherDone := false
		for !watcherDone {
			select {
			case <-ctx.Done():
				watcher.Stop()
				return
			case event, ok := <-watcher.ResultChan():
				if !ok {
					debug.LogWatcher("Resource watcher channel closed, will reconnect", map[string]interface{}{"type": resourceType})
					watcher.Stop()
					watcherDone = true
					// Add a small delay before reconnecting to avoid tight loops
					select {
					case <-ctx.Done():
						return
					case <-time.After(1 * time.Second):
					}
					break
				}

				// Handle ERROR events from the watch stream
				if event.Type == "ERROR" {
					debug.LogWatcher("Resource watcher received ERROR event", map[string]interface{}{"type": resourceType})
					// Check if it's a "too old" resourceVersion error
					if status, ok := event.Object.(*metav1.Status); ok {
						debug.LogWatcher("Watch ERROR status", map[string]interface{}{"reason": status.Reason, "message": status.Message})
						if status.Reason == metav1.StatusReasonExpired || status.Reason == metav1.StatusReasonGone {
							// Clear the stale resourceVersion
							clearResourceVersion()
						}
					}
					watcher.Stop()
					watcherDone = true
					consecutiveFailures++
					break
				}

				// Convert to unstructured map to extract resourceVersion
				resourceMap, err := k8s.RuntimeObjectToMap(event.Object)
				if err != nil {
					debug.LogWatcher("Failed to convert resource to map", map[string]interface{}{"error": err.Error()})
					continue
				}

				// Extract and store resourceVersion for resumable watches
				if metadata, ok := resourceMap["metadata"].(map[string]interface{}); ok {
					if rv, ok := metadata["resourceVersion"].(string); ok {
						setResourceVersion(rv)
					}
				}

				// Handle BOOKMARK events (just update resourceVersion, don't emit)
				if event.Type == "BOOKMARK" {
					continue
				}

				// Only emit ADDED, MODIFIED, DELETED events
				if event.Type == "ADDED" || event.Type == "MODIFIED" || event.Type == "DELETED" {
					// Guard: skip emit if context was cancelled (prevents stale
					// events leaking to a new context after StopAll)
					if ctx.Err() != nil {
						watcher.Stop()
						return
					}

					// Track event for performance metrics
					a.recordWatcherEvent(watcherKey, string(event.Type))

					// Extract namespace from resource metadata
					resourceNs := ""
					if metadata, ok := resourceMap["metadata"].(map[string]interface{}); ok {
						if ns, ok := metadata["namespace"].(string); ok {
							resourceNs = ns
						}
					}

					// Use event coalescer for batched emission (60fps frame batching)
					a.eventCoalescer.Emit(ResourceEvent{
						Type:         string(event.Type),
						ResourceType: resourceType,
						Namespace:    resourceNs,
						Resource:     resourceMap,
					})
				}
			}
		}
	}
}

// watchCRDLoop is the watch loop for Custom Resource Definitions using dynamic client.
// It includes reconnection logic with exponential backoff and resourceVersion tracking
// for resumable watches that avoid duplicate ADDED events on reconnection.
func (a *App) watchCRDLoop(ctx context.Context, group, version, resource, namespace string) {
	crdResourceType := fmt.Sprintf("crd:%s/%s/%s", group, version, resource)
	watcherKey := crdResourceType + ":" + namespace

	defer func() {
		if r := recover(); r != nil {
			crashlog.LogError("PANIC in watchCRDLoop: gvr=%s/%s/%s, namespace=%s, panic=%v\nStack: %s",
				group, version, resource, namespace, r, string(rtdebug.Stack()))
		}
		debug.LogWatcher("CRD watcher stopped", map[string]interface{}{"group": group, "version": version, "resource": resource, "namespace": namespace})
		a.emitEvent("watcher-status", WatcherStatusEvent{
			ResourceType: crdResourceType,
			Namespace:    namespace,
			Status:       "stopped",
			Context:      a.GetCurrentContext(),
		})
	}()

	if a.k8sClient == nil {
		debug.LogWatcher("watchCRDLoop: k8s client not initialized", nil)
		return
	}

	// Get watcher for resourceVersion tracking
	var rw *ResourceWatcher
	if a.watcherManager != nil {
		a.watcherManager.mutex.RLock()
		rw = a.watcherManager.watchers[watcherKey]
		a.watcherManager.mutex.RUnlock()
	}

	// Helper to get current resourceVersion
	getResourceVersion := func() string {
		if rw == nil {
			return ""
		}
		rw.mu.RLock()
		defer rw.mu.RUnlock()
		return rw.ResourceVersion
	}

	// Helper to update resourceVersion
	setResourceVersion := func(rv string) {
		if rw != nil && rv != "" {
			rw.mu.Lock()
			rw.ResourceVersion = rv
			rw.mu.Unlock()
		}
	}

	// Helper to clear resourceVersion (for expired/gone errors)
	clearResourceVersion := func() {
		if rw != nil {
			rw.mu.Lock()
			rw.ResourceVersion = ""
			rw.mu.Unlock()
		}
	}

	// Reconnection parameters - more resilient for high-latency environments
	// Uses infinite retries with exponential backoff capped at 2 minutes
	consecutiveFailures := 0
	baseDelay := 1 * time.Second
	maxDelay := 2 * time.Minute

	for {
		// Check if context is canceled before starting/reconnecting
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Get last known resourceVersion for resumable watch
		resourceVersion := getResourceVersion()

		watcher, err := a.k8sClient.WatchCRD(ctx, group, version, resource, namespace, resourceVersion)
		if err != nil {
			consecutiveFailures++
			debug.LogWatcher("Failed to start CRD watcher", map[string]interface{}{"group": group, "version": version, "resource": resource, "error": err.Error(), "failures": consecutiveFailures})

			// If we have a stale resourceVersion, clear it and retry fresh
			if resourceVersion != "" && (strings.Contains(err.Error(), "too old") || strings.Contains(err.Error(), "expired")) {
				debug.LogWatcher("CRD ResourceVersion too old, resetting", map[string]interface{}{"group": group, "version": version, "resource": resource})
				clearResourceVersion()
			}

			// Emit error event (always recoverable with infinite retries)
			a.emitEvent("watcher-error", WatcherErrorEvent{
				ResourceType: crdResourceType,
				Namespace:    namespace,
				Error:        err.Error(),
				Recoverable:  true,
				Context:      a.GetCurrentContext(),
			})

			// Exponential backoff
			delay := baseDelay * time.Duration(1<<uint(min(consecutiveFailures, 7))) //nolint:gosec // capped at 7, safe for uint
			if delay > maxDelay {
				delay = maxDelay
			}

			a.emitEvent("watcher-status", WatcherStatusEvent{
				ResourceType: crdResourceType,
				Namespace:    namespace,
				Status:       "reconnecting",
				Context:      a.GetCurrentContext(),
			})

			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				continue
			}
		}

		// Successfully connected
		consecutiveFailures = 0
		a.emitEvent("watcher-status", WatcherStatusEvent{
			ResourceType: crdResourceType,
			Namespace:    namespace,
			Status:       "connected",
			Context:      a.GetCurrentContext(),
		})

		// Process events from this watcher
		watcherDone := false
		for !watcherDone {
			select {
			case <-ctx.Done():
				watcher.Stop()
				return
			case event, ok := <-watcher.ResultChan():
				if !ok {
					debug.LogWatcher("CRD watcher channel closed, will reconnect", map[string]interface{}{"group": group, "version": version, "resource": resource})
					watcher.Stop()
					watcherDone = true
					// Add a small delay before reconnecting to avoid tight loops
					select {
					case <-ctx.Done():
						return
					case <-time.After(1 * time.Second):
					}
					break
				}

				// Handle ERROR events from the watch stream
				if event.Type == "ERROR" {
					debug.LogWatcher("CRD watcher received ERROR event", map[string]interface{}{"group": group, "version": version, "resource": resource})
					// Check if it's a "too old" resourceVersion error
					if status, ok := event.Object.(*metav1.Status); ok {
						debug.LogWatcher("CRD Watch ERROR status", map[string]interface{}{"reason": status.Reason, "message": status.Message})
						if status.Reason == metav1.StatusReasonExpired || status.Reason == metav1.StatusReasonGone {
							// Clear the stale resourceVersion
							clearResourceVersion()
						}
					}
					watcher.Stop()
					watcherDone = true
					consecutiveFailures++
					break
				}

				// Convert to unstructured map to extract resourceVersion
				resourceMap, err := k8s.RuntimeObjectToMap(event.Object)
				if err != nil {
					debug.LogWatcher("Failed to convert CRD to map", map[string]interface{}{"error": err.Error()})
					continue
				}

				// Extract and store resourceVersion for resumable watches
				if metadata, ok := resourceMap["metadata"].(map[string]interface{}); ok {
					if rv, ok := metadata["resourceVersion"].(string); ok {
						setResourceVersion(rv)
					}
				}

				// Handle BOOKMARK events (just update resourceVersion, don't emit)
				if event.Type == "BOOKMARK" {
					continue
				}

				// Only emit ADDED, MODIFIED, DELETED events
				if event.Type == "ADDED" || event.Type == "MODIFIED" || event.Type == "DELETED" {
					// Guard: skip emit if context was cancelled (prevents stale
					// events leaking to a new context after StopAll)
					if ctx.Err() != nil {
						watcher.Stop()
						return
					}

					// Track event for performance metrics
					a.recordWatcherEvent(watcherKey, string(event.Type))

					// Extract namespace from resource metadata
					resourceNs := ""
					if metadata, ok := resourceMap["metadata"].(map[string]interface{}); ok {
						if ns, ok := metadata["namespace"].(string); ok {
							resourceNs = ns
						}
					}

					// Use event coalescer for batched emission (60fps frame batching)
					a.eventCoalescer.Emit(ResourceEvent{
						Type:         string(event.Type),
						ResourceType: crdResourceType,
						Namespace:    resourceNs,
						Resource:     resourceMap,
					})
				}
			}
		}
	}
}

func (a *App) GetDeploymentYaml(namespace, name string) (string, error) {
	debug.LogWatcher("GetDeploymentYaml called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetDeploymentYaml(namespace, name)
}

func (a *App) UpdateDeploymentYaml(namespace, name, yamlContent string) error {
	debug.LogWatcher("UpdateDeploymentYaml called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateDeploymentYaml(namespace, name, yamlContent)
}

func (a *App) DeleteDeployment(namespace, name string) error {
	contextName := a.GetCurrentContext()
	debug.LogWatcher("DeleteDeployment called", map[string]interface{}{"context": contextName, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeleteDeployment(contextName, namespace, name)
	if err != nil {
		debug.LogWatcher("DeleteDeployment error", map[string]interface{}{"error": err.Error()})
	} else {
		debug.LogWatcher("DeleteDeployment success", nil)
	}
	return err
}

func (a *App) RestartDeployment(namespace, name string) error {
	contextName := a.GetCurrentContext()
	debug.LogWatcher("RestartDeployment called", map[string]interface{}{"context": contextName, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.RestartDeployment(contextName, namespace, name)
	if err != nil {
		debug.LogWatcher("RestartDeployment error", map[string]interface{}{"error": err.Error()})
	} else {
		debug.LogWatcher("RestartDeployment success", nil)
	}
	return err
}
