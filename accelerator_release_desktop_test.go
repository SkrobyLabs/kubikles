//go:build !headless

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/events"
)

func TestDesktopResolverUsesExactBuildAndStorageRoot(t *testing.T) {
	oldBuildVersion := BuildVersion
	oldConfigDir := desktopUserConfigDir
	oldFactory := desktopAcceleratorReleaseResolverFactory
	t.Cleanup(func() {
		BuildVersion = oldBuildVersion
		desktopUserConfigDir = oldConfigDir
		desktopAcceleratorReleaseResolverFactory = oldFactory
	})
	BuildVersion = "v1.4.2"
	configRoot := filepath.Join(t.TempDir(), "private-config")
	desktopUserConfigDir = func() (string, error) { return configRoot, nil }
	var gotVersion, gotRoot string
	desktopAcceleratorReleaseResolverFactory = func(version, root string) *acceleratorrelease.Resolver {
		gotVersion, gotRoot = version, root
		return acceleratorrelease.New(version, root)
	}
	resolver, err := newDesktopAcceleratorReleaseResolver()
	if err != nil || resolver == nil {
		t.Fatalf("constructor = %v, %v", resolver, err)
	}
	wantRoot := filepath.Join(configRoot, "kubikles", "accelerator", "releases")
	if gotVersion != BuildVersion || gotRoot != wantRoot {
		t.Fatalf("identity = %q, %q; want %q, %q", gotVersion, gotRoot, BuildVersion, wantRoot)
	}
	if _, err := os.Stat(wantRoot); !os.IsNotExist(err) {
		t.Fatalf("construction touched cache: %v", err)
	}
	sentinel := errors.New("config root unavailable")
	desktopUserConfigDir = func() (string, error) { return "", sentinel }
	if resolver, err := newDesktopAcceleratorReleaseResolver(); resolver != nil || !errors.Is(err, sentinel) {
		t.Fatalf("config error = %v, %v", resolver, err)
	}
}

func TestAppStartupDoesNotResolveAcceleratorRelease(t *testing.T) {
	configureOrdinaryTestPaths(t)
	oldFactory := desktopAcceleratorReleaseResolverFactory
	t.Cleanup(func() { desktopAcceleratorReleaseResolverFactory = oldFactory })
	factoryCalls := 0
	desktopAcceleratorReleaseResolverFactory = func(string, string) *acceleratorrelease.Resolver {
		factoryCalls++
		panic("Accelerator release resolver must remain dormant")
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	releaseRoot := filepath.Join(configRoot, "kubikles", "accelerator", "releases")
	app := NewApp()
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
	app.startup(context.Background())
	app.SetEmitter(&events.NoopEmitter{})
	app.runShutdownPhases(context.Background())
	if factoryCalls != 0 {
		t.Fatalf("resolver factory calls = %d", factoryCalls)
	}
	if _, err := os.Stat(releaseRoot); !os.IsNotExist(err) {
		t.Fatalf("startup touched release cache: %v", err)
	}
}
