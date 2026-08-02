package server

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kubikles/pkg/agent"
)

type browserHTTPFixture struct {
	handler      http.Handler
	manager      *BrowserSessionManager
	clock        *testClock
	creatorToken string
}
type alwaysEnabledBrowserEntry struct{}

func (alwaysEnabledBrowserEntry) BrowserEntryEnabled() bool { return true }

func newBrowserHTTPFixture(t *testing.T, revoker BrowserSessionRevoker) *browserHTTPFixture {
	t.Helper()
	clock := &testClock{now: time.Date(2026, 8, 1, 5, 6, 7, 123, time.UTC)}
	entropy := make([]byte, 8192)
	for i := range entropy {
		entropy[i] = byte(i)
	}
	manager := newBrowserSessionManager(clock.Now, bytes.NewReader(entropy), revoker)
	creatorRaw := bytes.Repeat([]byte{0x52}, 32)
	creatorToken := base64.RawURLEncoding.EncodeToString(creatorRaw)
	token, err := ParseCreatorToken(creatorToken)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := newCreatorAuthenticator(DeriveCreatorVerifier(token).Encoded(), bytes.NewReader(bytes.Repeat([]byte{0x53}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	options := AcceleratorOptions(0, nil, CreatorOrBrowserGuard(creator, manager))
	options.BrowserSessions = manager
	options.BrowserEntryAvailability = alwaysEnabledBrowserEntry{}
	options.MethodAuthorizer = MethodAuthorizerFunc(func(_ agent.AuthenticatedCallContext, _ string) bool { return true })
	server, err := NewWithOptions(nil, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	return &browserHTTPFixture{handler: server.Handler(), manager: manager, clock: clock, creatorToken: creatorToken}
}

func (f *browserHTTPFixture) request(method, target, authorization string, body io.Reader) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, body)
	r.Host = "localhost"
	if authorization != "" {
		r.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

func (f *browserHTTPFixture) mintDirect(t *testing.T) BrowserTicket {
	t.Helper()
	ticket, _, err := f.manager.Mint()
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func (f *browserHTTPFixture) browserDirect(t *testing.T) BrowserBearer {
	t.Helper()
	ticket := f.mintDirect(t)
	bearer, _, err := f.manager.Exchange(ticket)
	if err != nil {
		t.Fatal(err)
	}
	return bearer
}

func TestMintBrowserTicketCreatorOnly(t *testing.T) {
	t.Run("exact success", func(t *testing.T) {
		f := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
		w := f.request(http.MethodPost, "/api/accelerator-browser-ticket", "Bearer "+f.creatorToken, nil)
		if w.Code != http.StatusCreated || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Set-Cookie") != "" {
			t.Fatalf("response = %d %q %#v", w.Code, w.Body.String(), w.Header())
		}
		var response struct {
			Ticket    string `json:"ticket"`
			ExpiresAt string `json:"expiresAt"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Ticket) != 43 || strings.Contains(response.Ticket, "=") || response.ExpiresAt != f.clock.Now().Add(BrowserTicketLifetime).Format(time.RFC3339Nano) {
			t.Fatalf("response DTO = %#v", response)
		}
		if want := fmt.Sprintf("{\"ticket\":%q,\"expiresAt\":%q}\n", response.Ticket, response.ExpiresAt); w.Body.String() != want {
			t.Fatalf("body = %q, want %q", w.Body.String(), want)
		}
	})

	for _, test := range []struct {
		name string
		body io.Reader
	}{
		{name: "one byte", body: strings.NewReader("x")},
		{name: "oversized", body: strings.NewReader("xx")},
		{name: "read error", body: errReader{}},
	} {
		t.Run("reject body "+test.name, func(t *testing.T) {
			f := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
			w := f.request(http.MethodPost, "/api/accelerator-browser-ticket", "Bearer "+f.creatorToken, test.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%q", w.Code, w.Body.String())
			}
			f.manager.state.mu.Lock()
			defer f.manager.state.mu.Unlock()
			if f.manager.state.ticket != nil {
				t.Fatal("invalid body minted ticket")
			}
		})
	}

	t.Run("query rejected before replacement", func(t *testing.T) {
		f := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
		outstanding := f.mintDirect(t)
		w := f.request(http.MethodPost, "/api/accelerator-browser-ticket?ticket=ignored", "Bearer "+f.creatorToken, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatal(w.Code)
		}
		if _, _, err := f.manager.Exchange(outstanding); err != nil {
			t.Fatalf("query mutated ticket: %v", err)
		}
	})

	t.Run("bare query rejected before replacement", func(t *testing.T) {
		f := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
		outstanding := f.mintDirect(t)
		r := httptest.NewRequest(http.MethodPost, "/api/accelerator-browser-ticket", nil)
		r.Host = "localhost"
		r.Header.Set("Authorization", "Bearer "+f.creatorToken)
		r.URL.ForceQuery = true
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatal(w.Code)
		}
		if _, _, err := f.manager.Exchange(outstanding); err != nil {
			t.Fatalf("bare query mutated ticket: %v", err)
		}
	})

	t.Run("roles", func(t *testing.T) {
		f := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
		browser := f.browserDirect(t)
		for _, test := range []struct {
			name string
			auth string
			want int
		}{
			{name: "browser", auth: "Bearer " + browser.encoded(), want: http.StatusForbidden},
			{name: "missing", want: http.StatusUnauthorized},
			{name: "invalid", auth: "Bearer " + strings.Repeat("A", 43), want: http.StatusUnauthorized},
		} {
			t.Run(test.name, func(t *testing.T) {
				w := f.request(http.MethodPost, "/api/accelerator-browser-ticket", test.auth, nil)
				if w.Code != test.want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" {
					t.Fatalf("response = %d %q %#v", w.Code, w.Body.String(), w.Header())
				}
			})
		}
		f.manager.state.mu.Lock()
		defer f.manager.state.mu.Unlock()
		if f.manager.state.ticket != nil {
			t.Fatal("role failure minted ticket")
		}
	})
}

func TestExchangeBrowserSessionStrictPublicContract(t *testing.T) {
	validBody := func(ticket BrowserTicket) string { return `{"ticket":"` + ticket.encoded() + `"}` }

	t.Run("exact success and reuse", func(t *testing.T) {
		f := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
		ticket := f.mintDirect(t)
		w := f.request(http.MethodPost, "/api/accelerator-browser-session", "", strings.NewReader(validBody(ticket)))
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Set-Cookie") != "" {
			t.Fatalf("response = %d %q %#v", w.Code, w.Body.String(), w.Header())
		}
		var response map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if len(response) != 2 || len(response["bearer"]) != 43 || response["expiresAt"] != f.clock.Now().Add(BrowserSessionHardTTL).Format(time.RFC3339Nano) {
			t.Fatalf("response = %#v", response)
		}
		reuse := f.request(http.MethodPost, "/api/accelerator-browser-session", "", strings.NewReader(validBody(ticket)))
		if reuse.Code != http.StatusUnauthorized || reuse.Body.String() != "{\"error\":\"unauthorized\"}\n" || strings.Contains(reuse.Body.String(), ticket.encoded()) || strings.Contains(reuse.Body.String(), response["bearer"]) {
			t.Fatalf("reuse response = %d %q", reuse.Code, reuse.Body.String())
		}
	})

	t.Run("entropy failure preserves ticket", func(t *testing.T) {
		f := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
		ticket := f.mintDirect(t)
		f.manager.entropy = errReader{}
		failed := f.request(http.MethodPost, "/api/accelerator-browser-session", "", strings.NewReader(validBody(ticket)))
		if failed.Code != http.StatusInternalServerError || failed.Body.String() != "{\"error\":\"internal error\"}\n" {
			t.Fatalf("entropy failure = %d %q", failed.Code, failed.Body.String())
		}
		f.manager.entropy = bytes.NewReader(bytes.Repeat([]byte{0x44}, 64))
		if good := f.request(http.MethodPost, "/api/accelerator-browser-session", "", strings.NewReader(validBody(ticket))); good.Code != http.StatusOK {
			t.Fatalf("entropy failure consumed ticket: %d %q", good.Code, good.Body.String())
		}
	})

	tests := []struct {
		name   string
		body   func(BrowserTicket) string
		mutate func(*http.Request, BrowserTicket)
		want   int
	}{
		{name: "upper case field", body: func(v BrowserTicket) string { return `{"Ticket":"` + v.encoded() + `"}` }, want: 400},
		{name: "mixed case field", body: func(v BrowserTicket) string { return `{"tIcket":"` + v.encoded() + `"}` }, want: 400},
		{name: "duplicate field", body: func(v BrowserTicket) string { return `{"ticket":"` + v.encoded() + `","ticket":"` + v.encoded() + `"}` }, want: 400},
		{name: "unknown field", body: func(v BrowserTicket) string { return `{"ticket":"` + v.encoded() + `","extra":1}` }, want: 400},
		{name: "alternate field", body: func(v BrowserTicket) string { return `{"token":"` + v.encoded() + `"}` }, want: 400},
		{name: "empty object", body: func(BrowserTicket) string { return `{}` }, want: 400},
		{name: "array", body: func(v BrowserTicket) string { return `["` + v.encoded() + `"]` }, want: 400},
		{name: "null", body: func(BrowserTicket) string { return `{"ticket":null}` }, want: 400},
		{name: "number", body: func(BrowserTicket) string { return `{"ticket":1}` }, want: 400},
		{name: "trailing object", body: func(v BrowserTicket) string { return validBody(v) + `{}` }, want: 400},
		{name: "oversized", body: func(v BrowserTicket) string {
			return validBody(v) + strings.Repeat(" ", int(BrowserExchangeMaxBodyBytes))
		}, want: 413},
		{name: "authorization", body: validBody, mutate: func(r *http.Request, v BrowserTicket) { r.Header.Set("Authorization", "Bearer "+v.encoded()) }, want: 400},
		{name: "cookie", body: validBody, mutate: func(r *http.Request, v BrowserTicket) { r.Header.Set("Cookie", "ticket="+v.encoded()) }, want: 400},
		{name: "malformed raw cookie", body: validBody, mutate: func(r *http.Request, _ BrowserTicket) { r.Header["Cookie"] = []string{";=="} }, want: 400},
		{name: "query", body: validBody, mutate: func(r *http.Request, v BrowserTicket) { r.URL.RawQuery = "ticket=" + v.encoded() }, want: 400},
		{name: "bare query", body: validBody, mutate: func(r *http.Request, _ BrowserTicket) { r.URL.ForceQuery = true }, want: 400},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
			ticket := f.mintDirect(t)
			r := httptest.NewRequest(http.MethodPost, "/api/accelerator-browser-session", strings.NewReader(test.body(ticket)))
			r.Host = "localhost"
			if test.mutate != nil {
				test.mutate(r, ticket)
			}
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, r)
			if w.Code != test.want || strings.Contains(w.Body.String(), ticket.encoded()) || w.Header().Get("Set-Cookie") != "" {
				t.Fatalf("response = %d %q %#v", w.Code, w.Body.String(), w.Header())
			}
			// Every structural/transport failure leaves the ticket available.
			good := f.request(http.MethodPost, "/api/accelerator-browser-session", "", strings.NewReader(validBody(ticket)))
			if good.Code != http.StatusOK {
				t.Fatalf("failure consumed ticket: %d %q", good.Code, good.Body.String())
			}
		})
	}

	t.Run("invalid replaced and expired are identical", func(t *testing.T) {
		f := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
		first := f.mintDirect(t)
		second := f.mintDirect(t)
		invalid := BrowserTicket{value: [32]byte{0xfe}}
		for name, ticket := range map[string]BrowserTicket{"replaced": first, "invalid": invalid} {
			t.Run(name, func(t *testing.T) {
				w := f.request(http.MethodPost, "/api/accelerator-browser-session", "", strings.NewReader(validBody(ticket)))
				if w.Code != http.StatusUnauthorized || w.Body.String() != "{\"error\":\"unauthorized\"}\n" {
					t.Fatalf("response = %d %q", w.Code, w.Body.String())
				}
			})
		}
		f.clock.Set(f.clock.Now().Add(BrowserTicketLifetime))
		w := f.request(http.MethodPost, "/api/accelerator-browser-session", "", strings.NewReader(validBody(second)))
		if w.Code != http.StatusUnauthorized || w.Body.String() != "{\"error\":\"unauthorized\"}\n" {
			t.Fatalf("expired response = %d %q", w.Code, w.Body.String())
		}
	})
}

func TestRevokeBrowserSessionCreatorOnlyAndIdempotent(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	revoker := browserRevokerFunc(func(context.Context, agent.SessionID) {
		calls.Add(1)
		close(entered)
		<-release
	})
	f := newBrowserHTTPFixture(t, revoker)
	browser := f.browserDirect(t)
	_ = f.mintDirect(t)
	denied := f.request(http.MethodPost, "/api/accelerator-browser-session/revoke", "Bearer "+browser.encoded(), nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("browser revoke response = %d %q", denied.Code, denied.Body.String())
	}
	f.manager.state.mu.Lock()
	if f.manager.state.session == nil || f.manager.state.ticket == nil {
		t.Fatal("browser revoke changed state")
	}
	f.manager.state.mu.Unlock()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- f.request(http.MethodPost, "/api/accelerator-browser-session/revoke", "Bearer "+f.creatorToken, nil)
	}()
	<-entered
	select {
	case <-done:
		t.Fatal("endpoint returned before callback")
	default:
	}
	f.manager.state.mu.Lock()
	if f.manager.state.session != nil || f.manager.state.ticket != nil {
		t.Fatal("state not cleared before callback")
	}
	f.manager.state.mu.Unlock()
	close(release)
	w := <-done
	if w.Code != http.StatusNoContent || w.Body.Len() != 0 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response = %d %q %#v", w.Code, w.Body.String(), w.Header())
	}
	if again := f.request(http.MethodPost, "/api/accelerator-browser-session/revoke", "Bearer "+f.creatorToken, nil); again.Code != http.StatusNoContent || calls.Load() != 1 {
		t.Fatalf("idempotent response/calls = %d/%d", again.Code, calls.Load())
	}

	for _, test := range []struct {
		name   string
		target string
		auth   string
		body   io.Reader
		want   int
	}{
		{name: "missing auth", target: "/api/accelerator-browser-session/revoke", want: 401},
		{name: "query", target: "/api/accelerator-browser-session/revoke?ticket=x", auth: "Bearer " + f.creatorToken, want: 400},
		{name: "body", target: "/api/accelerator-browser-session/revoke", auth: "Bearer " + f.creatorToken, body: strings.NewReader("x"), want: 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := f.request(http.MethodPost, test.target, test.auth, test.body)
			if w.Code != test.want {
				t.Fatalf("response = %d %q", w.Code, w.Body.String())
			}
		})
	}
	t.Run("bare query preserves state", func(t *testing.T) {
		bareFixture := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
		bearer := bareFixture.browserDirect(t)
		_ = bareFixture.mintDirect(t)
		r := httptest.NewRequest(http.MethodPost, "/api/accelerator-browser-session/revoke", nil)
		r.Host = "localhost"
		r.Header.Set("Authorization", "Bearer "+bareFixture.creatorToken)
		r.URL.ForceQuery = true
		w := httptest.NewRecorder()
		bareFixture.handler.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("response = %d %q", w.Code, w.Body.String())
		}
		bareFixture.manager.state.mu.Lock()
		hasTicket := bareFixture.manager.state.ticket != nil
		hasSession := bareFixture.manager.state.session != nil
		bareFixture.manager.state.mu.Unlock()
		if !hasTicket || !hasSession {
			t.Fatal("bare query revoked state")
		}
		if _, ok := bareFixture.manager.Authenticate(bearer); !ok {
			t.Fatal("bare query revoked bearer")
		}
	})
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions, http.MethodHead} {
		if w := f.request(method, "/api/accelerator-browser-session/revoke", "Bearer "+f.creatorToken, nil); w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("method %s response = %d %#v", method, w.Code, w.Header())
		}
	}
	if w := f.request(http.MethodPost, "/api/accelerator-browser-session/revoke/", "Bearer "+f.creatorToken, nil); w.Code != http.StatusNotFound {
		t.Fatalf("alternate path = %d", w.Code)
	}
}

func TestBrowserEntryCredentialRedaction(t *testing.T) {
	// ASCII raw values make both the raw bytes and their base64url encodings
	// distinctive, printable leak needles.
	ticketRaw := []byte("TICKET-RAW-0123456789-ABCDEFGHIJ")
	bearerRaw := []byte("BEARER-RAW-0123456789-ABCDEFGHIJ")
	if len(ticketRaw) != 32 || len(bearerRaw) != 32 {
		t.Fatal("credential fixture must be exactly 32 bytes")
	}
	entropy := append(append([]byte{}, ticketRaw...), bearerRaw...)
	entropy = append(entropy, bytes.Repeat([]byte{0xc1}, 16)...)
	entropy = append(entropy, bytes.Repeat([]byte{0xc2}, 16)...)
	entropy = append(entropy, bytes.Repeat([]byte{0xd1}, 32+32+16+16)...)

	var callbackArtifacts []string
	revoker := browserRevokerFunc(func(ctx context.Context, id agent.SessionID) {
		callbackArtifacts = append(callbackArtifacts,
			fmt.Sprintf("ctx=%v id=%v", ctx, id),
			fmt.Sprintf("session formats: %v|%+v|%#v|%s|%q|%x", id, id, id, id, id, id))
	})
	clock := &testClock{now: time.Date(2026, 8, 1, 5, 6, 7, 123, time.UTC)}
	manager := newBrowserSessionManager(clock.Now, bytes.NewReader(entropy), revoker)
	creatorRaw := bytes.Repeat([]byte{0x52}, 32)
	creatorText := base64.RawURLEncoding.EncodeToString(creatorRaw)
	creatorToken, err := ParseCreatorToken(creatorText)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := newCreatorAuthenticator(DeriveCreatorVerifier(creatorToken).Encoded(), bytes.NewReader(bytes.Repeat([]byte{0x53}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	caller := &recordingMethodCaller{}
	options := AcceleratorOptions(0, nil, CreatorOrBrowserGuard(creator, manager))
	options.BrowserSessions = manager
	options.BrowserEntryAvailability = alwaysEnabledBrowserEntry{}
	options.AcceleratorInfoProvider = NewAuthenticatedAcceleratorInfo(agent.BuildIdentity{BuildVersion: "redaction-test"}, "redaction-instance", agent.CapabilityResolution{Capabilities: []agent.Capability{agent.CapabilitySecretsList, agent.CapabilitySecretsDetail}})
	options.MethodAuthorizer = NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: []agent.Capability{agent.CapabilitySecretsList, agent.CapabilitySecretsDetail}})
	srv, err := NewWithOptions(caller, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}

	var capturedLog bytes.Buffer
	previousWriter, previousFlags, previousPrefix := log.Writer(), log.Flags(), log.Prefix()
	log.SetOutput(&capturedLog)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() { log.SetOutput(previousWriter); log.SetFlags(previousFlags); log.SetPrefix(previousPrefix) })
	log.Print("browser credential redaction corpus begin")

	type artifact struct{ bucket, value string }
	var artifacts []artifact
	add := func(bucket string, values ...string) {
		for _, value := range values {
			artifacts = append(artifacts, artifact{bucket, value})
		}
	}
	serve := func(r *http.Request, bucket string) *httptest.ResponseRecorder {
		r.Host = "localhost"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		add(bucket, w.Body.String(), fmt.Sprint(w.Header()), fmt.Sprint(w.Result().Header), fmt.Sprint(w.Result().Cookies()), fmt.Sprint(w.Result().Location()))
		return w
	}

	// Successful mint and exchange bodies are the only intentional credential
	// disclosures. Their headers and all logs remain in the scanned corpus.
	mintRequest := httptest.NewRequest(http.MethodPost, "/api/accelerator-browser-ticket", nil)
	mintRequest.Header.Set("Authorization", "Bearer "+creatorText)
	mintArtifactStart := len(artifacts)
	mint := serve(mintRequest, "mint-success-metadata")
	artifacts = append(artifacts[:mintArtifactStart], artifacts[mintArtifactStart+1:]...)
	wantTicket := base64.RawURLEncoding.EncodeToString(ticketRaw)
	if mint.Code != http.StatusCreated || !strings.Contains(mint.Body.String(), wantTicket) {
		t.Fatalf("mint did not return distinctive ticket: %d %q", mint.Code, mint.Body.String())
	}
	exchange := httptest.NewRequest(http.MethodPost, "/api/accelerator-browser-session", strings.NewReader(`{"ticket":"`+wantTicket+`"}`))
	exchangeArtifactStart := len(artifacts)
	exchanged := serve(exchange, "exchange-success-metadata")
	artifacts = append(artifacts[:exchangeArtifactStart], artifacts[exchangeArtifactStart+1:]...)
	wantBearer := base64.RawURLEncoding.EncodeToString(bearerRaw)
	if exchanged.Code != http.StatusOK || !strings.Contains(exchanged.Body.String(), wantBearer) {
		t.Fatalf("exchange did not return distinctive bearer: %d %q", exchanged.Code, exchanged.Body.String())
	}

	protected := func(method, target, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+wantBearer)
		return serve(r, "protected-success")
	}
	info := protected(http.MethodGet, "/api/accelerator-info", "")
	rpc := protected(http.MethodPost, "/api/call", `{"method":"GetSecretData"}`)
	if info.Code != http.StatusOK || !strings.Contains(info.Body.String(), `"runtime":"accelerator"`) || rpc.Code != http.StatusOK || !strings.Contains(rpc.Body.String(), `"data":"ok"`) || caller.callCount() != 1 {
		t.Fatalf("protected successes missing: info=%d %q rpc=%d %q calls=%d", info.Code, info.Body.String(), rpc.Code, rpc.Body.String(), caller.callCount())
	}

	// Exercise every credential-bearing request channel. Only response artifacts
	// are retained: persisting the hostile request itself would manufacture a leak.
	channelRequests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/accelerator-info", nil),
		httptest.NewRequest(http.MethodPost, "/api/accelerator-browser-session", strings.NewReader(`{"ticket":"`+wantTicket+`"}`)),
		httptest.NewRequest(http.MethodGet, "/api/accelerator-info?bearer="+url.QueryEscape(wantBearer), nil),
		httptest.NewRequest(http.MethodPost, "/api/accelerator-browser-session/"+wantTicket, strings.NewReader(wantBearer)),
	}
	channelRequests[0].Header.Set("Authorization", "Bearer "+wantBearer+"x")
	channelRequests[0].Header.Set("X-Credential", wantTicket)
	channelRequests[1].Header["Cookie"] = []string{"ticket=" + wantTicket, ";==" + wantBearer}
	channelRequests[2].URL.ForceQuery = true
	channelRequests[3].URL.RawPath = "/api/accelerator-browser-session/" + url.PathEscape(wantBearer)
	for _, r := range channelRequests {
		w := serve(r, "input-channel-failure")
		if w.Code < 400 {
			t.Fatalf("credential-channel request unexpectedly succeeded: %s %s => %d", r.Method, r.URL, w.Code)
		}
	}

	// Method/path failures cover all standard verbs and non-success response
	// bodies, headers, Location URLs, and cookies.
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions, http.MethodHead, http.MethodConnect, http.MethodTrace} {
		r := httptest.NewRequest(method, "/api/accelerator-browser-session/revoke", nil)
		r.Header.Set("Authorization", "Bearer "+wantBearer)
		serve(r, "all-verb-failure")
	}

	invalidTicket, ticketErr := ParseBrowserTicket(wantTicket + "=")
	invalidBearer, bearerErr := ParseBrowserBearer(wantBearer + "=")
	verifier := deriveBrowserVerifier(browserBearerDomain, bearerRawToArray(t, bearerRaw))
	add("parser-errors-sentinels", fmt.Sprint(ticketErr, bearerErr, ErrInvalidBrowserTicket, ErrInvalidBrowserBearer))
	add("all-verb-formats",
		fmt.Sprintf("ticket=%v|%+v|%#v|%s|%q|%x", invalidTicket, invalidTicket, invalidTicket, invalidTicket, invalidTicket, invalidTicket),
		fmt.Sprintf("bearer=%v|%+v|%#v|%s|%q|%x", invalidBearer, invalidBearer, invalidBearer, invalidBearer, invalidBearer, invalidBearer),
		fmt.Sprintf("verifier=%v|%+v|%#v|%s|%q|%x", verifier, verifier, verifier, verifier, verifier, verifier),
		fmt.Sprintf("manager=%v|%+v|%#v|%s|%q|%x", manager, manager, manager, manager, manager, manager),
		fmt.Sprintf("dto=%v|%+v|%#v|%q|%x", browserExchangeRequest{Ticket: wantTicket}, browserTicketResponse{Ticket: wantTicket}, browserBearerResponse{Bearer: wantBearer}, browserExchangeRequest{Ticket: wantTicket}, browserBearerResponse{Bearer: wantBearer}))
	manager.state.mu.Lock()
	add("state-inspection", fmt.Sprintf("store=%v|%+v|%#v session=%v|%+v|%#v", manager.state, manager.state, manager.state, manager.state.session, manager.state.session, manager.state.session))
	manager.state.mu.Unlock()

	// Replacing the live session invokes the real callback seam; revocation then
	// exercises the endpoint callback and its response material.
	replacementTicket, _, err := manager.Mint()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = manager.Exchange(replacementTicket); err != nil {
		t.Fatal(err)
	}
	if len(callbackArtifacts) == 0 {
		t.Fatal("replacement did not invoke revocation callback")
	}
	add("revocation-callback", callbackArtifacts...)
	revoke := httptest.NewRequest(http.MethodPost, "/api/accelerator-browser-session/revoke", nil)
	revoke.Header.Set("Authorization", "Bearer "+creatorText)
	if w := serve(revoke, "revocation-response"); w.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d %q", w.Code, w.Body.String())
	}
	add("standard-logs", capturedLog.String())
	if !strings.Contains(capturedLog.String(), "browser credential redaction corpus begin") {
		t.Fatal("standard log capture is empty")
	}

	counts := map[string]int{}
	for _, a := range artifacts {
		counts[a.bucket]++
		for _, secret := range []string{string(ticketRaw), wantTicket, string(bearerRaw), wantBearer} {
			if strings.Contains(a.value, secret) {
				t.Fatalf("%s artifact leaked %q: %q", a.bucket, secret, a.value)
			}
		}
	}
	for _, bucket := range []string{"mint-success-metadata", "exchange-success-metadata", "protected-success", "input-channel-failure", "all-verb-failure", "parser-errors-sentinels", "all-verb-formats", "state-inspection", "revocation-callback", "revocation-response", "standard-logs"} {
		if counts[bucket] == 0 {
			t.Fatalf("redaction artifact bucket %q is empty", bucket)
		}
	}
}

func bearerRawToArray(t *testing.T, raw []byte) [32]byte {
	t.Helper()
	var value [32]byte
	if len(raw) != len(value) {
		t.Fatalf("raw browser credential length = %d", len(raw))
	}
	copy(value[:], raw)
	return value
}

func TestBrowserEndpointMethodAndPathMatrix(t *testing.T) {
	f := newBrowserHTTPFixture(t, NoopBrowserSessionRevoker{})
	for _, target := range []string{"/api/accelerator-browser-ticket", "/api/accelerator-browser-session", "/api/accelerator-browser-session/revoke"} {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions, http.MethodHead} {
			w := f.request(method, target, "Bearer "+f.creatorToken, nil)
			if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != http.MethodPost {
				t.Fatalf("%s %s response = %d %#v", method, target, w.Code, w.Header())
			}
		}
	}
	for _, target := range []string{
		"/api/accelerator-browser-ticket/", "/api/accelerator-browser-session/", "/api/accelerator-browser-session/revoke/",
		"/api/accelerator-browser-sessions", "/api/browser-session", "/api/login", "/api/logout", "/ticket",
	} {
		if w := f.request(http.MethodPost, target, "Bearer "+f.creatorToken, nil); w.Code != http.StatusNotFound {
			t.Fatalf("alternate path %s response = %d %q", target, w.Code, w.Body.String())
		}
	}
}
