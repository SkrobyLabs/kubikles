package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"kubikles/pkg/agent"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

const (
	secretWatcherConnected                   = "connected"
	secretWatcherReconnecting                = "reconnecting"
	secretWatchUnavailable                   = "watch_unavailable"
	secretWatchRVExpired                     = "resource_version_expired"
	secretWatchMalformed                     = "malformed_watch_event"
	acceleratorSecretWatchMaxSpecsPerSession = 128
)

var errAcceleratorSecretWatchLimit = errors.New("Accelerator Secret watch subscription limit reached")

type secretWatchSource func(context.Context, string, string, k8s.SecretListOptions) (watch.Interface, error)
type secretWatchSleep func(context.Context, time.Duration) error
type secretWatchStream struct {
	spec   secretWatchSpec
	id     SecretWatchSpecID
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

type AcceleratorSecretWatchManager struct {
	mu                 sync.Mutex
	streams            map[SecretWatchSpecID]*secretWatchStream
	sessionSpecs       map[agent.SessionID]map[SecretWatchSpecID]server.AcceleratorSocketGeneration
	sessionGenerations map[agent.SessionID]server.AcceleratorSocketGeneration
	closed             bool
	workerWG           sync.WaitGroup
	clearStarted       bool
	clearDone          chan struct{}
	clearErr           error
	source             secretWatchSource
	emit               func([]server.AcceleratorSessionTarget, server.Event)
	lease              func(agent.SessionID) (server.AcceleratorSessionLease, bool)
	sleep              secretWatchSleep
	logger             func(string)
}

func newAcceleratorSecretWatchManager(source secretWatchSource, lease func(agent.SessionID) (server.AcceleratorSessionLease, bool), emit func([]server.AcceleratorSessionTarget, server.Event)) *AcceleratorSecretWatchManager {
	return &AcceleratorSecretWatchManager{
		streams:            make(map[SecretWatchSpecID]*secretWatchStream),
		sessionSpecs:       make(map[agent.SessionID]map[SecretWatchSpecID]server.AcceleratorSocketGeneration),
		sessionGenerations: make(map[agent.SessionID]server.AcceleratorSocketGeneration),
		source:             source,
		lease:              lease,
		emit:               emit,
		logger:             func(message string) { log.Print(message) },
		sleep: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
}

func (m *AcceleratorSecretWatchManager) Subscribe(call agent.AuthenticatedCallContext, namespace string, exclude bool) (SecretWatchSubscription, error) {
	if m == nil || !call.IsAuthenticated() || m.lease == nil {
		return SecretWatchSubscription{}, errors.New("Accelerator Secret watch unavailable")
	}
	lease, ok := m.lease(call.SessionID)
	if !ok || !lease.Connected {
		return SecretWatchSubscription{}, errors.New("Accelerator session is not connected")
	}
	spec := secretWatchSpec{namespace: namespace, excludeHelmReleases: exclude}
	id := spec.id()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return SecretWatchSubscription{}, errors.New("Accelerator Secret watch unavailable")
	}
	if now, ok := m.lease(call.SessionID); !ok || !now.Connected || now.Generation != lease.Generation {
		return SecretWatchSubscription{}, errors.New("Accelerator session changed")
	}
	m.sessionGenerations[call.SessionID] = lease.Generation
	members := m.sessionSpecs[call.SessionID]
	if members == nil {
		members = make(map[SecretWatchSpecID]server.AcceleratorSocketGeneration)
		m.sessionSpecs[call.SessionID] = members
	} else if _, exists := members[id]; !exists && len(members) >= acceleratorSecretWatchMaxSpecsPerSession {
		return SecretWatchSubscription{}, errAcceleratorSecretWatchLimit
	}
	members[id] = lease.Generation
	if m.streams[id] == nil {
		ctx, cancel := context.WithCancel(context.Background())
		stream := &secretWatchStream{spec: spec, id: id, ctx: ctx, cancel: cancel, done: make(chan struct{})}
		m.streams[id] = stream
		m.workerWG.Add(1)
		go m.run(stream)
	}
	return SecretWatchSubscription{WatcherSpecID: id}, nil
}

// Unsubscribe captures the retained lease before entering the manager barrier,
// then rechecks that exact lease while holding the manager mutex. A stale,
// guessed, revoked, or missing request is therefore a harmless no-op.
func (m *AcceleratorSecretWatchManager) Unsubscribe(call agent.AuthenticatedCallContext, id SecretWatchSpecID) error {
	if m == nil || !call.IsAuthenticated() || m.lease == nil {
		return nil
	}
	captured, ok := m.lease(call.SessionID)
	if !ok {
		return nil
	}
	var stop *secretWatchStream
	m.mu.Lock()
	current, currentOK := m.lease(call.SessionID)
	members := m.sessionSpecs[call.SessionID]
	membershipGeneration, member := members[id]
	retainedGeneration, retained := m.sessionGenerations[call.SessionID]
	if m.closed || !currentOK || current.Generation != captured.Generation || !retained || retainedGeneration != captured.Generation || !member || membershipGeneration != captured.Generation {
		m.mu.Unlock()
		return nil
	}
	delete(members, id)
	if len(members) == 0 {
		delete(m.sessionSpecs, call.SessionID)
		delete(m.sessionGenerations, call.SessionID)
	}
	if !m.hasMemberLocked(id) {
		stop = m.streams[id]
		delete(m.streams, id)
	}
	m.mu.Unlock()
	if stop != nil {
		stop.cancel()
		<-stop.done
	}
	return nil
}

func (m *AcceleratorSecretWatchManager) hasMemberLocked(id SecretWatchSpecID) bool {
	for _, specs := range m.sessionSpecs {
		if _, ok := specs[id]; ok {
			return true
		}
	}
	return false
}

// emitFor deliberately holds the manager mutex through the registry call. The
// target generation and nonblocking queue publication are one atomic fence.
func (m *AcceleratorSecretWatchManager) emitFor(stream *secretWatchStream, name string, data interface{}) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.streams[stream.id] != stream {
		return false
	}
	targets := make([]server.AcceleratorSessionTarget, 0)
	for session, specs := range m.sessionSpecs {
		if generation, ok := specs[stream.id]; ok {
			targets = append(targets, server.AcceleratorSessionTarget{SessionID: session, Generation: generation})
		}
	}
	if m.emit != nil {
		m.emit(targets, server.Event{Type: "event", Name: name, Data: data})
		return true
	}
	return false
}

func (m *AcceleratorSecretWatchManager) emitStatus(stream *secretWatchStream, status string) {
	m.emitFor(stream, "watcher-status", AcceleratorSecretWatcherStatus{WatcherSpecID: stream.id, Status: status})
}

func (m *AcceleratorSecretWatchManager) emitRestart(stream *secretWatchStream, code string) {
	if stream.ctx.Err() != nil {
		return
	}
	accepted := m.emitFor(stream, "watcher-error", AcceleratorSecretWatcherError{WatcherSpecID: stream.id, Code: code, Recoverable: true})
	m.emitStatus(stream, secretWatcherReconnecting)
	if accepted && m.logger != nil {
		switch code {
		case secretWatchRVExpired:
			m.logger("Accelerator Secret watch resource version expired")
		case secretWatchMalformed:
			m.logger("Accelerator Secret watch received a malformed event")
		default:
			m.logger("Accelerator Secret watch unavailable")
		}
	}
}

// run has one terminal reason and one restart path for each watch attempt.
// Kubernetes errors are reduced to fixed codes before they reach events/logs.
func (m *AcceleratorSecretWatchManager) run(stream *secretWatchStream) {
	defer m.workerWG.Done()
	defer close(stream.done)
	var resourceVersion string
	failures := 0
	for stream.ctx.Err() == nil {
		var active watch.Interface
		var err error
		if m.source != nil {
			active, err = m.source(stream.ctx, stream.spec.namespace, resourceVersion, k8s.SecretListOptions{ExcludeHelmReleases: stream.spec.excludeHelmReleases})
		} else {
			err = errors.New("watch source unavailable")
		}
		if stream.ctx.Err() != nil {
			return
		}
		terminalCode := secretWatchUnavailable
		if err != nil && (apierrors.IsResourceExpired(err) || apierrors.IsGone(err)) {
			terminalCode = secretWatchRVExpired
			resourceVersion = ""
		}
		if err == nil && active != nil {
			failures = 0
			m.emitStatus(stream, secretWatcherConnected)
			terminalCode, resourceVersion = m.consumeWatch(stream, active, resourceVersion)
			active.Stop()
			if stream.ctx.Err() != nil {
				return
			}
		}
		failures++
		m.emitRestart(stream, terminalCode)
		if stream.ctx.Err() != nil || m.sleep == nil || m.sleep(stream.ctx, secretWatchRetryDelay(failures)) != nil {
			return
		}
	}
}

func (m *AcceleratorSecretWatchManager) consumeWatch(stream *secretWatchStream, active watch.Interface, resourceVersion string) (string, string) {
	for {
		select {
		case <-stream.ctx.Done():
			return secretWatchUnavailable, resourceVersion
		case event, open := <-active.ResultChan():
			if !open {
				return secretWatchUnavailable, resourceVersion
			}
			switch event.Type {
			case watch.Error:
				if status, ok := event.Object.(*metav1.Status); ok && (status.Reason == metav1.StatusReasonExpired || status.Reason == metav1.StatusReasonGone || status.Code == 410) {
					return secretWatchRVExpired, ""
				}
				return secretWatchUnavailable, resourceVersion
			case watch.Bookmark:
				accessor, err := meta.Accessor(event.Object)
				if err != nil {
					return secretWatchMalformed, resourceVersion
				}
				if rv := accessor.GetResourceVersion(); rv != "" {
					resourceVersion = rv
				}
			case watch.Added, watch.Modified, watch.Deleted:
				secret, ok := event.Object.(*v1.Secret)
				if !ok {
					return secretWatchMalformed, resourceVersion
				}
				if secret.ResourceVersion != "" {
					resourceVersion = secret.ResourceVersion
				}
				if stream.spec.excludeHelmReleases && string(secret.Type) == k8s.HelmReleaseSecretType {
					continue
				}
				m.emitFor(stream, "resource-event", AcceleratorSecretResourceEvent{
					Type:          string(event.Type),
					ResourceType:  "secrets",
					Namespace:     secret.Namespace,
					WatcherSpecID: stream.id,
					Resource:      projectAcceleratorSecretListItem(k8s.ProjectSecretListItem(secret)),
				})
			default:
				return secretWatchMalformed, resourceVersion
			}
		}
	}
}

func secretWatchRetryDelay(failures int) time.Duration {
	shift := failures - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 7 {
		shift = 7
	}
	delay := time.Second * time.Duration(1<<shift)
	if delay > 2*time.Minute {
		return 2 * time.Minute
	}
	return delay
}

func (m *AcceleratorSecretWatchManager) SessionConnected(snapshot server.AcceleratorSessionSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	old := m.sessionGenerations[snapshot.CallContext.SessionID]
	if snapshot.Generation <= old {
		return
	}
	m.sessionGenerations[snapshot.CallContext.SessionID] = snapshot.Generation
	for id := range m.sessionSpecs[snapshot.CallContext.SessionID] {
		m.sessionSpecs[snapshot.CallContext.SessionID][id] = snapshot.Generation
	}
}

func (m *AcceleratorSecretWatchManager) SessionDisconnected(server.AcceleratorSessionSnapshot) {}

func (m *AcceleratorSecretWatchManager) SessionRevoked(snapshot server.AcceleratorSessionSnapshot) {
	m.removeSession(snapshot.CallContext.SessionID, snapshot.Generation)
}

func (m *AcceleratorSecretWatchManager) removeSession(session agent.SessionID, generation server.AcceleratorSocketGeneration) {
	var stops []*secretWatchStream
	m.mu.Lock()
	if current, ok := m.sessionGenerations[session]; !ok || current != generation {
		m.mu.Unlock()
		return
	}
	ids := m.sessionSpecs[session]
	delete(m.sessionSpecs, session)
	delete(m.sessionGenerations, session)
	for id := range ids {
		if !m.hasMemberLocked(id) {
			if stream := m.streams[id]; stream != nil {
				delete(m.streams, id)
				stops = append(stops, stream)
			}
		}
	}
	m.mu.Unlock()
	for _, stream := range stops {
		stream.cancel()
		<-stream.done
	}
}

// ClearAll starts one terminal operation. Every caller observes the same
// completion; a caller cancellation only stops that caller from waiting and
// never poisons the cleanup shared by later callers.
func (m *AcceleratorSecretWatchManager) ClearAll(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if !m.clearStarted {
		m.clearStarted = true
		m.closed = true
		m.clearDone = make(chan struct{})
		streams := make([]*secretWatchStream, 0, len(m.streams))
		for _, stream := range m.streams {
			streams = append(streams, stream)
		}
		m.streams = make(map[SecretWatchSpecID]*secretWatchStream)
		m.sessionSpecs = make(map[agent.SessionID]map[SecretWatchSpecID]server.AcceleratorSocketGeneration)
		m.sessionGenerations = make(map[agent.SessionID]server.AcceleratorSocketGeneration)
		done := m.clearDone
		go func() {
			for _, stream := range streams {
				stream.cancel()
			}
			m.workerWG.Wait()
			m.mu.Lock()
			m.clearErr = nil
			close(done)
			m.mu.Unlock()
		}()
	}
	done := m.clearDone
	m.mu.Unlock()
	select {
	case <-done:
		m.mu.Lock()
		err := m.clearErr
		m.mu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
