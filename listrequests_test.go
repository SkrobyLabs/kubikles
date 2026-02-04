package main

import (
	"sync"
	"testing"
	"time"
)

func TestNewListRequestManager(t *testing.T) {
	m := NewListRequestManager()
	if m == nil {
		t.Fatal("NewListRequestManager returned nil")
	}
	if m.requests == nil {
		t.Fatal("requests map not initialized")
	}

	stats := m.GetStats()
	if stats.Total != 0 || stats.Pending != 0 || stats.Completed != 0 || stats.Canceled != 0 {
		t.Errorf("expected all stats to be 0, got %+v", stats)
	}
}

func TestStartRequest_CreatesContext(t *testing.T) {
	m := NewListRequestManager()

	ctx, seq := m.StartRequest("test-1")

	if ctx == nil {
		t.Fatal("StartRequest returned nil context")
	}
	if seq != 1 {
		t.Errorf("expected sequence 1, got %d", seq)
	}

	// Context should not be canceled yet
	select {
	case <-ctx.Done():
		t.Error("context should not be done yet")
	default:
		// good
	}

	stats := m.GetStats()
	if stats.Total != 1 {
		t.Errorf("expected Total=1, got %d", stats.Total)
	}
	if stats.Pending != 1 {
		t.Errorf("expected Pending=1, got %d", stats.Pending)
	}
}

func TestStartRequest_SameIDCancelsPrevious(t *testing.T) {
	m := NewListRequestManager()

	// Start first request
	ctx1, seq1 := m.StartRequest("test-1")
	if seq1 != 1 {
		t.Errorf("expected sequence 1, got %d", seq1)
	}

	// Start second request with same ID - should cancel first
	ctx2, seq2 := m.StartRequest("test-1")
	if seq2 != 2 {
		t.Errorf("expected sequence 2, got %d", seq2)
	}

	// First context should be canceled
	select {
	case <-ctx1.Done():
		// good - context was canceled
	default:
		t.Error("first context should be canceled")
	}

	// Second context should still be active
	select {
	case <-ctx2.Done():
		t.Error("second context should not be done yet")
	default:
		// good
	}

	stats := m.GetStats()
	if stats.Total != 2 {
		t.Errorf("expected Total=2, got %d", stats.Total)
	}
	if stats.Pending != 1 {
		t.Errorf("expected Pending=1, got %d", stats.Pending)
	}
	if stats.Canceled != 1 {
		t.Errorf("expected Canceled=1, got %d", stats.Canceled)
	}
}

func TestStartRequest_DifferentIDsIndependent(t *testing.T) {
	m := NewListRequestManager()

	ctx1, _ := m.StartRequest("test-1")
	ctx2, _ := m.StartRequest("test-2")

	// Both contexts should be active
	select {
	case <-ctx1.Done():
		t.Error("context 1 should not be done")
	default:
	}
	select {
	case <-ctx2.Done():
		t.Error("context 2 should not be done")
	default:
	}

	stats := m.GetStats()
	if stats.Pending != 2 {
		t.Errorf("expected Pending=2, got %d", stats.Pending)
	}
}

func TestCompleteRequest_MatchingSequence(t *testing.T) {
	m := NewListRequestManager()

	_, seq := m.StartRequest("test-1")
	m.CompleteRequest("test-1", seq)

	stats := m.GetStats()
	if stats.Completed != 1 {
		t.Errorf("expected Completed=1, got %d", stats.Completed)
	}
	if stats.Pending != 0 {
		t.Errorf("expected Pending=0, got %d", stats.Pending)
	}
}

func TestCompleteRequest_WrongSequence_NoOp(t *testing.T) {
	m := NewListRequestManager()

	// Start request, get sequence 1
	_, seq1 := m.StartRequest("test-1")

	// Start another request with same ID, get sequence 2
	// This cancels the first one
	_, seq2 := m.StartRequest("test-1")

	// Try to complete with old sequence - should be no-op
	m.CompleteRequest("test-1", seq1)

	stats := m.GetStats()
	// Should still have 1 pending (the second request)
	if stats.Pending != 1 {
		t.Errorf("expected Pending=1, got %d", stats.Pending)
	}
	// Completed should still be 0 (old sequence was rejected)
	if stats.Completed != 0 {
		t.Errorf("expected Completed=0, got %d", stats.Completed)
	}

	// Now complete with correct sequence
	m.CompleteRequest("test-1", seq2)

	stats = m.GetStats()
	if stats.Completed != 1 {
		t.Errorf("expected Completed=1, got %d", stats.Completed)
	}
	if stats.Pending != 0 {
		t.Errorf("expected Pending=0, got %d", stats.Pending)
	}
}

func TestCompleteRequest_NonExistent_NoOp(t *testing.T) {
	m := NewListRequestManager()

	// Complete a request that doesn't exist
	m.CompleteRequest("non-existent", 999)

	stats := m.GetStats()
	if stats.Completed != 0 {
		t.Errorf("expected Completed=0, got %d", stats.Completed)
	}
}

func TestCancelRequest_Exists(t *testing.T) {
	m := NewListRequestManager()

	ctx, _ := m.StartRequest("test-1")

	canceled := m.CancelRequest("test-1")
	if !canceled {
		t.Error("CancelRequest should return true for existing request")
	}

	// Context should be canceled
	select {
	case <-ctx.Done():
		// good
	default:
		t.Error("context should be canceled")
	}

	stats := m.GetStats()
	if stats.Canceled != 1 {
		t.Errorf("expected Canceled=1, got %d", stats.Canceled)
	}
	if stats.Pending != 0 {
		t.Errorf("expected Pending=0, got %d", stats.Pending)
	}
}

func TestCancelRequest_NonExistent(t *testing.T) {
	m := NewListRequestManager()

	canceled := m.CancelRequest("non-existent")
	if canceled {
		t.Error("CancelRequest should return false for non-existent request")
	}

	stats := m.GetStats()
	if stats.Canceled != 0 {
		t.Errorf("expected Canceled=0, got %d", stats.Canceled)
	}
}

func TestCancelRequest_AlreadyCancelled(t *testing.T) {
	m := NewListRequestManager()

	m.StartRequest("test-1")
	m.CancelRequest("test-1")

	// Try to cancel again
	canceled := m.CancelRequest("test-1")
	if canceled {
		t.Error("CancelRequest should return false for already canceled request")
	}

	stats := m.GetStats()
	if stats.Canceled != 1 {
		t.Errorf("expected Canceled=1, got %d", stats.Canceled)
	}
}

func TestContextTimeout(t *testing.T) {
	// This test verifies the context has a timeout
	// We can't easily test the 30s timeout, but we can verify context.Err() behavior
	m := NewListRequestManager()

	ctx, _ := m.StartRequest("test-1")

	// Context should have a deadline
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("context should have a deadline")
	}

	// Deadline should be approximately 30 seconds from now
	expectedDeadline := time.Now().Add(DefaultListRequestTimeout)
	if deadline.Before(expectedDeadline.Add(-time.Second)) || deadline.After(expectedDeadline.Add(time.Second)) {
		t.Errorf("deadline %v not within expected range around %v", deadline, expectedDeadline)
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewListRequestManager()
	var wg sync.WaitGroup
	numGoroutines := 100

	// Spawn goroutines that all try to start/complete/cancel requests
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			requestID := "concurrent-test"

			// Start a request
			_, seq := m.StartRequest(requestID)

			// Simulate some work
			time.Sleep(time.Millisecond)

			// Randomly complete or cancel
			if id%2 == 0 {
				m.CompleteRequest(requestID, seq)
			} else {
				m.CancelRequest(requestID)
			}
		}(i)
	}

	wg.Wait()

	// Verify no panics occurred and stats are consistent
	stats := m.GetStats()

	// Total should equal numGoroutines
	if stats.Total != int64(numGoroutines) {
		t.Errorf("expected Total=%d, got %d", numGoroutines, stats.Total)
	}

	// Pending should be 0 (all requests either completed or canceled)
	if stats.Pending != 0 {
		t.Errorf("expected Pending=0, got %d", stats.Pending)
	}

	// Completed + Canceled should account for all requests
	// Note: some completes may fail due to sequence mismatch, so we can't be exact
	t.Logf("Stats after concurrent test: Total=%d, Pending=%d, Completed=%d, Canceled=%d",
		stats.Total, stats.Pending, stats.Completed, stats.Canceled)
}

func TestRaceCondition_OldRequestCompletesAfterNew(t *testing.T) {
	// This test simulates the race condition where:
	// 1. Request A starts (seq=1)
	// 2. Request B starts with same ID (seq=2), canceling A
	// 3. Request A's goroutine tries to complete with seq=1
	// 4. This should be rejected because seq doesn't match

	m := NewListRequestManager()

	// Start request A
	_, seqA := m.StartRequest("test")

	// Start request B (cancels A)
	_, seqB := m.StartRequest("test")

	// Verify A was canceled
	stats := m.GetStats()
	if stats.Canceled != 1 {
		t.Errorf("expected Canceled=1 after second StartRequest, got %d", stats.Canceled)
	}

	// Try to complete with A's sequence (should be rejected)
	m.CompleteRequest("test", seqA)

	stats = m.GetStats()
	if stats.Completed != 0 {
		t.Errorf("completing with old sequence should not increment Completed, got %d", stats.Completed)
	}
	if stats.Pending != 1 {
		t.Errorf("expected Pending=1 (request B still active), got %d", stats.Pending)
	}

	// Complete with B's sequence (should work)
	m.CompleteRequest("test", seqB)

	stats = m.GetStats()
	if stats.Completed != 1 {
		t.Errorf("expected Completed=1 after completing B, got %d", stats.Completed)
	}
	if stats.Pending != 0 {
		t.Errorf("expected Pending=0, got %d", stats.Pending)
	}
}

func TestStatsConsistency(t *testing.T) {
	m := NewListRequestManager()

	// Start 5 requests with different IDs
	for i := 0; i < 5; i++ {
		m.StartRequest("request-" + string(rune('A'+i)))
	}

	stats := m.GetStats()
	if stats.Total != 5 || stats.Pending != 5 {
		t.Errorf("after 5 starts: expected Total=5, Pending=5, got Total=%d, Pending=%d", stats.Total, stats.Pending)
	}

	// Complete 2
	m.CompleteRequest("request-A", 1)
	m.CompleteRequest("request-B", 2)

	stats = m.GetStats()
	if stats.Completed != 2 || stats.Pending != 3 {
		t.Errorf("after 2 completes: expected Completed=2, Pending=3, got Completed=%d, Pending=%d", stats.Completed, stats.Pending)
	}

	// Cancel 2
	m.CancelRequest("request-C")
	m.CancelRequest("request-D")

	stats = m.GetStats()
	if stats.Canceled != 2 || stats.Pending != 1 {
		t.Errorf("after 2 cancels: expected Canceled=2, Pending=1, got Canceled=%d, Pending=%d", stats.Canceled, stats.Pending)
	}

	// Replace E with new request (same ID)
	m.StartRequest("request-E")

	stats = m.GetStats()
	if stats.Total != 6 || stats.Canceled != 3 || stats.Pending != 1 {
		t.Errorf("after replacement: expected Total=6, Canceled=3, Pending=1, got Total=%d, Canceled=%d, Pending=%d",
			stats.Total, stats.Canceled, stats.Pending)
	}
}
