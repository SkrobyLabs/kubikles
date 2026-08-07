package supplychain

import (
	"path/filepath"
	"testing"
)

func TestWorkflowEventPermissionAuthorityMatrix(t *testing.T) {
	root := repositoryRoot(t)
	toolchain, err := LoadToolchain(filepath.Join(root, "security", "accelerator-toolchain.json"))
	if err != nil || ValidateWorkflows(root, toolchain) != nil {
		t.Fatal("workflow authority contract")
	}
}
