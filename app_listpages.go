package main

import (
	"context"
	"strings"

	"kubikles/pkg/k8s"
)

// ListOutcomeResult is the out-of-band result of a list request. The list
// bindings return plain item arrays, so a paged list (whose pages were all
// streamed as list-page events) returns an empty array and the details live
// here.
type ListOutcomeResult struct {
	Found bool `json:"found"`
	// Context is the kubeconfig context the list ran against.
	Context string `json:"context"`
	// ResourceVersion is the list snapshot version; "" when unknown.
	ResourceVersion string `json:"resourceVersion"`
	// Streamed is true when the items were delivered only as list-page events.
	Streamed bool `json:"streamed"`
	// Loaded is the number of items the list produced.
	Loaded int `json:"loaded"`
}

// GetListOutcome returns what is known about a finished list request. Found is
// false when the request is unknown, expired or did not report an outcome, in
// which case callers fall back to the full-result and unanchored-watch paths.
func (a *App) GetListOutcome(requestID string) ListOutcomeResult {
	outcome, contextName, ok := a.listRequestManager.Outcome(requestID)
	if !ok {
		return ListOutcomeResult{}
	}
	return ListOutcomeResult{
		Found:           true,
		Context:         contextName,
		ResourceVersion: outcome.ResourceVersion,
		Streamed:        outcome.Streamed,
		Loaded:          outcome.Loaded,
	}
}

// Correlate pages to an individual request, never just a resource type. The
// frontend registers its listener before invoking the corresponding list call.
func (a *App) startResourceListRequest(requestID string) (context.Context, int64) {
	ctx, sequence := a.listRequestManager.StartRequest(requestID)
	contextName := a.GetCurrentContext()
	ctx = k8s.WithListOutcomeSink(ctx, func(outcome k8s.ListOutcome) {
		a.listRequestManager.RecordOutcome(requestID, contextName, outcome)
	})
	if !strings.HasPrefix(requestID, "page-list-") {
		return ctx, sequence
	}
	return k8s.WithListPageObserver(ctx, func(page k8s.ListPage) {
		a.emitEvent("list-page", struct {
			k8s.ListPage
			RequestID string `json:"requestId"`
			Context   string `json:"context"`
		}{page, requestID, contextName})
	}), sequence
}
