package k8s

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// MultiLogRequest specifies what pods to stream logs from
type MultiLogRequest struct {
	Namespace     string            `json:"namespace"`
	LabelSelector map[string]string `json:"labelSelector"` // e.g. {"app": "nginx"}
	PodNames      []string          `json:"podNames"`      // Alternative: specific pod names
	Container     string            `json:"container"`     // Optional: specific container
	TailLines     int64             `json:"tailLines"`     // Number of lines to tail (default 100)
	SinceSeconds  int64             `json:"sinceSeconds"`  // Logs since N seconds ago
	Follow        bool              `json:"follow"`        // Stream logs continuously
	Timestamps    bool              `json:"timestamps"`    // Include timestamps
}

// MultiLogEntry represents a single log line with metadata
type MultiLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	PodName   string    `json:"podName"`
	Container string    `json:"container"`
	Message   string    `json:"message"`
	Color     string    `json:"color"` // Hex color for UI differentiation
}

// MultiLogSession manages an active multi-pod log streaming session
type MultiLogSession struct {
	ID         string
	Request    MultiLogRequest
	client     *Client
	clientset  kubernetes.Interface
	ctx        context.Context
	cancel     context.CancelFunc
	entries    chan MultiLogEntry
	podStreams map[string]io.ReadCloser
	mu         sync.Mutex
	closed     bool
}

// Pod colors for UI differentiation (colorblind-friendly palette)
var podColors = []string{
	"#3B82F6", // Blue
	"#10B981", // Green
	"#F59E0B", // Amber
	"#EF4444", // Red
	"#8B5CF6", // Purple
	"#EC4899", // Pink
	"#06B6D4", // Cyan
	"#84CC16", // Lime
	"#F97316", // Orange
	"#6366F1", // Indigo
}

// StartMultiLogSession starts streaming logs from multiple pods
func (c *Client) StartMultiLogSession(req MultiLogRequest) (*MultiLogSession, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	session := &MultiLogSession{
		ID:         fmt.Sprintf("multilog-%d", time.Now().UnixNano()),
		Request:    req,
		client:     c,
		clientset:  cs,
		ctx:        ctx,
		cancel:     cancel,
		entries:    make(chan MultiLogEntry, 1000),
		podStreams: make(map[string]io.ReadCloser),
	}

	// Set defaults
	if req.TailLines == 0 {
		req.TailLines = 100
	}

	// Find pods to stream from
	var pods []corev1.Pod

	if len(req.PodNames) > 0 {
		// Fetch specific pods by name
		for _, name := range req.PodNames {
			pod, err := cs.CoreV1().Pods(req.Namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				continue // Skip pods that don't exist
			}
			pods = append(pods, *pod)
		}
	} else if len(req.LabelSelector) > 0 {
		// Fetch pods by label selector
		selector := labels.SelectorFromSet(req.LabelSelector)
		podList, err := cs.CoreV1().Pods(req.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector.String(),
		})
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to list pods: %w", err)
		}
		pods = podList.Items
	} else {
		cancel()
		return nil, fmt.Errorf("either podNames or labelSelector must be specified")
	}

	if len(pods) == 0 {
		cancel()
		return nil, fmt.Errorf("no pods found matching the criteria")
	}

	// Start streaming from each pod
	var wg sync.WaitGroup
	for i, pod := range pods {
		color := podColors[i%len(podColors)]

		// Determine containers to stream
		containers := []string{}
		if req.Container != "" {
			containers = []string{req.Container}
		} else {
			for _, c := range pod.Spec.Containers {
				containers = append(containers, c.Name)
			}
		}

		for _, containerName := range containers {
			wg.Add(1)
			go func(p corev1.Pod, cont string, col string) {
				defer wg.Done()
				session.streamPodLogs(p, cont, col, req)
			}(pod, containerName, color)
		}
	}

	// Close entries channel when all streams complete
	go func() {
		wg.Wait()
		session.mu.Lock()
		if !session.closed {
			close(session.entries)
			session.closed = true
		}
		session.mu.Unlock()
	}()

	return session, nil
}

// streamPodLogs streams logs from a single pod/container
func (s *MultiLogSession) streamPodLogs(pod corev1.Pod, container, color string, req MultiLogRequest) {
	// Build log options
	opts := &corev1.PodLogOptions{
		Container:  container,
		Follow:     req.Follow,
		Timestamps: req.Timestamps,
	}

	if req.TailLines > 0 {
		opts.TailLines = &req.TailLines
	}
	if req.SinceSeconds > 0 {
		opts.SinceSeconds = &req.SinceSeconds
	}

	logReq := s.clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts)
	stream, err := logReq.Stream(s.ctx)
	if err != nil {
		// Send error as log entry
		select {
		case s.entries <- MultiLogEntry{
			Timestamp: time.Now(),
			PodName:   pod.Name,
			Container: container,
			Message:   fmt.Sprintf("[ERROR] Failed to stream logs: %v", err),
			Color:     color,
		}:
		case <-s.ctx.Done():
		}
		return
	}
	defer stream.Close()

	s.mu.Lock()
	s.podStreams[pod.Name+"/"+container] = stream
	s.mu.Unlock()

	// Read logs line by line
	scanner := bufio.NewScanner(stream)
	// Increase buffer size for long log lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		select {
		case <-s.ctx.Done():
			return
		default:
			line := scanner.Text()
			timestamp, message := parseLogLine(line, req.Timestamps)

			select {
			case s.entries <- MultiLogEntry{
				Timestamp: timestamp,
				PodName:   pod.Name,
				Container: container,
				Message:   message,
				Color:     color,
			}:
			case <-s.ctx.Done():
				return
			}
		}
	}
}

// parseLogLine extracts timestamp from log line if present
func parseLogLine(line string, hasTimestamp bool) (time.Time, string) {
	if !hasTimestamp || len(line) < 30 {
		return time.Now(), line
	}

	// Kubernetes timestamp format: 2006-01-02T15:04:05.999999999Z
	spaceIdx := strings.Index(line, " ")
	if spaceIdx < 20 || spaceIdx > 35 {
		return time.Now(), line
	}

	timestampStr := line[:spaceIdx]
	message := line[spaceIdx+1:]

	t, err := time.Parse(time.RFC3339Nano, timestampStr)
	if err != nil {
		t, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return time.Now(), line
		}
	}

	return t, message
}

// ReadEntries returns the channel for reading log entries
func (s *MultiLogSession) ReadEntries() <-chan MultiLogEntry {
	return s.entries
}

// Stop terminates the log streaming session
func (s *MultiLogSession) Stop() {
	s.cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, stream := range s.podStreams {
		stream.Close()
	}
	s.podStreams = make(map[string]io.ReadCloser)

	if !s.closed {
		close(s.entries)
		s.closed = true
	}
}

// GetMultiPodLogsBatch fetches a batch of logs from multiple pods (non-streaming)
func (c *Client) GetMultiPodLogsBatch(req MultiLogRequest) ([]MultiLogEntry, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset: %w", err)
	}

	ctx := context.Background()

	// Set defaults
	if req.TailLines == 0 {
		req.TailLines = 100
	}

	// Find pods
	var pods []corev1.Pod

	if len(req.PodNames) > 0 {
		fmt.Printf("[GetMultiPodLogsBatch] Looking up %d pods by name in ns=%s\n", len(req.PodNames), req.Namespace)
		for _, name := range req.PodNames {
			pod, err := cs.CoreV1().Pods(req.Namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				fmt.Printf("[GetMultiPodLogsBatch] Pod not found: %s - %v\n", name, err)
				continue
			}
			pods = append(pods, *pod)
		}
	} else if len(req.LabelSelector) > 0 {
		selector := labels.SelectorFromSet(req.LabelSelector)
		fmt.Printf("[GetMultiPodLogsBatch] Listing pods with selector=%s in ns=%s\n", selector.String(), req.Namespace)
		podList, err := cs.CoreV1().Pods(req.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector.String(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list pods: %w", err)
		}
		fmt.Printf("[GetMultiPodLogsBatch] Found %d pods matching selector\n", len(podList.Items))
		pods = podList.Items
	} else {
		return nil, fmt.Errorf("either podNames or labelSelector must be specified")
	}

	if len(pods) == 0 {
		return nil, fmt.Errorf("no pods found matching the criteria")
	}

	// Collect logs from all pods in parallel
	var allEntries []MultiLogEntry
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, pod := range pods {
		color := podColors[i%len(podColors)]

		containers := []string{}
		if req.Container != "" {
			containers = []string{req.Container}
		} else {
			for _, c := range pod.Spec.Containers {
				containers = append(containers, c.Name)
			}
		}
		fmt.Printf("[GetMultiPodLogsBatch] Pod %s has %d containers: %v\n", pod.Name, len(containers), containers)

		for _, containerName := range containers {
			wg.Add(1)
			go func(p corev1.Pod, cont string, col string) {
				defer wg.Done()

				opts := &corev1.PodLogOptions{
					Container:  cont,
					Timestamps: true,
				}
				if req.TailLines > 0 {
					opts.TailLines = &req.TailLines
				}
				if req.SinceSeconds > 0 {
					opts.SinceSeconds = &req.SinceSeconds
				}
				fmt.Printf("[GetMultiPodLogsBatch] Fetching logs for %s/%s (tail=%d, since=%d)\n", p.Name, cont, req.TailLines, req.SinceSeconds)

				logReq := cs.CoreV1().Pods(p.Namespace).GetLogs(p.Name, opts)
				stream, err := logReq.Stream(ctx)
				if err != nil {
					fmt.Printf("[GetMultiPodLogsBatch] ERROR streaming logs for %s/%s: %v\n", p.Name, cont, err)
					mu.Lock()
					allEntries = append(allEntries, MultiLogEntry{
						Timestamp: time.Now(),
						PodName:   p.Name,
						Container: cont,
						Message:   fmt.Sprintf("[ERROR] Failed to get logs: %v", err),
						Color:     col,
					})
					mu.Unlock()
					return
				}
				defer stream.Close()
				fmt.Printf("[GetMultiPodLogsBatch] Stream opened successfully for %s/%s\n", p.Name, cont)

				scanner := bufio.NewScanner(stream)
				buf := make([]byte, 0, 64*1024)
				scanner.Buffer(buf, 1024*1024)

				var entries []MultiLogEntry
				lineCount := 0
				for scanner.Scan() {
					lineCount++
					line := scanner.Text()
					timestamp, message := parseLogLine(line, true)
					entries = append(entries, MultiLogEntry{
						Timestamp: timestamp,
						PodName:   p.Name,
						Container: cont,
						Message:   message,
						Color:     col,
					})
				}
				if scanErr := scanner.Err(); scanErr != nil {
					fmt.Printf("[GetMultiPodLogsBatch] Scanner error for %s/%s: %v\n", p.Name, cont, scanErr)
				}
				fmt.Printf("[GetMultiPodLogsBatch] Read %d lines from %s/%s\n", lineCount, p.Name, cont)

				mu.Lock()
				allEntries = append(allEntries, entries...)
				mu.Unlock()
			}(pod, containerName, color)
		}
	}

	wg.Wait()

	// Sort by timestamp
	sortMultiLogEntries(allEntries)

	return allEntries, nil
}

// sortMultiLogEntries sorts log entries by timestamp (oldest first)
func sortMultiLogEntries(entries []MultiLogEntry) {
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Timestamp.Before(entries[i].Timestamp) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}
