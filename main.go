//go:build !headless

package main

import (
	"embed"
	"flag"
	"os"
	"strings"

	"kubikles/pkg/crashlog"
	"kubikles/pkg/mcp"
)

//go:embed all:frontend/dist
var assets embed.FS

var (
	serverMode = flag.Bool("server", false, "Run in server mode (HTTP/WebSocket) instead of desktop app")
	serverPort = flag.Int("port", 8080, "Port for server mode")
)

func main() {
	// MCP server mode: run as stdin/stdout JSON-RPC server for Claude CLI
	// Check this before flag.Parse() since MCP mode uses its own arg format
	if len(os.Args) > 1 && os.Args[1] == "--mcp-server" {
		k8sContext := ""
		var allowedTools []string
		for i := 2; i < len(os.Args); i++ {
			if os.Args[i] == "--k8s-context" && i+1 < len(os.Args) {
				k8sContext = os.Args[i+1]
				i++
			} else if os.Args[i] == "--allowed-tools" && i+1 < len(os.Args) {
				allowedTools = strings.Split(os.Args[i+1], ",")
				i++
			}
		}
		if err := mcp.Run(k8sContext, allowedTools); err != nil {
			os.Exit(1)
		}
		return
	}

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
