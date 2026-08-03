package acceleratorprovision

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	browserLaunchReceiptDomain = "kubikles/accelerator/browser-launch/v1\x00"
	browserLaunchBodyLimit     = 4096
)

type browserLaunchStatus uint8

const (
	browserLaunchConfirmed browserLaunchStatus = iota + 1
	browserLaunchEnded
)

type browserLaunchTicket struct {
	ticket, receipt string
	expiresAt       time.Time
}

func (browserLaunchTicket) String() string { return "<redacted>" }
func (browserLaunchTicket) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<redacted>")
}

type browserLaunchWaiter struct {
	updates   chan browserLaunchStatus
	confirmed bool
	ended     bool
}

type browserLaunchControlEnvelope struct {
	Type string          `json:"type"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

type browserLaunchControlData struct {
	Receipt string `json:"receipt"`
	Status  string `json:"status"`
}

func browserLaunchReceiptKey(raw string) (string, bool) {
	if len(raw) != 43 {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return "", false
	}
	verifier := sha256.Sum256(append([]byte(browserLaunchReceiptDomain), decoded...))
	return base64.RawURLEncoding.EncodeToString(verifier[:]), true
}

func canonicalLaunchControlKey(raw string) bool {
	if len(raw) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == raw
}

func (s *ConnectedSession) registerBrowserLaunchWaiter(receipt string) (<-chan browserLaunchStatus, func(), bool) {
	key, ok := browserLaunchReceiptKey(receipt)
	if s == nil || !ok || !s.isExactCurrentCreatorSession() {
		return nil, func() {}, false
	}
	waiter := &browserLaunchWaiter{updates: make(chan browserLaunchStatus, 2)}
	s.launchMu.Lock()
	if s.launchWaiters == nil {
		s.launchWaiters = make(map[string]*browserLaunchWaiter)
	}
	if _, exists := s.launchWaiters[key]; exists {
		s.launchMu.Unlock()
		return nil, func() {}, false
	}
	s.launchWaiters[key] = waiter
	s.launchMu.Unlock()
	return waiter.updates, func() {
		s.launchMu.Lock()
		if s.launchWaiters[key] == waiter {
			delete(s.launchWaiters, key)
			close(waiter.updates)
		}
		s.launchMu.Unlock()
	}, true
}

func (s *ConnectedSession) closeBrowserLaunchWaiters() {
	if s == nil {
		return
	}
	s.launchMu.Lock()
	for key, waiter := range s.launchWaiters {
		delete(s.launchWaiters, key)
		close(waiter.updates)
	}
	s.launchMu.Unlock()
}

func (s *ConnectedSession) interceptBrowserLaunchControl(payload []byte) (handled, valid bool) {
	var envelope browserLaunchControlEnvelope
	if json.Unmarshal(payload, &envelope) != nil || envelope.Type != "event" || envelope.Name != "browser-launch" {
		return false, true
	}
	var control browserLaunchControlData
	if json.Unmarshal(envelope.Data, &control) != nil || !canonicalLaunchControlKey(control.Receipt) || (control.Status != "confirmed" && control.Status != "ended") {
		return true, false
	}
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	waiter := s.launchWaiters[control.Receipt]
	if waiter == nil {
		return true, true
	}
	if control.Status == "confirmed" {
		if waiter.confirmed || waiter.ended {
			return true, true
		}
		waiter.confirmed = true
		waiter.updates <- browserLaunchConfirmed
		return true, true
	}
	if waiter.ended {
		return true, true
	}
	waiter.ended = true
	waiter.updates <- browserLaunchEnded
	return true, true
}

func (s *ConnectedSession) isExactCurrentCreatorSession() bool {
	if s == nil || s.self != s || s.receipt == nil || s.receipt.owner == nil || s.ownerState == nil || s.tunnel == nil || tunnelEnded(s.tunnel) {
		return false
	}
	select {
	case <-s.done:
		return false
	default:
	}
	state := s.ownerState
	state.mu.Lock()
	defer state.mu.Unlock()
	return !state.closed && !state.disposing && state.receipt == s.receipt && state.currentSession == s && s.receipt.owner.credential != nil && matchesReceipt(s.receipt.owner, s.receipt)
}

func (s *ConnectedSession) mintBrowserLaunch(ctx context.Context) (browserLaunchTicket, error) {
	if ctx == nil || !s.isExactCurrentCreatorSession() {
		return browserLaunchTicket{}, errors.New("browser launch unavailable")
	}
	endpoint := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", s.tunnel.Port()))
	request, err := newLocalRequest(ctx, http.MethodPost, endpoint, "/api/accelerator-browser-ticket", nil)
	if err != nil {
		return browserLaunchTicket{}, errors.New("browser launch unavailable")
	}
	var response *http.Response
	err = s.receipt.owner.credential.withCreatorAuthorization(ctx, func(authCtx context.Context, authorization creatorAuthorizationLease) error {
		request = request.WithContext(authCtx)
		var requestErr error
		response, requestErr = authorization.DoHTTP(localClient(endpoint), request)
		return requestErr
	})
	if err != nil || response == nil {
		return browserLaunchTicket{}, errors.New("browser launch unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated || !strictJSONResponse(response) {
		return browserLaunchTicket{}, errors.New("browser launch unavailable")
	}
	var wire struct {
		Ticket        string `json:"ticket"`
		LaunchReceipt string `json:"launchReceipt"`
		ExpiresAt     string `json:"expiresAt"`
	}
	if decodeStrictJSON(response.Body, browserLaunchBodyLimit, &wire) != nil || len(wire.Ticket) != 43 || !canonicalLaunchControlKey(wire.Ticket) {
		return browserLaunchTicket{}, errors.New("browser launch unavailable")
	}
	if _, ok := browserLaunchReceiptKey(wire.LaunchReceipt); !ok {
		return browserLaunchTicket{}, errors.New("browser launch unavailable")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, wire.ExpiresAt)
	if err != nil || !time.Now().Before(expiresAt) || expiresAt.After(time.Now().Add(time.Minute)) {
		return browserLaunchTicket{}, errors.New("browser launch unavailable")
	}
	if !s.isExactCurrentCreatorSession() {
		return browserLaunchTicket{}, errors.New("browser launch unavailable")
	}
	return browserLaunchTicket{ticket: wire.Ticket, receipt: wire.LaunchReceipt, expiresAt: expiresAt}, nil
}

func (s *ConnectedSession) revokeBrowserLaunch(ctx context.Context) bool {
	if ctx == nil || !s.isExactCurrentCreatorSession() {
		return false
	}
	endpoint := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", s.tunnel.Port()))
	request, err := newLocalRequest(ctx, http.MethodPost, endpoint, "/api/accelerator-browser-session/revoke", nil)
	if err != nil {
		return false
	}
	valid := false
	err = s.receipt.owner.credential.withCreatorAuthorization(ctx, func(authCtx context.Context, authorization creatorAuthorizationLease) error {
		response, requestErr := authorization.DoHTTP(localClient(endpoint), request.WithContext(authCtx))
		if requestErr != nil || response == nil {
			return requestErr
		}
		defer response.Body.Close()
		body, readErr := io.ReadAll(&io.LimitedReader{R: response.Body, N: 2})
		valid = readErr == nil && len(body) == 0 && response.StatusCode == http.StatusNoContent &&
			len(response.Header.Values("Cache-Control")) == 1 && response.Header.Values("Cache-Control")[0] == "no-store" &&
			len(response.Header.Values("Set-Cookie")) == 0 && len(response.Header.Values("Content-Encoding")) == 0
		return nil
	})
	return err == nil && valid
}

func (s *ConnectedSession) browserLaunchURL(ticket browserLaunchTicket) (string, bool) {
	if !s.isExactCurrentCreatorSession() || len(ticket.ticket) != 43 || !canonicalLaunchControlKey(ticket.ticket) {
		return "", false
	}
	target := url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", s.tunnel.Port())), Path: "/accelerator/browser/", Fragment: "ticket=" + ticket.ticket}
	return target.String(), true
}
