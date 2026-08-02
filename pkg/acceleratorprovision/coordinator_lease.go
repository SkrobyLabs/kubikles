package acceleratorprovision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

type SecretDemandLease struct {
	coordinator *Coordinator
	slot        *contextSlot
	demandEpoch uint64
	closed      atomic.Bool
	closeOnce   sync.Once
	stopMu      sync.Mutex
	stopContext func() bool
}

func (*SecretDemandLease) String() string { return "<accelerator secret demand lease>" }
func (*SecretDemandLease) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator secret demand lease>")
}
func (*SecretDemandLease) MarshalJSON() ([]byte, error) {
	return json.Marshal("<accelerator secret demand lease>")
}

func (l *SecretDemandLease) Changes() <-chan struct{} {
	if l == nil || l.slot == nil || l.closed.Load() {
		return closedCoordinatorSignal()
	}
	l.slot.mu.Lock()
	defer l.slot.mu.Unlock()
	if l.closed.Load() || l.slot.demandEpoch != l.demandEpoch || l.slot.state == CoordinatorClosed {
		return closedCoordinatorSignal()
	}
	return l.slot.change
}

func (l *SecretDemandLease) TrySession() (*SessionLease, bool) {
	if l == nil || l.slot == nil || l.closed.Load() {
		return nil, false
	}
	s := l.slot
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.closed.Load() || s.demandEpoch != l.demandEpoch || s.state != CoordinatorActive || s.session == nil {
		return nil, false
	}
	select {
	case <-s.session.terminalStarted():
		return nil, false
	default:
	}
	select {
	case <-s.session.Done():
		return nil, false
	default:
	}
	s.sessionLeaseCount++
	return &SessionLease{coordinator: l.coordinator, slot: s, leaseEpoch: s.sessionLeaseEpoch, session: s.session}, true
}

func (l *SecretDemandLease) Close() {
	if l == nil {
		return
	}
	l.closeOnce.Do(func() {
		l.closed.Store(true)
		l.stopMu.Lock()
		stop := l.stopContext
		l.stopContext = nil
		l.stopMu.Unlock()
		if stop != nil {
			stop()
		}
		if l.coordinator != nil {
			l.coordinator.releaseDemand(l)
		}
	})
}

type SessionLease struct {
	coordinator *Coordinator
	slot        *contextSlot
	leaseEpoch  uint64
	session     *ConnectedSession
	closeOnce   sync.Once
}

func (*SessionLease) String() string { return "<accelerator session lease>" }
func (*SessionLease) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator session lease>")
}
func (*SessionLease) MarshalJSON() ([]byte, error) {
	return json.Marshal("<accelerator session lease>")
}
func (l *SessionLease) Session() *ConnectedSession {
	if l == nil {
		return nil
	}
	return l.session
}
func (l *SessionLease) Close() {
	if l == nil {
		return
	}
	l.closeOnce.Do(func() {
		if l.coordinator != nil {
			l.coordinator.releaseSession(l)
		}
	})
}

var coordinatorClosedSignal = func() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()

func closedCoordinatorSignal() <-chan struct{} { return coordinatorClosedSignal }

func attachDemandContext(ctx context.Context, lease *SecretDemandLease) {
	if ctx == nil || lease == nil || ctx.Done() == nil {
		return
	}
	stop := context.AfterFunc(ctx, lease.Close)
	lease.stopMu.Lock()
	if lease.closed.Load() {
		lease.stopMu.Unlock()
		stop()
		return
	}
	lease.stopContext = stop
	lease.stopMu.Unlock()
}
