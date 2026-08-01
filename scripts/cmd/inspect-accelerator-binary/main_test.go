package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectRejectsNondeterministicBuildSettings(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing trimpath", args: []string{"-buildvcs=false", "-ldflags=-s -w -buildid="}, want: "-trimpath=\"\""},
		{name: "Go build ID", args: []string{"-trimpath", "-buildvcs=false", "-ldflags=-s -w -buildid=present"}, want: "unexpected section .note.go.buildid"},
		{name: "extra tag", args: []string{"-trimpath", "-buildvcs=false", "-tags=headless accelerator extra", "-ldflags=-s -w -buildid="}, want: "tags=\"headless,accelerator,extra\""},
		{name: "VCS metadata", args: []string{"-trimpath", "-buildvcs=false", "-tags=headless accelerator", "-ldflags=-s -w -buildid="}, want: "unexpected VCS build setting vcs."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binary := buildFixture(t, tc.args...)
			if tc.name == "VCS metadata" {
				injectVCSSetting(t, binary)
			}
			err := inspect(binary, "amd64", "version", "commit", "dirty")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("inspect error = %v, want %q", err, tc.want)
			}
		})
	}
}

func injectVCSSetting(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	index := strings.LastIndex(string(b), "GOOS")
	if index < 0 {
		t.Fatal("fixture has no GOOS build setting")
	}
	copy(b[index:index+len("GOOS")], "vcs.")
	if err := os.WriteFile(path, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func buildFixture(t *testing.T, args ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture")
	command := append([]string{"build", "-o", path}, args...)
	command = append(command, "./testdata/fixture")
	cmd := exec.Command("go", command...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, output)
	}
	return path
}
