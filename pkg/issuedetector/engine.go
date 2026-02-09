package issuedetector

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"kubikles/pkg/k8s"
)

// ProgressCallback is called to report scan progress.
type ProgressCallback func(ScanProgress)

// ScanEngine orchestrates resource fetching and rule evaluation.
type ScanEngine struct {
	mu         sync.RWMutex
	builtin    []Rule
	user       []Rule
	rulesDir   string
	onProgress ProgressCallback
}

// NewScanEngine creates a scan engine with built-in rules and loads user rules from rulesDir.
func NewScanEngine(rulesDir string, onProgress ProgressCallback) *ScanEngine {
	e := &ScanEngine{
		rulesDir:   rulesDir,
		onProgress: onProgress,
	}
	e.builtin = registerBuiltinRules()
	e.loadUserRules()
	return e
}

// allRules returns built-in + user rules.
func (e *ScanEngine) allRules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	all := make([]Rule, 0, len(e.builtin)+len(e.user))
	all = append(all, e.builtin...)
	all = append(all, e.user...)
	return all
}

// ListRules returns info about all loaded rules.
func (e *ScanEngine) ListRules() []RuleInfo {
	rules := e.allRules()
	infos := make([]RuleInfo, len(rules))
	builtinIDs := make(map[string]bool)
	e.mu.RLock()
	for _, r := range e.builtin {
		builtinIDs[r.ID()] = true
	}
	e.mu.RUnlock()

	for i, r := range rules {
		infos[i] = RuleInfo{
			ID:          r.ID(),
			Name:        r.Name(),
			Description: r.Description(),
			Severity:    r.Severity(),
			Category:    r.Category(),
			IsBuiltin:   builtinIDs[r.ID()],
			Requires:    r.RequiredResources(),
		}
	}
	return infos
}

// ReloadUserRules reloads YAML rules from disk and returns the updated rule list.
func (e *ScanEngine) ReloadUserRules() []RuleInfo {
	e.loadUserRules()
	return e.ListRules()
}

// RulesDir returns the path to the user rules directory.
func (e *ScanEngine) RulesDir() string {
	return e.rulesDir
}

// RunScan executes a scan against the cluster.
func (e *ScanEngine) RunScan(ctx context.Context, client *k8s.Client, req ScanRequest) (*ScanResult, error) {
	start := time.Now()

	// 1. Filter rules
	allRules := e.allRules()
	disabledSet := make(map[string]bool)
	for _, id := range req.DisabledRules {
		disabledSet[id] = true
	}
	categorySet := make(map[Category]bool)
	for _, c := range req.Categories {
		categorySet[c] = true
	}

	var activeRules []Rule
	for _, r := range allRules {
		if disabledSet[r.ID()] {
			continue
		}
		if len(categorySet) > 0 && !categorySet[r.Category()] {
			continue
		}
		activeRules = append(activeRules, r)
	}

	if len(activeRules) == 0 {
		return &ScanResult{
			Findings:         []Finding{},
			ResourcesFetched: map[string]int{},
			DurationMs:       time.Since(start).Milliseconds(),
		}, nil
	}

	// 2. Collect required resource kinds (deduplicated)
	kindSet := make(map[string]bool)
	for _, r := range activeRules {
		for _, k := range r.RequiredResources() {
			// Skip cluster-scoped resources unless clusterWide is set
			if !req.ClusterWide && isClusterScoped(k) {
				continue
			}
			kindSet[k] = true
		}
	}
	// Always include cluster-scoped if clusterWide
	if req.ClusterWide {
		for _, r := range activeRules {
			for _, k := range r.RequiredResources() {
				kindSet[k] = true
			}
		}
	}
	var kinds []string
	for k := range kindSet {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	// 3. Fetch resources
	e.emitProgress(ScanProgress{
		Phase:       "fetching",
		Description: fmt.Sprintf("Fetching %d resource types...", len(kinds)),
		Percent:     10,
	})

	cache := NewResourceCache(client, req.Namespaces)
	if err := cache.Fetch(ctx, kinds); err != nil {
		return nil, fmt.Errorf("resource fetch failed: %w", err)
	}

	// 4. Build resource counts
	resCounts := make(map[string]int)
	for _, k := range kinds {
		resCounts[k] = cache.ResourceCount(k)
	}

	// 5. Evaluate rules
	e.emitProgress(ScanProgress{
		Phase:       "analyzing",
		Description: fmt.Sprintf("Analyzing with %d rules...", len(activeRules)),
		Percent:     50,
	})

	var findings []Finding
	var scanErrors []string
	scanErrors = append(scanErrors, cache.Errors()...)

	for i, r := range activeRules {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		pct := 50.0 + (float64(i)/float64(len(activeRules)))*45.0
		e.emitProgress(ScanProgress{
			Phase:       "analyzing",
			Description: fmt.Sprintf("Rule %s...", r.ID()),
			Percent:     pct,
		})

		result, err := r.Evaluate(ctx, cache)
		if err != nil {
			log.Printf("[IssueDetector] Rule %s error: %v", r.ID(), err)
			scanErrors = append(scanErrors, fmt.Sprintf("rule %s: %v", r.ID(), err))
			continue
		}
		findings = append(findings, result...)
	}

	// 6. Sort findings by severity
	sort.Slice(findings, func(i, j int) bool {
		si := severityOrder(findings[i].Severity)
		sj := severityOrder(findings[j].Severity)
		if si != sj {
			return si < sj
		}
		return findings[i].RuleID < findings[j].RuleID
	})

	e.emitProgress(ScanProgress{
		Phase:       "complete",
		Description: fmt.Sprintf("Found %d issues", len(findings)),
		Percent:     100,
	})

	return &ScanResult{
		Findings:         findings,
		RulesRun:         len(activeRules),
		ResourcesFetched: resCounts,
		DurationMs:       time.Since(start).Milliseconds(),
		Errors:           scanErrors,
	}, nil
}

func (e *ScanEngine) emitProgress(p ScanProgress) {
	if e.onProgress != nil {
		e.onProgress(p)
	}
}

func (e *ScanEngine) loadUserRules() {
	rules, err := LoadYAMLRules(e.rulesDir)
	if err != nil {
		log.Printf("[IssueDetector] Error loading user rules from %s: %v", e.rulesDir, err)
		rules = nil
	}
	e.mu.Lock()
	e.user = rules
	e.mu.Unlock()
	log.Printf("[IssueDetector] Loaded %d user rules from %s", len(rules), e.rulesDir)
}

// isClusterScoped returns true for resource kinds that are cluster-scoped.
func isClusterScoped(kind string) bool {
	switch kind {
	case "nodes", "pvs", "ingressclasses":
		return true
	default:
		return false
	}
}
