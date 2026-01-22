package main

import (
	"context"
	"sync"
	"sync/atomic"
)

// MetricsRequestStats contains statistics about metrics requests
type MetricsRequestStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Completed int64 `json:"completed"`
	Cancelled int64 `json:"cancelled"`
}

// MetricsRequestManager manages cancellable metrics requests
type MetricsRequestManager struct {
	mu       sync.RWMutex
	requests map[string]context.CancelFunc

	// Stats counters
	total     atomic.Int64
	pending   atomic.Int64
	completed atomic.Int64
	cancelled atomic.Int64
}

// NewMetricsRequestManager creates a new request manager
func NewMetricsRequestManager() *MetricsRequestManager {
	return &MetricsRequestManager{
		requests: make(map[string]context.CancelFunc),
	}
}

// StartRequest creates a new cancellable context for a request
// Returns the context to use for the request
func (m *MetricsRequestManager) StartRequest(requestId string) context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel any existing request with this ID
	if cancel, exists := m.requests[requestId]; exists {
		cancel()
		m.cancelled.Add(1)
		m.pending.Add(-1)
	}

	// Create new cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	m.requests[requestId] = cancel

	m.total.Add(1)
	m.pending.Add(1)

	return ctx
}

// CompleteRequest marks a request as completed and removes it from tracking
func (m *MetricsRequestManager) CompleteRequest(requestId string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.requests[requestId]; exists {
		delete(m.requests, requestId)
		m.completed.Add(1)
		m.pending.Add(-1)
	}
}

// CancelRequest cancels an in-flight request
func (m *MetricsRequestManager) CancelRequest(requestId string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, exists := m.requests[requestId]; exists {
		cancel()
		delete(m.requests, requestId)
		m.cancelled.Add(1)
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
		Cancelled: m.cancelled.Load(),
	}
}
