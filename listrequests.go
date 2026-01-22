package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ListRequestStats contains statistics about list requests
type ListRequestStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Completed int64 `json:"completed"`
	Cancelled int64 `json:"cancelled"`
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

	// Stats counters
	total     atomic.Int64
	pending   atomic.Int64
	completed atomic.Int64
	cancelled atomic.Int64
}

// DefaultListRequestTimeout is the default timeout for list requests
const DefaultListRequestTimeout = 30 * time.Second

// NewListRequestManager creates a new list request manager
func NewListRequestManager() *ListRequestManager {
	return &ListRequestManager{
		requests: make(map[string]*requestEntry),
	}
}

// StartRequest creates a new cancellable context for a request.
// If a request with the same ID exists, it will be cancelled first.
// Returns the context to use and the sequence number for completing.
func (m *ListRequestManager) StartRequest(requestId string) (context.Context, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel any existing request with this ID
	if entry, exists := m.requests[requestId]; exists {
		entry.cancel()
		m.cancelled.Add(1)
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

// CancelRequest cancels an in-flight request by ID
func (m *ListRequestManager) CancelRequest(requestId string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, exists := m.requests[requestId]; exists {
		entry.cancel()
		delete(m.requests, requestId)
		m.cancelled.Add(1)
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
		Cancelled: m.cancelled.Load(),
	}
}
