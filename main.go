//go:build !headless

package main

import (
	"embed"
	"flag"
	"os"
	"path/filepath"
	"runtime"
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

// enrichPATH adds common CLI tool directories to PATH so that exec-based
// kubeconfig credential plugins (aws, gke-gcloud-auth-plugin, etc.) are
// found when the app is launched from Finder/Dock (which inherits a minimal
// GUI-session PATH that excludes Homebrew and user bin dirs).
func enrichPATH() {
	home, _ := os.UserHomeDir()
	if home == "" {
		return
	}

	dirs := []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "go", "bin"),
	}

	switch runtime.GOOS {
	case "darwin":
		dirs = append(dirs,
			"/opt/homebrew/bin",
			"/opt/homebrew/sbin",
			"/usr/local/bin",
			"/usr/local/sbin",
		)
	case "linux":
		dirs = append(dirs,
			"/usr/local/bin",
			"/usr/local/sbin",
			"/snap/bin",
		)
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		programFiles := os.Getenv("ProgramFiles")
		dirs = append(dirs,
			filepath.Join(home, "scoop", "shims"),
			filepath.Join(home, "AppData", "Local", "Microsoft", "WinGet", "Links"),
		)
		if programFiles != "" {
			dirs = append(dirs, filepath.Join(programFiles, "Amazon", "AWSCLIV2"))
		}
		if localAppData != "" {
			dirs = append(dirs, filepath.Join(localAppData, "Google", "Cloud SDK", "google-cloud-sdk", "bin"))
		}
	}

	current := os.Getenv("PATH")
	existing := make(map[string]bool)
	for _, d := range filepath.SplitList(current) {
		existing[d] = true
	}

	var added []string
	for _, d := range dirs {
		if !existing[d] {
			added = append(added, d)
		}
	}

	if len(added) > 0 {
		os.Setenv("PATH", current+string(os.PathListSeparator)+strings.Join(added, string(os.PathListSeparator)))
	}
}

func main() {
	enrichPATH()

	// MCP server mode: run as stdin/stdout JSON-RPC server for Claude CLI
	// Check this before flag.Parse() since MCP mode uses its own arg format
	if len(os.Args) > 1 && os.Args[1] == "--mcp-server" {
		k8sContext := ""
		var allowedTools []string
		var allowedCommands []string
		for i := 2; i < len(os.Args); i++ {
			if os.Args[i] == "--k8s-context" && i+1 < len(os.Args) {
				k8sContext = os.Args[i+1]
				i++
			} else if os.Args[i] == "--allowed-tools" && i+1 < len(os.Args) {
				allowedTools = strings.Split(os.Args[i+1], ",")
				i++
			} else if os.Args[i] == "--allowed-commands" && i+1 < len(os.Args) {
				if val := os.Args[i+1]; val != "" {
					allowedCommands = strings.Split(val, "|")
				} else {
					allowedCommands = []string{} // explicitly empty = no commands allowed
				}
				i++
			}
		}
		if err := mcp.RunWithOptions(k8sContext, allowedTools, false, allowedCommands); err != nil {
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
