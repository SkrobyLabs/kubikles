//go:build accelerator_e2e

package acceleratorprovision

import "testing"

func TestAcceleratorWorkloadImageRepositoryE2EOverride(t *testing.T) {
	t.Setenv(acceleratorE2EImageRepositoryEnvironment, "127.0.0.1:49152/skrobylabs/kubikles-accelerator")
	if got := acceleratorWorkloadImageRepository(); got != "127.0.0.1:49152/skrobylabs/kubikles-accelerator" {
		t.Fatalf("local repository = %q", got)
	}
}

func TestAcceleratorWorkloadImageRepositoryE2EOverrideFailsClosed(t *testing.T) {
	for _, repository := range []string{
		"ghcr.io/skrobylabs/kubikles-accelerator",
		"127.0.0.1:0/skrobylabs/kubikles-accelerator",
		"127.0.0.1:49152/other/image",
	} {
		t.Run(repository, func(t *testing.T) {
			t.Setenv(acceleratorE2EImageRepositoryEnvironment, repository)
			if got := acceleratorWorkloadImageRepository(); got != "" {
				t.Fatalf("invalid repository accepted: %q", got)
			}
		})
	}
}
