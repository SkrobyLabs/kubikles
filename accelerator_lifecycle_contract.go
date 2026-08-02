//go:build !accelerator

package main

import (
	"context"

	"kubikles/pkg/acceleratorprovision"
)

type desktopAcceleratorCoordinator interface {
	RuntimeLifecycle
	AcquireSecretDemand(context.Context, string) acceleratorprovision.DemandResult
	FenceContextSwitch(string)
	ContextSwitched(string, bool)
}

type directOnlyAcceleratorCoordinator struct{}

func (directOnlyAcceleratorCoordinator) AcquireSecretDemand(context.Context, string) acceleratorprovision.DemandResult {
	return acceleratorprovision.DemandResult{Reason: acceleratorprovision.DemandRuntimeClosing}
}
func (directOnlyAcceleratorCoordinator) FenceContextSwitch(string)     {}
func (directOnlyAcceleratorCoordinator) ContextSwitched(string, bool)  {}
func (directOnlyAcceleratorCoordinator) Quiesce(context.Context)       {}
func (directOnlyAcceleratorCoordinator) StopProducers(context.Context) {}
func (directOnlyAcceleratorCoordinator) Close(context.Context)         {}

type chainedRuntimeLifecycle struct {
	accelerator RuntimeLifecycle
	next        RuntimeLifecycle
}

func (l chainedRuntimeLifecycle) Quiesce(ctx context.Context) {
	l.accelerator.Quiesce(ctx)
	l.next.Quiesce(ctx)
}
func (l chainedRuntimeLifecycle) StopProducers(ctx context.Context) {
	l.accelerator.StopProducers(ctx)
	l.next.StopProducers(ctx)
}
func (l chainedRuntimeLifecycle) Close(ctx context.Context) {
	l.accelerator.Close(ctx)
	l.next.Close(ctx)
}
