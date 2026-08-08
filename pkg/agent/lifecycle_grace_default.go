//go:build !accelerator_e2e

package agent

import "time"

// EffectiveAcceleratorIdleReconnectGrace is deliberately separate from the
// exported production contract. Ordinary and release binaries never consult
// the acceptance environment.
func EffectiveAcceleratorIdleReconnectGrace() time.Duration {
	return AcceleratorIdleReconnectGrace
}
