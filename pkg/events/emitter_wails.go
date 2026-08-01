//go:build !accelerator

package events

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WailsEmitter emits events through Wails runtime.
type WailsEmitter struct{ ctx context.Context }

func NewWailsEmitter(ctx context.Context) *WailsEmitter { return &WailsEmitter{ctx: ctx} }

func (e *WailsEmitter) Emit(name string, data ...interface{}) {
	if e.ctx != nil {
		runtime.EventsEmit(e.ctx, name, data...)
	}
}

func (e *WailsEmitter) SetContext(ctx context.Context) { e.ctx = ctx }
