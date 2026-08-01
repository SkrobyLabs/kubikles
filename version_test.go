package main

import (
	"testing"

	"kubikles/pkg/agent"
)

func TestGetVersionInfoUsesBuildVersion(t *testing.T) {
	originalBuildVersion, originalCommit, originalDirty := BuildVersion, GitCommit, GitDirty
	t.Cleanup(func() { BuildVersion, GitCommit, GitDirty = originalBuildVersion, originalCommit, originalDirty })
	BuildVersion, GitCommit, GitDirty = "v1.3.0+opaque", "abcdef0123456789", "true"

	info := (&App{}).GetVersionInfo()
	if info.Version != "v1.3.0+opaque" || info.Commit != "abcdef0123456789" || !info.IsDirty || info.IsDev {
		t.Fatalf("version info = %+v", info)
	}
}

func TestGetVersionInfoDevBuild(t *testing.T) {
	if BuildVersion != "dev" {
		t.Fatalf("unmodified BuildVersion = %q, want literal dev", BuildVersion)
	}
	if BuildVersion != agent.DefaultBuildVersion {
		t.Fatalf("unmodified BuildVersion = %q, want agent.DefaultBuildVersion", BuildVersion)
	}
	originalBuildVersion, originalCommit, originalDirty := BuildVersion, GitCommit, GitDirty
	t.Cleanup(func() { BuildVersion, GitCommit, GitDirty = originalBuildVersion, originalCommit, originalDirty })
	GitCommit, GitDirty = "abcdef0123456789", "true"

	info := (&App{}).GetVersionInfo()
	if info.Version != "dev" || info.Commit != "abcdef0123456789" || !info.IsDirty || !info.IsDev {
		t.Fatalf("version info = %+v", info)
	}
}
