//go:build headless && accelerator

package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

//go:embed all:frontend/dist
var assets embed.FS

// acceleratorBuildIdentity is an exact, unambiguous build-time marker used by
// the deterministic binary inspector. Runtime identity continues to use the
// settled BuildVersion/GitCommit/GitDirty values below.
var acceleratorBuildIdentity = "kubikles-accelerator-build-identity:unset"

type acceleratorRun func(context.Context, embed.FS, int, string, AppOptions) error
type acceleratorSignalNotifyContext func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)

func runAccelerator(ctx context.Context, assets embed.FS, args []string, run acceleratorRun, stderr io.Writer, exit func(int)) {
	if acceleratorBuildIdentity == "" {
		fmt.Fprintln(stderr, "accelerator build identity is missing")
		exit(1)
		return
	}
	if len(args) != 0 {
		fmt.Fprintln(stderr, "accelerator does not accept command-line arguments")
		exit(1)
		return
	}
	if err := run(ctx, assets, 8080, "Accelerator", acceleratorAppOptions()); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, "accelerator server failed")
		exit(1)
	}
}

func acceleratorMain(notify acceleratorSignalNotifyContext, assets embed.FS, args []string, run acceleratorRun, stderr io.Writer, exit func(int)) {
	ctx, stop := notify(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runAccelerator(ctx, assets, args, run, stderr, exit)
}

func main() {
	acceleratorMain(signal.NotifyContext, assets, os.Args[1:], RunServerWithOptions, os.Stderr, os.Exit)
}
