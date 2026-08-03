//go:build !accelerator

package main

import "kubikles/pkg/acceleratorprovision"

var acceleratorBrowserNavigatorFactory = acceleratorBrowserNavigator

// OpenAcceleratorBrowser exposes no arguments or launch material. All URL and
// credential handling remains inside the desktop coordinator call.
func (a *App) OpenAcceleratorBrowser() acceleratorprovision.BrowserOpenResult {
	if a == nil || a.runtimeMode != RuntimeModeDesktop || a.ctx == nil || a.GetCurrentContext() == "" || a.acceleratorLifecycle == nil {
		return acceleratorprovision.BrowserUnavailable
	}
	navigator := acceleratorBrowserNavigatorFactory(a)
	if navigator == nil {
		return acceleratorprovision.BrowserUnavailable
	}
	return a.acceleratorLifecycle.OpenAcceleratorBrowser(a.ctx, navigator)
}
