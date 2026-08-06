package supplychain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestWorkflowEventPermissionAuthorityMatrix(t *testing.T) {
	root := repositoryRoot(t)
	toolchain, err := LoadToolchain(filepath.Join(root, "security", "accelerator-toolchain.json"))
	if err != nil || ValidateWorkflows(root, toolchain) != nil {
		t.Fatal("workflow authority contract")
	}
	for _, name := range []string{"build.yml", "pull-request.yml", "main.yml", "release.yml"} {
		data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", name))
		var parsed any
		if err != nil || yaml.Unmarshal(data, &parsed) != nil {
			t.Fatalf("workflow %s is not valid YAML", name)
		}
	}
	pull, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "pull-request.yml"))
	main, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "main.yml"))
	release, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	build, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "build.yml"))
	for _, text := range []string{string(pull), string(main)} {
		for _, authority := range []string{"packages: write", "contents: write", "id-token: write", "attestations: write", "artifact-metadata: write", "docker/login-action"} {
			if strings.Contains(text, authority) {
				t.Fatalf("read-only workflow contains %q", authority)
			}
		}
	}
	positions := []string{"\n  release-acceptance:", "\n  accelerator-prepare:", "\n  accelerator-publish:", "\n  accelerator-attest:", "\n  release:"}
	previous := -1
	for _, marker := range positions {
		index := strings.Index(string(release), marker)
		if index <= previous {
			t.Fatalf("release authority is not ordered at %s", marker)
		}
		previous = index
	}
	if strings.Count(string(release), "id-token: write") != 1 || strings.Count(string(release), "attestations: write") != 1 || strings.Count(string(release), "artifact-metadata: write") != 1 {
		t.Fatal("release OIDC authority is not exclusive")
	}
	if !strings.Contains(string(build), "if: ${{ inputs.accelerator_supply_chain == true }}") || !strings.Contains(string(release), "accelerator_supply_chain: false") {
		t.Fatal("release repeats reusable Accelerator supply-chain preparation")
	}
}
