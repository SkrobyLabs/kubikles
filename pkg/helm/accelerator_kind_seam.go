//go:build helm && accelerator_provision_kind

package helm

import (
	"net/http"
	"sync"
)

var acceleratorRegistrySeamMu sync.Mutex

// SetAcceleratorRegistryHTTPClientForTest is compiled only into the disposable
// Kind smoke. It changes transport delivery only: PullAcceleratorChart still
// receives and validates the fixed production GHCR digest reference.
func SetAcceleratorRegistryTransportForTest(factory func(http.RoundTripper) http.RoundTripper) func() {
	acceleratorRegistrySeamMu.Lock()
	previous := acceleratorRegistryTransportForTest
	acceleratorRegistryTransportForTest = factory
	return func() {
		acceleratorRegistryTransportForTest = previous
		acceleratorRegistrySeamMu.Unlock()
	}
}
