package server

import (
	"errors"
	"sync"
	"time"

	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/agent"
)

var (
	errAcceleratorRPCProtocol    = errors.New("Accelerator RPC protocol failure")
	errAcceleratorRPCUnavailable = errors.New("Accelerator RPC unavailable")
)

// AcceleratorRPCDispatcher is the creator-only bridge from the closed Secret
// wire contract to the already-composed authenticated method caller.
type AcceleratorRPCDispatcher struct {
	caller     MethodCaller
	authorizer MethodAuthorizer
}

func NewAcceleratorRPCDispatcher(caller MethodCaller, authorizer MethodAuthorizer) *AcceleratorRPCDispatcher {
	return &AcceleratorRPCDispatcher{caller: caller, authorizer: authorizer}
}

type acceleratorRPCConnection struct {
	dispatcher *AcceleratorRPCDispatcher
	snapshot   AcceleratorSessionSnapshot
	sink       func([]byte) bool
	terminal   <-chan struct{}
	sequence   acceleratorsecret.CallIDSequence
	inflight   chan struct{}
	logf       func(string, ...interface{})

	mu        sync.Mutex
	accepting bool
	failure   error
	workers   sync.WaitGroup
}

func (d *AcceleratorRPCDispatcher) attach(snapshot AcceleratorSessionSnapshot, sink func([]byte) bool, terminal <-chan struct{}) *acceleratorRPCConnection {
	return d.attachWithLogger(snapshot, sink, terminal, nil)
}

func (d *AcceleratorRPCDispatcher) attachWithLogger(snapshot AcceleratorSessionSnapshot, sink func([]byte) bool, terminal <-chan struct{}, logf func(string, ...interface{})) *acceleratorRPCConnection {
	if d == nil || d.caller == nil || d.authorizer == nil || !snapshot.CallContext.IsAuthenticated() || snapshot.Generation == 0 || sink == nil || terminal == nil {
		return nil
	}
	select {
	case <-terminal:
		return nil
	default:
	}
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	return &acceleratorRPCConnection{
		dispatcher: d,
		snapshot:   snapshot,
		sink:       sink,
		terminal:   terminal,
		logf:       logf,
		inflight:   make(chan struct{}, acceleratorsecret.MaxConcurrentCalls),
		accepting:  true,
	}
}

// handle validates and admits a call without waiting on generated/App work.
// False means the owning socket must take its single bounded close path.
func (c *acceleratorRPCConnection) handle(payload []byte) bool {
	if c == nil || !c.admitting() {
		return false
	}
	call, err := acceleratorsecret.DecodeCall(payload)
	if err != nil || !c.sequence.Accept(call.ID) {
		c.fail(errAcceleratorRPCProtocol)
		return false
	}
	if _, known := agent.LookupMethodPolicy(string(call.Operation)); !known {
		c.fail(errAcceleratorRPCProtocol)
		return false
	}
	if !c.dispatcher.authorizer.Authorize(c.snapshot.CallContext, string(call.Operation)) {
		c.logCall("rejected", call.Operation, "forbidden", 0)
		return c.sendError(call.ID, acceleratorsecret.ReasonForbidden)
	}
	select {
	case c.inflight <- struct{}{}:
		c.workers.Add(1)
		go c.dispatch(call)
		return true
	default:
		c.logCall("rejected", call.Operation, "capacity", 0)
		return c.sendError(call.ID, acceleratorsecret.ReasonCapacity)
	}
}

func (c *acceleratorRPCConnection) dispatch(call acceleratorsecret.CallFrame) {
	defer c.workers.Done()
	defer func() { <-c.inflight }()
	started := time.Now()
	c.logCall("started", call.Operation, "", 0)
	result, err := c.dispatcher.caller.CallMethod(c.snapshot.CallContext, string(call.Operation), call.Args)
	if err != nil {
		_ = c.sendError(call.ID, acceleratorsecret.ReasonRemoteUnavailable)
		c.logCall("completed", call.Operation, "remote_error", time.Since(started))
		return
	}
	payload, err := acceleratorsecret.EncodeResultOK(call.ID, result)
	if err != nil || len(payload) > acceleratorsecret.MaxCreatorResponseFrameBytes {
		_ = c.sendError(call.ID, acceleratorsecret.ReasonRemoteUnavailable)
		c.logCall("completed", call.Operation, "invalid_response", time.Since(started))
		return
	}
	if !c.send(payload) {
		c.logCall("completed", call.Operation, "connection_unavailable", time.Since(started))
		return
	}
	c.logCall("completed", call.Operation, "ok", time.Since(started))
}

func (c *acceleratorRPCConnection) logCall(event string, operation acceleratorsecret.Operation, outcome string, duration time.Duration) {
	if c == nil {
		return
	}
	if duration > 0 {
		c.logf("Accelerator proxy call %s session=%q generation=%d operation=%q outcome=%q duration=%s", event, c.snapshot.CallContext.SessionID, c.snapshot.Generation, operation, outcome, duration.Round(time.Millisecond))
		return
	}
	if outcome != "" {
		c.logf("Accelerator proxy call %s session=%q generation=%d operation=%q outcome=%q", event, c.snapshot.CallContext.SessionID, c.snapshot.Generation, operation, outcome)
		return
	}
	c.logf("Accelerator proxy call %s session=%q generation=%d operation=%q", event, c.snapshot.CallContext.SessionID, c.snapshot.Generation, operation)
}

func (c *acceleratorRPCConnection) sendError(id string, reason acceleratorsecret.SecretClientReason) bool {
	payload, err := acceleratorsecret.EncodeResultError(id, reason)
	if err != nil {
		c.fail(errAcceleratorRPCProtocol)
		return false
	}
	return c.send(payload)
}

func (c *acceleratorRPCConnection) send(payload []byte) bool {
	if !c.admitting() {
		clear(payload)
		return false
	}
	if !c.sink(payload) {
		clear(payload)
		c.fail(errAcceleratorRPCUnavailable)
		return false
	}
	return true
}

func (c *acceleratorRPCConnection) admitting() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.accepting {
		return false
	}
	select {
	case <-c.terminal:
		c.accepting = false
		if c.failure == nil {
			c.failure = errAcceleratorRPCUnavailable
		}
		return false
	default:
		return true
	}
}

func (c *acceleratorRPCConnection) fail(err error) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.accepting = false
	if c.failure == nil {
		c.failure = err
	}
	c.mu.Unlock()
}

func (c *acceleratorRPCConnection) err() error {
	if c == nil {
		return errAcceleratorRPCUnavailable
	}
	_ = c.admitting()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failure
}

func (c *acceleratorRPCConnection) waitWorkers() { c.workers.Wait() }
