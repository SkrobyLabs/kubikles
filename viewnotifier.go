package main

import (
	"sync"
	"time"

	"kubikles/pkg/rows"
)

// viewUpdatesEvent is the one event the view protocol uses. Its payload is a batch of small
// notices ([]rows.ViewNotice) and never carries rows: clients pull data with GET window fetches.
const viewUpdatesEvent = "view-updates"

// defaultNoticeDelay is used when no event coalescer exists (tests, early startup).
const defaultNoticeDelay = 16 * time.Millisecond

// noticeSource is the part of the view manager the notifier reads.
type noticeSource interface {
	TakeNotices() []rows.ViewNotice
}

// ViewNotifier turns "a view has something to tell" into one batched view-updates event per
// frame. It follows the EventCoalescer's cadence (its adaptive interval and its background
// mode) so views and legacy watch events share one rhythm, and it flushes at once when the
// window returns to the foreground.
type ViewNotifier struct {
	src   func() noticeSource
	delay func() time.Duration
	emit  func(name string, data ...interface{})

	mu    sync.Mutex
	timer *time.Timer
	armed bool
}

func newViewNotifier(src func() noticeSource, delay func() time.Duration, emit func(string, ...interface{})) *ViewNotifier {
	return &ViewNotifier{src: src, delay: delay, emit: emit}
}

// Mark records that a view became dirty and arms the frame timer if it is not already armed. It
// never blocks, so it is safe to call with view locks held.
func (n *ViewNotifier) Mark() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.armed {
		return
	}
	n.armed = true
	d := defaultNoticeDelay
	if n.delay != nil {
		if v := n.delay(); v > 0 {
			d = v
		}
	}
	n.timer = time.AfterFunc(d, n.flush)
}

// FlushNow sends pending notices immediately (window returned to the foreground, shutdown).
func (n *ViewNotifier) FlushNow() {
	n.mu.Lock()
	if n.timer != nil {
		n.timer.Stop()
	}
	n.mu.Unlock()
	n.flush()
}

func (n *ViewNotifier) flush() {
	n.mu.Lock()
	n.armed = false
	n.mu.Unlock()
	src := n.src()
	if src == nil {
		return
	}
	if notices := src.TakeNotices(); len(notices) > 0 {
		n.emit(viewUpdatesEvent, notices)
	}
}

// Stop cancels a pending flush.
func (n *ViewNotifier) Stop() {
	n.mu.Lock()
	if n.timer != nil {
		n.timer.Stop()
	}
	n.armed = false
	n.mu.Unlock()
}
