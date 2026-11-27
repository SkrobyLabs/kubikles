package store

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"

	"context"

	"kubikles/pkg/k8s"
	"kubikles/pkg/watchevents"
)

// ClientProvider supplies the clients shards are built from. The application
// implements it over k8s.Client; tests inject fakes. Any method may fail, and
// Metadata may report ErrNoMetadata when the context has no REST
// configuration, in which case the shard converts full objects in process.
type ClientProvider interface {
	Kube(contextName string) (kubernetes.Interface, error)
	Dynamic(contextName string) (dynamic.Interface, error)
	Metadata(contextName string) (metadata.Interface, error)
}

// ErrNoMetadata is returned by ClientProvider.Metadata when no metadata client
// can be built for the context.
var ErrNoMetadata = errors.New("no metadata client available")

// Default tuning, matching the legacy watchers where a counterpart exists.
const (
	DefaultPageSize   int64 = 1000
	DefaultLinger           = 5 * time.Second
	ViewLinger              = 30 * time.Second
	defaultBaseDelay        = time.Second
	defaultMaxDelay         = 2 * time.Minute
	defaultForbidWait       = 2 * time.Minute
)

// DefaultMaxBytes is the byte budget the application applies. Measured
// (TestIngestHeapPerEntry): 50,000 mid-sized pods are about 390 MiB resident,
// and the estimate tracks that within 15 percent, so 1 GiB holds roughly
// 130,000 such pods of idle, evictable shards. Leased shards are never evicted,
// so the budget only bounds what lingers.
const DefaultMaxBytes int64 = 1 << 30

// EnvDisableTable forces every shard to full objects.
const EnvDisableTable = "KUBIKLES_DISABLE_TABLE"

// Options configures a Manager. The zero value is usable.
type Options struct {
	PageSize     int64         // LIST page size when progress is wanted; 0 = DefaultPageSize
	Linger       time.Duration // keep-warm after the last release; 0 = DefaultLinger
	MaxBytes     int64         // estimated byte budget over all shards; 0 = unlimited
	PollInterval time.Duration // > 0 replaces the watch with periodic relist+diff
	Clock        Clock
	WatchTimeout int64 // seconds; 0 = legacy default

	// Retry tuning, exposed for tests.
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	ForbidWait time.Duration
	// Jitter spreads backoff delays; nil uses a random shortening of up to 20 percent. Tests
	// pass an identity function to keep delays exact.
	Jitter func(time.Duration) time.Duration

	StatusSink func(watchevents.Status)
	ErrorSink  func(watchevents.Error)

	// DisableTable forces full objects; defaults to the KUBIKLES_DISABLE_TABLE
	// environment variable.
	DisableTable *bool
}

// Hint says what an acquirer needs from a shard.
type Hint struct {
	// Mode is the lightest representation the caller can work with. A shard that
	// already holds more serves it; one that holds less is upgraded.
	Mode Mode
	// Linger overrides how long the shard stays warm after the last lease is
	// released. Views use ViewLinger.
	Linger time.Duration
	// NoProgress asks for a single unbounded LIST (watch-cache friendly) instead
	// of pages. It only applies when the acquire creates the shard.
	NoProgress bool
	// Prune lists the top-level fields a full-object custom resource shard keeps besides
	// metadata, apiVersion and kind. Nil keeps whole objects. It only applies to crd: types
	// and to the full-object representation (including the fallback from Table).
	Prune []string
}

// Manager owns the shards.
type Manager struct {
	clients ClientProvider
	clock   Clock

	mu       sync.Mutex
	shards   map[Key]*Shard
	opts     Options
	rootCtx  context.Context
	rootStop context.CancelFunc
	disabled bool

	pruneMu sync.Mutex
	prune   map[string][]string
}

// New creates a manager. Shards run until released and lingered out, or until
// Reset.
func New(clients ClientProvider, opts Options) *Manager {
	if opts.PageSize <= 0 {
		opts.PageSize = DefaultPageSize
	}
	if opts.Linger <= 0 {
		opts.Linger = DefaultLinger
	}
	if opts.BaseDelay <= 0 {
		opts.BaseDelay = defaultBaseDelay
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = defaultMaxDelay
	}
	if opts.Jitter == nil {
		opts.Jitter = defaultJitter
	}
	if opts.ForbidWait <= 0 {
		opts.ForbidWait = defaultForbidWait
	}
	clock := opts.Clock
	if clock == nil {
		clock = realClock{}
	}
	disabled := os.Getenv(EnvDisableTable) == "1"
	if opts.DisableTable != nil {
		disabled = *opts.DisableTable
	}
	m := &Manager{clients: clients, clock: clock, shards: make(map[Key]*Shard), opts: opts, disabled: disabled}
	m.rootCtx, m.rootStop = context.WithCancel(context.Background())
	return m
}

// SetSinks installs the watcher-status and watcher-error sinks. Shards created
// afterwards use them.
func (m *Manager) SetSinks(status func(watchevents.Status), errs func(watchevents.Error)) {
	m.mu.Lock()
	m.opts.StatusSink, m.opts.ErrorSink = status, errs
	m.mu.Unlock()
}

// SetMaxBytes sets the byte budget (0 = unlimited) and evicts idle shards if
// the store is over it.
func (m *Manager) SetMaxBytes(n int64) {
	m.mu.Lock()
	m.opts.MaxBytes = n
	victims := m.evictLocked()
	m.mu.Unlock()
	stopAll(victims)
}

// SetPollInterval switches shards between watching (0) and periodic relist
// plus diff. It takes effect at the next loop iteration of every shard; kick
// them so it applies at once.
func (m *Manager) SetPollInterval(d time.Duration) {
	m.mu.Lock()
	m.opts.PollInterval = d
	shards := m.shardListLocked()
	m.mu.Unlock()
	for _, s := range shards {
		s.kick()
	}
}

func (m *Manager) pollInterval() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.opts.PollInterval
}

func (m *Manager) shardListLocked() []*Shard {
	out := make([]*Shard, 0, len(m.shards))
	for _, s := range m.shards {
		out = append(out, s)
	}
	return out
}

func stopAll(shards []*Shard) {
	for _, s := range shards {
		s.stop()
	}
}

// normalize validates the type and drops the namespace of cluster-scoped types.
func normalize(key Key) (Key, error) {
	if def, ok := k8s.LookupResource(key.Type); ok {
		if !def.Namespaced {
			key.Namespace = ""
		}
		return key, nil
	}
	if _, ok := ParseCRDType(key.Type); ok {
		return key, nil
	}
	return key, fmt.Errorf("unsupported resource type: %s", key.Type)
}

func gvrFor(key Key) (schema.GroupVersionResource, bool) {
	if def, ok := k8s.LookupResource(key.Type); ok {
		return def.GVR, true
	}
	return ParseCRDType(key.Type)
}

// buildSource creates the source for key honouring the requested mode where it
// can, and reports why not where it cannot.
func (m *Manager) buildSource(key Key, want Mode) (Source, Mode, string, error) {
	m.mu.Lock()
	timeout, disabled := m.opts.WatchTimeout, m.disabled
	m.mu.Unlock()
	return m.buildSourceWith(key, want, timeout, disabled)
}

func (m *Manager) buildSourceWith(key Key, want Mode, timeout int64, disabled bool) (Source, Mode, string, error) {

	full := func() (Source, error) {
		if gvr, ok := ParseCRDType(key.Type); ok {
			dc, err := m.clients.Dynamic(key.Context)
			if err != nil {
				return nil, err
			}
			var src Source = newDynamicSource(dc, gvr, key.Namespace, timeout)
			if roots := m.pruneRoots(key.Type); roots != nil {
				src = newPruneSource(src, roots)
			}
			return src, nil
		}
		cs, err := m.clients.Kube(key.Context)
		if err != nil {
			return nil, err
		}
		src, err := newTypedSource(cs, key.Type, key.Namespace, timeout)
		if err == nil && key.Type == "secrets" {
			return scrubSecretSource{inner: src}, nil
		}
		return src, err
	}
	fullOnly := func(why string) (Source, Mode, string, error) {
		src, err := full()
		return src, ModeFull, why, err
	}

	switch want {
	case ModeTable:
		if disabled {
			return fullOnly(EnvDisableTable + " is set")
		}
		gvr, _ := gvrFor(key)
		cs, err := m.clients.Kube(key.Context)
		if err != nil {
			return nil, ModeFull, "", err
		}
		rc := k8s.RESTClientOf(cs)
		if rc == nil {
			return fullOnly("no REST client; serving full objects")
		}
		return newTableSource(rc, gvr, key.Namespace, timeout), ModeTable, "", nil
	case ModeMetadata:
		if disabled {
			return fullOnly(EnvDisableTable + " is set")
		}
		gvr, _ := gvrFor(key)
		md, err := m.clients.Metadata(key.Context)
		if err != nil || md == nil {
			inner, ferr := full()
			if ferr != nil {
				return nil, ModeMetadata, "", ferr
			}
			return convertedSource{inner: inner}, ModeMetadata, "no metadata client; converting full objects in process", nil
		}
		return newMetaSource(md, gvr, key.Namespace, timeout), ModeMetadata, "", nil
	default:
		src, err := full()
		return src, ModeFull, "", err
	}
}

func (m *Manager) newShardLocked(key Key, hint Hint) (*Shard, error) {
	src, mode, why, err := m.buildSourceWith(key, hint.Mode, m.opts.WatchTimeout, m.disabled)
	if err != nil {
		return nil, err
	}
	pageSize := m.opts.PageSize
	if hint.NoProgress {
		pageSize = 0
	}
	cfg := shardConfig{
		key:        key,
		build:      m.buildSource,
		clock:      m.clock,
		pageSize:   func() int64 { return pageSize },
		pollEvery:  m.pollInterval,
		baseDelay:  m.opts.BaseDelay,
		maxDelay:   m.opts.MaxDelay,
		forbidWait: m.opts.ForbidWait,
		jitter:     m.opts.Jitter,
		onPanic:    m.dropShard,
		onStatus:   m.opts.StatusSink,
		onError:    m.opts.ErrorSink,
		onSynced:   m.afterSync,
	}
	s := newShard(cfg, src, mode, hint.Mode, why)
	s.lastUsed = m.clock.Now()
	return s, nil
}

// afterSync enforces the byte budget once a LIST has grown a shard.
func (m *Manager) afterSync() {
	m.mu.Lock()
	victims := m.evictLocked()
	m.mu.Unlock()
	if len(victims) > 0 {
		// Never stop synchronously: the caller is a reflector goroutine and a
		// victim may be its own shard, whose stop waits for that goroutine.
		go stopAll(victims)
	}
}

// Lease is one holder of a shard. Release it when done; the shard then lingers
// before it stops.
type Lease struct {
	m         *Manager
	sh        *Shard
	key       Key
	namespace string // filter applied when a cluster-wide shard serves a namespaced key
	linger    time.Duration
	once      sync.Once
}

// Key is the key the lease was acquired for.
func (l *Lease) Key() Key { return l.key }

// Snapshot returns the entries visible to this lease and their sequence.
func (l *Lease) Snapshot() ([]*Entry, uint64) { return l.sh.Snapshot(l.namespace) }

// Subscribe registers for changes visible to this lease.
func (l *Lease) Subscribe(fn func([]Change), opts SubOptions) *Subscription {
	if l.namespace != "" {
		opts.Namespace = l.namespace
	}
	return l.sh.Subscribe(fn, opts)
}

// State describes the underlying shard.
func (l *Lease) State() State { return l.sh.State() }

// Columns returns the Table columns of the underlying shard.
func (l *Lease) Columns() []metav1.TableColumnDefinition { return l.sh.Columns() }

// Done is closed when the underlying shard stops (context switch, eviction).
// The holder should re-acquire.
func (l *Lease) Done() <-chan struct{} { return l.sh.Stopped() }

// ServedByClusterWide reports whether a cluster-wide shard backs this
// namespaced lease.
func (l *Lease) ServedByClusterWide() bool { return l.namespace != "" && l.sh.key.Namespace == "" }

// Release returns the lease. It is safe to call more than once.
func (l *Lease) Release() {
	l.once.Do(func() { l.m.release(l.sh, l.linger) })
}

// pruneRoots returns the pruning roots recorded for a custom resource type, nil for none.
func (m *Manager) pruneRoots(typ string) []string {
	m.pruneMu.Lock()
	defer m.pruneMu.Unlock()
	return m.prune[typ]
}

// IsPruned reports whether shards of the type hold pruned objects, so they must not answer for
// full-object reads.
func (m *Manager) IsPruned(typ string) bool { return m.pruneRoots(typ) != nil }

// setPrune records the roots for a type and reports whether they changed.
func (m *Manager) setPrune(typ string, roots []string) bool {
	if _, ok := ParseCRDType(typ); !ok {
		return false
	}
	m.pruneMu.Lock()
	defer m.pruneMu.Unlock()
	old := m.prune[typ]
	if sameStrings(old, roots) && (old == nil) == (roots == nil) {
		return false
	}
	if m.prune == nil {
		m.prune = map[string][]string{}
	}
	if roots == nil {
		delete(m.prune, typ)
	} else {
		m.prune[typ] = append([]string(nil), roots...)
	}
	return true
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, x := range a {
		set[x] = true
	}
	for _, x := range b {
		if !set[x] {
			return false
		}
	}
	return true
}

// Acquire returns a lease on the shard for key, creating and starting it when
// needed. A warm cluster-wide shard serves a namespaced acquire; the reverse
// never happens.
func (m *Manager) Acquire(key Key, hint Hint) (*Lease, error) {
	key, err := normalize(key)
	if err != nil {
		return nil, err
	}
	linger := hint.Linger
	if linger <= 0 {
		linger = m.opts.Linger
	}
	pruneChanged := m.setPrune(key.Type, hint.Prune)

	m.mu.Lock()
	sh := m.shards[key]
	served := ""
	if sh == nil && key.Namespace != "" {
		if cw := m.shards[key.ClusterWide()]; cw != nil {
			if st := cw.State(); st.Synced && st.Mode.strength() >= hint.Mode.strength() {
				sh, served = cw, key.Namespace
			}
		}
	}
	var upgrade *Shard
	if sh == nil {
		sh, err = m.newShardLocked(key, hint)
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
		m.shards[key] = sh
		sh.refs++
		m.mu.Unlock()
		sh.start(m.rootCtx)
		return &Lease{m: m, sh: sh, key: key, linger: linger}, nil
	}
	m.touchLocked(sh)
	sh.refs++
	if sh.Mode().strength() < hint.Mode.strength() {
		upgrade = sh
	}
	m.mu.Unlock()

	if upgrade == nil && pruneChanged && sh.Mode() == ModeFull && served == "" {
		// The set of fields the columns read changed; rebuild the shard with the new roots.
		upgrade = sh
	}
	if upgrade != nil {
		if err := m.upgrade(upgrade, hint.Mode); err != nil {
			m.release(sh, 0)
			return nil, err
		}
	}
	return &Lease{m: m, sh: sh, key: key, namespace: served, linger: linger}, nil
}

// upgrade rebuilds a shard in a stronger mode. Subscribers see a reset.
func (m *Manager) upgrade(sh *Shard, want Mode) error {
	src, mode, why, err := m.buildSource(sh.key, want)
	if err != nil {
		return err
	}
	sh.setRequested(want)
	sh.replaceSource(src, mode, why, true)
	return nil
}

// touchLocked cancels a pending linger expiry and marks the shard used.
func (m *Manager) touchLocked(sh *Shard) {
	sh.lingerSeq++
	if sh.lingerStop != nil {
		sh.lingerStop()
		sh.lingerStop = nil
	}
	sh.lastUsed = m.clock.Now()
}

func (m *Manager) release(sh *Shard, linger time.Duration) {
	m.mu.Lock()
	if m.shards[sh.key] != sh {
		// Already removed by Reset or eviction.
		m.mu.Unlock()
		return
	}
	sh.refs--
	if sh.refs > 0 {
		m.mu.Unlock()
		return
	}
	sh.lastUsed = m.clock.Now()
	sh.lingerSeq++
	seq := sh.lingerSeq
	if linger <= 0 {
		delete(m.shards, sh.key)
		m.mu.Unlock()
		sh.stop()
		return
	}
	sh.lingerStop = m.clock.AfterFunc(linger, func() { m.expire(sh, seq) })
	victims := m.evictLocked()
	m.mu.Unlock()
	stopAll(victims)
}

// dropShard unregisters a shard whose reflector panicked and stops it, so the next Acquire of
// its key builds a fresh one. Held leases see Done close, like after a Reset.
func (m *Manager) dropShard(sh *Shard) {
	m.mu.Lock()
	if m.shards[sh.key] == sh {
		delete(m.shards, sh.key)
		sh.lingerSeq++
		if sh.lingerStop != nil {
			sh.lingerStop()
			sh.lingerStop = nil
		}
	}
	m.mu.Unlock()
	sh.stop()
}

func (m *Manager) expire(sh *Shard, seq uint64) {
	m.mu.Lock()
	if m.shards[sh.key] != sh || sh.refs > 0 || sh.lingerSeq != seq {
		m.mu.Unlock()
		return
	}
	delete(m.shards, sh.key)
	m.mu.Unlock()
	sh.stop()
}

// evictLocked removes idle (unleased) shards, least recently used first, until
// the estimated size is within MaxBytes. Leased shards are never evicted. The
// caller stops the returned shards after unlocking.
func (m *Manager) evictLocked() []*Shard {
	if m.opts.MaxBytes <= 0 {
		return nil
	}
	var total int64
	var idle []*Shard
	for _, s := range m.shards {
		total += s.estimatedBytes()
		if s.refs == 0 {
			idle = append(idle, s)
		}
	}
	if total <= m.opts.MaxBytes {
		return nil
	}
	sort.Slice(idle, func(i, j int) bool { return idle[i].lastUsed.Before(idle[j].lastUsed) })
	var victims []*Shard
	for _, s := range idle {
		if total <= m.opts.MaxBytes {
			break
		}
		total -= s.estimatedBytes()
		delete(m.shards, s.key)
		s.lingerSeq++
		if s.lingerStop != nil {
			s.lingerStop()
			s.lingerStop = nil
		}
		victims = append(victims, s)
	}
	return victims
}

// Pin keeps a shard warm for ttl on behalf of owner (a metrics poll, an issue
// scan). It is an Acquire that releases itself; the returned lease may be
// released earlier.
func (m *Manager) Pin(key Key, owner string, ttl time.Duration, hint Hint) (*Lease, error) {
	lease, err := m.Acquire(key, hint)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	lease.sh.pins[owner]++
	m.mu.Unlock()
	stop := m.clock.AfterFunc(ttl, lease.Release)
	_ = stop
	return lease, nil
}

// PinExisting is Pin for a shard that already exists (held by a lease or lingering). It never
// creates a shard and reports false when there is none, so a reader that only wants to keep a
// warm shard warm cannot start a cluster-wide LIST+WATCH on its own.
func (m *Manager) PinExisting(key Key, owner string, ttl time.Duration) bool {
	key, err := normalize(key)
	if err != nil {
		return false
	}
	m.mu.Lock()
	sh := m.shards[key]
	if sh == nil {
		m.mu.Unlock()
		return false
	}
	m.touchLocked(sh)
	sh.refs++
	sh.pins[owner]++
	m.mu.Unlock()
	lease := &Lease{m: m, sh: sh, key: key, linger: m.opts.Linger}
	m.clock.AfterFunc(ttl, lease.Release)
	return true
}

// Reset stops every shard (context switch) and waits for their goroutines.
// Held leases become inert: Done is closed and Release is a no-op.
func (m *Manager) Reset() {
	m.mu.Lock()
	shards := m.shardListLocked()
	m.shards = make(map[Key]*Shard)
	for _, s := range shards {
		if s.lingerStop != nil {
			s.lingerStop()
			s.lingerStop = nil
		}
		s.lingerSeq++
	}
	m.mu.Unlock()
	stopAll(shards)
}

// Resync makes the shards of the given keys (all shards when none are given)
// rebuild their clients and relist, diffing against what they hold. It is the
// RestartAll counterpart, used after a kubeconfig edit.
func (m *Manager) Resync(keys ...Key) {
	m.mu.Lock()
	var targets []*Shard
	if len(keys) == 0 {
		targets = m.shardListLocked()
	} else {
		for _, k := range keys {
			if s := m.shards[k]; s != nil {
				targets = append(targets, s)
			}
		}
	}
	m.mu.Unlock()
	for _, s := range targets {
		st := s.State()
		src, mode, why, err := m.buildSource(s.key, st.Requested)
		if err != nil {
			s.kick()
			continue
		}
		s.replaceSource(src, mode, why, false)
	}
}

// Close stops everything and prevents further use.
func (m *Manager) Close() {
	m.Reset()
	m.rootStop()
}

// shardFor finds the shard that can answer a read for key without acquiring.
func (m *Manager) shardFor(key Key) (*Shard, string) {
	key, err := normalize(key)
	if err != nil {
		return nil, ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.shards[key]; s != nil {
		return s, ""
	}
	if key.Namespace != "" {
		if s := m.shards[key.ClusterWide()]; s != nil {
			return s, key.Namespace
		}
	}
	return nil, ""
}

// Peek returns the stored objects of key as values of T together with the
// shard resourceVersion, without ever starting a LIST. ok is false when the
// shard is absent, not yet synced, backing off, or does not hold T objects
// (Table and metadata shards, or a different type). Items are shallow value
// copies assembled fresh for the caller; share nothing mutable through them.
func Peek[T any](m *Manager, key Key) (items []T, rv string, ok bool) {
	if m == nil {
		return nil, "", false
	}
	sh, ns := m.shardFor(key)
	if sh == nil || !sh.peekable() {
		return nil, "", false
	}
	entries, _ := sh.Snapshot(ns)
	out := make([]T, 0, len(entries))
	for _, e := range entries {
		p, isT := any(e.Obj).(*T)
		if !isT {
			return nil, "", false
		}
		out = append(out, *p)
	}
	m.mu.Lock()
	sh.lastUsed = m.clock.Now()
	m.mu.Unlock()
	return out, sh.State().RV, true
}

// ShardStats describes one shard for the performance panel.
type ShardStats struct {
	Key            string `json:"key"`
	Context        string `json:"context"`
	Type           string `json:"type"`
	Namespace      string `json:"namespace"`
	Phase          string `json:"phase"`
	Mode           string `json:"mode"`      // what the shard holds
	Requested      string `json:"requested"` // what was asked for
	Degraded       bool   `json:"degraded"`
	DegradedReason string `json:"degradedReason"`
	Entries        int    `json:"entries"`
	EstimatedBytes int64  `json:"estimatedBytes"`
	Refs           int    `json:"refs"`
	Lingering      bool   `json:"lingering"` // no lease holds it; kept warm until the linger expires
	Subscribers    int    `json:"subscribers"`
	Loaded         int    `json:"loaded"`
	Expected       int    `json:"expected"`
	Error          string `json:"error"`
	Forbidden      bool   `json:"forbidden"`
	Seq            uint64 `json:"seq"`
}

// Stats summarises the store.
type Stats struct {
	Shards         []ShardStats `json:"shards"`
	Entries        int          `json:"entries"`
	EstimatedBytes int64        `json:"estimatedBytes"`
	MaxBytes       int64        `json:"maxBytes"`
}

// Stats returns a snapshot of every shard, sorted by key.
func (m *Manager) Stats() Stats {
	m.mu.Lock()
	shards := m.shardListLocked()
	refs := make(map[*Shard]int, len(shards))
	for _, s := range shards {
		refs[s] = s.refs
	}
	max := m.opts.MaxBytes
	m.mu.Unlock()

	// A nil slice would encode as JSON null and break the frontend's iteration.
	out := Stats{MaxBytes: max, Shards: []ShardStats{}}
	for _, s := range shards {
		st := s.State()
		s.mu.RLock()
		subs := len(s.subs)
		s.mu.RUnlock()
		b := s.estimatedBytes()
		out.Shards = append(out.Shards, ShardStats{
			Key: s.key.String(), Context: s.key.Context, Type: s.key.Type, Namespace: s.key.Namespace,
			Phase: string(st.Phase), Mode: st.Mode.String(), Requested: st.Requested.String(),
			Degraded: st.Degraded, DegradedReason: st.DegradedReason, Entries: st.Entries,
			EstimatedBytes: b, Refs: refs[s], Lingering: refs[s] == 0, Subscribers: subs, Loaded: st.Loaded, Expected: st.Expected,
			Error: st.Error, Forbidden: st.Forbidden, Seq: st.Seq,
		})
		out.Entries += st.Entries
		out.EstimatedBytes += b
	}
	sort.Slice(out.Shards, func(i, j int) bool { return out.Shards[i].Key < out.Shards[j].Key })
	return out
}

// Lookup returns one stored object of a warm full-object shard, without ever starting a LIST.
// It finds by uid when given, otherwise by namespace and name. ok is false when the shard is
// absent, not synced, not holding full objects, or has no such object. Managed fields are
// already stripped; treat the object as read-only.
func (m *Manager) Lookup(key Key, namespace, name, uid string) (obj runtime.Object, rv string, ok bool) {
	if m == nil {
		return nil, "", false
	}
	if m.IsPruned(key.Type) {
		return nil, "", false
	}
	sh, ns := m.shardFor(key)
	if sh == nil || !sh.peekable() {
		return nil, "", false
	}
	e := sh.find(namespace, name, uid)
	if e == nil || (ns != "" && e.Namespace != ns) {
		return nil, "", false
	}
	m.mu.Lock()
	sh.lastUsed = m.clock.Now()
	m.mu.Unlock()
	return e.Obj, e.RV, true
}
