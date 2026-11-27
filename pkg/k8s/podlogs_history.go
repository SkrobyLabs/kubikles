package k8s

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// History paging ("load the first page", "load the page before X") must not
// download a whole pod log. The Kubernetes log API can only stream forward from
// a start point, so the strategy is:
//
//   - first page: stream from the beginning and stop after N lines;
//   - page before a cutoff: stream from a SinceTime window that ends at the
//     cutoff and stop reading at the first line at or after the cutoff. The
//     window starts small and grows geometrically until it holds a full page
//     (plus one line, to know whether older lines exist) or the unbounded
//     final attempt reaches the true start of the log.
//
// Edge: lines sharing the cutoff's 30-character timestamp prefix are all
// treated as "at or after" the cutoff. Pages therefore never overlap, but a
// line that shares its exact nanosecond timestamp with the cutoff is only
// reachable from the other side of the boundary, same as before this rewrite.

const (
	// maxHistoryLogPageBytes bounds the bytes of one returned page. Larger than the
	// polling limit because a user explicitly asked for history.
	maxHistoryLogPageBytes int64 = 16 * 1024 * 1024
	// maxHistoryScanBytes bounds how much of the stream one windowed attempt may
	// read before giving up on that window size.
	maxHistoryScanBytesDefault int64 = 32 * 1024 * 1024
	// logHistoryConcurrency limits parallel log requests for merged views.
	logHistoryConcurrency = 8

	logTimestampLen = 30
)

var (
	// initialBeforeWindow is the first SinceTime window tried; each retry multiplies it.
	initialBeforeWindow = time.Minute
	beforeWindowGrowth  = 8
	// maxBeforeWindow is the largest bounded window; beyond it the attempt is unbounded.
	maxBeforeWindow = 30 * 24 * time.Hour

	// maxHistoryScanBytes is a var so tests can force the dense-log fallback.
	maxHistoryScanBytes = maxHistoryScanBytesDefault

	errScanBudget = errors.New("log window exceeded the scan budget")
)

const (
	defaultLogLimit = 200
	// maxLogLimit keeps limit+1 arithmetic and allocations sane when a caller
	// passes a huge line limit. The page byte caps bite long before this.
	maxLogLimit = 1000000
)

// clampLogLimit maps a caller supplied line limit into [1, maxLogLimit],
// treating zero and negative values as the default page size.
func clampLogLimit(n int) int {
	if n <= 0 {
		return defaultLogLimit
	}
	if n > maxLogLimit {
		return maxLogLimit
	}
	return n
}

// readLogHead returns the first maxLines lines of a container log (from sinceTime
// when set), reading only what it needs. The result has no trailing newline when
// the page is full, matching the previous truncate-and-join behaviour.
func (c *Client) readLogHead(namespace, podName, containerName string, timestamps, previous bool, sinceTime string, maxLines int) (string, error) {
	maxLines = clampLogLimit(maxLines)
	if IsDebugClusterContext(c.GetCurrentContext()) {
		return "", fmt.Errorf("logs are not available on the debug cluster")
	}
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	opts := buildPodLogOptions(containerName, nil, timestamps, previous, sinceTime)
	// Server-side cap so a single enormous line cannot stream unbounded.
	limit := maxHistoryLogPageBytes + 1
	opts.LimitBytes = &limit

	stream, err := cs.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	page, err := readBoundedLogPage(stream, maxHistoryLogPageBytes, maxLines)
	if err != nil {
		return "", err
	}
	if strings.Count(page, "\n") >= maxLines {
		page = strings.TrimSuffix(page, "\n")
	}
	return page, nil
}

// GetPodLogsFromStart returns the first lineLimit lines (default 200).
func (c *Client) GetPodLogsFromStart(namespace, podName, containerName string, timestamps bool, previous bool, lineLimit int) (string, error) {
	return c.readLogHead(namespace, podName, containerName, timestamps, previous, "", lineLimit)
}

// GetPodLogsBefore returns up to lineLimit lines strictly before beforeTime
// (RFC3339, nanoseconds allowed) in chronological order, and whether older lines
// remain. The result carries no trailing newline.
func (c *Client) GetPodLogsBefore(namespace, podName, containerName string, timestamps bool, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	lineLimit = clampLogLimit(lineLimit)
	lines, more, err := c.logLinesBefore(namespace, podName, containerName, previous, beforeTime, lineLimit)
	if err != nil {
		return "", false, err
	}
	if !timestamps {
		for i, line := range lines {
			lines[i] = stripLogTimestamp(line)
		}
	}
	return strings.Join(lines, "\n"), more, nil
}

func stripLogTimestamp(line string) string {
	if len(line) > logTimestampLen+1 && line[logTimestampLen] == ' ' {
		return line[logTimestampLen+1:]
	}
	return line
}

// beforeAttempt is the outcome of reading one window up to the cutoff.
type beforeAttempt struct {
	lines   []string // the last lines before the cutoff, at most limit+1 of them
	total   int      // lines seen before the cutoff in this window
	trimmed bool     // older lines were dropped by the byte cap
}

// logLinesBefore returns the last `limit` timestamped lines before beforeTime and
// whether more exist before them.
func (c *Client) logLinesBefore(namespace, podName, containerName string, previous bool, beforeTime string, limit int) ([]string, bool, error) {
	limit = clampLogLimit(limit)
	if IsDebugClusterContext(c.GetCurrentContext()) {
		return nil, false, fmt.Errorf("logs are not available on the debug cluster")
	}
	cs, err := c.getClientset()
	if err != nil {
		return nil, false, err
	}

	prefixLen := logTimestampLen
	if len(beforeTime) < prefixLen {
		prefixLen = len(beforeTime)
	}
	cutoffPrefix := beforeTime[:prefixLen]

	// Anchor windows at the cutoff, but never in the future: a cutoff past the
	// last line (initial tail load) should look at the recent past.
	ref := time.Now()
	if t, perr := time.Parse(time.RFC3339Nano, beforeTime); perr == nil && t.Before(ref) {
		ref = t
	}

	open := func(since string) (io.ReadCloser, context.CancelFunc, error) {
		ctx, cancel := c.contextWithTimeout()
		opts := buildPodLogOptions(containerName, nil, true, previous, since)
		stream, serr := cs.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
		if serr != nil {
			cancel()
			return nil, nil, serr
		}
		return stream, cancel, nil
	}

	attempt := func(since string, budget int64) (beforeAttempt, error) {
		stream, cancel, err := open(since)
		if err != nil {
			return beforeAttempt{}, err
		}
		defer cancel()
		defer stream.Close()
		return scanLinesBefore(stream, cutoffPrefix, limit+1, maxHistoryLogPageBytes, budget)
	}

	finish := func(a beforeAttempt, more bool) ([]string, bool, error) {
		lines := a.lines
		if len(lines) > limit {
			lines = lines[len(lines)-limit:]
			more = true
		}
		return lines, more || a.trimmed, nil
	}

	window := initialBeforeWindow
	var failedWindow time.Duration // smallest window known to exceed the scan budget
	var goodWindow time.Duration   // largest window that completed within the budget
	var best *beforeAttempt
	// between picks an untried window strictly inside (goodWindow, failedWindow),
	// or 0 when none is left at one-second resolution.
	between := func() time.Duration {
		mid := (goodWindow + (failedWindow-goodWindow)/2).Truncate(time.Second)
		if mid <= goodWindow || mid >= failedWindow || mid < time.Second {
			return 0
		}
		return mid
	}
	for {
		if window > maxBeforeWindow {
			a, err := attempt("", 0)
			if err != nil {
				return nil, false, err
			}
			// Unbounded: this reached the real start of the log, so nothing older exists.
			return finish(a, false)
		}

		since := ref.Add(-window).UTC().Truncate(time.Second).Format(time.RFC3339)
		a, err := attempt(since, maxHistoryScanBytes)
		switch {
		case errors.Is(err, errScanBudget):
			failedWindow = window
			if best != nil && best.total > 0 {
				// Dense log: settle for the smaller window, older lines surely exist.
				return finish(*best, true)
			}
			if goodWindow > 0 {
				// An empty window completed but a larger one is over budget:
				// bisect between them rather than repeating the empty one.
				window = between()
			} else {
				window /= time.Duration(beforeWindowGrowth)
			}
			if window < time.Second {
				return nil, false, errLogPageTooLarge
			}
			continue
		case err != nil:
			return nil, false, err
		}

		best = &a
		goodWindow = window
		if a.total > limit {
			return finish(a, true)
		}
		window *= time.Duration(beforeWindowGrowth)
		if failedWindow > 0 && window >= failedWindow {
			// Growing again would repeat a window that already blew the budget;
			// probe the gap below it instead.
			window = between()
			if window == 0 {
				if a.total == 0 {
					return nil, false, errLogPageTooLarge
				}
				return finish(a, true)
			}
		}
	}
}

// scanLinesBefore reads lines until the first one whose 30-char timestamp prefix
// is >= cutoffPrefix, keeping only the last `keep` lines (and at most pageBytes of
// them). scanBudget > 0 aborts with errScanBudget once that many bytes were read
// without reaching the cutoff or the end of the stream.
func scanLinesBefore(source io.Reader, cutoffPrefix string, keep int, pageBytes, scanBudget int64) (beforeAttempt, error) {
	reader := bufio.NewReaderSize(source, 64*1024)
	var res beforeAttempt
	var ring []string
	var ringBytes int64
	var scanned int64

	for {
		line, err := reader.ReadString('\n')
		scanned += int64(len(line))
		if scanBudget > 0 && scanned > scanBudget {
			return beforeAttempt{}, errScanBudget
		}
		eof := err == io.EOF
		if err != nil && !eof {
			return beforeAttempt{}, err
		}
		line = strings.TrimSuffix(line, "\n")
		if eof && line == "" {
			break
		}
		if len(line) >= logTimestampLen && line[:logTimestampLen] >= cutoffPrefix {
			break
		}

		res.total++
		ring = append(ring, line)
		ringBytes += int64(len(line))
		for len(ring) > keep || (ringBytes > pageBytes && len(ring) > 1) {
			if len(ring) <= keep {
				res.trimmed = true
			}
			ringBytes -= int64(len(ring[0]))
			ring[0] = ""
			ring = ring[1:]
		}
		// Re-slicing keeps the backing array alive; compact now and then.
		if cap(ring) > 4*keep+64 && len(ring) < cap(ring)/4 {
			ring = append([]string(nil), ring...)
		}
		if eof {
			break
		}
	}
	res.lines = ring
	return res, nil
}

// ---------------------------------------------------------------------------
// Merged (multi-container / multi-pod) history pages
// ---------------------------------------------------------------------------

// logTarget is one container log feeding a merged view.
type logTarget struct {
	namespace string
	pod       string
	container string
	prefix    string // text shown in the [prefix] tag of merged lines
}

func containerTargets(podName string, containerNames []string) []logTarget {
	out := make([]logTarget, 0, len(containerNames))
	for _, cn := range containerNames {
		out = append(out, logTarget{pod: podName, container: cn, prefix: cn})
	}
	return out
}

// podTargets mirrors the pod/container expansion of getAllPodsLogs.
func podTargets(pods []PodContainerPair, allContainers bool) []logTarget {
	var out []logTarget
	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 1 {
			for _, cn := range p.ContainerNames {
				out = append(out, logTarget{pod: p.PodName, container: cn, prefix: p.PodName + "/" + cn})
			}
			continue
		}
		cn := ""
		prefix := p.PodName
		if len(p.ContainerNames) > 0 {
			cn = p.ContainerNames[0]
			if allContainers {
				prefix = p.PodName + "/" + cn
			}
		}
		out = append(out, logTarget{pod: p.PodName, container: cn, prefix: prefix})
	}
	return out
}

type targetPage struct {
	target logTarget
	lines  []string // timestamped lines as returned by the API
	more   bool
	err    error
}

// fetchTargetPages runs fetch for every target with bounded concurrency,
// preserving target order in the result.
func fetchTargetPages(targets []logTarget, fetch func(logTarget) ([]string, bool, error)) []targetPage {
	pages := make([]targetPage, len(targets))
	sem := make(chan struct{}, logHistoryConcurrency)
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, t logTarget) {
			defer wg.Done()
			defer func() { <-sem }()
			lines, more, err := fetch(t)
			pages[i] = targetPage{target: t, lines: lines, more: more, err: err}
		}(i, t)
	}
	wg.Wait()
	return pages
}

// mergeTargetPages merges per-target pages into one chronologically ordered list
// of "<ts> [prefix] content" lines. Per-target errors become trailing error lines
// (as in the full-log merges); a too-large page aborts the whole request.
func mergeTargetPages(pages []targetPage) (merged []timestampedLogLine, anyMore bool, errLines []string, err error) {
	for _, p := range pages {
		if errors.Is(p.err, errLogPageTooLarge) {
			return nil, false, nil, p.err
		}
		if p.err != nil {
			errLines = append(errLines, fmt.Sprintf("[%s] Error fetching logs: %v", p.target.prefix, p.err))
			continue
		}
		anyMore = anyMore || p.more
		for _, line := range p.lines {
			if line == "" {
				continue
			}
			ts, content := "", line
			if len(line) >= logTimestampLen+1 && line[logTimestampLen] == ' ' {
				ts, content = line[:logTimestampLen], line[logTimestampLen+1:]
			}
			full := fmt.Sprintf("[%s] %s", p.target.prefix, content)
			if ts != "" {
				full = ts + " " + full
			}
			merged = append(merged, timestampedLogLine{timestamp: ts, content: full})
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].timestamp < merged[j].timestamp })
	return merged, anyMore, errLines, nil
}

func renderMergedLines(lines []timestampedLogLine, errLines []string, timestamps bool) string {
	out := make([]string, 0, len(lines)+len(errLines))
	for _, l := range lines {
		if timestamps {
			out = append(out, l.content)
		} else {
			out = append(out, stripLogTimestamp(l.content))
		}
	}
	out = append(out, errLines...)
	return strings.Join(out, "\n")
}

func (c *Client) mergedLogsFromStart(targets []logTarget, timestamps, previous bool, lineLimit int) (string, error) {
	if len(targets) == 0 {
		return "", nil
	}
	lineLimit = clampLogLimit(lineLimit)
	pages := fetchTargetPages(targets, func(t logTarget) ([]string, bool, error) {
		page, err := c.readLogHead(t.namespace, t.pod, t.container, true, previous, "", lineLimit)
		if err != nil {
			return nil, false, err
		}
		return strings.Split(strings.TrimSuffix(page, "\n"), "\n"), false, nil
	})
	merged, _, errLines, err := mergeTargetPages(pages)
	if err != nil {
		return "", err
	}
	// The first N merged lines are always within the first N lines of each source.
	if len(merged) > lineLimit {
		merged = merged[:lineLimit]
	}
	return renderMergedLines(merged, errLines, timestamps), nil
}

func (c *Client) mergedLogsBefore(targets []logTarget, timestamps, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	if len(targets) == 0 {
		return "", false, nil
	}
	lineLimit = clampLogLimit(lineLimit)
	pages := fetchTargetPages(targets, func(t logTarget) ([]string, bool, error) {
		return c.logLinesBefore(t.namespace, t.pod, t.container, previous, beforeTime, lineLimit)
	})
	merged, anyMore, errLines, err := mergeTargetPages(pages)
	if err != nil {
		return "", false, err
	}
	// The last N merged lines are always within the last N lines of each source.
	more := anyMore
	if len(merged) > lineLimit {
		merged = merged[len(merged)-lineLimit:]
		more = true
	}
	return renderMergedLines(merged, errLines, timestamps), more, nil
}

// GetAllContainersLogsFromStart returns the first lineLimit lines across the
// containers, merged by timestamp.
func (c *Client) GetAllContainersLogsFromStart(namespace, podName string, containerNames []string, timestamps bool, previous bool, lineLimit int) (string, error) {
	return c.mergedLogsFromStart(withNamespace(containerTargets(podName, containerNames), namespace), timestamps, previous, lineLimit)
}

// GetAllContainersLogsBefore returns up to lineLimit merged lines before beforeTime.
func (c *Client) GetAllContainersLogsBefore(namespace, podName string, containerNames []string, timestamps bool, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	return c.mergedLogsBefore(withNamespace(containerTargets(podName, containerNames), namespace), timestamps, previous, beforeTime, lineLimit)
}

// GetAllPodsLogsFromStart returns the first lineLimit lines across pods, merged by timestamp.
func (c *Client) GetAllPodsLogsFromStart(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, lineLimit int) (string, error) {
	return c.mergedLogsFromStart(withNamespace(podTargets(pods, allContainers), namespace), timestamps, previous, lineLimit)
}

// GetAllPodsLogsBefore returns up to lineLimit merged lines before beforeTime.
func (c *Client) GetAllPodsLogsBefore(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	return c.mergedLogsBefore(withNamespace(podTargets(pods, allContainers), namespace), timestamps, previous, beforeTime, lineLimit)
}

func withNamespace(targets []logTarget, namespace string) []logTarget {
	for i := range targets {
		targets[i].namespace = namespace
	}
	return targets
}
