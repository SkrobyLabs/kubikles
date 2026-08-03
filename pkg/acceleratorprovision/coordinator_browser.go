package acceleratorprovision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

type BrowserOpenResult string

const (
	BrowserOpening     BrowserOpenResult = "opening"
	BrowserOpened      BrowserOpenResult = "opened"
	BrowserAlreadyOpen BrowserOpenResult = "already_open"
	BrowserUnavailable BrowserOpenResult = "unavailable"
)

func (BrowserOpenResult) String() string { return "<accelerator browser result>" }
func (r BrowserOpenResult) MarshalJSON() ([]byte, error) {
	switch r {
	case BrowserOpening, BrowserOpened, BrowserAlreadyOpen, BrowserUnavailable:
		return json.Marshal(string(r))
	default:
		return json.Marshal(string(BrowserUnavailable))
	}
}

type coordinatorBrowserLaunch struct {
	epoch  uint64
	done   chan struct{}
	result BrowserOpenResult
}

type coordinatorBrowserHold struct {
	epoch      uint64
	session    *ConnectedSession
	updates    <-chan browserLaunchStatus
	unregister func()
}

type coordinatorBrowserDisposer interface {
	DisposeAfterBrowser(context.Context, *browserOwnedWorkload) DisposalResult
}

// OpenAcceleratorBrowser serializes one private launch for the exact current
// slot. The navigator receives the transient fragment URL and returns only
// whether the platform accepted the open request.
func (c *Coordinator) OpenAcceleratorBrowser(ctx context.Context, navigator func(string) bool) BrowserOpenResult {
	if c == nil || ctx == nil || navigator == nil {
		return BrowserUnavailable
	}
	c.mu.Lock()
	if c.closed || c.quiesced || c.switching || c.currentName == "" {
		c.mu.Unlock()
		return BrowserUnavailable
	}
	contextName := c.currentName
	c.mu.Unlock()
	demand := c.AcquireSecretDemand(ctx, contextName)
	if !demand.Accepted || demand.Lease == nil {
		return BrowserUnavailable
	}
	defer demand.Lease.Close()
	s := demand.Lease.slot

	s.mu.Lock()
	if s.state == CoordinatorBrowserOwned || s.browserOwned != nil || s.browserHold != nil {
		s.mu.Unlock()
		return BrowserAlreadyOpen
	}
	if existing := s.launch; existing != nil {
		done := existing.done
		s.mu.Unlock()
		select {
		case <-done:
			return existing.result
		case <-ctx.Done():
			return BrowserUnavailable
		}
	}
	s.launchEpoch++
	launch := &coordinatorBrowserLaunch{epoch: s.launchEpoch, done: make(chan struct{}), result: BrowserUnavailable}
	s.launch = launch
	s.mu.Unlock()

	result := c.runBrowserLaunch(ctx, demand.Lease, launch, navigator)
	s.mu.Lock()
	if s.launch == launch {
		s.launch = nil
		launch.result = result
		close(launch.done)
	}
	s.mu.Unlock()
	return result
}

func (c *Coordinator) runBrowserLaunch(ctx context.Context, demand *SecretDemandLease, launch *coordinatorBrowserLaunch, navigator func(string) bool) BrowserOpenResult {
	s := demand.slot
	var sessionLease *SessionLease
	for sessionLease == nil {
		if candidate, ok := demand.TrySession(); ok {
			sessionLease = candidate
			break
		}
		change := demand.Changes()
		select {
		case <-ctx.Done():
			return BrowserUnavailable
		case <-change:
		}
		s.mu.Lock()
		terminal := s.state == CoordinatorClosed || s.state == CoordinatorBrowserOwned || s.launch != launch
		s.mu.Unlock()
		if terminal {
			return BrowserUnavailable
		}
	}
	defer sessionLease.Close()
	session := sessionLease.Session()
	if !c.browserLaunchCurrent(s, launch, session) {
		return BrowserUnavailable
	}
	ticket, err := session.mintBrowserLaunch(ctx)
	if err != nil || !c.browserLaunchCurrent(s, launch, session) {
		session.revokeBrowserLaunch(ctx)
		return BrowserUnavailable
	}
	updates, unregister, ok := session.registerBrowserLaunchWaiter(ticket.receipt)
	if !ok {
		session.revokeBrowserLaunch(ctx)
		return BrowserUnavailable
	}
	keepWaiter := false
	defer func() {
		if !keepWaiter {
			unregister()
		}
	}()
	if !c.browserLaunchCurrent(s, launch, session) {
		session.revokeBrowserLaunch(ctx)
		return BrowserUnavailable
	}
	target, ok := session.browserLaunchURL(ticket)
	if !ok {
		session.revokeBrowserLaunch(ctx)
		return BrowserUnavailable
	}
	s.navigationMu.Lock()
	current := c.browserLaunchCurrent(s, launch, session)
	opened := current && navigator(target)
	target = ""
	s.navigationMu.Unlock()
	if !opened {
		session.revokeBrowserLaunch(ctx)
		return BrowserUnavailable
	}
	timer := time.NewTimer(time.Until(ticket.expiresAt))
	defer timer.Stop()
	select {
	case status, open := <-updates:
		if !open || status != browserLaunchConfirmed {
			session.revokeBrowserLaunch(ctx)
			return BrowserUnavailable
		}
	case <-timer.C:
		session.revokeBrowserLaunch(ctx)
		return BrowserUnavailable
	case <-ctx.Done():
		session.revokeBrowserLaunch(context.WithoutCancel(ctx))
		return BrowserUnavailable
	}
	if !c.browserLaunchCurrent(s, launch, session) {
		session.revokeBrowserLaunch(ctx)
		return BrowserUnavailable
	}
	hold := &coordinatorBrowserHold{epoch: launch.epoch, session: session, updates: updates, unregister: unregister}
	s.mu.Lock()
	if s.launch != launch || s.launchEpoch != launch.epoch || s.state != CoordinatorActive || s.session != session || s.browserHold != nil {
		s.mu.Unlock()
		session.revokeBrowserLaunch(ctx)
		return BrowserUnavailable
	}
	s.browserHold = hold
	s.mu.Unlock()
	keepWaiter = true
	go c.monitorBrowserHold(s, hold)
	return BrowserOpened
}

func (c *Coordinator) browserLaunchCurrent(s *contextSlot, launch *coordinatorBrowserLaunch, session *ConnectedSession) bool {
	if !c.isCurrentSlot(s) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.launch == launch && s.launchEpoch == launch.epoch && s.state == CoordinatorActive && s.session == session && s.workload != nil && s.browserHold == nil
}

func (c *Coordinator) monitorBrowserHold(s *contextSlot, hold *coordinatorBrowserHold) {
	for status := range hold.updates {
		if status != browserLaunchEnded {
			continue
		}
		s.mu.Lock()
		if s.browserHold == hold && s.state == CoordinatorActive && s.session == hold.session {
			s.browserHold = nil
			hold.unregister()
			if s.demandCount == 0 && s.sessionLeaseCount == 0 {
				s.settleDemandLocked()
				c.startIdleLocked(s)
			}
		}
		s.mu.Unlock()
		return
	}
}

func (c *Coordinator) startBrowserHandoffLocked(s *contextSlot) {
	if s == nil || s.state != CoordinatorActive || s.demandCount != 0 || s.sessionLeaseCount != 0 || s.session == nil || s.workload == nil || s.browserHold == nil {
		return
	}
	hold := s.browserHold
	handoff := browserHandoffFence{contextEpoch: s.contextEpoch, workload: s.workload, session: s.session, hold: hold}
	s.sessionLeaseEpoch++
	s.signalLocked()
	c.startWorkerLocked(s, CoordinatorBrowserOwned, func(ctx context.Context, fence operationFence) {
		exact := handoff
		exact.operationEpoch = fence.operationEpoch
		c.runBrowserHandoff(ctx, s, exact)
	})
}

func (c *Coordinator) runBrowserHandoff(ctx context.Context, s *contextSlot, fence browserHandoffFence) {
	s.mu.Lock()
	if !s.matchesBrowserHandoffLocked(fence) {
		s.mu.Unlock()
		return
	}
	workload, session, hold := fence.workload, fence.session, fence.hold
	s.mu.Unlock()
	owned, ok := session.detachCreatorForBrowser(ctx)
	s.mu.Lock()
	if !s.matchesBrowserHandoffLocked(fence) {
		s.mu.Unlock()
		if ok && owned != nil {
			owned.tunnel.Stop()
		}
		return
	}
	hold.unregister()
	s.browserHold = nil
	if !ok || owned == nil {
		s.state = CoordinatorDisposing
		s.mu.Unlock()
		c.disposeAndContinue(s, operationFence{contextEpoch: fence.contextEpoch}, workload, false, true)
		return
	}
	s.session = nil
	s.browserOwned = owned
	s.state = CoordinatorBrowserOwned
	s.signalLocked()
	s.mu.Unlock()
	c.coordinateBrowserDisposal(s, fence, owned)
}

func (c *Coordinator) coordinateBrowserDisposal(s *contextSlot, fence browserHandoffFence, owned *browserOwnedWorkload) {
	disposer, ok := c.disposer.(coordinatorBrowserDisposer)
	if !ok || owned == nil || owned.workload == nil {
		return
	}
	workload := owned.workload
	c.disposalMu.Lock()
	ownerEpoch, valid := c.validateDisposalOwnerLocked(s, workload)
	if !valid {
		c.disposalMu.Unlock()
		return
	}
	if existing := c.disposals[workload]; existing != nil {
		c.registerDisposalOwnerLocked(existing, s, ownerEpoch)
		done := existing.done
		c.disposalMu.Unlock()
		go c.continueAfterBrowserDisposal(s, fence, done)
	} else {
		operation := newCoordinatorDisposal(true)
		c.registerDisposalOwnerLocked(operation, s, ownerEpoch)
		c.disposals[workload] = operation
		c.disposalMu.Unlock()
		go func() {
			_ = disposer.DisposeAfterBrowser(context.Background(), owned)
			operation.markFenced()
			c.finishCoordinatorDisposal(workload, operation)
		}()
		go c.continueAfterBrowserDisposal(s, fence, operation.done)
	}
	return
}

func (c *Coordinator) continueAfterBrowserDisposal(s *contextSlot, fence browserHandoffFence, done <-chan struct{}) {
	<-done
	current := c.isCurrentSlot(s)
	s.mu.Lock()
	if s.contextEpoch == fence.contextEpoch && s.workload == nil && s.state == CoordinatorBrowserOwned {
		s.state = CoordinatorDirectOnly
		if current && s.demandCount > 0 {
			c.startActivationLocked(s, 0)
		}
	}
	s.mu.Unlock()
}

func (r BrowserOpenResult) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator browser result>")
}
