package k8s

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// FlowTimelineEntry represents a single entry in the flow timeline
type FlowTimelineEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	EntryType   string    `json:"entryType"`   // "event", "log", "change"
	Severity    string    `json:"severity"`    // "info", "warning", "error"
	ResourceRef string    `json:"resourceRef"` // "Pod/api-abc"
	Kind        string    `json:"kind"`        // "Pod", "Service", etc.
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`
	Message     string    `json:"message"`
	Details     string    `json:"details,omitempty"`
}

// FlowTimelineRequest specifies what timeline to fetch
type FlowTimelineRequest struct {
	ResourceType    string `json:"resourceType"`
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	DurationMinutes int    `json:"durationMinutes"`
	MaxEntries      int    `json:"maxEntries"`
	IncludeLogs     bool   `json:"includeLogs"`
}

// resourceRef identifies a resource for event fetching
type resourceRef struct {
	Kind      string
	Name      string
	Namespace string
}

// Error patterns to detect in logs
var errorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\berror\b`),
	regexp.MustCompile(`(?i)\bfailed\b`),
	regexp.MustCompile(`(?i)\bfailure\b`),
	regexp.MustCompile(`(?i)\bexception\b`),
	regexp.MustCompile(`(?i)\bpanic\b`),
	regexp.MustCompile(`(?i)\bfatal\b`),
	regexp.MustCompile(`(?i)\btimeout\b`),
	regexp.MustCompile(`(?i)\brefused\b`),
	regexp.MustCompile(`(?i)\bunauthorized\b`),
	regexp.MustCompile(`(?i)\bdenied\b`),
}

// GetFlowTimeline fetches a unified timeline of events and logs for a resource and its dependencies
func (c *Client) GetFlowTimeline(req FlowTimelineRequest) ([]FlowTimelineEntry, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset: %w", err)
	}

	// Set defaults
	if req.MaxEntries == 0 {
		req.MaxEntries = 200
	}
	if req.DurationMinutes == 0 {
		req.DurationMinutes = 10
	}

	duration := time.Duration(req.DurationMinutes) * time.Minute
	cutoff := time.Now().Add(-duration)
	ctx := context.Background()

	// 1. Get dependency graph to find related resources (empty context = current)
	graph, err := c.GetResourceDependencies("", req.ResourceType, req.Namespace, req.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get dependencies: %w", err)
	}

	// 2. Extract unique resource references from graph
	refs := extractResourceRefs(graph)

	// Add the root resource itself
	refs = append(refs, resourceRef{
		Kind:      capitalizeKind(req.ResourceType),
		Name:      req.Name,
		Namespace: req.Namespace,
	})

	// Deduplicate refs
	refs = deduplicateRefs(refs)

	// 3. Fetch events and logs in parallel
	var entries []FlowTimelineEntry
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Fetch events for all resources
	for _, ref := range refs {
		wg.Add(1)
		go func(r resourceRef) {
			defer wg.Done()
			events := c.getEventsForResource(ctx, cs, r, cutoff)
			mu.Lock()
			entries = append(entries, events...)
			mu.Unlock()
		}(ref)
	}

	// Fetch error logs for pods if requested
	if req.IncludeLogs {
		for _, ref := range refs {
			if ref.Kind == "Pod" {
				wg.Add(1)
				go func(r resourceRef) {
					defer wg.Done()
					logs := c.getRecentErrorLogs(ctx, cs, r, cutoff)
					mu.Lock()
					entries = append(entries, logs...)
					mu.Unlock()
				}(ref)
			}
		}
	}

	wg.Wait()

	// 4. Sort by timestamp descending (most recent first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	// 5. Limit entries
	if len(entries) > req.MaxEntries {
		entries = entries[:req.MaxEntries]
	}

	return entries, nil
}

// extractResourceRefs gets unique resource references from a dependency graph
func extractResourceRefs(graph *DependencyGraph) []resourceRef {
	var refs []resourceRef
	for _, node := range graph.Nodes {
		if node.IsSummary {
			continue // Skip summary nodes
		}
		refs = append(refs, resourceRef{
			Kind:      node.Kind,
			Name:      node.Name,
			Namespace: node.Namespace,
		})
	}
	return refs
}

// deduplicateRefs removes duplicate resource references
func deduplicateRefs(refs []resourceRef) []resourceRef {
	seen := make(map[string]bool)
	var result []resourceRef
	for _, ref := range refs {
		key := fmt.Sprintf("%s/%s/%s", ref.Kind, ref.Namespace, ref.Name)
		if !seen[key] {
			seen[key] = true
			result = append(result, ref)
		}
	}
	return result
}

// capitalizeKind converts resource type to Kind (e.g., "pod" -> "Pod")
func capitalizeKind(resourceType string) string {
	kindMap := map[string]string{
		"pod":                   "Pod",
		"deployment":            "Deployment",
		"service":               "Service",
		"configmap":             "ConfigMap",
		"secret":                "Secret",
		"ingress":               "Ingress",
		"statefulset":           "StatefulSet",
		"daemonset":             "DaemonSet",
		"replicaset":            "ReplicaSet",
		"job":                   "Job",
		"cronjob":               "CronJob",
		"persistentvolumeclaim": "PersistentVolumeClaim",
		"persistentvolume":      "PersistentVolume",
		"serviceaccount":        "ServiceAccount",
		"namespace":             "Namespace",
		"node":                  "Node",
		"hpa":                   "HorizontalPodAutoscaler",
		"pdb":                   "PodDisruptionBudget",
		"networkpolicy":         "NetworkPolicy",
	}
	if kind, ok := kindMap[strings.ToLower(resourceType)]; ok {
		return kind
	}
	// Default: capitalize first letter
	if len(resourceType) > 0 {
		return strings.ToUpper(resourceType[:1]) + resourceType[1:]
	}
	return resourceType
}

// getEventsForResource fetches Kubernetes events for a specific resource
func (c *Client) getEventsForResource(ctx context.Context, cs kubernetes.Interface, ref resourceRef, cutoff time.Time) []FlowTimelineEntry {
	var entries []FlowTimelineEntry

	// Build field selector for involved object
	fieldSelector := fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=%s", ref.Name, ref.Kind)

	namespace := ref.Namespace
	if namespace == "" {
		namespace = "default"
	}

	events, err := cs.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		// Silently skip - resource might not have events
		return entries
	}

	for _, e := range events.Items {
		// Use LastTimestamp, fall back to EventTime, then FirstTimestamp
		eventTime := e.LastTimestamp.Time
		if eventTime.IsZero() {
			eventTime = e.EventTime.Time
		}
		if eventTime.IsZero() {
			eventTime = e.FirstTimestamp.Time
		}

		// Skip events older than cutoff
		if eventTime.Before(cutoff) {
			continue
		}

		severity := "info"
		if e.Type == "Warning" {
			severity = "warning"
		}

		// Check message for error indicators
		msgLower := strings.ToLower(e.Message)
		if strings.Contains(msgLower, "error") || strings.Contains(msgLower, "failed") ||
			strings.Contains(msgLower, "backoff") || strings.Contains(msgLower, "unhealthy") {
			severity = "error"
		}

		entries = append(entries, FlowTimelineEntry{
			Timestamp:   eventTime,
			EntryType:   "event",
			Severity:    severity,
			ResourceRef: fmt.Sprintf("%s/%s", ref.Kind, ref.Name),
			Kind:        ref.Kind,
			Name:        ref.Name,
			Namespace:   ref.Namespace,
			Message:     fmt.Sprintf("%s: %s", e.Reason, e.Message),
			Details:     formatEventDetails(e),
		})
	}

	return entries
}

// formatEventDetails creates a detailed string for an event
func formatEventDetails(e corev1.Event) string {
	details := []string{
		fmt.Sprintf("Reason: %s", e.Reason),
		fmt.Sprintf("Message: %s", e.Message),
		fmt.Sprintf("Count: %d", e.Count),
		fmt.Sprintf("Source: %s/%s", e.Source.Component, e.Source.Host),
	}
	return strings.Join(details, "\n")
}

// getRecentErrorLogs fetches recent log lines that contain error patterns
func (c *Client) getRecentErrorLogs(ctx context.Context, cs kubernetes.Interface, ref resourceRef, cutoff time.Time) []FlowTimelineEntry {
	var entries []FlowTimelineEntry

	// Get pod to find containers
	pod, err := cs.CoreV1().Pods(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return entries
	}

	// Calculate since duration
	sinceSeconds := int64(time.Since(cutoff).Seconds())
	if sinceSeconds <= 0 {
		sinceSeconds = 600 // Default 10 minutes
	}

	// Get logs from each container
	for _, container := range pod.Spec.Containers {
		opts := &corev1.PodLogOptions{
			Container:    container.Name,
			Timestamps:   true,
			SinceSeconds: &sinceSeconds,
			TailLines:    int64Ptr(500), // Limit to last 500 lines per container
		}

		req := cs.CoreV1().Pods(ref.Namespace).GetLogs(ref.Name, opts)
		stream, err := req.Stream(ctx)
		if err != nil {
			continue
		}

		// Read logs and filter for errors
		buf := make([]byte, 1024*1024) // 1MB buffer
		n, _ := stream.Read(buf)
		stream.Close()

		if n > 0 {
			lines := strings.Split(string(buf[:n]), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}

				// Check if line matches error patterns
				isError := false
				for _, pattern := range errorPatterns {
					if pattern.MatchString(line) {
						isError = true
						break
					}
				}

				if !isError {
					continue
				}

				// Parse timestamp from log line (format: 2006-01-02T15:04:05.999999999Z ...)
				timestamp, content := parseLogTimestamp(line)
				if timestamp.IsZero() || timestamp.Before(cutoff) {
					continue
				}

				entries = append(entries, FlowTimelineEntry{
					Timestamp:   timestamp,
					EntryType:   "log",
					Severity:    "error",
					ResourceRef: fmt.Sprintf("%s/%s", ref.Kind, ref.Name),
					Kind:        ref.Kind,
					Name:        ref.Name,
					Namespace:   ref.Namespace,
					Message:     truncateString(content, 200),
					Details:     content,
				})
			}
		}
	}

	return entries
}

// parseLogTimestamp extracts timestamp from a log line with Kubernetes timestamp prefix
func parseLogTimestamp(line string) (time.Time, string) {
	// Kubernetes log format: 2006-01-02T15:04:05.999999999Z <content>
	if len(line) < 30 {
		return time.Time{}, line
	}

	// Find the space after timestamp
	spaceIdx := strings.Index(line, " ")
	if spaceIdx < 20 || spaceIdx > 35 {
		return time.Time{}, line
	}

	timestampStr := line[:spaceIdx]
	content := line[spaceIdx+1:]

	// Parse timestamp
	t, err := time.Parse(time.RFC3339Nano, timestampStr)
	if err != nil {
		// Try without nanoseconds
		t, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return time.Time{}, line
		}
	}

	return t, content
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// int64Ptr returns a pointer to an int64
func int64Ptr(i int64) *int64 {
	return &i
}
