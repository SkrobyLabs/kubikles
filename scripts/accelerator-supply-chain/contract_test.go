package supplychain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal("repository root")
	}
	return root
}

func TestToolchainAndReleaseIdentityContract(t *testing.T) {
	root := repositoryRoot(t)
	toolchain, err := LoadToolchain(filepath.Join(root, "security", "accelerator-toolchain.json"))
	if err != nil || ValidateToolchain(toolchain) != nil {
		t.Fatal("toolchain contract")
	}
	if chart, err := ValidateReleaseIdentity("v1.4.0", strings.Repeat("a", 40)); err != nil || chart != "1.4.0" {
		t.Fatal("stable identity")
	}
	if chart, err := ValidateReleaseIdentity("v1.4.0-alpha.1", strings.Repeat("a", 40)); err != nil || chart != "1.4.0-alpha.1" {
		t.Fatal("prerelease identity")
	}
	for _, invalid := range []string{"1.4.0", "v01.4.0", "v1.4", "v1.4.0a", "v1.4.0-alpha.01", "latest"} {
		if _, err := ValidateReleaseIdentity(invalid, strings.Repeat("a", 40)); err == nil {
			t.Fatalf("accepted invalid identity %q", invalid)
		}
	}
	assets, err := StableAssetNames("v1.4.0")
	if err != nil || len(assets) != 2 || assets[0] != "kubikles-accelerator-image-linux-amd64-v1.4.0.spdx.json" {
		t.Fatal("stable assets")
	}
	data, _ := os.ReadFile(filepath.Join(root, "security", "accelerator-toolchain.json"))
	for _, mutation := range []struct{ old, replacement string }{
		{"2f5adac4ecd194d9f8c10b7b5d7bceb5186853db1b26e5abd3a657af0b7e26ec", strings.Repeat("0", 64)},
		{"1.44.0", "latest"},
		{"0e91737aee2b5baf1d255b959630194a302335d848ff97bb07921eb6205b5f5a", strings.Repeat("0", 64)},
	} {
		path := filepath.Join(t.TempDir(), "toolchain.json")
		changed := strings.Replace(string(data), mutation.old, mutation.replacement, 1)
		if os.WriteFile(path, []byte(changed), 0o600) != nil {
			t.Fatal("toolchain fixture")
		}
		if _, err := LoadToolchain(path); err == nil {
			t.Fatal("accepted toolchain mutation")
		}
	}
}
