// Code split from pods.go; see that file for pod listing, CRUD, and owner resolution.
package k8s

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) GetPodLogs(namespace, podName, containerName string, timestamps bool, previous bool, sinceTime string) (string, error) {
	// When sinceTime is set, we need to get logs starting from that time (first N lines after sinceTime)
	// Kubernetes TailLines gives last N lines, so we fetch all and truncate to first 200
	if sinceTime != "" {
		allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, timestamps, previous, sinceTime)
		if err != nil {
			return "", err
		}
		lines := strings.Split(allLogs, "\n")
		if len(lines) <= 200 {
			return allLogs, nil
		}
		return strings.Join(lines[:200], "\n"), nil
	}
	// Default: get last 200 lines
	return c.getPodLogsWithOptions(namespace, podName, containerName, func(i int64) *int64 { return &i }(200), timestamps, previous, sinceTime)
}

func (c *Client) GetAllPodLogs(namespace, podName, containerName string, timestamps bool, previous bool) (string, error) {
	return c.getPodLogsWithOptions(namespace, podName, containerName, nil, timestamps, previous, "")
}

// GetPodLogsFromStart fetches all logs and returns the first N lines (default 200)
func (c *Client) GetPodLogsFromStart(namespace, podName, containerName string, timestamps bool, previous bool, lineLimit int) (string, error) {
	allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, timestamps, previous, "")
	if err != nil {
		return "", err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}
	lines := strings.Split(allLogs, "\n")
	if len(lines) <= lineLimit {
		return allLogs, nil
	}
	return strings.Join(lines[:lineLimit], "\n"), nil
}

// GetPodLogsBefore fetches logs before a given timestamp.
// Returns up to lineLimit lines that occur before the specified timestamp.
// The beforeTime should be in RFC3339 format (e.g., 2024-11-26T14:30:00Z).
// Returns the logs and a boolean indicating if there are more logs before these.
func (c *Client) GetPodLogsBefore(namespace, podName, containerName string, timestamps bool, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, true, previous, "") // Always fetch with timestamps to find position
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	// Normalize beforeTime - extract just the comparable portion (first 30 chars if available)
	compareLen := 30
	if len(beforeTime) < compareLen {
		compareLen = len(beforeTime)
	}
	beforeTimePrefix := beforeTime[:compareLen]

	// Find the line index where timestamp >= beforeTime (strict: we want lines BEFORE this)
	cutoffIndex := -1
	for i, line := range lines {
		if len(line) >= 30 { // Timestamp is at least 30 chars: 2024-11-26T14:30:00.123456789Z
			lineTime := line[:30]
			// Use >= to find the first line at or after beforeTime
			// We exclude this line and all after it
			if lineTime >= beforeTimePrefix {
				cutoffIndex = i
				break
			}
		}
	}

	var resultLines []string
	hasMoreBefore := false

	if cutoffIndex == -1 {
		// beforeTime is after all logs, return last lineLimit lines
		if len(lines) > lineLimit {
			resultLines = lines[len(lines)-lineLimit:]
			hasMoreBefore = true
		} else {
			resultLines = lines
		}
	} else if cutoffIndex == 0 {
		// beforeTime is before all logs, nothing to return
		return "", false, nil
	} else {
		// Return lineLimit lines before cutoffIndex
		startIndex := cutoffIndex - lineLimit
		if startIndex < 0 {
			startIndex = 0
		} else {
			hasMoreBefore = true
		}
		resultLines = lines[startIndex:cutoffIndex]
	}

	// If caller doesn't want timestamps, strip them
	if !timestamps {
		for i, line := range resultLines {
			if len(line) > 31 {
				resultLines[i] = line[31:] // Skip timestamp and space
			}
		}
	}

	return strings.Join(resultLines, "\n"), hasMoreBefore, nil
}

// GetPodLogsAfter fetches logs after a given timestamp.
// Returns up to lineLimit lines that occur after the specified timestamp.
// The afterTime should be in RFC3339 format.
// Returns the logs and a boolean indicating if there are more logs after these.
func (c *Client) GetPodLogsAfter(namespace, podName, containerName string, timestamps bool, previous bool, afterTime string, lineLimit int) (string, bool, error) {
	sinceTime := nextLogSinceTime(afterTime)
	// Always fetch with timestamps so we can properly compare
	allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, true, previous, sinceTime)
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	cutoff, cutoffErr := time.Parse(time.RFC3339Nano, sinceTime)
	// Skip lines before the exclusive cursor. SinceTime has already advanced by
	// one nanosecond, equality belongs to the new result set.
	startIdx := 0
	for i, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) > 0 && cutoffErr == nil {
			lineTime, err := time.Parse(time.RFC3339Nano, parts[0])
			if err == nil && lineTime.Before(cutoff) {
				startIdx = i + 1
				continue
			}
		}
		break
	}

	if startIdx >= len(lines) {
		return "", false, nil
	}

	lines = lines[startIdx:]

	hasMoreAfter := len(lines) > lineLimit
	if hasMoreAfter {
		lines = lines[:lineLimit]
	}

	// Strip timestamps if caller doesn't want them
	if !timestamps {
		for i, line := range lines {
			if len(line) > 31 {
				lines[i] = line[31:] // Skip timestamp and space
			}
		}
	}

	return strings.Join(lines, "\n"), hasMoreAfter, nil
}

func nextLogSinceTime(afterTime string) string {
	// Kubernetes exposes only a timestamp cursor for logs. Making it exclusive
	// prevents duplicates and pagination loops. If more than one full page has
	// exactly the same nanosecond timestamp, entries beyond that page cannot be
	// addressed by the API and may be skipped.
	t, err := time.Parse(time.RFC3339Nano, afterTime)
	if err != nil {
		return afterTime
	}
	return t.Add(time.Nanosecond).Format(time.RFC3339Nano)
}

func (c *Client) getPodLogsWithOptions(namespace, podName, containerName string, tailLines *int64, timestamps bool, previous bool, sinceTime string) (string, error) {
	if IsDebugClusterContext(c.GetCurrentContext()) {
		return "", fmt.Errorf("logs are not available on the debug cluster")
	}

	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	opts := &v1.PodLogOptions{
		TailLines:  tailLines,
		Timestamps: timestamps,
		Previous:   previous,
	}
	if containerName != "" {
		opts.Container = containerName
	}
	if sinceTime != "" {
		t, err := time.Parse(time.RFC3339, sinceTime)
		if err == nil {
			mt := metav1.NewTime(t)
			opts.SinceTime = &mt
		}
	}

	req := cs.CoreV1().Pods(namespace).GetLogs(podName, opts)

	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buf := new(strings.Builder)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// StreamPodLogs streams logs from a pod container and calls the callback for each line.
// It continues until the context is canceled or the pod terminates.
// Transient disconnects (unexpected EOF, connection reset) are retried automatically
// using the last received timestamp to avoid duplicate lines.
func (c *Client) StreamPodLogs(ctx context.Context, namespace, podName, containerName string, timestamps bool, tailLines int64, onLine func(line string)) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	const maxConsecutiveFailures = 5
	const reconnectDelay = 2 * time.Second

	var lastTimestamp string
	consecutiveFailures := 0
	firstConnect := true

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		opts := &v1.PodLogOptions{
			Follow:     true,
			Timestamps: true, // Always request timestamps for reconnect tracking
		}
		if containerName != "" {
			opts.Container = containerName
		}

		if firstConnect {
			if tailLines > 0 {
				opts.TailLines = &tailLines
			}
			firstConnect = false
		} else if lastTimestamp != "" {
			// Reconnect: resume from last received timestamp + 1ns to avoid duplicates
			t, parseErr := time.Parse(time.RFC3339Nano, lastTimestamp)
			if parseErr == nil {
				sinceTime := metav1.NewTime(t.Add(time.Nanosecond))
				opts.SinceTime = &sinceTime
			}
		}

		req := cs.CoreV1().Pods(namespace).GetLogs(podName, opts)
		stream, err := req.Stream(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			// Stream open failed — likely terminal (pod deleted, not found, etc.)
			return err
		}

		scanner := bufio.NewScanner(stream)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		linesReceived := 0
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				stream.Close()
				return ctx.Err()
			default:
			}

			line := scanner.Text()
			linesReceived++

			// Extract timestamp (always present since Timestamps: true)
			if len(line) >= 30 {
				lastTimestamp = line[:30]
			}

			// Strip timestamp prefix if caller doesn't want timestamps
			if !timestamps {
				if len(line) > 31 && line[30] == ' ' {
					line = line[31:]
				}
			}

			onLine(line)
		}

		stream.Close()

		scanErr := scanner.Err()

		// Clean EOF (scanErr == nil): pod/container terminated normally — don't reconnect
		if scanErr == nil {
			return nil
		}

		// Context cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Transient error — attempt reconnect
		if linesReceived > 0 {
			consecutiveFailures = 0
		} else {
			consecutiveFailures++
		}

		if consecutiveFailures >= maxConsecutiveFailures {
			return fmt.Errorf("log stream failed after %d consecutive reconnect attempts: %w", maxConsecutiveFailures, scanErr)
		}

		log.Printf("[K8s Client] Log stream interrupted for %s/%s/%s (err: %v), reconnecting in %v...",
			namespace, podName, containerName, scanErr, reconnectDelay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(reconnectDelay):
		}
	}
}

// timestampedLogLine represents a log line with parsed timestamp for sorting

type timestampedLogLine struct {
	timestamp string // RFC3339 format timestamp
	content   string // Full line content including timestamp and container prefix
}

// GetAllContainersLogs fetches logs from all containers in a pod, merges them by timestamp,
// and prefixes each line with [containerName]. Returns the last 200 lines by default.
func (c *Client) GetAllContainersLogs(namespace, podName string, containerNames []string, timestamps bool, previous bool, sinceTime string) (string, error) {
	return c.getAllContainersLogs(namespace, podName, containerNames, timestamps, previous, sinceTime, 200)
}

func (c *Client) getAllContainersLogs(namespace, podName string, containerNames []string, timestamps bool, previous bool, sinceTime string, maxLines int) (string, error) {
	if len(containerNames) == 0 {
		return "", nil
	}

	// Fetch logs from all containers concurrently
	type containerLogs struct {
		containerName string
		logs          string
		err           error
	}

	results := make(chan containerLogs, len(containerNames))
	var wg sync.WaitGroup

	for _, containerName := range containerNames {
		wg.Add(1)
		go func(cn string) {
			defer wg.Done()
			// Always fetch with timestamps so we can sort
			logs, err := c.getPodLogsWithOptions(namespace, podName, cn, nil, true, previous, sinceTime)
			results <- containerLogs{containerName: cn, logs: logs, err: err}
		}(containerName)
	}

	wg.Wait()
	close(results)

	// Collect all log lines with timestamps
	var allLines []timestampedLogLine
	for result := range results {
		if result.err != nil {
			// Add error as a log line
			allLines = append(allLines, timestampedLogLine{
				timestamp: time.Now().Format(time.RFC3339Nano),
				content:   fmt.Sprintf("[%s] Error fetching logs: %v", result.containerName, result.err),
			})
			continue
		}

		lines := strings.Split(result.logs, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			// Parse timestamp from line (first 30 chars)
			var ts, content string
			if len(line) >= 31 && line[30] == ' ' {
				ts = line[:30]
				content = line[31:]
			} else {
				ts = ""
				content = line
			}

			// Build the merged line with container prefix
			var mergedLine string
			if timestamps && ts != "" {
				mergedLine = fmt.Sprintf("%s [%s] %s", ts, result.containerName, content)
			} else if ts != "" {
				mergedLine = fmt.Sprintf("%s [%s] %s", ts, result.containerName, content)
			} else {
				mergedLine = fmt.Sprintf("[%s] %s", result.containerName, content)
			}

			allLines = append(allLines, timestampedLogLine{
				timestamp: ts,
				content:   mergedLine,
			})
		}
	}

	// Sort by timestamp
	sort.SliceStable(allLines, func(i, j int) bool {
		return allLines[i].timestamp < allLines[j].timestamp
	})

	// Build result, taking last 200 lines if sinceTime is empty
	var resultLines []string
	for _, line := range allLines {
		if timestamps {
			resultLines = append(resultLines, line.content)
		} else {
			// Strip the timestamp prefix if caller doesn't want timestamps
			if len(line.content) > 31 && line.content[30] == ' ' {
				resultLines = append(resultLines, line.content[31:])
			} else {
				resultLines = append(resultLines, line.content)
			}
		}
	}

	// If sinceTime is set, return first 200 lines after that time
	// Otherwise return last 200 lines
	if maxLines > 0 && sinceTime != "" && len(resultLines) > maxLines {
		resultLines = resultLines[:maxLines]
	} else if maxLines > 0 && sinceTime == "" && len(resultLines) > maxLines {
		resultLines = resultLines[len(resultLines)-maxLines:]
	}

	return strings.Join(resultLines, "\n"), nil
}

// GetAllContainersLogsAll fetches all logs from all containers, merged by timestamp
func (c *Client) GetAllContainersLogsAll(namespace, podName string, containerNames []string, timestamps bool, previous bool) (string, error) {
	if len(containerNames) == 0 {
		return "", nil
	}

	// Fetch logs from all containers concurrently
	type containerLogs struct {
		containerName string
		logs          string
		err           error
	}

	results := make(chan containerLogs, len(containerNames))
	var wg sync.WaitGroup

	for _, containerName := range containerNames {
		wg.Add(1)
		go func(cn string) {
			defer wg.Done()
			logs, err := c.getPodLogsWithOptions(namespace, podName, cn, nil, true, previous, "")
			results <- containerLogs{containerName: cn, logs: logs, err: err}
		}(containerName)
	}

	wg.Wait()
	close(results)

	// Collect and merge all log lines
	var allLines []timestampedLogLine
	for result := range results {
		if result.err != nil {
			allLines = append(allLines, timestampedLogLine{
				timestamp: time.Now().Format(time.RFC3339Nano),
				content:   fmt.Sprintf("[%s] Error fetching logs: %v", result.containerName, result.err),
			})
			continue
		}

		lines := strings.Split(result.logs, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			var ts, content string
			if len(line) >= 31 && line[30] == ' ' {
				ts = line[:30]
				content = line[31:]
			} else {
				ts = ""
				content = line
			}

			var mergedLine string
			if ts != "" {
				mergedLine = fmt.Sprintf("%s [%s] %s", ts, result.containerName, content)
			} else {
				mergedLine = fmt.Sprintf("[%s] %s", result.containerName, content)
			}

			allLines = append(allLines, timestampedLogLine{
				timestamp: ts,
				content:   mergedLine,
			})
		}
	}

	sort.SliceStable(allLines, func(i, j int) bool {
		return allLines[i].timestamp < allLines[j].timestamp
	})

	var resultLines []string
	for _, line := range allLines {
		if timestamps {
			resultLines = append(resultLines, line.content)
		} else {
			if len(line.content) > 31 && line.content[30] == ' ' {
				resultLines = append(resultLines, line.content[31:])
			} else {
				resultLines = append(resultLines, line.content)
			}
		}
	}

	return strings.Join(resultLines, "\n"), nil
}

// GetAllContainersLogsFromStart fetches the first N lines from all containers, merged by timestamp
func (c *Client) GetAllContainersLogsFromStart(namespace, podName string, containerNames []string, timestamps bool, previous bool, lineLimit int) (string, error) {
	allLogs, err := c.GetAllContainersLogsAll(namespace, podName, containerNames, timestamps, previous)
	if err != nil {
		return "", err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}
	lines := strings.Split(allLogs, "\n")
	if len(lines) <= lineLimit {
		return allLogs, nil
	}
	return strings.Join(lines[:lineLimit], "\n"), nil
}

// GetAllContainersLogsBefore fetches logs before a given timestamp from all containers
func (c *Client) GetAllContainersLogsBefore(namespace, podName string, containerNames []string, timestamps bool, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	// Fetch all logs with timestamps to properly merge and find position
	allLogs, err := c.GetAllContainersLogsAll(namespace, podName, containerNames, true, previous)
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	// Normalize beforeTime
	compareLen := 30
	if len(beforeTime) < compareLen {
		compareLen = len(beforeTime)
	}
	beforeTimePrefix := beforeTime[:compareLen]

	// Find cutoff index
	cutoffIndex := -1
	for i, line := range lines {
		if len(line) >= 30 {
			lineTime := line[:30]
			if lineTime >= beforeTimePrefix {
				cutoffIndex = i
				break
			}
		}
	}

	var resultLines []string
	hasMoreBefore := false

	if cutoffIndex == -1 {
		if len(lines) > lineLimit {
			resultLines = lines[len(lines)-lineLimit:]
			hasMoreBefore = true
		} else {
			resultLines = lines
		}
	} else if cutoffIndex == 0 {
		return "", false, nil
	} else {
		startIndex := cutoffIndex - lineLimit
		if startIndex < 0 {
			startIndex = 0
		} else {
			hasMoreBefore = true
		}
		resultLines = lines[startIndex:cutoffIndex]
	}

	// Strip timestamps if caller doesn't want them
	if !timestamps {
		for i, line := range resultLines {
			if len(line) > 31 {
				resultLines[i] = line[31:]
			}
		}
	}

	return strings.Join(resultLines, "\n"), hasMoreBefore, nil
}

// GetAllContainersLogsAfter fetches logs after a given timestamp from all containers
func (c *Client) GetAllContainersLogsAfter(namespace, podName string, containerNames []string, timestamps bool, previous bool, afterTime string, lineLimit int) (string, bool, error) {
	if lineLimit <= 0 {
		lineLimit = 200
	}
	allLogs, err := c.getAllContainersLogs(namespace, podName, containerNames, true, previous, nextLogSinceTime(afterTime), lineLimit+1)
	if err != nil {
		return "", false, err
	}
	lines := strings.Split(allLogs, "\n")
	hasMoreAfter := len(lines) > lineLimit
	if hasMoreAfter {
		lines = lines[:lineLimit]
	}

	// Strip timestamps if caller doesn't want them
	if !timestamps {
		for i, line := range lines {
			if len(line) > 31 {
				lines[i] = line[31:]
			}
		}
	}

	return strings.Join(lines, "\n"), hasMoreAfter, nil
}

// StreamAllContainersLogs streams logs from all containers, merging them in real-time by timestamp.
// Each line is prefixed with [containerName].
func (c *Client) StreamAllContainersLogs(ctx context.Context, namespace, podName string, containerNames []string, timestamps bool, tailLines int64, onLine func(line string)) error {
	if len(containerNames) == 0 {
		return nil
	}

	// For real-time streaming, we need to collect lines from all containers
	// and emit them in timestamp order. We use a priority queue approach.
	type streamLine struct {
		timestamp     string
		containerName string
		content       string
		fullLine      string
	}

	lineChan := make(chan streamLine, 1000)
	var wg sync.WaitGroup
	errChan := make(chan error, len(containerNames))

	// Start a goroutine for each container
	for _, containerName := range containerNames {
		wg.Add(1)
		go func(cn string) {
			defer wg.Done()
			err := c.StreamPodLogs(ctx, namespace, podName, cn, true, tailLines, func(line string) {
				var ts, content string
				if len(line) >= 31 && line[30] == ' ' {
					ts = line[:30]
					content = line[31:]
				} else {
					ts = ""
					content = line
				}

				var fullLine string
				if timestamps && ts != "" {
					fullLine = fmt.Sprintf("%s [%s] %s", ts, cn, content)
				} else if ts != "" {
					fullLine = fmt.Sprintf("%s [%s] %s", ts, cn, content)
				} else {
					fullLine = fmt.Sprintf("[%s] %s", cn, content)
				}

				select {
				case lineChan <- streamLine{timestamp: ts, containerName: cn, content: content, fullLine: fullLine}:
				case <-ctx.Done():
					return
				}
			})
			if err != nil && err != context.Canceled {
				errChan <- err
			}
		}(containerName)
	}

	// Close channels when all goroutines complete
	go func() {
		wg.Wait()
		close(lineChan)
		close(errChan)
	}()

	// Buffer for sorting incoming lines within a small time window
	var buffer []streamLine
	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()

	flushBuffer := func() {
		if len(buffer) == 0 {
			return
		}
		// Sort buffer by timestamp
		sort.SliceStable(buffer, func(i, j int) bool {
			return buffer[i].timestamp < buffer[j].timestamp
		})
		for _, line := range buffer {
			if timestamps {
				onLine(line.fullLine)
			} else {
				// Strip timestamp from output
				if len(line.fullLine) > 31 && line.fullLine[30] == ' ' {
					onLine(line.fullLine[31:])
				} else {
					onLine(line.fullLine)
				}
			}
		}
		buffer = buffer[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flushBuffer()
			return ctx.Err()
		case line, ok := <-lineChan:
			if !ok {
				flushBuffer()
				// Check for errors
				for err := range errChan {
					if err != nil {
						return err
					}
				}
				return nil
			}
			buffer = append(buffer, line)
		case <-flushTicker.C:
			flushBuffer()
		}
	}
}

// PodContainerPair represents a pod and its containers for multi-pod log fetching
type PodContainerPair struct {
	PodName        string
	ContainerNames []string // If empty or single, just use [podName] prefix; if multiple, use [podName/containerName]
}

// GetAllPodsLogs fetches logs from multiple pods, merges them by timestamp.
// When allContainers is true, prefixes with [podName/containerName], otherwise [podName].
func (c *Client) GetAllPodsLogs(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, sinceTime string) (string, error) {
	return c.getAllPodsLogs(namespace, pods, allContainers, timestamps, previous, sinceTime, 200)
}

func (c *Client) getAllPodsLogs(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, sinceTime string, maxLines int) (string, error) {
	if len(pods) == 0 {
		return "", nil
	}

	type podContainerLogs struct {
		podName       string
		containerName string
		logs          string
		err           error
	}

	// Count total fetches needed
	totalFetches := 0
	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 0 {
			totalFetches += len(p.ContainerNames)
		} else {
			totalFetches++
		}
	}

	results := make(chan podContainerLogs, totalFetches)
	var wg sync.WaitGroup

	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 1 {
			// Fetch from all containers
			for _, cn := range p.ContainerNames {
				wg.Add(1)
				go func(podName, containerName string) {
					defer wg.Done()
					logs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, true, previous, sinceTime)
					results <- podContainerLogs{podName: podName, containerName: containerName, logs: logs, err: err}
				}(p.PodName, cn)
			}
		} else {
			// Fetch from single/first container
			containerName := ""
			if len(p.ContainerNames) > 0 {
				containerName = p.ContainerNames[0]
			}
			wg.Add(1)
			go func(podName, cn string) {
				defer wg.Done()
				logs, err := c.getPodLogsWithOptions(namespace, podName, cn, nil, true, previous, sinceTime)
				results <- podContainerLogs{podName: podName, containerName: cn, logs: logs, err: err}
			}(p.PodName, containerName)
		}
	}

	wg.Wait()
	close(results)

	var allLines []timestampedLogLine
	for result := range results {
		prefix := result.podName
		if allContainers && result.containerName != "" {
			prefix = result.podName + "/" + result.containerName
		}

		if result.err != nil {
			allLines = append(allLines, timestampedLogLine{
				timestamp: time.Now().Format(time.RFC3339Nano),
				content:   fmt.Sprintf("[%s] Error fetching logs: %v", prefix, result.err),
			})
			continue
		}

		lines := strings.Split(result.logs, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			var ts, content string
			if len(line) >= 31 && line[30] == ' ' {
				ts = line[:30]
				content = line[31:]
			} else {
				ts = ""
				content = line
			}

			var mergedLine string
			if ts != "" {
				mergedLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
			} else {
				mergedLine = fmt.Sprintf("[%s] %s", prefix, content)
			}

			allLines = append(allLines, timestampedLogLine{
				timestamp: ts,
				content:   mergedLine,
			})
		}
	}

	sort.SliceStable(allLines, func(i, j int) bool {
		return allLines[i].timestamp < allLines[j].timestamp
	})

	var resultLines []string
	for _, line := range allLines {
		if timestamps {
			resultLines = append(resultLines, line.content)
		} else {
			if len(line.content) > 31 && line.content[30] == ' ' {
				resultLines = append(resultLines, line.content[31:])
			} else {
				resultLines = append(resultLines, line.content)
			}
		}
	}

	if maxLines > 0 && sinceTime != "" && len(resultLines) > maxLines {
		resultLines = resultLines[:maxLines]
	} else if maxLines > 0 && sinceTime == "" && len(resultLines) > maxLines {
		resultLines = resultLines[len(resultLines)-maxLines:]
	}

	return strings.Join(resultLines, "\n"), nil
}

// GetAllPodsLogsAll fetches all logs from multiple pods, merged by timestamp (no truncation)
func (c *Client) GetAllPodsLogsAll(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool) (string, error) {
	if len(pods) == 0 {
		return "", nil
	}

	type podContainerLogs struct {
		podName       string
		containerName string
		logs          string
		err           error
	}

	totalFetches := 0
	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 0 {
			totalFetches += len(p.ContainerNames)
		} else {
			totalFetches++
		}
	}

	results := make(chan podContainerLogs, totalFetches)
	var wg sync.WaitGroup

	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 1 {
			for _, cn := range p.ContainerNames {
				wg.Add(1)
				go func(podName, containerName string) {
					defer wg.Done()
					logs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, true, previous, "")
					results <- podContainerLogs{podName: podName, containerName: containerName, logs: logs, err: err}
				}(p.PodName, cn)
			}
		} else {
			containerName := ""
			if len(p.ContainerNames) > 0 {
				containerName = p.ContainerNames[0]
			}
			wg.Add(1)
			go func(podName, cn string) {
				defer wg.Done()
				logs, err := c.getPodLogsWithOptions(namespace, podName, cn, nil, true, previous, "")
				results <- podContainerLogs{podName: podName, containerName: cn, logs: logs, err: err}
			}(p.PodName, containerName)
		}
	}

	wg.Wait()
	close(results)

	var allLines []timestampedLogLine
	for result := range results {
		prefix := result.podName
		if allContainers && result.containerName != "" {
			prefix = result.podName + "/" + result.containerName
		}

		if result.err != nil {
			allLines = append(allLines, timestampedLogLine{
				timestamp: time.Now().Format(time.RFC3339Nano),
				content:   fmt.Sprintf("[%s] Error fetching logs: %v", prefix, result.err),
			})
			continue
		}

		lines := strings.Split(result.logs, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			var ts, content string
			if len(line) >= 31 && line[30] == ' ' {
				ts = line[:30]
				content = line[31:]
			} else {
				ts = ""
				content = line
			}

			var mergedLine string
			if ts != "" {
				mergedLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
			} else {
				mergedLine = fmt.Sprintf("[%s] %s", prefix, content)
			}

			allLines = append(allLines, timestampedLogLine{
				timestamp: ts,
				content:   mergedLine,
			})
		}
	}

	sort.SliceStable(allLines, func(i, j int) bool {
		return allLines[i].timestamp < allLines[j].timestamp
	})

	var resultLines []string
	for _, line := range allLines {
		if timestamps {
			resultLines = append(resultLines, line.content)
		} else {
			if len(line.content) > 31 && line.content[30] == ' ' {
				resultLines = append(resultLines, line.content[31:])
			} else {
				resultLines = append(resultLines, line.content)
			}
		}
	}

	return strings.Join(resultLines, "\n"), nil
}

// GetAllPodsLogsFromStart fetches the first N lines from multiple pods, merged by timestamp
func (c *Client) GetAllPodsLogsFromStart(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, lineLimit int) (string, error) {
	allLogs, err := c.GetAllPodsLogsAll(namespace, pods, allContainers, timestamps, previous)
	if err != nil {
		return "", err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}
	lines := strings.Split(allLogs, "\n")
	if len(lines) <= lineLimit {
		return allLogs, nil
	}
	return strings.Join(lines[:lineLimit], "\n"), nil
}

// GetAllPodsLogsBefore fetches logs before a given timestamp from multiple pods
func (c *Client) GetAllPodsLogsBefore(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	allLogs, err := c.GetAllPodsLogsAll(namespace, pods, allContainers, true, previous)
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	compareLen := 30
	if len(beforeTime) < compareLen {
		compareLen = len(beforeTime)
	}
	beforeTimePrefix := beforeTime[:compareLen]

	cutoffIndex := -1
	for i, line := range lines {
		if len(line) >= 30 {
			lineTime := line[:30]
			if lineTime >= beforeTimePrefix {
				cutoffIndex = i
				break
			}
		}
	}

	var resultLines []string
	hasMoreBefore := false

	if cutoffIndex == -1 {
		if len(lines) > lineLimit {
			resultLines = lines[len(lines)-lineLimit:]
			hasMoreBefore = true
		} else {
			resultLines = lines
		}
	} else if cutoffIndex == 0 {
		return "", false, nil
	} else {
		startIndex := cutoffIndex - lineLimit
		if startIndex < 0 {
			startIndex = 0
		} else {
			hasMoreBefore = true
		}
		resultLines = lines[startIndex:cutoffIndex]
	}

	if !timestamps {
		for i, line := range resultLines {
			if len(line) > 31 {
				resultLines[i] = line[31:]
			}
		}
	}

	return strings.Join(resultLines, "\n"), hasMoreBefore, nil
}

// GetAllPodsLogsAfter fetches logs after a given timestamp from multiple pods
func (c *Client) GetAllPodsLogsAfter(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, afterTime string, lineLimit int) (string, bool, error) {
	if lineLimit <= 0 {
		lineLimit = 200
	}
	allLogs, err := c.getAllPodsLogs(namespace, pods, allContainers, true, previous, nextLogSinceTime(afterTime), lineLimit+1)
	if err != nil {
		return "", false, err
	}
	lines := strings.Split(allLogs, "\n")
	hasMoreAfter := len(lines) > lineLimit
	if hasMoreAfter {
		lines = lines[:lineLimit]
	}

	if !timestamps {
		for i, line := range lines {
			if len(line) > 31 {
				lines[i] = line[31:]
			}
		}
	}

	return strings.Join(lines, "\n"), hasMoreAfter, nil
}

// StreamAllPodsLogs streams logs from multiple pods, merging them in real-time by timestamp.
func (c *Client) StreamAllPodsLogs(ctx context.Context, namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, tailLines int64, onLine func(line string)) error {
	if len(pods) == 0 {
		return nil
	}

	type streamLine struct {
		timestamp string
		fullLine  string
	}

	lineChan := make(chan streamLine, 1000)
	var wg sync.WaitGroup
	errChan := make(chan error, len(pods)*10)

	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 1 {
			for _, cn := range p.ContainerNames {
				wg.Add(1)
				go func(podName, containerName string) {
					defer wg.Done()
					prefix := podName + "/" + containerName
					err := c.StreamPodLogs(ctx, namespace, podName, containerName, true, tailLines, func(line string) {
						var ts, content string
						if len(line) >= 31 && line[30] == ' ' {
							ts = line[:30]
							content = line[31:]
						} else {
							ts = ""
							content = line
						}

						var fullLine string
						if timestamps && ts != "" {
							fullLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
						} else if ts != "" {
							fullLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
						} else {
							fullLine = fmt.Sprintf("[%s] %s", prefix, content)
						}

						select {
						case lineChan <- streamLine{timestamp: ts, fullLine: fullLine}:
						case <-ctx.Done():
							return
						}
					})
					if err != nil && err != context.Canceled {
						errChan <- err
					}
				}(p.PodName, cn)
			}
		} else {
			containerName := ""
			if len(p.ContainerNames) > 0 {
				containerName = p.ContainerNames[0]
			}
			wg.Add(1)
			go func(podName, cn string) {
				defer wg.Done()
				prefix := podName
				err := c.StreamPodLogs(ctx, namespace, podName, cn, true, tailLines, func(line string) {
					var ts, content string
					if len(line) >= 31 && line[30] == ' ' {
						ts = line[:30]
						content = line[31:]
					} else {
						ts = ""
						content = line
					}

					var fullLine string
					if timestamps && ts != "" {
						fullLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
					} else if ts != "" {
						fullLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
					} else {
						fullLine = fmt.Sprintf("[%s] %s", prefix, content)
					}

					select {
					case lineChan <- streamLine{timestamp: ts, fullLine: fullLine}:
					case <-ctx.Done():
						return
					}
				})
				if err != nil && err != context.Canceled {
					errChan <- err
				}
			}(p.PodName, containerName)
		}
	}

	go func() {
		wg.Wait()
		close(lineChan)
		close(errChan)
	}()

	var buffer []streamLine
	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()

	flushBuffer := func() {
		if len(buffer) == 0 {
			return
		}
		sort.SliceStable(buffer, func(i, j int) bool {
			return buffer[i].timestamp < buffer[j].timestamp
		})
		for _, line := range buffer {
			if timestamps {
				onLine(line.fullLine)
			} else {
				if len(line.fullLine) > 31 && line.fullLine[30] == ' ' {
					onLine(line.fullLine[31:])
				} else {
					onLine(line.fullLine)
				}
			}
		}
		buffer = buffer[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flushBuffer()
			return ctx.Err()
		case line, ok := <-lineChan:
			if !ok {
				flushBuffer()
				for err := range errChan {
					if err != nil {
						return err
					}
				}
				return nil
			}
			buffer = append(buffer, line)
		case <-flushTicker.C:
			flushBuffer()
		}
	}
}
