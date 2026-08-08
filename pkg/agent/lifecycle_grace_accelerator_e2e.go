//go:build accelerator_e2e

package agent

import (
	"flag"
	"os"
	"strconv"
	"time"
)

const acceleratorE2EReconnectGraceEnvironment = "KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS"

// EffectiveAcceleratorIdleReconnectGrace is available only to an explicitly
// tagged acceptance binary. It intentionally fails closed: accepting an
// ambient or malformed value would silently weaken the production lifecycle
// contract while a Kind fixture is being mutated.
func EffectiveAcceleratorIdleReconnectGrace() time.Duration {
	// Unit tests retain their controlled-clock proof of the production
	// boundary. The tagged runtime is an executable, never a Go test binary.
	if flag.Lookup("test.v") != nil && os.Getenv("ACCELERATOR_ACCEPTANCE_COMPOSED_KIND") != "1" {
		return AcceleratorIdleReconnectGrace
	}
	raw := os.Getenv(acceleratorE2EReconnectGraceEnvironment)
	if raw == "" || len(raw) > 2 || raw != strconv.Itoa(mustAcceptanceGraceSeconds(raw)) {
		panic("accelerator acceptance reconnect grace invalid")
	}
	return time.Duration(mustAcceptanceGraceSeconds(raw)) * time.Second
}

func mustAcceptanceGraceSeconds(raw string) int {
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 1 || seconds > 30 {
		return -1
	}
	return seconds
}
