//go:build !headless

package main

import (
	"context"
	"testing"

	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/events"
)

func TestDesktopResolverUsesBuildVersion(t *testing.T) {
	oldBuildVersion := BuildVersion
	oldFactory := desktopAcceleratorReleaseResolverFactory
	t.Cleanup(func() {
		BuildVersion = oldBuildVersion
		desktopAcceleratorReleaseResolverFactory = oldFactory
	})
	BuildVersion = "v1.4.2"
	gotVersion := ""
	desktopAcceleratorReleaseResolverFactory = func(version string) *acceleratorrelease.Resolver {
		gotVersion = version
		return acceleratorrelease.New(version)
	}
	resolver := newDesktopAcceleratorReleaseResolver()
	if resolver == nil || gotVersion != BuildVersion {
		t.Fatalf("resolver=%v version=%q", resolver, gotVersion)
	}
}

func TestAppStartupDoesNotResolveAcceleratorRelease(t *testing.T) {
	configureOrdinaryTestPaths(t)
	oldFactory := desktopAcceleratorReleaseResolverFactory
	t.Cleanup(func() { desktopAcceleratorReleaseResolverFactory = oldFactory })
	factoryCalls := 0
	desktopAcceleratorReleaseResolverFactory = func(string) *acceleratorrelease.Resolver {
		factoryCalls++
		panic("Accelerator release resolver must remain dormant")
	}
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
}
