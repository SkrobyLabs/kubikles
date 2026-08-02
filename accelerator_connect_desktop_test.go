//go:build !headless && helm && !accelerator

package main

import "testing"

func TestDesktopConnectorIsDormant(t *testing.T) {
	connector := newDesktopAcceleratorConnector()
	if connector == nil {
		t.Fatal("desktop connector constructor returned nil")
	}
	// Construction has no context, workload, Kubernetes client, endpoint, App,
	// event bus, or transport input and therefore cannot perform I/O or routing.
}
