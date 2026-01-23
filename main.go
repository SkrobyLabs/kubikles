package main

import (
	"embed"

	"kubikles/pkg/crashlog"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Initialize crash logging - writes to kubikles.log in config dir
	cleanup := crashlog.Init()
	defer cleanup()
	defer crashlog.LogPanic()

	// Create an instance of the app structure
	app := NewApp()

	// Create application menu with zoom controls
	appMenu := menu.NewMenu()

	// App menu (required for macOS)
	appMenu.Append(menu.AppMenu())

	// Edit menu (standard copy/paste/etc)
	appMenu.Append(menu.EditMenu())

	// View menu with zoom controls and theme selection
	viewMenu := menu.NewMenu()
	viewMenu.Append(menu.Text("Zoom In", keys.CmdOrCtrl("plus"), func(_ *menu.CallbackData) {
		runtime.EventsEmit(app.ctx, "zoom:in")
	}))
	viewMenu.Append(menu.Text("Zoom In", keys.CmdOrCtrl("="), func(_ *menu.CallbackData) {
		runtime.EventsEmit(app.ctx, "zoom:in")
	}))
	viewMenu.Append(menu.Text("Zoom Out", keys.CmdOrCtrl("-"), func(_ *menu.CallbackData) {
		runtime.EventsEmit(app.ctx, "zoom:out")
	}))
	viewMenu.Append(menu.Text("Reset Zoom", keys.CmdOrCtrl("0"), func(_ *menu.CallbackData) {
		runtime.EventsEmit(app.ctx, "zoom:reset")
	}))

	viewMenu.Append(menu.Separator())

	// Theme submenu
	themeMenu := menu.NewMenu()
	themeMenu.Append(menu.Text("Default Dark", nil, func(_ *menu.CallbackData) {
		app.SetTheme("default-dark")
	}))
	themeMenu.Append(menu.Text("Solarized Dark", nil, func(_ *menu.CallbackData) {
		app.SetTheme("solarized-dark")
	}))
	themeMenu.Append(menu.Text("Solarized Midnight", nil, func(_ *menu.CallbackData) {
		app.SetTheme("solarized-midnight")
	}))
	themeMenu.Append(menu.Text("Phosphor", nil, func(_ *menu.CallbackData) {
		app.SetTheme("phosphor")
	}))
	themeMenu.Append(menu.Separator())
	themeMenu.Append(menu.Text("Reload User Themes", nil, func(_ *menu.CallbackData) {
		app.ReloadThemes()
	}))
	themeMenu.Append(menu.Text("Open Themes Folder...", nil, func(_ *menu.CallbackData) {
		app.OpenThemesDir()
	}))
	viewMenu.Append(menu.SubMenu("Theme", themeMenu))

	appMenu.Append(menu.SubMenu("View", viewMenu))

	// Window menu (standard minimize/fullscreen/etc)
	appMenu.Append(menu.WindowMenu())

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Kubikles",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 30, G: 30, B: 30, A: 1},
		WindowStartState: options.Maximised,
		Menu:             appMenu,
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
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
			Theme:                             windows.Dark,
		},
	})

	if err != nil {
		crashlog.LogFatal("Wails error: %v", err)
	}
}
