package acceleratoracceptance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceAcceleratorDependencyClosure(t *testing.T) {
	type goListResult struct {
		GoFiles []string
		Deps    []string
		Error   *struct{ Err string }
	}
	contains := func(values []string, value string) bool {
		for _, candidate := range values {
			if candidate == value {
				return true
			}
		}
		return false
	}
	root := filepath.Join("..", "..")
	tests := []struct {
		name, tags, selected string
		excluded             []string
		wantDesktopDeps      bool
	}{
		{"Accelerator", "headless accelerator", "", []string{"accelerator_dispose_desktop.go", "accelerator_dispose_desktop_stub.go"}, false},
		{"ordinary headless without Helm tag", "headless", "accelerator_dispose_desktop_stub.go", []string{"accelerator_dispose_desktop.go"}, true},
		{"desktop with Helm tag", "helm", "accelerator_dispose_desktop.go", []string{"accelerator_dispose_desktop_stub.go"}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("go", "list", "-e", "-json", "-tags", test.tags, ".")
			command.Dir = root
			command.Env = append(os.Environ(), "GOTOOLCHAIN=go1.24.2", "GOPROXY=off", "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
			output, err := command.Output()
			if err != nil {
				t.Fatal("go list dependency closure")
			}
			var result goListResult
			if json.Unmarshal(output, &result) != nil || result.Error != nil && !strings.Contains(result.Error.Err, "frontend/dist") {
				t.Fatal("go list returned an unexpected package error")
			}
			if test.selected != "" && !contains(result.GoFiles, test.selected) {
				t.Fatalf("selected files omit %s", test.selected)
			}
			for _, excluded := range test.excluded {
				if contains(result.GoFiles, excluded) {
					t.Fatalf("selected files include %s", excluded)
				}
			}
			for _, dependency := range []string{"kubikles/pkg/acceleratorprovision", "kubikles/pkg/helm"} {
				if contains(result.Deps, dependency) != test.wantDesktopDeps {
					t.Fatalf("dependency closure for %s differs", dependency)
				}
			}
		})
	}
}
