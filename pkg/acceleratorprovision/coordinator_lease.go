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
	changes, valid := l.CurrentChanges()
	if !valid {
		return closedCoordinatorSignal()
	}
	return changes
}

// CurrentChanges atomically snapshots the replaceable demand notification
// channel and whether this lease still owns its coordinator demand epoch.
func (l *SecretDemandLease) CurrentChanges() (<-chan struct{}, bool) {
	if l == nil || l.slot == nil || l.closed.Load() {
		return nil, false
	}
	l.slot.mu.Lock()
	defer l.slot.mu.Unlock()
	if l.closed.Load() || l.slot.demandEpoch != l.demandEpoch || l.slot.state == CoordinatorClosed {
		return nil, false
	}
	return l.slot.change, l.slot.change != nil
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
	state := &sessionLeaseState{coordinator: l.coordinator, slot: s, leaseEpoch: s.sessionLeaseEpoch, session: s.session, revoked: s.sessionLeaseRevoked, closedCh: make(chan struct{})}
	return &SessionLease{state: state}, true
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
	state *sessionLeaseState
}

type sessionLeaseState struct {
	coordinator *Coordinator
	slot        *contextSlot
	leaseEpoch  uint64
	session     *ConnectedSession
	revoked     <-chan struct{}
	closedCh    chan struct{}
	closed      atomic.Bool
	closeOnce   sync.Once
	claimMu     sync.Mutex
	claimed     bool
}

func (*SessionLease) String() string { return "<accelerator session lease>" }
func (*SessionLease) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator session lease>")
}
func (*SessionLease) MarshalJSON() ([]byte, error) {
	return json.Marshal("<accelerator session lease>")
}
func (l *SessionLease) Session() *ConnectedSession {
	if l == nil || l.state == nil {
		return nil
	}
	return l.state.currentSession()
}
func (l *SessionLease) Close() {
	if l == nil || l.state == nil {
		return
	}
	state := l.state
	state.closeOnce.Do(func() {
		state.closed.Store(true)
		close(state.closedCh)
		if state.coordinator != nil {
			state.coordinator.releaseSession(state)
		}
	})
}

func (s *sessionLeaseState) currentSession() *ConnectedSession {
	if s == nil || s.closed.Load() || s.slot == nil || s.session == nil {
		return nil
	}
	select {
	case <-s.revoked:
		return nil
	default:
	}
	s.slot.mu.Lock()
	current := !s.closed.Load() && s.slot.sessionLeaseEpoch == s.leaseEpoch && s.slot.state == CoordinatorActive && s.slot.session == s.session
	s.slot.mu.Unlock()
	if !current {
		return nil
	}
	select {
	case <-s.session.terminalStarted():
		return nil
	default:
		return s.session
	}
}

func (l *SessionLease) claimForSecretClient() (*ConnectedSession, <-chan struct{}, <-chan struct{}, bool) {
	if l == nil || l.state == nil {
		return nil, closedCoordinatorSignal(), closedCoordinatorSignal(), false
	}
	state := l.state
	state.claimMu.Lock()
	defer state.claimMu.Unlock()
	if state.claimed {
		return nil, state.revoked, state.closedCh, false
	}
	session := state.currentSession()
	if session == nil {
		return nil, state.revoked, state.closedCh, false
	}
	state.claimed = true
	return session, state.revoked, state.closedCh, true
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
