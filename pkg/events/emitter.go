// Package events provides a unified interface for event emission
// that works in both Wails desktop mode and HTTP/WebSocket server mode.
package events

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Emitter is the interface for emitting events to the frontend.
// Implementations handle the actual transport (Wails IPC or WebSocket).
type Emitter interface {
	// Emit sends an event with the given name and optional data to all listeners.
	Emit(name string, data ...interface{})
}

// EmitterFunc is a function type that implements Emitter.
// This allows using simple functions as emitters.
type EmitterFunc func(name string, data ...interface{})

// Emit calls the function with the given arguments.
func (f EmitterFunc) Emit(name string, data ...interface{}) {
	f(name, data...)
}

// WailsEmitter emits events through Wails runtime.
type WailsEmitter struct {
	ctx context.Context
}

// NewWailsEmitter creates a new Wails-based emitter.
func NewWailsEmitter(ctx context.Context) *WailsEmitter {
	return &WailsEmitter{ctx: ctx}
}

// Emit sends an event through Wails runtime.
func (e *WailsEmitter) Emit(name string, data ...interface{}) {
	if e.ctx != nil {
		runtime.EventsEmit(e.ctx, name, data...)
	}
}

// SetContext updates the Wails context.
func (e *WailsEmitter) SetContext(ctx context.Context) {
	e.ctx = ctx
}

// NoopEmitter is an emitter that does nothing.
// Useful for testing or when events are not needed.
type NoopEmitter struct{}

// Emit does nothing.
func (e *NoopEmitter) Emit(name string, data ...interface{}) {}
