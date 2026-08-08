//go:build helm && !accelerator_e2e

package helm

func acceleratorRuntimeArchitecture() string { return "amd64" }
