package rows

import (
	"context"
	"sync"
	"time"
)

// OverlayResult is what one poll of a source produced.
type OverlayResult struct {
	// Available is whether the source could answer (the "metricsAvailable" state).
	Available bool
	// Records are the new per-object values; ignored when Keep is set.
	Records map[string]OverlayRecord
	// Keep leaves the previous records in place while still reporting Available (a poll that
	// failed outright: the legacy hooks keep their last map and flip only the availability).
	Keep bool
}

// OverlayFetch polls a source for a kube context.
type OverlayFetch func(ctx context.Context, contextName string) OverlayResult

// PollerConfig configures an OverlayPoller.
type PollerConfig struct {
	Hub *OverlayHub
	// Interval returns the poll interval, read before every wait so a setting change applies
	// at the next tick.
	Interval func() time.Duration
	// Visible reports whether the window can be seen. Nothing is polled while it cannot.
	Visible func() bool
	// FetchTimeout bounds one poll; zero means 30 seconds.
	FetchTimeout time.Duration
}

// OverlayPoller runs the periodic source behind each overlay. A source is polled only while
// some view wants it (the hub's demand callback) and the window is visible: the first poll runs
// at once when demand starts or the window comes back, then one per interval. Hidden ticks are
// skipped and one catch-up poll runs on show, like startVisibleInterval in the legacy hooks.
type OverlayPoller struct {
	cfg PollerConfig

	mu      sync.Mutex
	fetches map[string]OverlayFetch
	running map[overlayKey]chan struct{}
	visible chan struct{} // closed and replaced on every visibility or interval change
	closed  bool
	wg      sync.WaitGroup
	// polls counts fetches per source, for tests and the performance panel.
	polls map[string]int
}

// NewOverlayPoller creates a poller and installs itself as the hub's demand hook.
func NewOverlayPoller(cfg PollerConfig) *OverlayPoller {
	if cfg.FetchTimeout <= 0 {
		cfg.FetchTimeout = 30 * time.Second
	}
	p := &OverlayPoller{cfg: cfg, fetches: map[string]OverlayFetch{}, running: map[overlayKey]chan struct{}{},
		visible: make(chan struct{}), polls: map[string]int{}}
	cfg.Hub.SetDemandHook(p.Demand)
	return p
}

// Register sets the fetch function of a source.
func (p *OverlayPoller) Register(name string, fetch OverlayFetch) {
	p.mu.Lock()
	p.fetches[name] = fetch
	p.mu.Unlock()
}

// Notify wakes every loop so it re-reads visibility and the interval.
func (p *OverlayPoller) Notify() {
	p.mu.Lock()
	close(p.visible)
	p.visible = make(chan struct{})
	p.mu.Unlock()
}

// Demand is the hub's demand hook: it starts or stops the loop of a source in a context.
//
// The hub runs the hook after releasing its lock, so an "off" can arrive after a later "on" (or
// the reverse). The passed value is therefore only a prompt: the loop follows the hub's current
// demand, read while holding p.mu. The hub holds no lock while calling the hook, so taking its
// lock here cannot invert an order.
func (p *OverlayPoller) Demand(contextName, name string, _ bool) {
	k := overlayKey{contextName, name}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	wanted := p.cfg.Hub.Demanded(contextName, name)
	stop, running := p.running[k]
	switch {
	case wanted && !running:
		fetch := p.fetches[name]
		if fetch == nil {
			return
		}
		stop = make(chan struct{})
		p.running[k] = stop
		p.wg.Add(1)
		go p.loop(k, fetch, stop)
	case !wanted && running:
		close(stop)
		delete(p.running, k)
	}
}

// Running reports whether a source has an active loop (tests).
func (p *OverlayPoller) Running(contextName, name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.running[overlayKey{contextName, name}]
	return ok
}

// Polls returns how many fetches a source has run.
func (p *OverlayPoller) Polls(name string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.polls[name]
}

// Close stops every loop and waits for them.
func (p *OverlayPoller) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	for k, stop := range p.running {
		close(stop)
		delete(p.running, k)
	}
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *OverlayPoller) wake() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.visible
}

func (p *OverlayPoller) loop(k overlayKey, fetch OverlayFetch, stop <-chan struct{}) {
	defer p.wg.Done()
	for {
		wake := p.wake()
		if !p.cfg.Visible() {
			// Hidden: no polling at all. Becoming visible wakes the loop, which then polls at once.
			select {
			case <-stop:
				return
			case <-wake:
				continue
			}
		}
		select {
		case <-stop:
			return
		default:
		}
		p.poll(k, fetch, stop)
		interval := p.cfg.Interval()
		if interval <= 0 {
			interval = 30 * time.Second
		}
		timer := time.NewTimer(interval)
		select {
		case <-stop:
			timer.Stop()
			return
		case <-timer.C:
		case <-wake:
			// Visibility or the interval changed: re-evaluate. When still visible this polls
			// at once, which is the catch-up after a hidden stretch.
			timer.Stop()
		}
	}
}

func (p *OverlayPoller) poll(k overlayKey, fetch OverlayFetch, stop <-chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.FetchTimeout)
	defer cancel()
	go func() {
		select {
		case <-stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	gen := p.cfg.Hub.Generation()
	res := fetch(ctx, k.context)
	// A loop whose source was stopped while the fetch ran must not publish: a newer loop may
	// already have published fresher data.
	select {
	case <-stop:
		return
	default:
	}
	p.mu.Lock()
	p.polls[k.name]++
	p.mu.Unlock()
	records := res.Records
	if res.Keep {
		if cur := p.cfg.Hub.Current(k.context, k.name); cur != nil {
			records = cur.Records
		}
	}
	p.cfg.Hub.PublishAt(gen, k.context, k.name, BoolPtr(res.Available), records)
}
