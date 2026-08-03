//go:build !accelerator

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSecretRPCDormantModeIsolation(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("repository path unavailable")
	}
	root := filepath.Dir(current)
	constructorDefinitions := 0
	constructorCalls := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "dist" || entry.Name() == ".agents") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(content)
		occurrences := strings.Count(text, "NewSecretRPCClient(")
		if occurrences == 0 {
			return nil
		}
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(root, "pkg", "acceleratorprovision", "secret_client.go")) && strings.Contains(text, "func NewSecretRPCClient(") {
			constructorDefinitions++
			occurrences--
		}
		constructorCalls += occurrences
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if constructorDefinitions != 1 || constructorCalls != 0 {
		t.Fatalf("Secret RPC production construction definitions/calls=%d/%d", constructorDefinitions, constructorCalls)
	}
	for _, path := range []string{"agent_router.go", "app.go", "app_configmaps.go", "app_secret_watchers.go"} {
		content, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		if strings.Contains(string(content), "SecretRPCClient") || strings.Contains(string(content), "NewSecretRPCClient") {
			t.Fatalf("%s activates the dormant Accelerator client", path)
		}
	}
}
