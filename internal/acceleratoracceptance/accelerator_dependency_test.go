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
		name, tags         string
		required, excluded []string
		wantDesktopDeps    bool
		requireNoError     bool
	}{
		{"Accelerator", "headless accelerator", nil, []string{"accelerator_dispose_desktop.go", "accelerator_dispose_desktop_stub.go"}, false, false},
		{"ordinary headless without Helm tag", "headless", []string{"accelerator_dispose_desktop_stub.go"}, []string{"accelerator_dispose_desktop.go"}, true, false},
		{"desktop with Helm tag", "helm", []string{"accelerator_dispose_desktop.go", "desktop_assets.go"}, []string{"accelerator_dispose_desktop_stub.go", "desktop_assets_accelerator_e2e.go"}, true, false},
		{"composed desktop acceptance", "helm accelerator_provision_kind accelerator_e2e", []string{"accelerator_dispose_desktop.go", "desktop_assets_accelerator_e2e.go"}, []string{"accelerator_dispose_desktop_stub.go", "desktop_assets.go"}, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("go", "list", "-e", "-json", "-tags", test.tags, ".")
			command.Dir = root
			command.Env = append(os.Environ(), "GOTOOLCHAIN=go1.25.12", "GOPROXY=off", "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
			output, err := command.Output()
			if err != nil {
				t.Fatal("go list dependency closure")
			}
			var result goListResult
			if json.Unmarshal(output, &result) != nil || result.Error != nil && (test.requireNoError || !strings.Contains(result.Error.Err, "frontend/dist")) {
				t.Fatal("go list returned an unexpected package error")
			}
			for _, required := range test.required {
				if !contains(result.GoFiles, required) {
					t.Fatalf("selected files omit %s", required)
				}
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
