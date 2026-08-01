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

	if err := RunServerWithOptions(ctx, assets, port, label, AppOptions{Mode: RuntimeModeServer}); err != nil {
		crashlog.LogFatal("Server error: %v", err)
	}
}

// RunServerWithOptions starts server mode with explicit App composition options.
func RunServerWithOptions(ctx context.Context, assets embed.FS, port int, label string, options AppOptions) error {
	app, err := NewAppWithOptions(options)
	if err != nil {
		return err
	}
	defer app.shutdown(ctx)

	srv := server.New(NewAppMethodCaller(app), assets, port)
	app.SetEmitter(events.EmitterFunc(func(name string, data ...interface{}) {
		if len(data) > 0 {
			srv.EmitEvent(name, data[0])
		} else {
			srv.EmitEvent(name, nil)
		}
	}))
	app.startupServerMode(ctx)
	for _, listener := range app.getDisconnectListeners() {
		srv.AddDisconnectListener(listener)
	}
	return srv.Run(ctx)
}
