package main

import (
	"context"
	"errors"
	"log"
	"sync"

	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

var (
	errAcceleratorIdleLifecycleUnbound = errors.New("accelerator idle lifecycle is not bound")
	errAcceleratorIdleLifecycleRebind  = errors.New("accelerator idle lifecycle is already bound")
)

// acceleratorSessionStateCleaner is intentionally the only downstream seam:
// it carries neither authentication state nor watcher protocol data.
type acceleratorSessionStateCleaner interface{ ClearAcceleratorSessionState(context.Context) error }
type acceleratorSessionStateCleanerFunc func(context.Context) error

func (f acceleratorSessionStateCleanerFunc) ClearAcceleratorSessionState(ctx context.Context) error {
	return f(ctx)
}

func (a *App) clearAcceleratorSessionState(context.Context) error {
	if a != nil && a.watcherManager != nil {
		a.watcherManager.StopAll()
	}
	return nil
}

type acceleratorDisposableLifecycle struct {
	mu  sync.Mutex
	srv interface {
		Quiesce()
	}
	browser  *server.BrowserSessionManager
	sessions *server.AcceleratorSessionRegistry
	// closeSessions is captured from the exact bound registry. Keeping the call
	// narrow also permits deterministic error-order tests without a live socket.
	closeSessions func(context.Context) error
	cleaner       acceleratorSessionStateCleaner
	cancel        context.CancelFunc
	bound         bool
	exitOnce      sync.Once
}

var _ agent.DisposableIdleLifecycle = (*acceleratorDisposableLifecycle)(nil)

func (l *acceleratorDisposableLifecycle) bind(srv interface {
	Quiesce()
}, browser *server.BrowserSessionManager, sessions *server.AcceleratorSessionRegistry, cleaner acceleratorSessionStateCleaner) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.bound {
		return errAcceleratorIdleLifecycleRebind
	}
	if srv == nil || browser == nil || sessions == nil || cleaner == nil || l.cancel == nil {
		return errAcceleratorIdleLifecycleUnbound
	}
	l.srv, l.browser, l.sessions, l.cleaner = srv, browser, sessions, cleaner
	l.closeSessions = sessions.Close
	l.bound = true
	return nil
}
func (l *acceleratorDisposableLifecycle) ClearSessionAndWatcherState(ctx context.Context) error {
	l.mu.Lock()
	if !l.bound {
		l.mu.Unlock()
		return errAcceleratorIdleLifecycleUnbound
	}
	srv, browser, closeSessions, cleaner := l.srv, l.browser, l.closeSessions, l.cleaner
	l.mu.Unlock()
	if srv != nil {
		srv.Quiesce()
	}
	if browser != nil {
		browser.RevokeAll(ctx)
	}
	var errs []error
	if closeSessions != nil {
		if err := closeSessions(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if cleaner != nil {
		if err := cleaner.ClearAcceleratorSessionState(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
func (l *acceleratorDisposableLifecycle) RequestProcessExit() {
	l.exitOnce.Do(func() {
		if l.cancel != nil {
			l.cancel()
		}
	})
}
func reportAcceleratorIdleCleanupFailure() { log.Print("Accelerator idle cleanup failed") }
