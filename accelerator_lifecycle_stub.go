//go:build !accelerator && (headless || !helm)

package main

func initializeDesktopAcceleratorLifecycle(app *App) {
	if app != nil {
		app.acceleratorLifecycle = directOnlyAcceleratorCoordinator{}
	}
}
