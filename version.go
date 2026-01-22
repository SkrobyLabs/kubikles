package main

// Build-time variables, set via ldflags
// Example: go build -ldflags "-X main.GitCommit=abc123 -X main.GitDirty=true"
var (
	// GitCommit is the full git commit hash (set at build time)
	GitCommit = ""

	// GitDirty indicates if there were uncommitted changes (set at build time)
	GitDirty = ""
)

// VersionInfo contains version information for the application
type VersionInfo struct {
	Version string `json:"version"` // Short version: "dev", or 8-char commit hash
	Commit  string `json:"commit"`  // Full commit hash (empty for dev)
	IsDirty bool   `json:"isDirty"` // Has uncommitted changes
	IsDev   bool   `json:"isDev"`   // Is dev build (no commit info)
}

// GetVersionInfo returns the current version information
func (a *App) GetVersionInfo() VersionInfo {
	// If no commit info, this is a dev build
	if GitCommit == "" {
		return VersionInfo{
			Version: "dev",
			Commit:  "",
			IsDirty: false,
			IsDev:   true,
		}
	}

	// Shorten commit to 8 chars for display
	shortCommit := GitCommit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}

	return VersionInfo{
		Version: shortCommit,
		Commit:  GitCommit,
		IsDirty: GitDirty == "true",
		IsDev:   false,
	}
}
