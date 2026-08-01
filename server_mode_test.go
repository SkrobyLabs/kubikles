package main

import (
	"context"
	"embed"
	"errors"
	"testing"

	"kubikles/pkg/k8s"
)

func TestRunServerWithOptionsReturnsAcceleratorConstructionErrorBeforeListen(t *testing.T) {
	want := errors.New("Accelerator client unavailable")
	err := RunServerWithOptions(context.Background(), embed.FS{}, 0, "test", AppOptions{
		Mode: RuntimeModeAccelerator,
		KubernetesClientFactory: func() (*k8s.Client, error) {
			return nil, want
		},
	})
	if !errors.Is(err, ErrAcceleratorClientInitialization) || !errors.Is(err, want) {
		t.Fatalf("RunServerWithOptions() error = %v, want wrapped construction error", err)
	}
}
