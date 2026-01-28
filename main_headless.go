//go:build headless

package main

import (
	"embed"
	"flag"

	"kubikles/pkg/crashlog"
)

//go:embed all:frontend/dist
var assets embed.FS

var serverPort = flag.Int("port", 8080, "Port for server")

func main() {
	flag.Parse()

	// Initialize crash logging
	cleanup := crashlog.Init()
	defer cleanup()
	defer crashlog.LogPanic()

	RunServer(assets, *serverPort, "Headless Server Mode")
}
