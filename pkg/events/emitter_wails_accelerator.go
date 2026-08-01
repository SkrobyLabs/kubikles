//go:build accelerator

package events

import "context"

// WailsEmitter is retained as a transport-neutral no-op solely so ordinary
// package tests compile under the global Accelerator tag; it imports no Wails
// runtime and is unreachable from the Accelerator App projection.
type WailsEmitter struct{}

func NewWailsEmitter(context.Context) *WailsEmitter { return &WailsEmitter{} }
func (*WailsEmitter) Emit(string, ...interface{})   {}
func (*WailsEmitter) SetContext(context.Context)    {}
