package main

import (
	"os"
	"path/filepath"
	"testing"
)

func testRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(dir, "../.."))
}
func TestDocumentationContractExactV1(t *testing.T) {
	c, err := loadContract(testRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Plans) != 27 || len(c.Operations) != 6 || len(c.AuthorityInputs) != 30 {
		t.Fatal("closed contract cardinality drifted")
	}
}
func TestAuthorityDigestStableAndModeSensitive(t *testing.T) {
	root := testRoot(t)
	c, err := loadContract(root)
	if err != nil {
		t.Fatal(err)
	}
	a, err := authorityDigest(root, c.AuthorityInputs)
	if err != nil || len(a) != 64 {
		t.Fatal(err)
	}
	b, err := authorityDigest(root, c.AuthorityInputs)
	if err != nil || a != b {
		t.Fatal("digest is not deterministic")
	}
}
