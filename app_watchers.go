package main

import (
	"context"
	"encoding/json"
	"fmt"
	rtdebug "runtime/debug"
	"time"

	"kubikles/pkg/crashlog"
	"kubikles/pkg/debug"
	"kubikles/pkg/watchevents"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"kubikles/pkg/k8s"
)

// =============================================================================
// Resource Watchers
// =============================================================================

// ResourceEvent is the generic event emitted for any resource type change.
// Resource is the object's JSON, marshalled once where the event is created;
// the bridge then only copies the bytes instead of re-walking a map.
type ResourceEvent struct {
	Type         string          `json:"type"`         // ADDED, MODIFIED, DELETED
	ResourceType string          `json:"resourceType"` // pods, namespaces, deployments, etc.
	Namespace    string          `json:"namespace"`    // Resource's namespace (empty for cluster-scoped)
	Resource     json.RawMessage `json:"resource"`     // Resource JSON (managedFields removed)
	Name         string          `json:"-"`            // metadata.name, used for coalescing keys only
}

// WatcherErrorEvent is emitted when a watcher encounters an error. The
// payload types live in pkg/watchevents so the shared store emits the same
// shape.
type WatcherErrorEvent = watchevents.Error

// WatcherStatusEvent is emitted when watcher status changes.
type WatcherStatusEvent = watchevents.Status

func prematureWatchError(resourceType, namespace, context string, receivedAny bool, openDuration time.Duration) WatcherErrorEvent {
	return WatcherErrorEvent{
		ResourceType: resourceType, Namespace: namespace,
		Error: "watch stream closed unexpectedly", Recoverable: true,
		Premature: true, ReceivedAny: receivedAny, OpenDurationMillis: openDuration.Milliseconds(), Context: context,
	}
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

// SubscribeResourceWatcherFrom is SubscribeResourceWatcher for a caller that
// has just listed the same resource type and namespace: resourceVersion is that
// list's version (see GetListOutcome), so a new watcher streams only what
// changed after the list instead of replaying every object. An empty or
// unusable version behaves exactly like SubscribeResourceWatcher.
func (a *App) SubscribeResourceWatcherFrom(resourceType, namespace, resourceVersion string) string {
	if a.watcherManager == nil {
		debug.LogWatcher("SubscribeResourceWatcherFrom: watcher manager not initialized", nil)
		return ""
	}
	return a.watcherManager.SubscribeFrom(resourceType, namespace, resourceVersion)
}

// SubscribeCRDWatcherFrom is SubscribeCRDWatcher with a list resourceVersion
// anchor; see SubscribeResourceWatcherFrom.
func (a *App) SubscribeCRDWatcherFrom(group, version, resource, namespace, resourceVersion string) string {
	if a.watcherManager == nil {
		debug.LogWatcher("SubscribeCRDWatcherFrom: watcher manager not initialized", nil)
		return ""
	}
	return a.watcherManager.SubscribeCRDFrom(group, version, resource, namespace, resourceVersion)
}

// UnsubscribeWatcher unsubscribes from a watcher by key
func (a *App) UnsubscribeWatcher(watcherKey string) {
	if a.watcherManager == nil {
		debug.LogWatcher("UnsubscribeWatcher: watcher manager not initialized", nil)
		return
	}
	a.watcherManager.Unsubscribe(watcherKey)
}

// StopAllWatchers stops the legacy watchers only. The frontend calls it on every switch to
// polling mode, where open views and store shards must stay alive; SwitchContext resets the
// store itself.
func (a *App) StopAllWatchers() {
	if a.watcherManager == nil {
		debug.LogWatcher("StopAllWatchers: watcher manager not initialized", nil)
		return
	}
	a.watcherManager.StopAll()
}

// watchSpec describes one watch loop. Standard resources and custom resources
// differ only in how the watch is opened and how they are named in events,
// logs and crash reports, so both share runWatchLoop.
type watchSpec struct {
	resourceType string // reported to the frontend in events and status updates
	namespace    string
	watcherKey   string
	label        string                 // "Resource" or "CRD", for log text
	panicDesc    string                 // identifies the watcher in crash logs
	logFields    map[string]interface{} // base fields for debug logging
	open         func(ctx context.Context, resourceVersion string) (watch.Interface, error)
}

// watchDebug logs a watcher debug message. The field map is only built when
// debug logging is on, so quiet sessions pay nothing on reconnect paths.
func (s *watchSpec) watchDebug(msg string, extra map[string]interface{}) {
	if !debug.IsEnabled() {
		return
	}
	fields := make(map[string]interface{}, len(s.logFields)+len(extra))
	for k, v := range s.logFields {
		fields[k] = v
	}
	for k, v := range extra {
		fields[k] = v
	}
	debug.LogWatcher(msg, fields)
}

// watchResourceLoop is the generic watch loop for standard Kubernetes resources.
// It includes reconnection logic with exponential backoff and resourceVersion tracking
// for resumable watches that avoid duplicate ADDED events on reconnection.
func (a *App) watchResourceLoop(ctx context.Context, resourceType, namespace string) {
	a.runWatchLoop(ctx, watchSpec{
		resourceType: resourceType,
		namespace:    namespace,
		watcherKey:   resourceType + ":" + namespace,
		label:        "Resource",
		panicDesc:    fmt.Sprintf("watchResourceLoop: type=%s, namespace=%s", resourceType, namespace),
		logFields:    map[string]interface{}{"type": resourceType, "namespace": namespace},
		open: func(ctx context.Context, rv string) (watch.Interface, error) {
			return a.k8sClient.WatchResource(ctx, resourceType, namespace, rv)
		},
	})
}

// watchCRDLoop is the watch loop for Custom Resource Definitions using dynamic client.
// It includes reconnection logic with exponential backoff and resourceVersion tracking
// for resumable watches that avoid duplicate ADDED events on reconnection.
func (a *App) watchCRDLoop(ctx context.Context, group, version, resource, namespace string) {
	crdResourceType := fmt.Sprintf("crd:%s/%s/%s", group, version, resource)
	a.runWatchLoop(ctx, watchSpec{
		resourceType: crdResourceType,
		namespace:    namespace,
		watcherKey:   crdResourceType + ":" + namespace,
		label:        "CRD",
		panicDesc:    fmt.Sprintf("watchCRDLoop: gvr=%s/%s/%s, namespace=%s", group, version, resource, namespace),
		logFields:    map[string]interface{}{"group": group, "version": version, "resource": resource, "namespace": namespace},
		open: func(ctx context.Context, rv string) (watch.Interface, error) {
			return a.k8sClient.WatchCRD(ctx, group, version, resource, namespace, rv)
		},
	})
}

func (a *App) runWatchLoop(ctx context.Context, spec watchSpec) {
	resourceType, namespace, watcherKey := spec.resourceType, spec.namespace, spec.watcherKey

	defer func() {
		if r := recover(); r != nil {
			crashlog.LogError("PANIC in %s, panic=%v\nStack: %s", spec.panicDesc, r, string(rtdebug.Stack()))
		}
		spec.watchDebug(spec.label+" watcher stopped", nil)
		a.emitEvent("watcher-status", WatcherStatusEvent{
			ResourceType: resourceType,
			Namespace:    namespace,
			Status:       "stopped",
			Context:      a.GetCurrentContext(),
		})
	}()

	if a.k8sClient == nil {
		spec.watchDebug("watch loop: k8s client not initialized", nil)
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

	// Resolved once so the per-event accounting is lock free.
	stats := a.watcherStatsFor(watcherKey)
	// Only consulted when the legacyEventDedupe setting is on.
	deduper := newEventDeduper()

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

		watcher, err := spec.open(ctx, resourceVersion)
		if err != nil {
			consecutiveFailures++
			spec.watchDebug("Failed to start "+spec.label+" watcher", map[string]interface{}{"error": err.Error(), "failures": consecutiveFailures})

			// A stale or rejected resourceVersion (an expired anchor from a list,
			// or one left over from a long disconnect) is dropped and the watch
			// reopened at once without a version, which replays current state.
			// This is not a connectivity failure, so no error is surfaced and no
			// backoff applies.
			if resourceVersion != "" && isStaleResourceVersionError(err) {
				spec.watchDebug(spec.label+" ResourceVersion too old, resetting", nil)
				clearResourceVersion()
				deduper.reset()
				consecutiveFailures--
				// The frontend re-lists on the reconnecting -> connected transition, which is
				// what removes objects deleted during the gap.
				a.emitEvent("watcher-status", WatcherStatusEvent{ResourceType: resourceType, Namespace: namespace, Status: "reconnecting", Context: a.GetCurrentContext()})
				continue
			}

			// Emit error event (always recoverable with infinite retries)
			a.emitEvent("watcher-error", WatcherErrorEvent{
				ResourceType: resourceType,
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

		// HTTP 200 only proves the stream opened. Some access proxies immediately
		// close the body, so reset failures only after receiving an event.
		a.emitEvent("watcher-status", WatcherStatusEvent{
			ResourceType: resourceType,
			Namespace:    namespace,
			Status:       "connected",
			Context:      a.GetCurrentContext(),
		})

		// Process events from this watcher
		watcherDone := false
		receivedAny := false
		openedAt := time.Now()
		for !watcherDone {
			select {
			case <-ctx.Done():
				watcher.Stop()
				return
			case event, ok := <-watcher.ResultChan():
				if !ok {
					spec.watchDebug(spec.label+" watcher channel closed, will reconnect", nil)
					watcher.Stop()
					watcherDone = true
					openDuration := time.Since(openedAt)
					immediateClosure := !receivedAny && openDuration < 5*time.Second
					if immediateClosure {
						consecutiveFailures++
						a.emitEvent("watcher-error", prematureWatchError(resourceType, namespace, a.GetCurrentContext(), false, openDuration))
					} else {
						consecutiveFailures = 0
					}
					a.emitEvent("watcher-status", WatcherStatusEvent{ResourceType: resourceType, Namespace: namespace, Status: "reconnecting", Context: a.GetCurrentContext()})
					delay := baseDelay
					if immediateClosure {
						delay = baseDelay * time.Duration(1<<uint(min(consecutiveFailures, 7))) //nolint:gosec
					}
					if delay > maxDelay {
						delay = maxDelay
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(delay):
					}
					break
				}
				receivedAny = true
				consecutiveFailures = 0

				// Handle ERROR events from the watch stream
				if event.Type == watch.Error {
					spec.watchDebug(spec.label+" watcher received ERROR event", nil)
					// Check if it's a "too old" resourceVersion error
					if status, ok := event.Object.(*metav1.Status); ok {
						spec.watchDebug("Watch ERROR status", map[string]interface{}{"reason": status.Reason, "message": status.Message})
						if status.Reason == metav1.StatusReasonExpired || status.Reason == metav1.StatusReasonGone {
							// Clear the stale resourceVersion. The replay that follows needs the
							// reconnecting status so the frontend re-lists and drops objects
							// deleted while the anchor was lost.
							clearResourceVersion()
							a.emitEvent("watcher-status", WatcherStatusEvent{ResourceType: resourceType, Namespace: namespace, Status: "reconnecting", Context: a.GetCurrentContext()})
						}
					}
					watcher.Stop()
					watcherDone = true
					consecutiveFailures++
					break
				}

				// Bookmarks and unknown event types only advance the stored
				// resourceVersion, so skip serializing them.
				if event.Type != watch.Added && event.Type != watch.Modified && event.Type != watch.Deleted {
					if info, err := k8s.WatchObjectInfo(event.Object); err == nil {
						setResourceVersion(info.ResourceVersion)
					}
					continue
				}

				// Serialize once; identity comes from meta.Accessor, not a map.
				info, raw, err := k8s.EncodeWatchObject(event.Object)
				if err != nil {
					spec.watchDebug("Failed to encode "+spec.label+" object", map[string]interface{}{"error": err.Error()})
					continue
				}

				// Store resourceVersion for resumable watches
				setResourceVersion(info.ResourceVersion)

				// Opt-in: a MODIFIED event that changes nothing but resourceVersion is dropped.
				// The resourceVersion above is still tracked, so the watch resumes correctly.
				if legacyEventDedupe.Load() {
					if !deduper.shouldEmit(string(event.Type), info.Namespace, info.Name, info.ResourceVersion, raw) {
						continue
					}
				} else if len(deduper.last) > 0 {
					deduper.reset()
				}

				// Guard: skip emit if context was cancelled (prevents stale
				// events leaking to a new context after StopAll)
				if ctx.Err() != nil {
					watcher.Stop()
					return
				}

				// Track event for performance metrics
				a.recordEvent(stats, string(event.Type))

				// Use event coalescer for batched emission (60fps frame batching)
				a.eventCoalescer.Emit(ResourceEvent{
					Type:         string(event.Type),
					ResourceType: resourceType,
					Namespace:    info.Namespace,
					Name:         info.Name,
					Resource:     raw,
				})
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

// isStaleResourceVersionError reports whether a watch open failed because the
// requested resourceVersion is no longer (or never was) usable. The rule is
// shared with the store reflector.
func isStaleResourceVersionError(err error) bool {
	return k8s.IsStaleResourceVersionError(err)
}
