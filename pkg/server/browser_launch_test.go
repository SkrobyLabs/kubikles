package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kubikles/pkg/agent"
)

func confirmedBrowserLaunchFixture(t *testing.T) (*AcceleratorSessionRegistry, *BrowserSessionManager, *fakeAcceleratorConn, agent.AuthenticatedCallContext, BrowserBearer, *time.Time) {
	t.Helper()
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	creator := acceleratorTestCall("launch-generation-owner")
	creatorConn := newFakeAcceleratorConn()
	if _, ok := registry.register(creator, creatorConn); !ok {
		t.Fatal("creator registration rejected")
	}
	waitAccelerator(t, "creator connected", func() bool { return creatorConn.writeCount() == 1 })
	now := time.Date(2026, 8, 3, 2, 0, 0, 0, time.UTC)
	manager := newBrowserSessionManager(func() time.Time { return now }, &incrementingEntropy{}, registry)
	ticket, _, _, err := manager.mintForCreator(creator)
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := manager.Exchange(ticket)
	if err != nil {
		t.Fatal(err)
	}
	call, ok := manager.Authenticate(bearer)
	if !ok {
		t.Fatal("browser bearer rejected")
	}
	manager.browserSocketActivated(call.SessionID, 1)
	waitAccelerator(t, "launch confirmed", func() bool { return creatorConn.writeCount() == 2 })
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = registry.Close(ctx)
	})
	return registry, manager, creatorConn, call, bearer, &now
}

func TestBrowserLaunchGenerationReplacementLifecycle(t *testing.T) {
	t.Run("failed replacement ends old generation exactly once", func(t *testing.T) {
		_, manager, creatorConn, call, _, _ := confirmedBrowserLaunchFixture(t)
		manager.state.mu.Lock()
		manager.browserSocketPreparedLocked(call.SessionID, 2)
		manager.state.mu.Unlock()
		manager.browserSocketEnded(call.SessionID, 1)
		if creatorConn.writeCount() != 2 {
			t.Fatal("old generation ended hold while replacement was prepared")
		}
		manager.browserSocketActivationFailed(call.SessionID, 2)
		waitAccelerator(t, "failed replacement ended", func() bool { return creatorConn.writeCount() == 3 })
		manager.browserSocketEnded(call.SessionID, 1)
		if creatorConn.writeCount() != 3 {
			t.Fatal("failed replacement emitted duplicate ended")
		}
	})

	t.Run("activated replacement owns end", func(t *testing.T) {
		_, manager, creatorConn, call, _, _ := confirmedBrowserLaunchFixture(t)
		manager.state.mu.Lock()
		manager.browserSocketPreparedLocked(call.SessionID, 2)
		manager.state.mu.Unlock()
		manager.browserSocketEnded(call.SessionID, 1)
		manager.browserSocketActivated(call.SessionID, 2)
		if creatorConn.writeCount() != 2 {
			t.Fatal("replacement activation emitted premature control")
		}
		manager.browserSocketEnded(call.SessionID, 1)
		if creatorConn.writeCount() != 2 {
			t.Fatal("replaced generation ended current hold")
		}
		manager.browserSocketEnded(call.SessionID, 2)
		waitAccelerator(t, "replacement ended", func() bool { return creatorConn.writeCount() == 3 })
	})
}

func TestConfirmedBrowserReplacementRemainsBoundAfterTicketDeadline(t *testing.T) {
	_, manager, creatorConn, call, _, now := confirmedBrowserLaunchFixture(t)
	manager.state.mu.Lock()
	manager.browserSocketPreparedLocked(call.SessionID, 2)
	manager.state.mu.Unlock()
	manager.browserSocketEnded(call.SessionID, 1)
	if creatorConn.writeCount() != 2 {
		t.Fatal("prepared replacement did not suppress predecessor end")
	}
	*now = now.Add(BrowserTicketLifetime)
	manager.browserSocketActivated(call.SessionID, 2)
	if creatorConn.writeCount() != 2 {
		t.Fatal("post-deadline replacement emitted premature control")
	}
	manager.browserSocketEnded(call.SessionID, 2)
	waitAccelerator(t, "post-deadline replacement ended", func() bool { return creatorConn.writeCount() == 3 })
	manager.browserSocketEnded(call.SessionID, 1)
	manager.browserSocketEnded(call.SessionID, 2)
	if creatorConn.writeCount() != 3 {
		t.Fatal("post-deadline replacement emitted duplicate end")
	}
}

func TestBrowserRequestCrossingExpiryEmitsOneTargetedEnd(t *testing.T) {
	_, manager, creatorConn, _, bearer, now := confirmedBrowserLaunchFixture(t)
	handler := CreatorOrBrowserGuard(nil, manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*now = now.Add(BrowserSessionIdleTTL)
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/secrets", nil)
	request.Header.Set("Authorization", "Bearer "+bearer.encoded())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
	waitAccelerator(t, "expiry end", func() bool { return creatorConn.writeCount() == 3 })
	manager.Touch("not-the-session")
	if creatorConn.writeCount() != 3 {
		t.Fatal("expiry emitted duplicate ended")
	}
}

func TestLaunchReceiptBindsMintExchangeAndCurrentBrowser(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = registry.Close(ctx)
	}()
	creator := acceleratorTestCall("launch-owner")
	creatorConn := newFakeAcceleratorConn()
	creatorSocket, ok := registry.register(creator, creatorConn)
	if !ok {
		t.Fatal("creator registration rejected")
	}
	waitAccelerator(t, "creator connected", func() bool { return creatorConn.writeCount() == 1 })

	now := time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)
	manager := newBrowserSessionManager(func() time.Time { return now }, bytes.NewReader(bytes.Repeat([]byte{0x42}, 512)), registry)
	ticket, receipt, expiry, err := manager.mintForCreator(creator)
	if err != nil || len(ticket.encoded()) != 43 || len(receipt.encoded()) != 43 || expiry != now.Add(BrowserTicketLifetime) {
		t.Fatalf("mint rejected: expiry=%s err=%v", expiry, err)
	}
	if creatorConn.writeCount() != 1 {
		t.Fatal("mint emitted confirmation")
	}
	bearer, _, err := manager.Exchange(ticket)
	if err != nil || creatorConn.writeCount() != 1 {
		t.Fatalf("exchange confirmed launch: %v", err)
	}
	browserCall, ok := manager.Authenticate(bearer)
	if !ok {
		t.Fatal("exchanged bearer rejected")
	}
	browserConn := newFakeAcceleratorConn()
	browserSocket, ok := registry.register(browserCall, browserConn)
	if !ok {
		t.Fatal("browser registration rejected")
	}
	manager.browserSocketActivated(browserCall.SessionID, browserSocket.snapshot.Generation)
	waitAccelerator(t, "targeted confirmation", func() bool { return creatorConn.writeCount() == 2 })
	events := creatorConn.events()
	if len(events) != 2 || events[1].Name != "browser-launch" {
		t.Fatalf("creator events=%v", events)
	}
	control, ok := events[1].Data.(map[string]interface{})
	if !ok || control["status"] != "confirmed" {
		t.Fatalf("confirmation=%#v", events[1].Data)
	}
	rawReceipt := receipt.encoded()
	if fmt.Sprint(events[1].Data) == rawReceipt {
		t.Fatal("raw receipt was retained in confirmation")
	}
	verifier := deriveBrowserVerifier(browserLaunchDomain, receipt.value)
	if control["receipt"] != base64.RawURLEncoding.EncodeToString(verifier.value[:]) {
		t.Fatalf("wrong receipt correlation: %#v", control)
	}

	manager.browserSocketEnded(browserCall.SessionID, browserSocket.snapshot.Generation)
	waitAccelerator(t, "targeted end", func() bool { return creatorConn.writeCount() == 3 })
	if got := creatorConn.events()[2]; got.Name != "browser-launch" || fmt.Sprint(got.Data) == rawReceipt {
		t.Fatalf("end=%#v", got)
	}
	if creatorSocket.snapshot.Generation == 0 {
		t.Fatal("creator generation was not pinned")
	}
}

func TestLaunchReceiptExpiresBeforeBrowserActivation(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	creator := acceleratorTestCall("launch-expiry")
	creatorConn := newFakeAcceleratorConn()
	_, _ = registry.register(creator, creatorConn)
	waitAccelerator(t, "creator connected", func() bool { return creatorConn.writeCount() == 1 })
	now := time.Unix(1_700_000_000, 0)
	manager := newBrowserSessionManager(func() time.Time { return now }, &incrementingEntropy{}, registry)
	ticket, _, _, err := manager.mintForCreator(creator)
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := manager.Exchange(ticket)
	if err != nil {
		t.Fatal(err)
	}
	call, _ := manager.Authenticate(bearer)
	now = now.Add(BrowserTicketLifetime)
	manager.browserSocketActivated(call.SessionID, 1)
	if creatorConn.writeCount() != 1 {
		t.Fatal("expired launch confirmed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = registry.Close(ctx)
}
