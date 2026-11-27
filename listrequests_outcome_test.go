package main

import (
	"fmt"
	"testing"
	"time"

	"kubikles/pkg/k8s"
)

func TestListOutcomeStoreRoundTrip(t *testing.T) {
	m := NewListRequestManager()
	if _, _, ok := m.Outcome("missing"); ok {
		t.Fatal("unknown request reported an outcome")
	}
	m.RecordOutcome("r1", "ctx", k8s.ListOutcome{ResourceVersion: "12", Streamed: true, Loaded: 5})
	got, contextName, ok := m.Outcome("r1")
	if !ok || contextName != "ctx" || got.ResourceVersion != "12" || !got.Streamed || got.Loaded != 5 {
		t.Fatalf("got %+v ctx %q ok %v", got, contextName, ok)
	}
}

func TestListOutcomeStoreExpiresAndIsBounded(t *testing.T) {
	m := NewListRequestManager()
	m.RecordOutcome("old", "c", k8s.ListOutcome{ResourceVersion: "1"})
	m.outcomeMu.Lock()
	stored := m.outcomes["old"]
	stored.expires = time.Now().Add(-time.Second)
	m.outcomes["old"] = stored
	m.outcomeMu.Unlock()
	if _, _, ok := m.Outcome("old"); ok {
		t.Fatal("expired outcome was returned")
	}

	for i := 0; i < maxListOutcomes+10; i++ {
		m.RecordOutcome(fmt.Sprintf("r%d", i), "c", k8s.ListOutcome{})
	}
	m.outcomeMu.Lock()
	size := len(m.outcomes)
	m.outcomeMu.Unlock()
	if size > maxListOutcomes {
		t.Fatalf("store holds %d outcomes, cap is %d", size, maxListOutcomes)
	}
	if _, _, ok := m.Outcome("r0"); ok {
		t.Fatal("oldest outcome survived eviction")
	}
	if _, _, ok := m.Outcome(fmt.Sprintf("r%d", maxListOutcomes+9)); !ok {
		t.Fatal("newest outcome was evicted")
	}
}

func TestGetListOutcomeBinding(t *testing.T) {
	app := &App{listRequestManager: NewListRequestManager()}
	if res := app.GetListOutcome("nope"); res.Found {
		t.Fatalf("unknown request found: %+v", res)
	}
	app.listRequestManager.RecordOutcome("x", "ctx", k8s.ListOutcome{ResourceVersion: "9", Loaded: 2})
	res := app.GetListOutcome("x")
	if !res.Found || res.Context != "ctx" || res.ResourceVersion != "9" || res.Streamed || res.Loaded != 2 {
		t.Fatalf("res = %+v", res)
	}
}

func TestStartResourceListRequestRecordsNothingUntilTheListReportsAnOutcome(t *testing.T) {
	app := &App{listRequestManager: NewListRequestManager()}
	ctx, seq := app.startResourceListRequest("page-list-pods-1")
	defer app.listRequestManager.CompleteRequest("page-list-pods-1", seq)
	if ctx == nil {
		t.Fatal("no context")
	}
	if res := app.GetListOutcome("page-list-pods-1"); res.Found {
		t.Fatal("outcome reported before the list ran")
	}
}
