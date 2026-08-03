//go:build !headless && helm && !accelerator

package main

import "github.com/wailsapp/wails/v2/pkg/runtime"

func acceleratorBrowserNavigator(app *App) func(string) bool {
	if app == nil || app.ctx == nil || app.runtimeMode != RuntimeModeDesktop {
		return nil
	}
	return func(target string) bool {
		runtime.BrowserOpenURL(app.ctx, target)
		return true
	}
}
