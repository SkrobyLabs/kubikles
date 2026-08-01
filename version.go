package main

import "kubikles/pkg/agent"

// Build-time variables, set via ldflags
// Example: go build -ldflags "-X main.BuildVersion=v1.3.0 -X main.GitCommit=abc123 -X main.GitDirty=true"
var (
	// BuildVersion is the exact build identity (set at build time).
	BuildVersion = agent.DefaultBuildVersion

	// GitCommit is the full git commit hash (set at build time)
	GitCommit = ""

	// GitDirty indicates if there were uncommitted changes (set at build time)
	GitDirty = ""
)

// VersionInfo contains version information for the application
type VersionInfo struct {
	Version string `json:"version"` // Exact build version
	Commit  string `json:"commit"`  // Full commit hash (empty for dev)
	IsDirty bool   `json:"isDirty"` // Has uncommitted changes
	IsDev   bool   `json:"isDev"`   // Is dev build
}

// GetVersionInfo returns the current version information
func (a *App) GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version: BuildVersion,
		Commit:  GitCommit,
		IsDirty: GitDirty == "true",
		IsDev:   BuildVersion == agent.DefaultBuildVersion,
	}
}
