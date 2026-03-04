//go:build !headless

package main

import (
	"kubikles/pkg/compressedassets"
	"kubikles/pkg/crashlog"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func runDesktopMode() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application menu
	appMenu := menu.NewMenu()

	// App menu (required for macOS)
	appMenu.Append(menu.AppMenu())

	// Edit menu (standard copy/paste/etc)
	appMenu.Append(menu.EditMenu())

	// Window menu (standard minimize/fullscreen/etc)
	appMenu.Append(menu.WindowMenu())

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Kubikles",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: compressedassets.WailsMiddleware(assets, "frontend/dist"),
		},
		BackgroundColour: &options.RGBA{R: 30, G: 30, B: 30, A: 1},
		WindowStartState: options.Maximised,
		Menu:             appMenu,
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "Kubikles",
				Message: "Kubernetes cluster management",
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableWindowIcon:                 false,
			DisableFramelessWindowDecorations: false,
			WebviewUserDataPath:               "", // Use default (AppData/Local/kubikles)
			WebviewBrowserPath:                "", // Use system WebView2
		},
	})

	if err != nil {
		crashlog.LogFatal("Wails error: %v", err)
	}
}
