package main

import "testing"

func TestPrematureWatchErrorReportsZeroEventClosure(t *testing.T) {
	event := prematureWatchError("pods", "default", "paralus", false)
	if !event.Premature || event.ReceivedAny || !event.Recoverable {
		t.Fatalf("unexpected premature closure event: %+v", event)
	}
	if event.ResourceType != "pods" || event.Namespace != "default" || event.Context != "paralus" {
		t.Fatalf("closure event lost watcher identity: %+v", event)
	}
}

func TestPrematureWatchErrorReportsWhetherEventsWereReceived(t *testing.T) {
	event := prematureWatchError("crd:example/v1/widgets", "", "direct", true)
	if !event.ReceivedAny {
		t.Fatal("expected receivedAny to distinguish a useful stream from an immediate close")
	}
}
