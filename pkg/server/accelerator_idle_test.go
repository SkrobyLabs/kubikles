package server

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kubikles/pkg/agent"
)

type idleTestTimer struct {
	mu        sync.Mutex
	ch        chan time.Time
	stopped   bool
	fired     bool
	stopCalls int
}

func (t *idleTestTimer) C() <-chan time.Time { return t.ch }
func (t *idleTestTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopCalls++
	wasLive := !t.stopped && !t.fired
	t.stopped = true
	return wasLive
}
func (t *idleTestTimer) fire() {
	t.mu.Lock()
	t.fired = true
	t.mu.Unlock()
	select {
	case t.ch <- time.Unix(0, 0):
	default:
	}
}
func (t *idleTestTimer) state() (bool, bool, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped, t.fired, t.stopCalls
}

type idleTestClock struct {
	mu        sync.Mutex
	timers    []*idleTestTimer
	durations []time.Duration
	maxLive   int
}

func (c *idleTestClock) NewTimer(d time.Duration) AcceleratorIdleTimer {
	timer := &idleTestTimer{ch: make(chan time.Time, 1)}
	c.mu.Lock()
	c.timers = append(c.timers, timer)
	c.durations = append(c.durations, d)
	live := 0
	for _, candidate := range c.timers {
		candidate.mu.Lock()
		if !candidate.stopped && !candidate.fired {
			live++
		}
		candidate.mu.Unlock()
	}
	if live > c.maxLive {
		c.maxLive = live
	}
	c.mu.Unlock()
	return timer
}
func (c *idleTestClock) liveAndMax() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	live := 0
	for _, timer := range c.timers {
		timer.mu.Lock()
		if !timer.stopped && !timer.fired {
			live++
		}
		timer.mu.Unlock()
	}
	return live, c.maxLive
}

func idleOpenWaiters(records []*acceleratorIdleTimerRecord) int {
	open := 0
	for _, record := range records {
		select {
		case <-record.waiterDone:
		default:
			open++
		}
	}
	return open
}
func (c *idleTestClock) snapshot() ([]*idleTestTimer, []time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*idleTestTimer(nil), c.timers...), append([]time.Duration(nil), c.durations...)
}

type idleTestLifecycle struct {
	mu           sync.Mutex
	clears       int
	exits        int
	clearErr     error
	clearStarted chan struct{}
	clearRelease <-chan struct{}
	exit         chan struct{}
	startOnce    sync.Once
}

type joinedIdleLifecycle struct {
	browser  *BrowserSessionManager
	registry *AcceleratorSessionRegistry
	clears   atomic.Int32
	exits    atomic.Int32
}

func (l *joinedIdleLifecycle) ClearSessionAndWatcherState(ctx context.Context) error {
	l.clears.Add(1)
	l.browser.RevokeAll(ctx)
	return l.registry.Close(ctx)
}
func (l *joinedIdleLifecycle) RequestProcessExit() { l.exits.Add(1) }

func newIdleTestLifecycle() *idleTestLifecycle {
	return &idleTestLifecycle{exit: make(chan struct{}, 1)}
}
func (l *idleTestLifecycle) ClearSessionAndWatcherState(context.Context) error {
	l.mu.Lock()
	l.clears++
	l.mu.Unlock()
	if l.clearStarted != nil {
		l.startOnce.Do(func() { close(l.clearStarted) })
	}
	if l.clearRelease != nil {
		<-l.clearRelease
	}
	return l.clearErr
}
func (l *idleTestLifecycle) RequestProcessExit() {
	l.mu.Lock()
	l.exits++
	l.mu.Unlock()
	select {
	case l.exit <- struct{}{}:
	default:
	}
}
func (l *idleTestLifecycle) counts() (int, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.clears, l.exits
}

func idleSnapshot(id agent.SessionID, generation AcceleratorSocketGeneration) AcceleratorSessionSnapshot {
	return AcceleratorSessionSnapshot{CallContext: agent.AuthenticatedCallContext{SessionID: id}, Generation: generation}
}

func waitIdleSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestAcceleratorIdleCoordinatorReadyAndCurrentSessions(t *testing.T) {
	clock := &idleTestClock{}
	life := newIdleTestLifecycle()
	c := newAcceleratorIdleCoordinator(clock, life, context.Background(), nil)

	if timers, _ := clock.snapshot(); len(timers) != 0 {
		t.Fatalf("timer count before ready = %d", len(timers))
	}
	c.SessionConnected(idleSnapshot("creator", 1))
	c.MarkReady()
	c.MarkReady()
	if timers, _ := clock.snapshot(); len(timers) != 0 {
		t.Fatalf("ready with current socket timers = %d", len(timers))
	}
	c.SessionDisconnected(idleSnapshot("creator", 1))
	timers, durations := clock.snapshot()
	if len(timers) != 1 || durations[0] != agent.AcceleratorIdleReconnectGrace {
		t.Fatalf("last disconnect timers/durations = %d/%v", len(timers), durations)
	}
	c.SessionConnected(idleSnapshot("creator", 2))
	if stopped, _, calls := timers[0].state(); !stopped || calls != 1 {
		t.Fatalf("reconnect stop state = %v/%d", stopped, calls)
	}
	c.SessionConnected(idleSnapshot("browser", 1))
	c.SessionDisconnected(idleSnapshot("creator", 2))
	if got, _ := clock.snapshot(); len(got) != 1 {
		t.Fatalf("removing one of two sessions started timer: %d", len(got))
	}
	c.SessionRevoked(idleSnapshot("browser", 1))
	if got, _ := clock.snapshot(); len(got) != 2 {
		t.Fatalf("last browser revoke timers = %d", len(got))
	}
	// HTTP activity has no observer callback and therefore cannot affect grace.
	if got, _ := clock.snapshot(); len(got) != 2 {
		t.Fatal("HTTP inactivity changed timer state")
	}
	c.Shutdown()
}

func TestAcceleratorIdleCoordinatorGenerationFence(t *testing.T) {
	clock := &idleTestClock{}
	life := newIdleTestLifecycle()
	c := newAcceleratorIdleCoordinator(clock, life, context.Background(), nil)
	c.MarkReady()
	timers, _ := clock.snapshot()
	c.SessionConnected(idleSnapshot("a", 2))
	c.SessionConnected(idleSnapshot("a", 2))
	c.SessionConnected(idleSnapshot("a", 1))
	c.SessionConnected(idleSnapshot("", 3))
	c.SessionConnected(idleSnapshot("invalid", 0))
	c.SessionDisconnected(idleSnapshot("a", 1))
	c.SessionRevoked(idleSnapshot("a", 1))
	if got, _ := clock.snapshot(); len(got) != 1 {
		t.Fatalf("stale/invalid callbacks changed timer count = %d", len(got))
	}
	c.SessionDisconnected(idleSnapshot("a", 2))
	timers, _ = clock.snapshot()
	if len(timers) != 2 {
		t.Fatalf("replacement disconnect timer count = %d", len(timers))
	}
	c.SessionConnected(idleSnapshot("a", 3))
	timers[1].fire()
	c.SessionDisconnected(idleSnapshot("a", 2))
	c.SessionRevoked(idleSnapshot("a", 2))
	c.SessionConnected(idleSnapshot("a", 4))
	c.SessionDisconnected(idleSnapshot("a", 3))
	if clears, exits := life.counts(); clears != 0 || exits != 0 {
		t.Fatalf("stale delivered timer expired current socket = %d/%d", clears, exits)
	}
	c.SessionRevoked(idleSnapshot("a", 4))
	timers, _ = clock.snapshot()
	if len(timers) != 3 {
		t.Fatalf("current revoke timer count = %d", len(timers))
	}
	timers[2].fire()
	waitIdleSignal(t, life.exit, "current generation expiry")
	if clears, exits := life.counts(); clears != 1 || exits != 1 {
		t.Fatalf("expiry counts = %d/%d", clears, exits)
	}
	c.Shutdown()

	selectedClock := &idleTestClock{}
	selectedLife := newIdleTestLifecycle()
	selected := newAcceleratorIdleCoordinator(selectedClock, selectedLife, context.Background(), nil)
	selected.MarkReady()
	selected.mu.Lock()
	selectedRecord := selected.timer
	selectedRecord.timer.(*idleTestTimer).fire()
	waitIdleSignal(t, selectedRecord.waiterDone, "already-selected stale tick")
	selected.sessionConnectedLocked(idleSnapshot("newer", 7))
	selected.mu.Unlock()
	selected.Shutdown()
	if clears, exits := selectedLife.counts(); clears != 0 || exits != 0 {
		t.Fatalf("already-selected stale tick expired newer generation = %d/%d", clears, exits)
	}
	selected.mu.Lock()
	newer := selected.current["newer"]
	selected.mu.Unlock()
	if newer != 7 {
		t.Fatalf("newer generation after selected stale tick = %d", newer)
	}
}

// TestBrowserEntryUsesAcceleratorSessionLifecycle documents that Browser
// connectivity uses the existing generation-fenced observer and sole grace
// coordinator; it introduces no entry-specific count or timer.
func TestBrowserEntryUsesAcceleratorSessionLifecycle(t *testing.T) {
	TestAcceleratorIdleCoordinatorGenerationFence(t)
}

func TestAcceleratorIdleCoordinatorExactGraceAndTimerEpoch(t *testing.T) {
	clock := &idleTestClock{}
	life := newIdleTestLifecycle()
	c := newAcceleratorIdleCoordinator(clock, life, context.Background(), nil)
	c.MarkReady()
	record := c.timer
	records := []*acceleratorIdleTimerRecord{record}
	if record == nil {
		t.Fatal("ready did not install timer record")
	}
	timers, durations := clock.snapshot()
	if len(timers) != 1 || len(durations) != 1 || durations[0] != agent.AcceleratorIdleReconnectGrace {
		t.Fatalf("timer duration = %v", durations)
	}
	if agent.AcceleratorIdleReconnectGrace != 2*time.Minute {
		t.Fatalf("frozen grace = %v", agent.AcceleratorIdleReconnectGrace)
	}
	if live, max := clock.liveAndMax(); live != 1 || max != 1 || idleOpenWaiters(records) != 1 {
		t.Fatalf("initial live/max/open waiters = %d/%d/%d", live, max, idleOpenWaiters(records))
	}
	c.SessionConnected(idleSnapshot("a", 1))
	waitIdleSignal(t, record.waiterDone, "canceled waiter acknowledgment")
	if live, max := clock.liveAndMax(); live != 0 || max != 1 || idleOpenWaiters(records) != 0 {
		t.Fatalf("canceled live/max/open waiters = %d/%d/%d", live, max, idleOpenWaiters(records))
	}
	if c.timer != nil {
		t.Fatal("canceled timer remained current")
	}
	c.SessionDisconnected(idleSnapshot("a", 1))
	current := c.timer
	records = append(records, current)
	if current == nil || current == record {
		t.Fatal("replacement timer identity not advanced")
	}
	if live, max := clock.liveAndMax(); live != 1 || max != 1 || idleOpenWaiters(records) != 1 {
		t.Fatalf("replacement live/max/open waiters = %d/%d/%d", live, max, idleOpenWaiters(records))
	}
	timers[0].fire()
	if clears, exits := life.counts(); clears != 0 || exits != 0 {
		t.Fatalf("old delivered tick expired = %d/%d", clears, exits)
	}
	current.timer.(*idleTestTimer).fire()
	waitIdleSignal(t, current.waiterDone, "current waiter fire acknowledgment")
	waitIdleSignal(t, life.exit, "exact-boundary expiry")
	c.Shutdown()
	if live, max := clock.liveAndMax(); live != 0 || max != 1 || idleOpenWaiters(records) != 0 {
		t.Fatalf("shutdown live/max/open waiters = %d/%d/%d", live, max, idleOpenWaiters(records))
	}
	for i, timer := range timers {
		stopped, fired, _ := timer.state()
		if i == len(timers)-1 && !fired {
			t.Fatalf("current timer %d did not record fire", i)
		}
		if i < len(timers)-1 && !stopped {
			t.Fatalf("old timer %d was not stopped", i)
		}
	}
}

func TestAcceleratorIdleCoordinatorExpiryRace(t *testing.T) {
	for i := 0; i < 300; i++ {
		clock := &idleTestClock{}
		life := newIdleTestLifecycle()
		c := newAcceleratorIdleCoordinator(clock, life, context.Background(), nil)
		c.MarkReady()
		timer := c.timer.timer.(*idleTestTimer)
		start := make(chan struct{})
		var racers sync.WaitGroup
		racers.Add(2)
		go func() { defer racers.Done(); <-start; timer.fire() }()
		go func() { defer racers.Done(); <-start; c.SessionConnected(idleSnapshot("race", 1)) }()
		close(start)
		racers.Wait()

		c.mu.Lock()
		expiring := c.expiring
		current := c.current["race"]
		c.mu.Unlock()
		if expiring {
			waitIdleSignal(t, life.exit, "race expiry")
			c.SessionConnected(idleSnapshot("late", 1))
		} else if current != 1 {
			t.Fatalf("iteration %d: neither expiry nor current connection won", i)
		}
		c.Shutdown()
		if clears, exits := life.counts(); clears > 1 || exits > 1 || clears != exits {
			t.Fatalf("iteration %d cleanup counts = %d/%d", i, clears, exits)
		}
	}

	clock := &idleTestClock{}
	release := make(chan struct{})
	life := newIdleTestLifecycle()
	life.clearStarted = make(chan struct{})
	life.clearRelease = release
	c := newAcceleratorIdleCoordinator(clock, life, context.Background(), nil)
	c.MarkReady()
	c.timer.timer.(*idleTestTimer).fire()
	waitIdleSignal(t, life.clearStarted, "winning expiry cleanup")
	shutdownDone := make(chan struct{})
	go func() { c.Shutdown(); close(shutdownDone) }()
	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned before winning expiry completed")
	default:
	}
	c.SessionConnected(idleSnapshot("too-late", 1))
	close(release)
	waitIdleSignal(t, shutdownDone, "Shutdown expiry wait")
	if clears, exits := life.counts(); clears != 1 || exits != 1 {
		t.Fatalf("winning expiry counts = %d/%d", clears, exits)
	}
}

func TestAcceleratorIdleCoordinatorCleanupFailureReporter(t *testing.T) {
	clock := &idleTestClock{}
	life := newIdleTestLifecycle()
	life.clearErr = errors.New("private cleanup detail")
	var mu sync.Mutex
	reports := 0
	c := newAcceleratorIdleCoordinator(clock, life, context.Background(), func() {
		mu.Lock()
		reports++
		mu.Unlock()
	})
	c.MarkReady()
	c.timer.timer.(*idleTestTimer).fire()
	waitIdleSignal(t, life.exit, "cleanup-error exit")
	c.Shutdown()
	mu.Lock()
	defer mu.Unlock()
	if reports != 1 {
		t.Fatalf("report calls = %d", reports)
	}
}

func TestAcceleratorIdleShutdownConcurrent(t *testing.T) {
	clock := &idleTestClock{}
	life := newIdleTestLifecycle()
	c := newAcceleratorIdleCoordinator(clock, life, context.Background(), nil)
	c.MarkReady()
	initial := c.timer.timer.(*idleTestTimer)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < 24; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for generation := 1; generation <= 40; generation++ {
				snapshot := idleSnapshot(agent.SessionID("shared"), AcceleratorSocketGeneration(generation))
				switch (worker + generation) % 5 {
				case 0:
					c.MarkReady()
				case 1:
					c.SessionConnected(snapshot)
				case 2:
					c.SessionDisconnected(snapshot)
				case 3:
					c.SessionRevoked(snapshot)
				case 4:
					c.Shutdown()
				}
			}
		}()
	}
	wg.Add(1)
	go func() { defer wg.Done(); <-start; initial.fire() }()
	close(start)
	wg.Wait()
	c.Shutdown()
	c.mu.Lock()
	stopped, timer := c.stopped, c.timer
	c.mu.Unlock()
	if !stopped || timer != nil {
		t.Fatalf("shutdown state stopped/timer = %v/%v", stopped, timer)
	}
	if clears, exits := life.counts(); clears > 1 || exits > 1 || clears != exits {
		t.Fatalf("concurrent shutdown counts = %d/%d", clears, exits)
	}
}

func TestAcceleratorIdleJoinedComponentShutdown(t *testing.T) {
	clock := &idleTestClock{}
	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	defer cancelCleanup()
	lifecycle := &joinedIdleLifecycle{}
	coordinator := newAcceleratorIdleCoordinator(clock, lifecycle, cleanupCtx, nil)
	registry := NewAcceleratorSessionRegistry("joined-idle", coordinator)
	browser := newBrowserSessionManager(
		func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) },
		bytes.NewReader(bytes.Repeat([]byte{0x73}, 96)), registry,
	)
	lifecycle.browser, lifecycle.registry = browser, registry

	coordinator.MarkReady()
	initialRecord := coordinator.timer
	_, _, browserCall := mintAndExchange(t, browser)
	connection := newFakeAcceleratorConn()
	socket, ok := registry.register(browserCall, connection)
	if !ok {
		t.Fatal("real registry rejected joined browser socket")
	}
	if initialRecord == nil {
		t.Fatal("initial ready grace record missing")
	}
	waitIdleSignal(t, initialRecord.waiterDone, "joined reconnect cancellation")

	// Model a transport callback already delivered ahead of the registry's
	// physical close. This leaves a real active socket while the current grace
	// races the joined browser/registry/coordinator teardown.
	coordinator.SessionDisconnected(socket.snapshot)
	currentRecord := coordinator.timer
	if currentRecord == nil || currentRecord == initialRecord {
		t.Fatal("joined current grace was not rearmed")
	}

	start := make(chan struct{})
	var joined sync.WaitGroup
	joined.Add(5)
	go func() { defer joined.Done(); <-start; currentRecord.timer.(*idleTestTimer).fire() }()
	go func() { defer joined.Done(); <-start; browser.RevokeAll(cleanupCtx) }()
	go func() { defer joined.Done(); <-start; _ = registry.Close(cleanupCtx) }()
	go func() { defer joined.Done(); <-start; cancelCleanup() }()
	go func() {
		defer joined.Done()
		<-start
		var shutdowns sync.WaitGroup
		for i := 0; i < 16; i++ {
			shutdowns.Add(1)
			go func() { defer shutdowns.Done(); coordinator.Shutdown() }()
		}
		shutdowns.Wait()
	}()
	close(start)
	joinedDone := make(chan struct{})
	go func() { joined.Wait(); close(joinedDone) }()
	waitIdleSignal(t, joinedDone, "joined component teardown")
	coordinator.Shutdown()
	if err := registry.Close(context.Background()); err != nil {
		t.Fatalf("final registry join: %v", err)
	}
	waitIdleSignal(t, socket.pumpsDone, "joined socket pumps")

	if clears, exits := lifecycle.clears.Load(), lifecycle.exits.Load(); clears > 1 || exits > 1 || clears != exits {
		t.Fatalf("joined cleanup/exit counts = %d/%d", clears, exits)
	}
	coordinator.mu.Lock()
	stopped, timer := coordinator.stopped, coordinator.timer
	coordinator.mu.Unlock()
	if !stopped || timer != nil {
		t.Fatalf("joined coordinator stopped/timer = %v/%v", stopped, timer)
	}
	for i, record := range []*acceleratorIdleTimerRecord{initialRecord, currentRecord} {
		select {
		case <-record.waiterDone:
		default:
			t.Fatalf("joined timer record %d waiter remained open", i)
		}
	}
	if live, max := clock.liveAndMax(); live != 0 || max > 1 {
		t.Fatalf("joined live/max waiters = %d/%d", live, max)
	}
	registry.mu.Lock()
	closed, records := registry.closed, len(registry.records)
	registry.mu.Unlock()
	if !closed || registry.accepting.Load() || records != 0 {
		t.Fatalf("joined registry closed/accepting/records = %v/%v/%d", closed, registry.accepting.Load(), records)
	}
}
