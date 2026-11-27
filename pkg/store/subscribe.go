package store

import (
	"sync"
)

// DefaultQueueSize bounds the changes a subscriber may have pending.
const DefaultQueueSize = 4096

// SubOptions configures a subscription.
type SubOptions struct {
	// Namespace restricts delivery to one namespace. A lease served by a
	// cluster-wide shard sets it automatically.
	Namespace string
	// QueueSize bounds pending changes; zero means DefaultQueueSize.
	QueueSize int
}

// Subscription is the result of subscribing: the entries present at the moment
// of subscribing and a cancel function. Changes delivered through the callback
// are exactly those after Seq.
type Subscription struct {
	Snapshot []*Entry
	Seq      uint64
	cancel   func()
}

// Cancel stops delivery. It is safe to call more than once and from the
// callback. After Cancel returns no further callback starts.
func (s *Subscription) Cancel() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

// subscriber delivers batches to one callback on its own goroutine, so a slow
// callback can never block the reflector. The pending queue is bounded: when it
// overflows it collapses into a single ChangeReset and later changes are
// dropped until the callback has consumed the reset, because the subscriber
// re-reads Snapshot and ignores changes at or below the snapshot's Seq.
type subscriber struct {
	fn        func([]Change)
	namespace string
	limit     int

	mu      sync.Mutex
	pending []Change
	reset   bool // pending is a single ChangeReset; further changes are dropped
	closed  bool
	wake    chan struct{}
	done    chan struct{}
	exited  chan struct{}
}

func newSubscriber(fn func([]Change), opts SubOptions) *subscriber {
	limit := opts.QueueSize
	if limit <= 0 {
		limit = DefaultQueueSize
	}
	return &subscriber{
		fn:        fn,
		namespace: opts.Namespace,
		limit:     limit,
		wake:      make(chan struct{}, 1),
		done:      make(chan struct{}),
		exited:    make(chan struct{}),
	}
}

func (s *subscriber) matches(ns string) bool {
	return s.namespace == "" || s.namespace == ns
}

// enqueue appends changes without ever blocking. Called with the shard lock
// held, which gives subscribers the shard's change order.
func (s *subscriber) enqueue(changes []Change) {
	s.mu.Lock()
	if s.closed || s.reset {
		s.mu.Unlock()
		return
	}
	n := 0
	for i := range changes {
		c := &changes[i]
		if c.Type != ChangeReset {
			ns := ""
			if c.New != nil {
				ns = c.New.Namespace
			} else if c.Old != nil {
				ns = c.Old.Namespace
			}
			if !s.matches(ns) {
				continue
			}
		}
		n++
	}
	if n == 0 {
		s.mu.Unlock()
		return
	}
	if len(s.pending)+n > s.limit {
		s.pending = append(s.pending[:0], Change{Type: ChangeReset})
		s.reset = true
	} else {
		for i := range changes {
			c := changes[i]
			if c.Type != ChangeReset {
				ns := ""
				if c.New != nil {
					ns = c.New.Namespace
				} else if c.Old != nil {
					ns = c.Old.Namespace
				}
				if !s.matches(ns) {
					continue
				}
			}
			s.pending = append(s.pending, c)
		}
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *subscriber) loop() {
	defer close(s.exited)
	for {
		select {
		case <-s.done:
			return
		case <-s.wake:
		}
		for {
			s.mu.Lock()
			batch := s.pending
			s.pending = nil
			s.reset = false
			s.mu.Unlock()
			if len(batch) == 0 {
				break
			}
			select {
			case <-s.done:
				return
			default:
			}
			s.fn(batch)
		}
	}
}

func (s *subscriber) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.pending = nil
	s.mu.Unlock()
	close(s.done)
}
