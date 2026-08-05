package acceleratorprovision

import (
	"context"
)

func (c *Coordinator) startResumeLocked(s *contextSlot, idle *coordinatorIdleRelease) {
	if s == nil || s.demandCount == 0 || s.workload == nil || s.session == nil || s.state == CoordinatorClosed || s.state == CoordinatorDisposing {
		return
	}
	if idle != nil && (idle.prior != s.session || idle.token == nil) {
		return
	}
	c.startWorkerLocked(s, CoordinatorReconnecting, func(ctx context.Context, fence operationFence) {
		c.runResume(ctx, s, fence, idle)
	})
}

func (c *Coordinator) runResume(ctx context.Context, s *contextSlot, fence operationFence, idle *coordinatorIdleRelease) {
	s.mu.Lock()
	if !s.matchesLocked(fence) || s.demandCount == 0 || s.workload == nil || s.session == nil {
		s.mu.Unlock()
		return
	}
	workload, prior := s.workload, s.session
	s.mu.Unlock()
	request := ResumeRequest{Prior: prior, Workload: workload}
	result := unavailableResume(ResumeInvalid)
	if c.reconnector != nil {
		if idle != nil {
			result = c.reconnector.resumeIdle(ctx, request, idle.token)
		} else {
			result = c.reconnector.Resume(ctx, request)
		}
	}
	if result.Availability == Available && result.Session != nil {
		if c.publishSession(s, fence, workload, result.Session) {
			return
		}
		_ = result.Session.Close(context.Background())
		c.disposeIfRetained(s, workload)
		return
	}
	if result.Session != nil {
		_ = result.Session.Close(context.Background())
	}
	class := classifyResumeFailure(result.Reason)
	c.recordDiagnostic(s, fence, "reconnection", string(result.Reason), 1)
	if !c.disposeRetained(ctx, s, fence, workload) {
		return
	}
	if c.latchReplacementFailure(s, fence) {
		return
	}
	if class == failureVersionMismatch {
		if !c.consumeMismatchRecreate(s, fence) {
			return
		}
		c.runActivation(ctx, s, fence, false, 0)
		return
	}
	if class == failureCancelled || ctx.Err() != nil {
		return
	}
	if class == failureAuthoritative {
		if !c.cooldown(ctx, s, fence) {
			return
		}
	}
	c.runActivation(ctx, s, fence, false, 0)
}

func (c *Coordinator) startIdleLocked(s *contextSlot) {
	if s == nil || s.state != CoordinatorActive || s.demandCount != 0 || s.sessionLeaseCount != 0 || s.session == nil || s.workload == nil {
		return
	}
	s.advanceSessionLeaseEpochLocked(false)
	s.signalLocked()
	c.startWorkerLocked(s, CoordinatorDraining, func(ctx context.Context, fence operationFence) {
		c.runIdleRelease(ctx, s, fence)
	})
}

func (c *Coordinator) runIdleRelease(ctx context.Context, s *contextSlot, fence operationFence) {
	s.mu.Lock()
	if !s.matchesLocked(fence) || s.state != CoordinatorDraining || s.session == nil || s.workload == nil {
		s.mu.Unlock()
		return
	}
	session, workload := s.session, s.workload
	s.mu.Unlock()
	release := session.releaseForIdle(ctx)
	if release == nil {
		deadline, transportEnded := session.transportReconnectDeadline()
		s.mu.Lock()
		if !s.matchesLocked(fence) || s.workload != workload {
			s.mu.Unlock()
			return
		}
		if transportEnded {
			s.normalEnded = true
			s.drainDeadline = deadline
			if s.demandCount > 0 {
				c.startResumeLocked(s, nil)
				s.mu.Unlock()
				return
			}
			remaining := deadline.Sub(c.clock.Now())
			s.mu.Unlock()
			if remaining > 0 && c.clock.Sleep(ctx, remaining) != nil {
				return
			}
			s.mu.Lock()
			if !s.matchesLocked(fence) || s.state != CoordinatorDraining || s.demandCount != 0 || !s.normalEnded || s.drainDeadline != deadline || s.workload != workload {
				s.mu.Unlock()
				return
			}
			s.state = CoordinatorDisposing
			s.mu.Unlock()
			c.disposeAndContinue(s, fence, workload, true, true)
			return
		}
		s.state = CoordinatorDisposing
		s.mu.Unlock()
		c.disposeAndContinue(s, fence, workload, false, true)
		return
	}
	s.mu.Lock()
	if !s.matchesLocked(fence) || s.state != CoordinatorDraining || s.session != session || s.workload != workload {
		s.mu.Unlock()
		return
	}
	s.idle = release
	if s.demandCount > 0 {
		c.startResumeLocked(s, release)
		s.mu.Unlock()
		return
	}
	remaining := release.deadline.Sub(c.clock.Now())
	s.mu.Unlock()
	if remaining > 0 && c.clock.Sleep(ctx, remaining) != nil {
		return
	}
	s.mu.Lock()
	if !s.matchesLocked(fence) || s.state != CoordinatorDraining || s.demandCount != 0 || s.idle != release || s.workload != workload {
		s.mu.Unlock()
		return
	}
	s.state = CoordinatorDisposing
	s.mu.Unlock()
	c.disposeAndContinue(s, fence, workload, true, true)
}

func (c *Coordinator) disposeAndContinue(s *contextSlot, fence operationFence, workload *ProvisionedWorkload, drain, freshOnDemand bool) {
	if drain {
		c.drainAndDisposeOwned(s, workload)
	} else {
		c.disposeNowOwned(s, workload)
	}
	current := c.isCurrentSlot(s)
	s.mu.Lock()
	if s.contextEpoch != fence.contextEpoch || s.workload != nil {
		s.mu.Unlock()
		return
	}
	if s.demandCount > 0 && freshOnDemand && current {
		s.state = CoordinatorDirectOnly
		c.startActivationLocked(s, 0)
	} else if s.state != CoordinatorClosed {
		s.state = CoordinatorDirectOnly
	}
	s.mu.Unlock()
}

// FenceContextSwitch synchronously revokes the old slot before the desktop
// mutates its Kubernetes client. Exact owned cleanup starts asynchronously.
func (c *Coordinator) FenceContextSwitch(oldContext string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed || c.switching || oldContext == "" || oldContext != c.currentName {
		c.mu.Unlock()
		return
	}
	c.switching = true
	slot := c.slots[c.currentEpoch]
	c.mu.Unlock()
	if slot == nil {
		return
	}
	slot.navigationMu.Lock()
	defer slot.navigationMu.Unlock()
	slot.mu.Lock()
	wasAvailable := slot.state == CoordinatorActive && slot.session != nil
	if slot.workerCancel != nil {
		slot.workerCancel()
	}
	slot.operationEpoch++
	slot.demandEpoch++
	slot.demandCount = 0
	slot.advanceSessionLeaseEpochLocked(false)
	slot.state = CoordinatorClosed
	workload := slot.workload
	if wasAvailable {
		slot.signalLocked()
	}
	slot.mu.Unlock()
	if workload != nil {
		done := c.startCoordinatedDisposeNow(slot, workload)
		go func() {
			if done != nil {
				<-done
			}
			c.pruneClosedSlot(slot)
		}()
	}
}

// ContextSwitched completes the exact epoch transition. A failed switch keeps
// the old context name but still creates a fresh, closed acceleration epoch.
func (c *Coordinator) ContextSwitched(contextName string, success bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.switching || c.closed {
		c.mu.Unlock()
		return
	}
	oldSlot := c.slots[c.currentEpoch]
	if success {
		c.currentName = contextName
	}
	c.currentEpoch++
	c.switching = false
	c.mu.Unlock()
	c.pruneClosedSlot(oldSlot)
}

func (c *Coordinator) Quiesce(context.Context) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.quiesced {
		c.mu.Unlock()
		return
	}
	c.quiesced = true
	slots := c.slotListLocked()
	c.mu.Unlock()
	for _, slot := range slots {
		slot.navigationMu.Lock()
		slot.mu.Lock()
		wasAvailable := slot.state == CoordinatorActive && slot.session != nil
		slot.operationEpoch++
		slot.demandEpoch++
		slot.demandCount = 0
		slot.advanceSessionLeaseEpochLocked(false)
		slot.state = CoordinatorClosed
		if wasAvailable {
			slot.signalLocked()
		}
		slot.mu.Unlock()
		slot.navigationMu.Unlock()
	}
}

func (c *Coordinator) StopProducers(ctx context.Context) {
	if c == nil {
		return
	}
	c.mu.Lock()
	slots := c.slotListLocked()
	c.mu.Unlock()
	for _, slot := range slots {
		slot.mu.Lock()
		cancel, session := slot.workerCancel, slot.session
		slot.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if session != nil {
			_ = session.Close(context.Background())
		}
	}
	done := make(chan struct{})
	go func() {
		c.workers.Wait()
		close(done)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (c *Coordinator) Close(context.Context) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	slots := c.slotListLocked()
	c.mu.Unlock()
	type retainedWorkload struct {
		slot     *contextSlot
		workload *ProvisionedWorkload
	}
	var workloads []retainedWorkload
	for _, slot := range slots {
		slot.navigationMu.Lock()
		slot.mu.Lock()
		if slot.workload != nil {
			workloads = append(workloads, retainedWorkload{slot: slot, workload: slot.workload})
		}
		slot.mu.Unlock()
		slot.navigationMu.Unlock()
	}
	if len(workloads) == 0 {
		return
	}
	done := make(chan struct{})
	remaining := make(chan struct{}, len(workloads))
	for _, workload := range workloads {
		go func(candidate retainedWorkload) {
			c.disposeNowOwned(candidate.slot, candidate.workload)
			c.pruneClosedSlot(candidate.slot)
			remaining <- struct{}{}
		}(workload)
	}
	go func() {
		for range workloads {
			<-remaining
		}
		close(done)
	}()
	timerCtx, cancelTimer := context.WithCancel(context.Background())
	defer cancelTimer()
	timedOut := make(chan struct{})
	go func() {
		_ = c.clock.Sleep(timerCtx, ShutdownWaitTimeout)
		close(timedOut)
	}()
	select {
	case <-done:
	case <-timedOut:
	}
}

func (c *Coordinator) slotListLocked() []*contextSlot {
	slots := make([]*contextSlot, 0, len(c.slots))
	for _, slot := range c.slots {
		slots = append(slots, slot)
	}
	return slots
}
