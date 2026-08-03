package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceleratorTerminologyAndClaims(t *testing.T) {
	root := testRoot(t)
	c, err := loadContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateClaims(root, c); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentationContractMatchesMergedAuthorities(t *testing.T) {
	root := testRoot(t)
	c, err := loadContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAuthorities(root, c); err != nil {
		t.Fatal(err)
	}
}

func TestMakeAndCIContract(t *testing.T) {
	root := testRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "test: test-frontend test-accelerator-docs") || !strings.Contains(s, "test-accelerator-docs:") || !strings.Contains(s, "capture-accelerator-docs-evidence:") {
		t.Fatal("Make integration drifted")
	}
	workflow, err := os.ReadFile(filepath.Join(root, ".github/workflows/build.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(workflow), "make test") || strings.Contains(string(workflow), "capture-accelerator-docs-evidence") {
		t.Fatal("CI docs contract drifted")
	}
}
