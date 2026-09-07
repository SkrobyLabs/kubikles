package main

import (
	"context"
	"strings"

	"kubikles/pkg/k8s"
)

// Correlate pages to an individual request, never just a resource type. The
// frontend registers its listener before invoking the corresponding list call.
func (a *App) startResourceListRequest(requestID string) (context.Context, int64) {
	ctx, sequence := a.listRequestManager.StartRequest(requestID)
	if !strings.HasPrefix(requestID, "page-list-") {
		return ctx, sequence
	}
	contextName := a.GetCurrentContext()
	return k8s.WithListPageObserver(ctx, func(page k8s.ListPage) {
		a.emitEvent("list-page", struct {
			k8s.ListPage
			RequestID string `json:"requestId"`
			Context   string `json:"context"`
		}{page, requestID, contextName})
	}), sequence
}
