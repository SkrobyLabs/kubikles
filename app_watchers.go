package main

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"kubikles/pkg/crashlog"

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
		a.logDebug("SubscribeResourceWatcher: watcher manager not initialized")
		return ""
	}
	return a.watcherManager.Subscribe(resourceType, namespace)
}

// SubscribeCRDWatcher subscribes to a CRD watcher using GVR, returning the watcher key
func (a *App) SubscribeCRDWatcher(group, version, resource, namespace string) string {
	if a.watcherManager == nil {
		a.logDebug("SubscribeCRDWatcher: watcher manager not initialized")
		return ""
	}
	return a.watcherManager.SubscribeCRD(group, version, resource, namespace)
}

// UnsubscribeWatcher unsubscribes from a watcher by key
func (a *App) UnsubscribeWatcher(watcherKey string) {
	if a.watcherManager == nil {
		a.logDebug("UnsubscribeWatcher: watcher manager not initialized")
		return
	}
	a.watcherManager.Unsubscribe(watcherKey)
}

// StopAllWatchers stops all active watchers (called on context switch)
func (a *App) StopAllWatchers() {
	if a.watcherManager == nil {
		a.logDebug("StopAllWatchers: watcher manager not initialized")
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
				resourceType, namespace, r, string(debug.Stack()))
		}
		a.logDebug("Resource watcher stopped: type=%s, namespace=%s", resourceType, namespace)
		a.emitEvent("watcher-status", WatcherStatusEvent{
			ResourceType: resourceType,
			Namespace:    namespace,
			Status:       "stopped",
			Context:      a.GetCurrentContext(),
		})
	}()

	if a.k8sClient == nil {
		a.logDebug("watchResourceLoop: k8s client not initialized")
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
			a.logDebug("Failed to start resource watcher: type=%s, err=%v, failures=%d", resourceType, err, consecutiveFailures)

			// If we have a stale resourceVersion, clear it and retry fresh
			if resourceVersion != "" && (strings.Contains(err.Error(), "too old") || strings.Contains(err.Error(), "expired")) {
				a.logDebug("ResourceVersion too old, resetting: type=%s", resourceType)
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
					a.logDebug("Resource watcher channel closed: type=%s, will reconnect", resourceType)
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
					a.logDebug("Resource watcher received ERROR event: type=%s", resourceType)
					// Check if it's a "too old" resourceVersion error
					if status, ok := event.Object.(*metav1.Status); ok {
						a.logDebug("Watch ERROR status: %s - %s", status.Reason, status.Message)
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
					a.logDebug("Failed to convert resource to map: %v", err)
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
				group, version, resource, namespace, r, string(debug.Stack()))
		}
		a.logDebug("CRD watcher stopped: gvr=%s/%s/%s, namespace=%s", group, version, resource, namespace)
		a.emitEvent("watcher-status", WatcherStatusEvent{
			ResourceType: crdResourceType,
			Namespace:    namespace,
			Status:       "stopped",
			Context:      a.GetCurrentContext(),
		})
	}()

	if a.k8sClient == nil {
		a.logDebug("watchCRDLoop: k8s client not initialized")
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
			a.logDebug("Failed to start CRD watcher: gvr=%s/%s/%s, err=%v, failures=%d", group, version, resource, err, consecutiveFailures)

			// If we have a stale resourceVersion, clear it and retry fresh
			if resourceVersion != "" && (strings.Contains(err.Error(), "too old") || strings.Contains(err.Error(), "expired")) {
				a.logDebug("CRD ResourceVersion too old, resetting: gvr=%s/%s/%s", group, version, resource)
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
					a.logDebug("CRD watcher channel closed: gvr=%s/%s/%s, will reconnect", group, version, resource)
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
					a.logDebug("CRD watcher received ERROR event: gvr=%s/%s/%s", group, version, resource)
					// Check if it's a "too old" resourceVersion error
					if status, ok := event.Object.(*metav1.Status); ok {
						a.logDebug("CRD Watch ERROR status: %s - %s", status.Reason, status.Message)
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
					a.logDebug("Failed to convert CRD to map: %v", err)
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
	a.logDebug("GetDeploymentYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetDeploymentYaml(namespace, name)
}

func (a *App) UpdateDeploymentYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateDeploymentYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateDeploymentYaml(namespace, name, yamlContent)
}

func (a *App) DeleteDeployment(namespace, name string) error {
	contextName := a.GetCurrentContext()
	a.logDebug("DeleteDeployment called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeleteDeployment(contextName, namespace, name)
	if err != nil {
		a.logDebug("DeleteDeployment error: %v", err)
	} else {
		a.logDebug("DeleteDeployment success")
	}
	return err
}

func (a *App) RestartDeployment(namespace, name string) error {
	contextName := a.GetCurrentContext()
	a.logDebug("RestartDeployment called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.RestartDeployment(contextName, namespace, name)
	if err != nil {
		a.logDebug("RestartDeployment error: %v", err)
	} else {
		a.logDebug("RestartDeployment success")
	}
	return err
}
