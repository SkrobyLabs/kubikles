//go:build !accelerator && (headless || !helm)

package main

import "testing"

func TestDesktopAcceleratorLifecycleStubIsFixedDirectOnly(t *testing.T) {
	app := &App{runtimeMode: RuntimeModeDesktop}
	initializeDesktopAcceleratorLifecycle(app)
	if _, ok := app.acceleratorLifecycle.(directOnlyAcceleratorCoordinator); !ok {
		t.Fatalf("stub=%T", app.acceleratorLifecycle)
	}
}
