package main

import "testing"

func TestAcceleratorNavigationAndAnchors(t *testing.T) {
	root := testRoot(t)
	c, err := loadContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateMarkdown(root, c); err != nil {
		t.Fatal(err)
	}
}
func TestGitHubSlug(t *testing.T) {
	if got := githubSlug("Exact operation `boundary`"); got != "exact-operation-boundary" {
		t.Fatalf("got %q", got)
	}
}
