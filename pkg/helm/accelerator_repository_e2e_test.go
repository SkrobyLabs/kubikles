//go:build helm && accelerator_e2e

package helm

import "testing"

func TestAcceleratorRuntimeImageRepositoryE2EOverride(t *testing.T) {
	t.Setenv(acceleratorE2EImageRepositoryEnvironment, "host.docker.internal:49152/skrobylabs/kubikles-accelerator")
	if got := acceleratorRuntimeImageRepository(); got != "host.docker.internal:49152/skrobylabs/kubikles-accelerator" {
		t.Fatalf("local repository = %q", got)
	}
}

func TestAcceleratorRuntimeImageRepositoryE2EOverrideFailsClosed(t *testing.T) {
	for _, repository := range []string{
		"ghcr.io/skrobylabs/kubikles-accelerator",
		"host.docker.internal:65536/skrobylabs/kubikles-accelerator",
		"host.docker.internal:49152/other/image",
	} {
		t.Run(repository, func(t *testing.T) {
			t.Setenv(acceleratorE2EImageRepositoryEnvironment, repository)
			if got := acceleratorRuntimeImageRepository(); got != "" {
				t.Fatalf("invalid repository accepted: %q", got)
			}
		})
	}
}
