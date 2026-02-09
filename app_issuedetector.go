package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	goruntime "runtime"

	"kubikles/pkg/issuedetector"
)

// RunIssueScan runs the issue detector scan against the current cluster.
func (a *App) RunIssueScan(namespaces []string, categories []string, disabledRules []string, clusterWide bool) (*issuedetector.ScanResult, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("no Kubernetes client available")
	}
	if a.scanEngine == nil {
		return nil, fmt.Errorf("issue detector not initialized")
	}

	cats := make([]issuedetector.Category, len(categories))
	for i, c := range categories {
		cats[i] = issuedetector.Category(c)
	}

	req := issuedetector.ScanRequest{
		Namespaces:    namespaces,
		Categories:    cats,
		DisabledRules: disabledRules,
		ClusterWide:   clusterWide,
	}

	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()

	return a.scanEngine.RunScan(ctx, a.k8sClient, req)
}

// ListIssueRules returns all loaded issue detection rules.
func (a *App) ListIssueRules() []issuedetector.RuleInfo {
	if a.scanEngine == nil {
		return []issuedetector.RuleInfo{}
	}
	return a.scanEngine.ListRules()
}

// ReloadIssueRules reloads user YAML rules from disk.
func (a *App) ReloadIssueRules() []issuedetector.RuleInfo {
	if a.scanEngine == nil {
		return []issuedetector.RuleInfo{}
	}
	return a.scanEngine.ReloadUserRules()
}

// GetIssueRulesDir returns the path to the user rules directory.
func (a *App) GetIssueRulesDir() string {
	if a.scanEngine == nil {
		return ""
	}
	return a.scanEngine.RulesDir()
}

// OpenIssueRulesDir opens the rules directory in the system file manager.
func (a *App) OpenIssueRulesDir() error {
	dir := a.GetIssueRulesDir()
	if dir == "" {
		return fmt.Errorf("rules directory not configured")
	}

	// Ensure the directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create rules directory: %w", err)
	}

	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}
