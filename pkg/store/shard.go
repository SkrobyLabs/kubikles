package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"reflect"
	rtdebug "runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"kubikles/pkg/k8s"
	"kubikles/pkg/watchevents"
)

// Phase is the reflector state of a shard.
type Phase string

const (
	PhaseIdle       Phase = "idle"
	PhaseLoading    Phase = "loading"    // first LIST, rows arrive page by page
	PhaseRefreshing Phase = "refreshing" // relist over existing entries
	PhaseWatching   Phase = "watching"   // synced and following the watch
	PhaseBackoff    Phase = "backoff"    // last attempt failed, waiting to retry
	PhaseIncomplete Phase = "incomplete" // forbidden or otherwise unable to list
	PhaseStopped    Phase = "stopped"
)

const (
	prematureWindow = 5 * time.Second
	maxBackoffShift = 7
	drainBatch      = 256
	sampleEvery     = 16
)

// State is a point-in-time description of a shard.
type State struct {
	Phase          Phase
	Loaded         int
	Expected       int // 0 when the server did not report a remaining count
	Error          string
	Forbidden      bool
	Synced         bool // a LIST has completed since the shard was (re)built
	Degraded       bool
	DegradedReason string
	Mode           Mode // what the shard actually holds
	Requested      Mode // what the first acquirer asked for
	RV             string
	Entries        int
	Seq            uint64
}

// sourceBuilder rebuilds a source for the shard's key in the requested mode.
// The manager supplies it; it returns the effective mode and, when the request
// could not be honoured, why.
type sourceBuilder func(key Key, want Mode) (Source, Mode, string, error)

type shardConfig struct {
	key        Key
	build      sourceBuilder
	clock      Clock
	pageSize   func() int64
	pollEvery  func() time.Duration
	baseDelay  time.Duration
	maxDelay   time.Duration
	onStatus   func(watchevents.Status)
	onError    func(watchevents.Error)
	onSynced   func()
	forbidWait time.Duration
	// jitter spreads a backoff delay; nil means none (tests rely on exact delays).
	jitter func(time.Duration) time.Duration
	// onPanic is called from a new goroutine after the reflector panicked, so the owner can
	// unregister the shard and a later Acquire builds a fresh one.
	onPanic func(*Shard)
}

// Shard holds the entries of one Key and the reflector that maintains them.
type Shard struct {
	cfg shardConfig
	key Key

	mu         sync.RWMutex
	entries    map[string]*Entry
	byNS       map[string]map[string]*Entry
	seq        uint64
	rv         string
	phase      Phase
	loaded     int
	expected   int
	errMsg     string
	forbidden  bool
	synced     bool
	degraded   bool
	degradeWhy string
	mode       Mode
	requested  Mode
	src        Source
	colsPtr    atomic.Pointer[[]metav1.TableColumnDefinition]
	subs       map[*subscriber]struct{}
	opCancel   context.CancelFunc

	cancel  context.CancelFunc
	done    chan struct{} // closed when the reflector goroutine exits
	stopped chan struct{} // closed by stop
	kickCh  chan struct{}
	kicked  atomic.Bool

	ingested    atomic.Int64
	sampleBytes atomic.Int64
	sampleCount atomic.Int64

	// Owned by the manager and guarded by its lock.
	refs       int
	lastUsed   time.Time
	lingerSeq  uint64
	lingerStop func() bool
	pins       map[string]int
	stopOnce   sync.Once
	lifeMu     sync.Mutex // guards cancel, started, stopping
	stopping   bool
	started    bool
}

func newShard(cfg shardConfig, src Source, mode, requested Mode, degradeWhy string) *Shard {
	s := &Shard{
		cfg:        cfg,
		key:        cfg.key,
		entries:    make(map[string]*Entry),
		byNS:       make(map[string]map[string]*Entry),
		subs:       make(map[*subscriber]struct{}),
		phase:      PhaseIdle,
		src:        src,
		mode:       mode,
		requested:  requested,
		degraded:   degradeWhy != "",
		degradeWhy: degradeWhy,
		done:       make(chan struct{}),
		stopped:    make(chan struct{}),
		kickCh:     make(chan struct{}, 1),
		pins:       make(map[string]int),
	}
	return s
}

// start launches the reflector. It is a no-op once stop has begun, so a
// shard stopped between registration and start never runs unreachable.
func (s *Shard) start(parent context.Context) {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	if s.stopping {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.started = true
	go s.run(ctx)
}

// Mode returns the representation currently held.
func (s *Shard) Mode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// Key returns the shard key.
func (s *Shard) Key() Key { return s.key }

// Stopped is closed when the shard has been stopped (context switch, eviction,
// linger expiry). Leases expose it as Done.
func (s *Shard) Stopped() <-chan struct{} { return s.stopped }

// State returns a consistent description of the shard.
func (s *Shard) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stateLocked()
}

func (s *Shard) stateLocked() State {
	return State{
		Phase: s.phase, Loaded: s.loaded, Expected: s.expected, Error: s.errMsg,
		Forbidden: s.forbidden, Synced: s.synced, Degraded: s.degraded, DegradedReason: s.degradeWhy,
		Mode: s.mode, Requested: s.requested, RV: s.rv, Entries: len(s.entries), Seq: s.seq,
	}
}

// Columns returns the Table column definitions in force. Empty outside Table
// mode.
func (s *Shard) Columns() []metav1.TableColumnDefinition {
	if p := s.colsPtr.Load(); p != nil {
		return *p
	}
	return nil
}

// noteColumns records the column definitions of a row, touching shared state
// only when the slice actually changed.
func (s *Shard) noteColumns(cols []metav1.TableColumnDefinition) {
	if len(cols) == 0 {
		return
	}
	if cur := s.colsPtr.Load(); cur != nil && len(*cur) == len(cols) && &(*cur)[0] == &cols[0] {
		return
	}
	s.colsPtr.Store(&cols)
}

// Snapshot returns the entries (optionally of one namespace) and the sequence
// number they are consistent with. The slice is the caller's; the entries are
// immutable. Order is unspecified.
func (s *Shard) Snapshot(namespace string) ([]*Entry, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked(namespace), s.seq
}

func (s *Shard) snapshotLocked(namespace string) []*Entry {
	if namespace == "" {
		out := make([]*Entry, 0, len(s.entries))
		for _, e := range s.entries {
			out = append(out, e)
		}
		return out
	}
	m := s.byNS[namespace]
	out := make([]*Entry, 0, len(m))
	for _, e := range m {
		out = append(out, e)
	}
	return out
}

// Subscribe registers fn for batched changes. The returned subscription holds
// the snapshot the changes follow, taken atomically with the registration.
func (s *Shard) Subscribe(fn func([]Change), opts SubOptions) *Subscription {
	sub := newSubscriber(fn, opts)
	s.mu.Lock()
	select {
	case <-s.stopped:
		s.mu.Unlock()
		sub.close()
		close(sub.exited)
		return &Subscription{}
	default:
	}
	snap := s.snapshotLocked(opts.Namespace)
	seq := s.seq
	s.subs[sub] = struct{}{}
	s.mu.Unlock()
	go sub.loop()
	return &Subscription{Snapshot: snap, Seq: seq, cancel: func() {
		s.mu.Lock()
		delete(s.subs, sub)
		s.mu.Unlock()
		sub.close()
	}}
}

// estimatedBytes extrapolates the encoded size of the stored objects from a
// sample of ingested ones.
func (s *Shard) estimatedBytes() int64 {
	n := s.sampleCount.Load()
	if n == 0 {
		return 0
	}
	avg := s.sampleBytes.Load() / n
	s.mu.RLock()
	count := int64(len(s.entries))
	s.mu.RUnlock()
	return avg * count * heapFactor
}

// heapFactor converts encoded JSON bytes to resident heap bytes. See
// BenchmarkIngest50kPods for the measurement behind it.
const heapFactor = 3

// stop ends the reflector, closes subscribers and waits for the goroutine.
func (s *Shard) stop() {
	s.stopOnce.Do(func() {
		s.lifeMu.Lock()
		s.stopping = true
		cancel, started := s.cancel, s.started
		s.lifeMu.Unlock()
		if cancel != nil {
			cancel()
		}
		if started {
			<-s.done
		}
		s.mu.Lock()
		s.phase = PhaseStopped
		for sub := range s.subs {
			sub.close()
		}
		s.subs = make(map[*subscriber]struct{})
		s.mu.Unlock()
		close(s.stopped)
	})
}

// kick aborts whatever the reflector is doing and makes it relist.
func (s *Shard) kick() {
	s.kicked.Store(true)
	select {
	case s.kickCh <- struct{}{}:
	default:
	}
	s.mu.Lock()
	c := s.opCancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

// replaceSource swaps the source (new clients after a kubeconfig edit, or a
// stronger mode) and relists. When the representation changes the entries are
// dropped, because their objects have a different shape.
func (s *Shard) replaceSource(src Source, mode Mode, degradeWhy string, clear bool) {
	s.mu.Lock()
	s.src = src
	if mode != s.mode {
		clear = true
	}
	s.mode = mode
	s.degraded, s.degradeWhy = degradeWhy != "", degradeWhy
	if clear {
		s.clearLocked()
	}
	s.mu.Unlock()
	s.kick()
}

// setRequested raises the requested mode of an existing shard.
func (s *Shard) setRequested(m Mode) {
	s.mu.Lock()
	s.requested = m
	s.mu.Unlock()
}

func (s *Shard) clearLocked() {
	s.entries = make(map[string]*Entry)
	s.byNS = make(map[string]map[string]*Entry)
	s.synced = false
	s.rv = ""
	s.colsPtr.Store(nil)
	s.loaded, s.expected = 0, 0
	s.seq++
	for sub := range s.subs {
		sub.enqueue([]Change{{Type: ChangeReset, Seq: s.seq}})
	}
}

func (s *Shard) currentSource() Source {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.src
}

func (s *Shard) setPhase(p Phase) {
	s.mu.Lock()
	s.phase = p
	s.mu.Unlock()
}

func (s *Shard) emitStatus(status string) {
	if s.cfg.onStatus != nil {
		s.cfg.onStatus(watchevents.Status{ResourceType: s.key.Type, Namespace: s.key.Namespace, Status: status, Context: s.key.Context})
	}
}

func (s *Shard) emitError(err error, recoverable bool) {
	if s.cfg.onError != nil {
		s.cfg.onError(watchevents.Error{ResourceType: s.key.Type, Namespace: s.key.Namespace, Error: err.Error(), Recoverable: recoverable, Context: s.key.Context})
	}
}

func (s *Shard) emitPremature(receivedAny bool, open time.Duration) {
	if s.cfg.onError != nil {
		s.cfg.onError(watchevents.Error{
			ResourceType: s.key.Type, Namespace: s.key.Namespace,
			Error: "watch stream closed unexpectedly", Recoverable: true,
			Premature: true, ReceivedAny: receivedAny, OpenDurationMillis: open.Milliseconds(), Context: s.key.Context,
		})
	}
}

func (s *Shard) backoffDelay(failures int) time.Duration {
	d := s.cfg.baseDelay * time.Duration(1<<uint(min(failures, maxBackoffShift))) //nolint:gosec // capped shift
	if d > s.cfg.maxDelay {
		d = s.cfg.maxDelay
	}
	if s.cfg.jitter != nil {
		d = s.cfg.jitter(d)
	}
	return d
}

// defaultJitter shortens a delay by up to 20 percent so shards that failed together do not
// retry in lockstep. It only shortens, so the configured maximum still holds.
func defaultJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return d - time.Duration(rand.Int63n(int64(d)/5+1))
}

// wait sleeps for d. It returns false when the shard is stopping and true when
// the delay elapsed or a kick asked for an immediate retry.
func (s *Shard) wait(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-s.kickCh:
		return true
	case <-s.cfg.clock.After(d):
		return true
	}
}

func (s *Shard) beginOp(ctx context.Context) (context.Context, context.CancelFunc) {
	opCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.opCancel = cancel
	s.mu.Unlock()
	if s.kicked.Load() {
		cancel()
	}
	return opCtx, cancel
}

// run is the reflector. Its retry rules are those of the legacy watch loop:
// failures back off 1s*2^n capped at 2 minutes, a stream that closes within
// five seconds without delivering anything counts as a failure, a stale
// resourceVersion is not a connectivity failure, and bookmarks only advance the
// version.
func (s *Shard) run(ctx context.Context) {
	defer close(s.done)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("store: panic in reflector %s: %v\n%s", s.key, r, rtdebug.Stack())
			s.mu.Lock()
			s.phase, s.errMsg = PhaseIncomplete, fmt.Sprintf("internal error: %v", r)
			s.mu.Unlock()
			if s.cfg.onPanic != nil {
				// stop waits for this goroutine to finish, so it cannot run inline.
				go s.cfg.onPanic(s)
			}
		}
		s.emitStatus("stopped")
	}()

	failures := 0
	// staleRelists counts consecutive stale-version rejections that delivered no events. A
	// successful LIST resets failures, so without this a server that keeps rejecting the
	// version would be a continuous LIST loop.
	staleRelists := 0
	needList := true
	rv := ""

	for {
		if ctx.Err() != nil {
			return
		}
		if s.kicked.Swap(false) {
			needList = true
		}
		select {
		case <-s.kickCh:
		default:
		}

		if needList {
			opCtx, cancel := s.beginOp(ctx)
			newRV, err := s.list(opCtx)
			cancel()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				if s.kicked.Load() {
					continue
				}
				switch {
				case isModeUnsupported(err):
					if !s.fallbackWithBackoff(ctx, err, &failures) {
						return
					}
					continue
				case apierrors.IsForbidden(err):
					s.setFailure(err, true)
					s.emitError(err, true)
					s.emitStatus("reconnecting")
					if !s.wait(ctx, s.cfg.forbidWait) {
						return
					}
					continue
				default:
					failures++
					s.setFailure(err, false)
					s.emitError(err, true)
					s.emitStatus("reconnecting")
					if !s.wait(ctx, s.backoffDelay(failures)) {
						return
					}
					continue
				}
			}
			rv, needList, failures = newRV, false, 0
			if s.cfg.onSynced != nil {
				s.cfg.onSynced()
			}
		}

		if every := s.cfg.pollEvery(); every > 0 {
			s.emitStatus("connected")
			if !s.wait(ctx, every) {
				return
			}
			needList = true
			continue
		}

		opCtx, cancel := s.beginOp(ctx)
		w, err := s.currentSource().Watch(opCtx, rv)
		if err != nil {
			cancel()
			if ctx.Err() != nil {
				return
			}
			if s.kicked.Load() {
				continue
			}
			if isModeUnsupported(err) {
				needList, rv = true, ""
				if !s.fallbackWithBackoff(ctx, err, &failures) {
					return
				}
				continue
			}
			if rv != "" && k8s.IsStaleResourceVersionError(err) {
				// Not a connectivity failure: relist (diffing) instead of replaying.
				needList = true
				if !s.staleRelistWait(ctx, &staleRelists) {
					return
				}
				continue
			}
			failures++
			s.setFailure(err, false)
			s.emitError(err, true)
			s.emitStatus("reconnecting")
			if !s.wait(ctx, s.backoffDelay(failures)) {
				return
			}
			continue
		}

		s.setPhase(PhaseWatching)
		s.emitStatus("connected")
		res := s.consume(opCtx, w, &rv)
		cancel()
		if ctx.Err() != nil {
			return
		}
		// An "unsupported" error event is a failure of the round, not a sign of health: without
		// this the retry delay would never grow past its first step.
		if res.receivedAny && res.kind != outcomeUnsupported {
			failures = 0
		}
		// Only real traffic, or a watch that stayed open for a while, shows the stream is healthy.
		// The Gone error that ends a stale watch is an event too, and counting it would reset the
		// pacing on every round of a server that accepts the watch and then rejects its version.
		// A quiet resource whose watch simply expires after minutes must not be paced either.
		if res.gotData || res.open >= prematureWindow {
			staleRelists = 0
		}
		switch res.kind {
		case outcomeGone:
			needList = true
			if !s.staleRelistWait(ctx, &staleRelists) {
				return
			}
		case outcomeUnsupported:
			needList, rv = true, ""
			if !s.fallbackWithBackoff(ctx, res.err, &failures) {
				return
			}
		case outcomeError:
			failures++
			s.emitStatus("reconnecting")
			if !s.wait(ctx, s.backoffDelay(failures)) {
				return
			}
		case outcomeClosed:
			delay := time.Duration(0)
			if res.premature {
				failures++
				s.emitPremature(res.receivedAny, res.open)
				delay = s.backoffDelay(failures)
			} else {
				failures = 0
				if res.open < s.cfg.baseDelay {
					delay = s.cfg.baseDelay
				}
			}
			s.emitStatus("reconnecting")
			if delay > 0 && !s.wait(ctx, delay) {
				return
			}
		case outcomeKicked:
			// the loop top turns the kick into a relist
		}
	}
}

func (s *Shard) setFailure(err error, forbidden bool) {
	s.mu.Lock()
	s.errMsg = err.Error()
	s.forbidden = forbidden
	if forbidden {
		s.phase = PhaseIncomplete
	} else {
		s.phase = PhaseBackoff
	}
	s.mu.Unlock()
}

// staleRelistWait paces the relist that follows a stale-version rejection: the first one is
// immediate, later ones back off until a watch delivers events again. It returns false when
// the shard is stopping.
func (s *Shard) staleRelistWait(ctx context.Context, count *int) bool {
	n := *count
	*count++
	if n == 0 {
		return true
	}
	return s.wait(ctx, s.backoffDelay(n))
}

// fallbackToFull switches a Table or metadata shard to full objects after the
// server proved unable to serve the lighter representation.
func (s *Shard) fallbackToFull(cause error) error {
	src, mode, _, err := s.cfg.build(s.key, ModeFull)
	if err != nil {
		s.setFailure(err, false)
		return err
	}
	why := "fell back to full objects"
	if cause != nil {
		why = "fell back to full objects: " + cause.Error()
	}
	s.mu.Lock()
	s.src, s.mode = src, mode
	s.degraded, s.degradeWhy = true, why
	s.clearLocked()
	s.mu.Unlock()
	return nil
}

// fallbackWithBackoff switches to full objects and, when the full source cannot be built,
// waits the normal backoff instead of retrying in a tight loop. It returns false when the
// shard is stopping.
func (s *Shard) fallbackWithBackoff(ctx context.Context, cause error, failures *int) bool {
	s.mu.RLock()
	alreadyFull := s.mode == ModeFull
	s.mu.RUnlock()
	// A shard already on full objects has nothing left to fall back to: rebuilding the same
	// source would succeed and loop straight back into the same rejection, so it waits instead.
	if !alreadyFull && s.fallbackToFull(cause) == nil {
		return true
	}
	*failures++
	s.emitStatus("reconnecting")
	return s.wait(ctx, s.backoffDelay(*failures))
}

type outcomeKind int

const (
	outcomeClosed outcomeKind = iota
	outcomeGone
	outcomeError
	outcomeUnsupported
	outcomeKicked
)

type consumeResult struct {
	kind        outcomeKind
	premature   bool
	receivedAny bool
	// gotData is true when an event other than an error arrived (object changes, bookmarks).
	gotData bool
	open    time.Duration
	err     error
}

// consume applies watch events until the stream ends. rv is advanced by every
// event, bookmarks included.
func (s *Shard) consume(ctx context.Context, w watch.Interface, rv *string) consumeResult {
	defer w.Stop()
	openedAt := s.cfg.clock.Now()
	receivedAny := false
	gotData := false
	events := make([]watch.Event, 0, 16)
	for {
		select {
		case <-ctx.Done():
			return consumeResult{kind: outcomeKicked}
		case <-s.kickCh:
			s.kicked.Store(true)
			return consumeResult{kind: outcomeKicked}
		case ev, ok := <-w.ResultChan():
			if !ok {
				open := s.cfg.clock.Now().Sub(openedAt)
				return consumeResult{kind: outcomeClosed, premature: !receivedAny && open < prematureWindow, receivedAny: receivedAny, gotData: gotData, open: open}
			}
			receivedAny = true
			events = append(events[:0], ev)
		drain:
			for len(events) < drainBatch {
				select {
				case more, ok := <-w.ResultChan():
					if !ok {
						break drain
					}
					events = append(events, more)
				default:
					break drain
				}
			}
			for i := range events {
				if events[i].Type != watch.Error {
					gotData = true
				}
			}
			if res, stop := s.applyEvents(events, rv); stop {
				res.receivedAny = true
				res.gotData = gotData
				res.open = s.cfg.clock.Now().Sub(openedAt)
				return res
			}
		}
	}
}

// applyEvents stores a batch of watch events. It reports stop=true when an
// ERROR event ends the stream.
func (s *Shard) applyEvents(events []watch.Event, rv *string) (consumeResult, bool) {
	prepared := make([]prepared, 0, len(events))
	var stopRes *consumeResult
	for _, ev := range events {
		switch ev.Type {
		case watch.Error:
			st, _ := ev.Object.(*metav1.Status)
			res := consumeResult{kind: outcomeError}
			if st != nil {
				res.err = fmt.Errorf("watch error: %s", st.Message)
				switch {
				case st.Reason == metav1.StatusReasonExpired || st.Reason == metav1.StatusReasonGone || st.Code == 410:
					res.kind = outcomeGone
				case st.Reason == metav1.StatusReasonNotAcceptable || st.Code == 406:
					res.kind = outcomeUnsupported
					res.err = fmt.Errorf("%w: %s", ErrModeUnsupported, st.Message)
				}
			}
			stopRes = &res
		case watch.Added, watch.Modified, watch.Deleted:
			if stopRes != nil {
				continue
			}
			p, err := s.prepare(ev)
			if err != nil {
				continue
			}
			*rv = p.entry.RV
			prepared = append(prepared, p)
		default:
			// Bookmarks and unknown types only advance the version.
			if info, err := k8s.WatchObjectInfo(ev.Object); err == nil && info.ResourceVersion != "" && stopRes == nil {
				*rv = info.ResourceVersion
			}
		}
		if stopRes != nil {
			break
		}
	}
	if len(prepared) > 0 {
		s.mu.Lock()
		changes := make([]Change, 0, len(prepared))
		for _, p := range prepared {
			if c, ok := s.applyLocked(p.typ, p.entry); ok {
				changes = append(changes, c)
			}
		}
		s.rv = *rv
		s.publishLocked(changes)
		s.mu.Unlock()
	} else if *rv != "" {
		s.mu.Lock()
		s.rv = *rv
		s.mu.Unlock()
	}
	if stopRes != nil {
		return *stopRes, true
	}
	return consumeResult{}, false
}

type prepared struct {
	typ   watch.EventType
	entry *Entry
}

// prepare builds the entry for an event outside the shard lock.
func (s *Shard) prepare(ev watch.Event) (prepared, error) {
	e, err := s.newEntry(ev.Object)
	if err != nil {
		return prepared{}, err
	}
	return prepared{typ: ev.Type, entry: e}, nil
}

// newEntry converts a source object into an Entry (without Seq).
func (s *Shard) newEntry(obj runtime.Object) (*Entry, error) {
	if tr, ok := obj.(*TableRow); ok {
		if tr.Meta == nil {
			return nil, errors.New("table row without metadata")
		}
		e := &Entry{
			UID: string(tr.Meta.UID), Namespace: tr.Meta.Namespace, Name: tr.Meta.Name,
			RV: tr.Meta.ResourceVersion, Obj: tr.Meta, Cells: tr.Cells,
		}
		s.sample(e, tr.Meta, tr.Cells)
		s.noteColumns(tr.Columns)
		return e, nil
	}
	acc, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	k8s.StripManagedFieldsObject(obj)
	e := &Entry{
		UID: string(acc.GetUID()), Namespace: acc.GetNamespace(), Name: acc.GetName(),
		RV: acc.GetResourceVersion(), Obj: obj,
	}
	s.sample(e, obj, nil)
	return e, nil
}

// sampleFirst is how many of the first ingested objects are always sampled, so small shards
// (a handful of custom resources) report a size instead of zero.
const sampleFirst = 16

// sample records the encoded size of the first sampleFirst ingested objects and then of every
// sampleEvery-th one.
func (s *Shard) sample(_ *Entry, obj runtime.Object, cells []interface{}) {
	if n := s.ingested.Add(1); n > sampleFirst && n%sampleEvery != 0 {
		return
	}
	n := 0
	if b, err := json.Marshal(obj); err == nil {
		n += len(b)
	}
	if cells != nil {
		if b, err := json.Marshal(cells); err == nil {
			n += len(b)
		}
	}
	s.sampleBytes.Add(int64(n))
	s.sampleCount.Add(1)
}

// applyLocked stores one prepared entry or deletion and returns the change.
// Idempotent replays (same resourceVersion) produce no change.
func (s *Shard) applyLocked(typ watch.EventType, e *Entry) (Change, bool) {
	id := e.ID()
	old := s.entries[id]
	if typ == watch.Deleted {
		if old == nil {
			return Change{}, false
		}
		s.removeLocked(old)
		s.seq++
		return Change{Type: ChangeDeleted, Old: old, Seq: s.seq}, true
	}
	if old != nil && old.RV == e.RV && e.RV != "" && sameShape(old, e) {
		return Change{}, false
	}
	s.seq++
	e.Seq = s.seq
	s.putLocked(e)
	if old == nil {
		return Change{Type: ChangeAdded, New: e, Seq: s.seq}, true
	}
	if old.Namespace != e.Namespace {
		s.removeFromNS(old)
	}
	return Change{Type: ChangeModified, Old: old, New: e, Seq: s.seq}, true
}

// sameShape reports whether two entries hold the same representation. An entry
// prepared from a source that has since been replaced has another shape and must
// not be mistaken for an idempotent replay.
func sameShape(a, b *Entry) bool {
	return reflect.TypeOf(a.Obj) == reflect.TypeOf(b.Obj) && (a.Cells == nil) == (b.Cells == nil)
}

func (s *Shard) putLocked(e *Entry) {
	id := e.ID()
	s.entries[id] = e
	m := s.byNS[e.Namespace]
	if m == nil {
		m = make(map[string]*Entry)
		s.byNS[e.Namespace] = m
	}
	m[id] = e
}

func (s *Shard) removeFromNS(e *Entry) {
	if m := s.byNS[e.Namespace]; m != nil {
		delete(m, e.ID())
		if len(m) == 0 {
			delete(s.byNS, e.Namespace)
		}
	}
}

func (s *Shard) removeLocked(e *Entry) {
	delete(s.entries, e.ID())
	s.removeFromNS(e)
}

func (s *Shard) publishLocked(changes []Change) {
	if len(changes) == 0 {
		return
	}
	for sub := range s.subs {
		sub.enqueue(changes)
	}
}

// list runs one LIST. Entries stream into the shard page by page, so a first
// load is queryable while it is still paging. Entries whose resourceVersion is
// unchanged are left alone and entries absent from the result are swept at the
// end, so a relist emits only what actually differs.
func (s *Shard) list(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.synced {
		s.phase = PhaseRefreshing
	} else {
		s.phase = PhaseLoading
	}
	s.errMsg, s.forbidden = "", false
	s.loaded, s.expected = 0, 0
	src := s.src
	s.mu.Unlock()

	var pageSize int64
	if s.cfg.pageSize != nil {
		pageSize = s.cfg.pageSize()
	}
	seen := make(map[string]struct{})

	rv, err := src.List(ctx, ListOpts{PageSize: pageSize, Namespace: s.key.Namespace}, func(p Page) {
		batch := make([]prepared, 0, len(p.Items))
		for _, obj := range p.Items {
			e, err := s.newEntry(obj)
			if err != nil {
				continue
			}
			seen[e.ID()] = struct{}{}
			batch = append(batch, prepared{typ: watch.Added, entry: e})
		}
		s.mu.Lock()
		changes := make([]Change, 0, len(batch))
		for _, b := range batch {
			if c, ok := s.applyLocked(b.typ, b.entry); ok {
				changes = append(changes, c)
			}
		}
		s.loaded = p.Loaded
		if p.Remaining != nil {
			s.expected = p.Loaded + int(*p.Remaining)
		}
		s.publishLocked(changes)
		s.mu.Unlock()
	})
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	var changes []Change
	for id, e := range s.entries {
		if _, ok := seen[id]; !ok {
			s.removeLocked(e)
			s.seq++
			changes = append(changes, Change{Type: ChangeDeleted, Old: e, Seq: s.seq})
		}
	}
	s.publishLocked(changes)
	s.rv = rv
	s.synced = true
	s.expected = s.loaded
	s.phase = PhaseWatching
	s.errMsg, s.forbidden = "", false
	s.mu.Unlock()
	return rv, nil
}

// peek returns the stored objects of a namespace when the shard may be read as
// a warm cache: synced, following the watch (or polling) and holding full
// objects.
func (s *Shard) peekable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.synced && s.mode == ModeFull && s.key.Type != "secrets" && (s.phase == PhaseWatching || s.phase == PhaseRefreshing)
}

// find returns the entry with the uid. A caller that names a uid means that
// object and no other: when it is gone, a different object that has since taken
// its name (a recreated StatefulSet pod) is not a match. Only a lookup without
// a real uid goes by namespace and name.
func (s *Shard) find(namespace, name, uid string) *Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if IsRealUID(uid) {
		if e := s.entries[uid]; e != nil && (name == "" || e.Name == name) {
			return e
		}
		return nil
	}
	for _, e := range s.byNS[namespace] {
		if e.Name == name {
			return e
		}
	}
	return nil
}

// IsRealUID reports whether id is an object UID rather than empty or the
// namespace/name key Entry.ID falls back to for objects that have none.
func IsRealUID(id string) bool {
	return id != "" && !strings.Contains(id, "/")
}
