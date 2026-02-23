package main

import (
	"time"

	"kubikles/pkg/debug"
)

// =============================================================================
// Configuration
// =============================================================================

// SetEventCoalescerFrameInterval updates the frame interval for resource event batching.
// Value is in milliseconds, clamped to 1-100ms. Default is 16ms (~60fps).
// Lower values = more responsive but more CPU usage.
// Higher values = less CPU usage but more latency.
func (a *App) SetEventCoalescerFrameInterval(ms int) {
	if a.eventCoalescer != nil {
		a.eventCoalescer.SetFrameInterval(ms)
	}
}

// SetK8sAPITimeout sets the timeout for Kubernetes API calls.
// Accepts timeout in milliseconds. Default is 60000ms (60 seconds).
func (a *App) SetK8sAPITimeout(ms int) {
	if a.k8sClient != nil && ms > 0 {
		a.k8sClient.SetAPITimeout(time.Duration(ms) * time.Millisecond)
	}
}

// SetForceHTTP1 enables or disables forcing HTTP/1.1 instead of HTTP/2.
// HTTP/1.1 opens multiple connections for parallel requests, avoiding
// HTTP/2 flow control bottlenecks. Requires context switch to take effect.
func (a *App) SetForceHTTP1(enabled bool) {
	if a.k8sClient != nil {
		a.k8sClient.SetForceHTTP1(enabled)
		debug.LogConfig("Force HTTP/1.1", map[string]interface{}{"enabled": enabled})
	}
}

// SetClientPoolSize sets the number of clientsets in the rotation pool.
// More clients = more parallel HTTP/2 connections. Set to 0 to disable pooling.
// Requires context switch to take effect.
func (a *App) SetClientPoolSize(size int) {
	if a.k8sClient != nil {
		a.k8sClient.SetClientPoolSize(size)
		debug.LogConfig("Client pool size", map[string]interface{}{"size": size})
	}
}

// TestEmit emits a test debug log event
func (a *App) TestEmit() {
	debug.LogConfig("TestEmit called from frontend", nil)
}
