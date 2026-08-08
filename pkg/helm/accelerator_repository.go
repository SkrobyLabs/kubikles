//go:build helm && !accelerator_e2e

package helm

func acceleratorRuntimeImageRepository() string { return acceleratorImageRepository }
