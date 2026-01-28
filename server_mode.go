package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"kubikles/pkg/crashlog"
	"kubikles/pkg/events"
	"kubikles/pkg/server"
)

// RunServer starts the HTTP/WebSocket server mode.
// This is shared between the desktop build (with -server flag) and headless builds.
func RunServer(assets embed.FS, port int, label string) {
	fmt.Printf("Kubikles - %s\n", label)
	fmt.Printf("Starting server on port %d...\n", port)

	// Create app instance
	app := NewApp()

	// Create server
	srv := server.New(app, assets, port)

	// Set up the app's event emitter to use the server's WebSocket broadcast
	app.SetEmitter(events.EmitterFunc(func(name string, data ...interface{}) {
		if len(data) > 0 {
			srv.EmitEvent(name, data[0])
		} else {
			srv.EmitEvent(name, nil)
		}
	}))

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Initialize app
	app.startupServerMode(ctx)

	// Run server (blocks until context is cancelled)
	if err := srv.Run(ctx); err != nil {
		crashlog.LogFatal("Server error: %v", err)
	}

	// Clean up resources (port forwards, terminal sessions, hosts file, etc.)
	app.shutdown(ctx)
}
