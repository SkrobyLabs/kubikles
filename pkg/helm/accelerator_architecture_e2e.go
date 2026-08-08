//go:build helm && accelerator_e2e

package helm

import "os"

func acceleratorRuntimeArchitecture() string {
	switch architecture := os.Getenv("KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE"); architecture {
	case "amd64", "arm64":
		return architecture
	default:
		return ""
	}
}
