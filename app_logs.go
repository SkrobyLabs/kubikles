package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// Pod Logs
// =============================================================================

func (a *App) GetPodLogs(namespace, podName, containerName string, timestamps bool, previous bool, sinceTime string) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetPodLogs called", map[string]interface{}{"context": currentContext, "ns": namespace, "pod": podName, "container": containerName, "timestamps": timestamps, "previous": previous, "sinceTime": sinceTime})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodLogs(namespace, podName, containerName, timestamps, previous, sinceTime)
}

func (a *App) GetAllPodLogs(namespace, podName, containerName string, timestamps bool, previous bool) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllPodLogs called", map[string]interface{}{"context": currentContext, "ns": namespace, "pod": podName, "container": containerName, "timestamps": timestamps, "previous": previous})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetAllPodLogs(namespace, podName, containerName, timestamps, previous)
}

func (a *App) GetPodLogsFromStart(namespace, podName, containerName string, timestamps bool, previous bool) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetPodLogsFromStart called", map[string]interface{}{"context": currentContext, "ns": namespace, "pod": podName, "container": containerName, "timestamps": timestamps, "previous": previous})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodLogsFromStart(namespace, podName, containerName, timestamps, previous, 200)
}

// LogChunkResult represents the result of a chunked log fetch
type LogChunkResult struct {
	Logs    string `json:"logs"`
	HasMore bool   `json:"hasMore"`
}

func (a *App) GetPodLogsBefore(namespace, podName, containerName string, timestamps bool, previous bool, beforeTime string, limit int) (*LogChunkResult, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetPodLogsBefore called", map[string]interface{}{"context": currentContext, "ns": namespace, "pod": podName, "container": containerName, "beforeTime": beforeTime, "limit": limit})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	logs, hasMore, err := a.k8sClient.GetPodLogsBefore(namespace, podName, containerName, timestamps, previous, beforeTime, limit)
	if err != nil {
		return nil, err
	}
	return &LogChunkResult{Logs: logs, HasMore: hasMore}, nil
}

func (a *App) GetPodLogsAfter(namespace, podName, containerName string, timestamps bool, previous bool, afterTime string, limit int) (*LogChunkResult, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetPodLogsAfter called", map[string]interface{}{"context": currentContext, "ns": namespace, "pod": podName, "container": containerName, "afterTime": afterTime, "limit": limit})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	logs, hasMore, err := a.k8sClient.GetPodLogsAfter(namespace, podName, containerName, timestamps, previous, afterTime, limit)
	if err != nil {
		return nil, err
	}
	return &LogChunkResult{Logs: logs, HasMore: hasMore}, nil
}

// GetAllContainersLogs fetches logs from all containers in a pod, merged by timestamp
func (a *App) GetAllContainersLogs(namespace, podName string, containerNames []string, timestamps bool, previous bool, sinceTime string) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllContainersLogs called", map[string]interface{}{"context": currentContext, "ns": namespace, "pod": podName, "containers": containerNames, "timestamps": timestamps, "previous": previous, "sinceTime": sinceTime})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetAllContainersLogs(namespace, podName, containerNames, timestamps, previous, sinceTime)
}

// GetAllContainersLogsAll fetches all logs from all containers, merged by timestamp
func (a *App) GetAllContainersLogsAll(namespace, podName string, containerNames []string, timestamps bool, previous bool) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllContainersLogsAll called", map[string]interface{}{"context": currentContext, "ns": namespace, "pod": podName, "containers": containerNames, "timestamps": timestamps, "previous": previous})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetAllContainersLogsAll(namespace, podName, containerNames, timestamps, previous)
}

// GetAllContainersLogsFromStart fetches the first N lines from all containers, merged by timestamp
func (a *App) GetAllContainersLogsFromStart(namespace, podName string, containerNames []string, timestamps bool, previous bool) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllContainersLogsFromStart called", map[string]interface{}{"context": currentContext, "ns": namespace, "pod": podName, "containers": containerNames, "timestamps": timestamps, "previous": previous})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetAllContainersLogsFromStart(namespace, podName, containerNames, timestamps, previous, 200)
}

// GetAllContainersLogsBefore fetches logs before a given timestamp from all containers
func (a *App) GetAllContainersLogsBefore(namespace, podName string, containerNames []string, timestamps bool, previous bool, beforeTime string, limit int) (*LogChunkResult, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllContainersLogsBefore called", map[string]interface{}{"context": currentContext, "ns": namespace, "pod": podName, "containers": containerNames, "beforeTime": beforeTime, "limit": limit})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	logs, hasMore, err := a.k8sClient.GetAllContainersLogsBefore(namespace, podName, containerNames, timestamps, previous, beforeTime, limit)
	if err != nil {
		return nil, err
	}
	return &LogChunkResult{Logs: logs, HasMore: hasMore}, nil
}

// GetAllContainersLogsAfter fetches logs after a given timestamp from all containers
func (a *App) GetAllContainersLogsAfter(namespace, podName string, containerNames []string, timestamps bool, previous bool, afterTime string, limit int) (*LogChunkResult, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllContainersLogsAfter called", map[string]interface{}{"context": currentContext, "ns": namespace, "pod": podName, "containers": containerNames, "afterTime": afterTime, "limit": limit})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	logs, hasMore, err := a.k8sClient.GetAllContainersLogsAfter(namespace, podName, containerNames, timestamps, previous, afterTime, limit)
	if err != nil {
		return nil, err
	}
	return &LogChunkResult{Logs: logs, HasMore: hasMore}, nil
}

// StartAllContainersLogStream starts streaming logs from all containers in a pod.
// Returns a stream ID that can be used to stop the stream.
func (a *App) StartAllContainersLogStream(namespace, podName string, containerNames []string, timestamps bool) (string, error) {
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}

	// Generate a unique stream ID
	var sb strings.Builder
	sb.WriteString(namespace)
	sb.WriteByte('/')
	sb.WriteString(podName)
	sb.WriteString("/__ALL__-")
	sb.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10))
	streamID := sb.String()
	debug.LogK8s("StartAllContainersLogStream", map[string]interface{}{"streamID": streamID, "containers": containerNames})

	// Create a cancellable context for this stream
	ctx, cancel := context.WithCancel(context.Background())

	// Store the cancel function
	a.logStreamsMutex.Lock()
	a.logStreams[streamID] = cancel
	a.logStreamsMutex.Unlock()

	// Start streaming in a goroutine
	go func() {
		defer func() {
			// Clean up when done
			a.logStreamsMutex.Lock()
			delete(a.logStreams, streamID)
			a.logStreamsMutex.Unlock()

			// Emit done event
			a.logCoalescer.EmitDone(streamID)
		}()

		err := a.k8sClient.StreamAllContainersLogs(ctx, namespace, podName, containerNames, timestamps, 200, func(line string) {
			a.logCoalescer.EmitLine(streamID, line)
		})

		if err != nil && err != context.Canceled {
			debug.LogK8s("All containers log stream error", map[string]interface{}{"error": err.Error()})
			a.logCoalescer.EmitError(streamID, err.Error())
		}
	}()

	return streamID, nil
}

// PodContainerPair is passed from frontend for multi-pod log fetching
type PodContainerPair struct {
	PodName        string   `json:"podName"`
	ContainerNames []string `json:"containerNames"`
}

// GetAllPodsLogs fetches logs from multiple pods, merged by timestamp
func (a *App) GetAllPodsLogs(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, sinceTime string) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllPodsLogs called", map[string]interface{}{"context": currentContext, "ns": namespace, "pods": len(pods), "allContainers": allContainers, "timestamps": timestamps, "previous": previous})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	// Convert to k8s.PodContainerPair
	k8sPods := make([]k8s.PodContainerPair, len(pods))
	for i, p := range pods {
		k8sPods[i] = k8s.PodContainerPair{PodName: p.PodName, ContainerNames: p.ContainerNames}
	}
	return a.k8sClient.GetAllPodsLogs(namespace, k8sPods, allContainers, timestamps, previous, sinceTime)
}

// GetAllPodsLogsAll fetches all logs from multiple pods, merged by timestamp
func (a *App) GetAllPodsLogsAll(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllPodsLogsAll called", map[string]interface{}{"context": currentContext, "ns": namespace, "pods": len(pods), "allContainers": allContainers})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	k8sPods := make([]k8s.PodContainerPair, len(pods))
	for i, p := range pods {
		k8sPods[i] = k8s.PodContainerPair{PodName: p.PodName, ContainerNames: p.ContainerNames}
	}
	return a.k8sClient.GetAllPodsLogsAll(namespace, k8sPods, allContainers, timestamps, previous)
}

// GetAllPodsLogsFromStart fetches the first N lines from multiple pods, merged by timestamp
func (a *App) GetAllPodsLogsFromStart(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllPodsLogsFromStart called", map[string]interface{}{"context": currentContext, "ns": namespace, "pods": len(pods), "allContainers": allContainers})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	k8sPods := make([]k8s.PodContainerPair, len(pods))
	for i, p := range pods {
		k8sPods[i] = k8s.PodContainerPair{PodName: p.PodName, ContainerNames: p.ContainerNames}
	}
	return a.k8sClient.GetAllPodsLogsFromStart(namespace, k8sPods, allContainers, timestamps, previous, 200)
}

// GetAllPodsLogsBefore fetches logs before a given timestamp from multiple pods
func (a *App) GetAllPodsLogsBefore(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, beforeTime string, limit int) (*LogChunkResult, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllPodsLogsBefore called", map[string]interface{}{"context": currentContext, "ns": namespace, "pods": len(pods), "allContainers": allContainers, "beforeTime": beforeTime})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	k8sPods := make([]k8s.PodContainerPair, len(pods))
	for i, p := range pods {
		k8sPods[i] = k8s.PodContainerPair{PodName: p.PodName, ContainerNames: p.ContainerNames}
	}
	logs, hasMore, err := a.k8sClient.GetAllPodsLogsBefore(namespace, k8sPods, allContainers, timestamps, previous, beforeTime, limit)
	if err != nil {
		return nil, err
	}
	return &LogChunkResult{Logs: logs, HasMore: hasMore}, nil
}

// GetAllPodsLogsAfter fetches logs after a given timestamp from multiple pods
func (a *App) GetAllPodsLogsAfter(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, afterTime string, limit int) (*LogChunkResult, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetAllPodsLogsAfter called", map[string]interface{}{"context": currentContext, "ns": namespace, "pods": len(pods), "allContainers": allContainers, "afterTime": afterTime})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	k8sPods := make([]k8s.PodContainerPair, len(pods))
	for i, p := range pods {
		k8sPods[i] = k8s.PodContainerPair{PodName: p.PodName, ContainerNames: p.ContainerNames}
	}
	logs, hasMore, err := a.k8sClient.GetAllPodsLogsAfter(namespace, k8sPods, allContainers, timestamps, previous, afterTime, limit)
	if err != nil {
		return nil, err
	}
	return &LogChunkResult{Logs: logs, HasMore: hasMore}, nil
}

// StartAllPodsLogStream starts streaming logs from multiple pods, merged by timestamp
func (a *App) StartAllPodsLogStream(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool) (string, error) {
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}

	// Generate a unique stream ID
	var sb strings.Builder
	sb.WriteString(namespace)
	sb.WriteString("/__ALL_PODS__-")
	sb.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10))
	streamID := sb.String()
	debug.LogK8s("StartAllPodsLogStream", map[string]interface{}{"streamID": streamID, "pods": len(pods), "allContainers": allContainers})

	k8sPods := make([]k8s.PodContainerPair, len(pods))
	for i, p := range pods {
		k8sPods[i] = k8s.PodContainerPair{PodName: p.PodName, ContainerNames: p.ContainerNames}
	}

	ctx, cancel := context.WithCancel(context.Background())

	a.logStreamsMutex.Lock()
	a.logStreams[streamID] = cancel
	a.logStreamsMutex.Unlock()

	go func() {
		defer func() {
			a.logStreamsMutex.Lock()
			delete(a.logStreams, streamID)
			a.logStreamsMutex.Unlock()
			a.logCoalescer.EmitDone(streamID)
		}()

		err := a.k8sClient.StreamAllPodsLogs(ctx, namespace, k8sPods, allContainers, timestamps, 200, func(line string) {
			a.logCoalescer.EmitLine(streamID, line)
		})

		if err != nil && err != context.Canceled {
			debug.LogK8s("All pods log stream error", map[string]interface{}{"error": err.Error()})
			a.logCoalescer.EmitError(streamID, err.Error())
		}
	}()

	return streamID, nil
}

// LogStreamEvent is emitted for each log line during streaming
type LogStreamEvent struct {
	StreamID string `json:"streamId"`
	Line     string `json:"line"`
	Error    string `json:"error,omitempty"`
	Done     bool   `json:"done,omitempty"`
}

// StartLogStream starts streaming logs from a pod container.
// Returns a stream ID that can be used to stop the stream.
// Log lines are emitted via "log-stream" events.
func (a *App) StartLogStream(namespace, podName, containerName string, timestamps bool) (string, error) {
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}

	// Generate a unique stream ID (avoid fmt.Sprintf for reduced allocations)
	var sb strings.Builder
	sb.WriteString(namespace)
	sb.WriteByte('/')
	sb.WriteString(podName)
	sb.WriteByte('/')
	sb.WriteString(containerName)
	sb.WriteByte('-')
	sb.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10))
	streamID := sb.String()
	debug.LogK8s("StartLogStream", map[string]interface{}{"streamID": streamID})

	// Create a cancellable context for this stream
	ctx, cancel := context.WithCancel(context.Background())

	// Store the cancel function
	a.logStreamsMutex.Lock()
	a.logStreams[streamID] = cancel
	a.logStreamsMutex.Unlock()

	// Start streaming in a goroutine
	go func() {
		defer func() {
			// Clean up when done
			a.logStreamsMutex.Lock()
			delete(a.logStreams, streamID)
			a.logStreamsMutex.Unlock()

			// Emit done event (flushes any pending lines first)
			a.logCoalescer.EmitDone(streamID)
		}()

		err := a.k8sClient.StreamPodLogs(ctx, namespace, podName, containerName, timestamps, 200, func(line string) {
			// Use log coalescer for 60fps batching - crucial for busy pods
			a.logCoalescer.EmitLine(streamID, line)
		})

		if err != nil && err != context.Canceled {
			debug.LogK8s("Log stream error", map[string]interface{}{"error": err.Error()})
			a.logCoalescer.EmitError(streamID, err.Error())
		}
	}()

	return streamID, nil
}

// StopLogStream stops an active log stream
func (a *App) StopLogStream(streamID string) {
	debug.LogK8s("StopLogStream", map[string]interface{}{"streamID": streamID})
	a.logStreamsMutex.Lock()
	defer a.logStreamsMutex.Unlock()

	if cancel, ok := a.logStreams[streamID]; ok {
		cancel()
		delete(a.logStreams, streamID)
	}
}
