//go:build headless && accelerator

package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"kubikles/pkg/server"
)

//go:embed all:frontend/dist
var acceleratorTestAssets embed.FS

func TestAcceleratorMainUsesFixedComposition(t *testing.T) {
	called := 0
	var output strings.Builder
	runAccelerator(context.Background(), acceleratorTestAssets, nil, func(_ context.Context, injected embed.FS, port int, label string, options AppOptions) error {
		called++
		if port != 8080 || label != "Accelerator" || options.Mode != RuntimeModeAccelerator {
			t.Fatalf("unexpected fixed composition: %d %q %#v", port, label, options)
		}
		if options.KubernetesClientFactory == nil || reflect.ValueOf(options.KubernetesClientFactory).Pointer() != reflect.ValueOf(acceleratorKubernetesClientFactory).Pointer() {
			t.Fatal("dedicated main does not use the exact in-cluster Kubernetes client factory")
		}
		if options.AgentRouter != nil || options.Lifecycle != nil {
			t.Fatalf("dedicated main supplied ordinary composition seams: %#v", options)
		}
		assertAcceleratorEmbeddedComposition(t, injected)
		serverOptions, err := serverOptionsForRuntime(options.Mode, port, nil)
		if err != nil || serverOptions.BoundaryMode != server.BoundaryModeAccelerator || serverOptions.ListenAddress != "127.0.0.1:8080" || serverOptions.ProtectedRouteGuard == nil {
			t.Fatalf("dedicated main boundary is not fail closed: %#v, %v", serverOptions, err)
		}
		return nil
	}, &output, func(int) { t.Fatal("exit called") })
	if called != 1 {
		t.Fatalf("runner calls = %d", called)
	}
	if output.Len() != 0 {
		t.Fatalf("successful run wrote stderr: %q", output.String())
	}
}

func assertAcceleratorEmbeddedComposition(t *testing.T, injected embed.FS) {
	t.Helper()
	paths := []string{
		"frontend/dist/index.html",
		"frontend/dist/accelerator-browser/.kubikles-browser-v1.json",
		"frontend/dist/accelerator-browser/assets/browser.js",
		"frontend/dist/accelerator-browser/assets/browser.css",
	}
	for _, path := range paths {
		got, err := injected.ReadFile(path)
		if err != nil || len(got) == 0 {
			t.Fatalf("injected asset %s = %d bytes, %v", path, len(got), err)
		}
		want, err := assets.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("injected asset %s differs from production embed: %v", path, err)
		}
	}
	markerBytes, err := injected.ReadFile("frontend/dist/accelerator-browser/.kubikles-browser-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var marker struct {
		SchemaVersion int    `json:"schemaVersion"`
		Kind          string `json:"kind"`
		BuildVersion  string `json:"buildVersion"`
	}
	if err := json.Unmarshal(markerBytes, &marker); err != nil {
		t.Fatalf("decode Browser marker: %v", err)
	}
	if marker.SchemaVersion != 1 || marker.Kind != "kubikles-accelerator-browser" || marker.BuildVersion != BuildVersion {
		t.Fatalf("Browser marker = %#v, want schema 1, exact kind, BuildVersion %q", marker, BuildVersion)
	}
	foundOrdinaryGzip := false
	err = fs.WalkDir(injected, "frontend/dist", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".gz") && !strings.Contains(path, "/accelerator-browser/") {
			contents, readErr := injected.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if len(contents) < 2 || contents[0] != 0x1f || contents[1] != 0x8b {
				t.Fatalf("ordinary embedded asset %s is not gzip", path)
			}
			foundOrdinaryGzip = true
		}
		return nil
	})
	if err != nil || !foundOrdinaryGzip {
		t.Fatalf("ordinary embedded gzip evidence = %v, found=%t", err, foundOrdinaryGzip)
	}
}

func TestAcceleratorMainRejectsArgumentsAndPropagatesFailures(t *testing.T) {
	var output strings.Builder
	exits := 0
	runAccelerator(context.Background(), acceleratorTestAssets, []string{"--port=9"}, func(context.Context, embed.FS, int, string, AppOptions) error { t.Fatal("runner called"); return nil }, &output, func(code int) {
		if code != 1 {
			t.Fatalf("exit %d", code)
		}
		exits++
	})
	if exits != 1 || !strings.Contains(output.String(), "does not accept") {
		t.Fatalf("argument result: exits=%d output=%q", exits, output.String())
	}
	output.Reset()
	runAccelerator(context.Background(), acceleratorTestAssets, nil, func(context.Context, embed.FS, int, string, AppOptions) error { return context.Canceled }, &output, func(int) { t.Fatal("cancellation exited") })
	if output.Len() != 0 {
		t.Fatalf("cancellation wrote stderr: %q", output.String())
	}
	output.Reset()
	runAccelerator(context.Background(), acceleratorTestAssets, nil, func(context.Context, embed.FS, int, string, AppOptions) error { return errors.New("secret sentinel") }, &output, func(code int) {
		if code != 1 {
			t.Fatal(code)
		}
		exits++
	})
	if exits != 2 {
		t.Fatalf("failure exit count = %d", exits)
	}
	if got := output.String(); got != "accelerator server failed\n" || strings.Contains(got, "secret sentinel") {
		t.Fatalf("failure stderr = %q", got)
	}
}

func TestAcceleratorSignalContextCancels(t *testing.T) {
	deliver := make(chan os.Signal, 1)
	runnerStarted := make(chan struct{})
	cleanupReturned := make(chan struct{})
	mainReturned := make(chan struct{})
	var notified []os.Signal
	stopCalled := false
	notify := func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
		notified = append([]os.Signal(nil), signals...)
		ctx, cancel := context.WithCancel(parent)
		go func() {
			select {
			case signal := <-deliver:
				if signal == syscall.SIGTERM {
					cancel()
				}
			case <-ctx.Done():
			}
		}()
		return ctx, func() {
			stopCalled = true
			cancel()
		}
	}
	go func() {
		acceleratorMain(notify, acceleratorTestAssets, nil, func(ctx context.Context, _ embed.FS, _ int, _ string, _ AppOptions) error {
			close(runnerStarted)
			<-ctx.Done()
			close(cleanupReturned)
			return ctx.Err()
		}, io.Discard, func(int) { panic("signal cancellation exited") })
		close(mainReturned)
	}()
	<-runnerStarted
	deliver <- syscall.SIGTERM
	select {
	case <-mainReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("dedicated main did not return after SIGTERM cancellation")
	}
	select {
	case <-cleanupReturned:
	default:
		t.Fatal("main returned before runner cleanup")
	}
	if !stopCalled {
		t.Fatal("signal notification cleanup was not called")
	}
	if len(notified) != 2 || notified[0] != os.Interrupt || notified[1] != syscall.SIGTERM {
		t.Fatalf("registered signals = %#v, want SIGINT and SIGTERM", notified)
	}
}
