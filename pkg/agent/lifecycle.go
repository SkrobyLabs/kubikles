package agent

import (
	"context"
	"time"
)

// AcceleratorIdleReconnectGrace is the exact reconnect grace after the final authenticated transport disconnects.
const AcceleratorIdleReconnectGrace = 2 * time.Minute

// DisposableIdleLifecycle supplies the downstream cleanup and natural-exit operations.
type DisposableIdleLifecycle interface {
	ClearSessionAndWatcherState(context.Context) error
	RequestProcessExit()
}

// ExpireDisposableIdle clears session and watcher state before requesting process exit.
// It always requests exit, including when cleanup returns an error.
func ExpireDisposableIdle(ctx context.Context, lifecycle DisposableIdleLifecycle) error {
	err := lifecycle.ClearSessionAndWatcherState(ctx)
	lifecycle.RequestProcessExit()
	return err
}
