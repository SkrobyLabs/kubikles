package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"kubikles/pkg/k8s"
)

// ListRequestStats contains statistics about list requests
type ListRequestStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Completed int64 `json:"completed"`
	Canceled  int64 `json:"canceled"`
}

// requestEntry tracks a single request with its sequence number
type requestEntry struct {
	cancel   context.CancelFunc
	sequence int64
}

// ListRequestManager manages cancellable list requests with sequence tracking
// to prevent race conditions when old requests complete after new ones start
type ListRequestManager struct {
	mu       sync.RWMutex
	requests map[string]*requestEntry

	// Global sequence counter for race condition prevention
	sequence atomic.Int64

	// Whether to actually cancel HTTP requests.
	//
	// Due to a Go HTTP/2 bug, canceling requests can cause O(N²) performance collapse
	// and connection pool issues. When a request is canceled, sync.Cond.Broadcast() wakes
	// up every goroutine waiting on the connection, causing severe slowdowns.
	//
	// When disabled, cancel() is not called - requests complete in background
	// but stale results are ignored via sequence number tracking.
	//
	// See: https://github.com/golang/go/issues/34944
	cancellationEnabled atomic.Bool

	// Outcomes of recently finished lists, keyed by request ID. Lets the
	// frontend learn a list's resourceVersion (to anchor a watch) and whether
	// its items were streamed as pages instead of returned.
	outcomeMu    sync.Mutex
	outcomes     map[string]storedListOutcome
	outcomeOrder []string

	// Stats counters
	total     atomic.Int64
	pending   atomic.Int64
	completed atomic.Int64
	canceled  atomic.Int64
}

// DefaultListRequestTimeout is the default timeout for list requests
// Set to 60s to handle large resources (secrets, configmaps) on high-latency connections
const DefaultListRequestTimeout = 60 * time.Second

// NewListRequestManager creates a new list request manager
func NewListRequestManager() *ListRequestManager {
	m := &ListRequestManager{
		requests: make(map[string]*requestEntry),
		outcomes: make(map[string]storedListOutcome),
	}
	m.cancellationEnabled.Store(true) // Enabled by default
	return m
}

// SetCancellationEnabled enables or disables actual HTTP request cancellation.
// See cancellationEnabled field comment for details on the Go HTTP/2 bug.
func (m *ListRequestManager) SetCancellationEnabled(enabled bool) {
	m.cancellationEnabled.Store(enabled)
}

// IsCancellationEnabled returns whether HTTP request cancellation is enabled.
func (m *ListRequestManager) IsCancellationEnabled() bool {
	return m.cancellationEnabled.Load()
}

// StartRequest creates a new cancellable context for a request.
// If a request with the same ID exists, it will be canceled first (if cancellation is enabled).
// Returns the context to use and the sequence number for completing.
func (m *ListRequestManager) StartRequest(requestId string) (context.Context, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel any existing request with this ID
	if entry, exists := m.requests[requestId]; exists {
		// Only actually cancel if cancellation is enabled
		if m.cancellationEnabled.Load() {
			entry.cancel()
		}
		// Always update stats - the request is logically canceled even if HTTP continues
		m.canceled.Add(1)
		m.pending.Add(-1)
	}

	// Create new cancellable context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), DefaultListRequestTimeout)
	seq := m.sequence.Add(1)
	m.requests[requestId] = &requestEntry{
		cancel:   cancel,
		sequence: seq,
	}

	m.total.Add(1)
	m.pending.Add(1)

	return ctx, seq
}

// CompleteRequest marks a request as completed and removes it from tracking.
// Only completes if the sequence number matches (prevents double-decrement race condition).
func (m *ListRequestManager) CompleteRequest(requestId string, sequence int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Only complete if sequence matches (prevents double-decrement when old request
	// completes after new one starts)
	if entry, exists := m.requests[requestId]; exists && entry.sequence == sequence {
		delete(m.requests, requestId)
		m.completed.Add(1)
		m.pending.Add(-1)
	}
}

// CancelRequest cancels an in-flight request by ID.
// If cancellation is disabled, the request is removed from tracking but the HTTP request continues.
func (m *ListRequestManager) CancelRequest(requestId string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, exists := m.requests[requestId]; exists {
		// Only actually cancel if cancellation is enabled
		if m.cancellationEnabled.Load() {
			entry.cancel()
		}
		delete(m.requests, requestId)
		m.canceled.Add(1)
		m.pending.Add(-1)
		return true
	}
	return false
}

// GetStats returns current statistics
func (m *ListRequestManager) GetStats() ListRequestStats {
	return ListRequestStats{
		Total:     m.total.Load(),
		Pending:   m.pending.Load(),
		Completed: m.completed.Load(),
		Canceled:  m.canceled.Load(),
	}
}

// Outcomes are only read right after their list completes, so a short TTL and
// a small cap are enough; the cap bounds memory when a caller never reads them.
const (
	listOutcomeTTL  = 60 * time.Second
	maxListOutcomes = 256
)

type storedListOutcome struct {
	outcome k8s.ListOutcome
	context string
	expires time.Time
}

// RecordOutcome stores the outcome of a finished list under its request ID.
func (m *ListRequestManager) RecordOutcome(requestId, contextName string, outcome k8s.ListOutcome) {
	m.outcomeMu.Lock()
	defer m.outcomeMu.Unlock()
	if _, exists := m.outcomes[requestId]; !exists {
		m.outcomeOrder = append(m.outcomeOrder, requestId)
	}
	m.outcomes[requestId] = storedListOutcome{outcome, contextName, time.Now().Add(listOutcomeTTL)}
	for len(m.outcomeOrder) > maxListOutcomes {
		delete(m.outcomes, m.outcomeOrder[0])
		m.outcomeOrder = m.outcomeOrder[1:]
	}
}

// Outcome returns the stored outcome for a request ID, if it is still fresh.
func (m *ListRequestManager) Outcome(requestId string) (k8s.ListOutcome, string, bool) {
	m.outcomeMu.Lock()
	defer m.outcomeMu.Unlock()
	stored, ok := m.outcomes[requestId]
	if !ok || time.Now().After(stored.expires) {
		return k8s.ListOutcome{}, "", false
	}
	return stored.outcome, stored.context, true
}
