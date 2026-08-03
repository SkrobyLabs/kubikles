//go:build !accelerator && (headless || !helm)

package main

func acceleratorBrowserNavigator(*App) func(string) bool { return nil }
