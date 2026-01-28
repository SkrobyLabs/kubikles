//go:build !headless

package main

import (
	"embed"
	"flag"

	"kubikles/pkg/crashlog"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

var (
	serverMode = flag.Bool("server", false, "Run in server mode (HTTP/WebSocket) instead of desktop app")
	serverPort = flag.Int("port", 8080, "Port for server mode")
)

func main() {
	flag.Parse()

	// Initialize crash logging
	cleanup := crashlog.Init()
	defer cleanup()
	defer crashlog.LogPanic()

	if *serverMode {
		RunServer(assets, *serverPort, "Server Mode")
	} else {
		runDesktopMode()
	}
}
