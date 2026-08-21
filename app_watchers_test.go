package main

import (
	"testing"
	"time"
)

func TestPrematureWatchErrorReportsZeroEventClosure(t *testing.T) {
	event := prematureWatchError("pods", "default", "paralus", false, 250*time.Millisecond)
	if !event.Premature || event.ReceivedAny || !event.Recoverable {
		t.Fatalf("unexpected premature closure event: %+v", event)
	}
	if event.ResourceType != "pods" || event.Namespace != "default" || event.Context != "paralus" {
		t.Fatalf("closure event lost watcher identity: %+v", event)
	}
	if event.OpenDurationMillis != 250 {
		t.Fatalf("closure event lost stream lifetime: %+v", event)
	}
}

func TestPrematureWatchErrorReportsWhetherEventsWereReceived(t *testing.T) {
	event := prematureWatchError("crd:example/v1/widgets", "", "direct", true, time.Second)
	if !event.ReceivedAny {
		t.Fatal("expected receivedAny to distinguish a useful stream from an immediate close")
	}
}
