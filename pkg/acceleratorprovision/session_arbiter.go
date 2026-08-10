package acceleratorprovision

import (
	"sync"

	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/debug"
)

type sessionFrameHandler func(acceleratorsecret.ServerFrame) bool

type sessionFrameArbiter struct {
	session *ConnectedSession

	deliveryMu sync.Mutex
	mu         sync.Mutex
	claimed    bool
	closed     bool
	handler    sessionFrameHandler
	pending    []acceleratorsecret.ServerFrame
}

func newSessionFrameArbiter(session *ConnectedSession) *sessionFrameArbiter {
	return &sessionFrameArbiter{session: session, pending: make([]acceleratorsecret.ServerFrame, 0, acceleratorsecret.SubscriptionEventSlots)}
}

func (a *sessionFrameArbiter) attach(handler sessionFrameHandler) (func(), bool) {
	if a == nil || handler == nil || a.session == nil {
		return func() {}, false
	}
	select {
	case <-a.session.terminalStarted():
		return func() {}, false
	default:
	}
	a.deliveryMu.Lock()
	defer a.deliveryMu.Unlock()
	a.mu.Lock()
	select {
	case <-a.session.terminalStarted():
		a.mu.Unlock()
		return func() {}, false
	default:
	}
	if a.claimed || a.closed {
		a.mu.Unlock()
		return func() {}, false
	}
	a.claimed = true
	a.handler = handler
	pending := a.pending
	a.pending = nil
	a.mu.Unlock()
	for index := range pending {
		accepted := handler(pending[index])
		clearServerFrame(&pending[index])
		if !accepted {
			debug.LogPortforward("Accelerator session frame routing failed", map[string]interface{}{"stage": "attach_pending_frame", "pendingIndex": index, "pendingFrames": len(pending)})
			for remaining := index + 1; remaining < len(pending); remaining++ {
				clearServerFrame(&pending[remaining])
			}
			a.mu.Lock()
			a.handler = nil
			a.mu.Unlock()
			a.session.beginTermination(SessionProtocolFailed, false)
			return func() {}, false
		}
	}
	return a.detach, true
}

func (a *sessionFrameArbiter) detach() {
	if a == nil {
		return
	}
	a.deliveryMu.Lock()
	defer a.deliveryMu.Unlock()
	a.mu.Lock()
	a.handler = nil
	a.mu.Unlock()
}

func (a *sessionFrameArbiter) route(frame acceleratorsecret.ServerFrame) bool {
	if a == nil {
		clearServerFrame(&frame)
		return false
	}
	a.deliveryMu.Lock()
	defer a.deliveryMu.Unlock()
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		clearServerFrame(&frame)
		debug.LogPortforward("Accelerator session frame routing failed", map[string]interface{}{"stage": "route_closed"})
		return false
	}
	if a.handler == nil {
		if len(a.pending) == cap(a.pending) {
			pendingFrames := len(a.pending)
			a.mu.Unlock()
			clearServerFrame(&frame)
			debug.LogPortforward("Accelerator session frame routing failed", map[string]interface{}{"stage": "pending_overflow", "pendingFrames": pendingFrames})
			return false
		}
		a.pending = append(a.pending, frame)
		a.mu.Unlock()
		return true
	}
	handler := a.handler
	a.mu.Unlock()
	accepted := handler(frame)
	clearServerFrame(&frame)
	if !accepted {
		debug.LogPortforward("Accelerator session frame routing failed", map[string]interface{}{"stage": "handler_rejected"})
	}
	return accepted
}

func (a *sessionFrameArbiter) close() {
	if a == nil {
		return
	}
	a.deliveryMu.Lock()
	a.mu.Lock()
	a.closed = true
	a.handler = nil
	for index := range a.pending {
		clearServerFrame(&a.pending[index])
	}
	a.pending = nil
	a.mu.Unlock()
	a.deliveryMu.Unlock()
}

func clearServerFrame(frame *acceleratorsecret.ServerFrame) {
	if frame == nil {
		return
	}
	if frame.Result != nil {
		clear(frame.Result.Result)
		frame.Result.Result = nil
	}
	if frame.Event != nil {
		clear(frame.Event.Data)
		frame.Event.Data = nil
	}
}
