package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsRequestStats contains statistics about metrics requests
type MetricsRequestStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Completed int64 `json:"completed"`
	Canceled  int64 `json:"canceled"`
}

// DefaultMetricsRequestTimeout is the default timeout for metrics requests
const DefaultMetricsRequestTimeout = 30 * time.Second

// metricsRequestEntry tracks a single request with its sequence number
type metricsRequestEntry struct {
	cancel   context.CancelFunc
	sequence int64
}

// MetricsRequestManager manages cancellable metrics requests with timeout
type MetricsRequestManager struct {
	mu       sync.RWMutex
	requests map[string]*metricsRequestEntry

	// Global sequence counter for race condition prevention
	sequence atomic.Int64

	// Stats counters
	total     atomic.Int64
	pending   atomic.Int64
	completed atomic.Int64
	canceled  atomic.Int64
}

// NewMetricsRequestManager creates a new request manager
func NewMetricsRequestManager() *MetricsRequestManager {
	return &MetricsRequestManager{
		requests: make(map[string]*metricsRequestEntry),
	}
}

// StartRequest creates a new cancellable context for a request with timeout.
// Returns the context to use and the sequence number for completing.
func (m *MetricsRequestManager) StartRequest(requestId string) (context.Context, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel any existing request with this ID
	if entry, exists := m.requests[requestId]; exists {
		entry.cancel()
		m.canceled.Add(1)
		m.pending.Add(-1)
	}

	// Create new cancellable context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), DefaultMetricsRequestTimeout)
	seq := m.sequence.Add(1)
	m.requests[requestId] = &metricsRequestEntry{
		cancel:   cancel,
		sequence: seq,
	}

	m.total.Add(1)
	m.pending.Add(1)

	return ctx, seq
}

// CompleteRequest marks a request as completed and removes it from tracking.
// Only completes if the sequence number matches (prevents double-decrement race condition).
func (m *MetricsRequestManager) CompleteRequest(requestId string, sequence int64) {
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

// CancelRequest cancels an in-flight request
func (m *MetricsRequestManager) CancelRequest(requestId string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, exists := m.requests[requestId]; exists {
		entry.cancel()
		delete(m.requests, requestId)
		m.canceled.Add(1)
		m.pending.Add(-1)
		return true
	}
	return false
}

// GetStats returns current statistics
func (m *MetricsRequestManager) GetStats() MetricsRequestStats {
	return MetricsRequestStats{
		Total:     m.total.Load(),
		Pending:   m.pending.Load(),
		Completed: m.completed.Load(),
		Canceled:  m.canceled.Load(),
	}
}
