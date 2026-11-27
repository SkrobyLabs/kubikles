package main

import (
	"context"
	"testing"
)

func TestNoClientsStopsEveryLogStream(t *testing.T) {
	a := &App{logStreams: make(map[string]context.CancelFunc)}
	ctxs := make([]context.Context, 3)
	for i := range ctxs {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs[i] = ctx
		a.logStreams[string(rune('a'+i))] = cancel
	}

	logStreamDisconnect{a}.OnNoClients()

	for i, ctx := range ctxs {
		if ctx.Err() == nil {
			t.Fatalf("stream %d was not cancelled", i)
		}
	}
}
