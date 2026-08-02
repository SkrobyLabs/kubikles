package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"kubikles/pkg/agent"
)

const (
	BrowserTicketLifetime = time.Minute
	BrowserSessionIdleTTL = 15 * time.Minute
	BrowserSessionHardTTL = 8 * time.Hour
)

const (
	browserTicketDomain = "kubikles/accelerator/browser-ticket/v1\x00"
	browserBearerDomain = "kubikles/accelerator/browser-bearer/v1\x00"
)

var (
	ErrInvalidBrowserTicket = errors.New("invalid browser ticket")
	ErrInvalidBrowserBearer = errors.New("invalid browser bearer")
)

type BrowserTicket struct{ value [32]byte }
type BrowserBearer struct{ value [32]byte }
type browserVerifier struct{ value [32]byte }
type browserStateRedactor struct{}

func parseBrowserValue(text string, target *[32]byte, sentinel error) error {
	if len(text) != 43 {
		return sentinel
	}
	decoded, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != text {
		return sentinel
	}
	copy(target[:], decoded)
	return nil
}
func ParseBrowserTicket(text string) (BrowserTicket, error) {
	var v BrowserTicket
	return v, parseBrowserValue(text, &v.value, ErrInvalidBrowserTicket)
}
func ParseBrowserBearer(text string) (BrowserBearer, error) {
	var v BrowserBearer
	return v, parseBrowserValue(text, &v.value, ErrInvalidBrowserBearer)
}
func deriveBrowserVerifier(domain string, value [32]byte) browserVerifier {
	return browserVerifier{sha256.Sum256(append([]byte(domain), value[:]...))}
}
func DeriveBrowserTicketVerifier(ticket BrowserTicket) [32]byte {
	return deriveBrowserVerifier(browserTicketDomain, ticket.value).value
}
func DeriveBrowserBearerVerifier(bearer BrowserBearer) [32]byte {
	return deriveBrowserVerifier(browserBearerDomain, bearer.value).value
}
func (v BrowserTicket) encoded() string            { return base64.RawURLEncoding.EncodeToString(v.value[:]) }
func (v BrowserBearer) encoded() string            { return base64.RawURLEncoding.EncodeToString(v.value[:]) }
func (v BrowserTicket) String() string             { return "<redacted>" }
func (v BrowserBearer) String() string             { return "<redacted>" }
func (v browserVerifier) String() string           { return "<redacted>" }
func (v BrowserTicket) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "<redacted>") }
func (v BrowserBearer) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "<redacted>") }
func (v browserVerifier) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "<redacted>")
}
func (browserStateRedactor) String() string { return "<redacted>" }
func (browserStateRedactor) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "<redacted>")
}

type BrowserSessionRevoker interface {
	RevokeBrowserSession(context.Context, agent.SessionID)
}

// BrowserSessionTimer is deliberately minimal so the hard-expiry boundary is
// deterministic in tests without adding another lifecycle policy.
type BrowserSessionTimer interface {
	C() <-chan time.Time
	Stop() bool
}
type BrowserSessionClock interface {
	NewTimer(time.Duration) BrowserSessionTimer
}
type browserSessionRealClock struct{}
type browserSessionRealTimer struct{ *time.Timer }

func (t browserSessionRealTimer) C() <-chan time.Time { return t.Timer.C }
func (browserSessionRealClock) NewTimer(d time.Duration) BrowserSessionTimer {
	return browserSessionRealTimer{time.NewTimer(d)}
}

type browserSessionTimerRecord struct {
	timer   BrowserSessionTimer
	session *browserSessionState
	cancel  chan struct{}
}
type NoopBrowserSessionRevoker struct{}

func (NoopBrowserSessionRevoker) RevokeBrowserSession(context.Context, agent.SessionID) {}

type browserTicketState struct {
	browserStateRedactor
	verifier  browserVerifier
	expiresAt time.Time
}
type browserSessionState struct {
	browserStateRedactor
	verifier                browserVerifier
	context                 agent.AuthenticatedCallContext
	createdAt, lastActivity time.Time
}

type browserSessionStore struct {
	browserStateRedactor
	mu      sync.Mutex
	ticket  *browserTicketState
	session *browserSessionState
	timer   *browserSessionTimerRecord
}

type BrowserSessionManager struct {
	state   *browserSessionStore
	now     func() time.Time
	entropy io.Reader
	revoker BrowserSessionRevoker
	clock   BrowserSessionClock
}

func (m BrowserSessionManager) String() string { return "<redacted>" }
func (m BrowserSessionManager) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "<redacted>")
}

func NewBrowserSessionManager(revoker BrowserSessionRevoker) *BrowserSessionManager {
	return NewBrowserSessionManagerWithDependencies(time.Now, rand.Reader, revoker)
}

// NewBrowserSessionManagerWithDependencies constructs the fixed browser
// session policy with injectable clock and entropy dependencies.
func NewBrowserSessionManagerWithDependencies(now func() time.Time, entropy io.Reader, revoker BrowserSessionRevoker) *BrowserSessionManager {
	return newBrowserSessionManagerWithClock(now, entropy, revoker, browserSessionRealClock{})
}
func newBrowserSessionManager(now func() time.Time, entropy io.Reader, revoker BrowserSessionRevoker) *BrowserSessionManager {
	return newBrowserSessionManagerWithClock(now, entropy, revoker, browserSessionRealClock{})
}
func newBrowserSessionManagerWithClock(now func() time.Time, entropy io.Reader, revoker BrowserSessionRevoker, clock BrowserSessionClock) *BrowserSessionManager {
	if now == nil {
		now = time.Now
	}
	if entropy == nil {
		entropy = rand.Reader
	}
	if revoker == nil {
		revoker = NoopBrowserSessionRevoker{}
	}
	if clock == nil {
		clock = browserSessionRealClock{}
	}
	return &BrowserSessionManager{state: &browserSessionStore{}, now: now, entropy: entropy, revoker: revoker, clock: clock}
}
func browserRandom(entropy io.Reader) ([32]byte, error) {
	var raw [32]byte
	if _, err := io.ReadFull(entropy, raw[:]); err != nil {
		return raw, errors.New("browser credential entropy unavailable")
	}
	return raw, nil
}
func browserID(entropy io.Reader, prefix string) (string, error) {
	var raw [16]byte
	if _, err := io.ReadFull(entropy, raw[:]); err != nil {
		return "", errors.New("browser identity entropy unavailable")
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (m *BrowserSessionManager) Mint() (BrowserTicket, time.Time, error) {
	raw, err := browserRandom(m.entropy)
	if err != nil {
		return BrowserTicket{}, time.Time{}, err
	}
	now := m.now()
	ticket := BrowserTicket{value: raw}
	expiry := now.Add(BrowserTicketLifetime)
	m.state.mu.Lock()
	m.state.ticket = &browserTicketState{verifier: deriveBrowserVerifier(browserTicketDomain, raw), expiresAt: expiry}
	m.state.mu.Unlock()
	return ticket, expiry, nil
}
func (m *BrowserSessionManager) Exchange(ticket BrowserTicket) (BrowserBearer, time.Time, error) {
	entryTime := m.now()
	wanted := deriveBrowserVerifier(browserTicketDomain, ticket.value)
	m.state.mu.Lock()
	if m.state.ticket == nil || !entryTime.Before(m.state.ticket.expiresAt) || subtle.ConstantTimeCompare(m.state.ticket.verifier.value[:], wanted.value[:]) != 1 {
		if m.state.ticket != nil && !entryTime.Before(m.state.ticket.expiresAt) {
			m.state.ticket = nil
		}
		m.state.mu.Unlock()
		return BrowserBearer{}, time.Time{}, ErrInvalidBrowserTicket
	}
	m.state.mu.Unlock()

	raw, err := browserRandom(m.entropy)
	if err != nil {
		return BrowserBearer{}, time.Time{}, err
	}
	principal, err := browserID(m.entropy, "browser-")
	if err != nil {
		return BrowserBearer{}, time.Time{}, err
	}
	sessionID, err := browserID(m.entropy, "browser-http-")
	if err != nil {
		return BrowserBearer{}, time.Time{}, err
	}
	now := m.now()
	m.state.mu.Lock()
	if m.state.ticket == nil || !now.Before(m.state.ticket.expiresAt) || subtle.ConstantTimeCompare(m.state.ticket.verifier.value[:], wanted.value[:]) != 1 {
		if m.state.ticket != nil && !now.Before(m.state.ticket.expiresAt) {
			m.state.ticket = nil
		}
		m.state.mu.Unlock()
		return BrowserBearer{}, time.Time{}, ErrInvalidBrowserTicket
	}
	m.state.ticket = nil
	old := m.state.session
	oldTimer := m.state.timer
	ctx := agent.AuthenticatedCallContext{PrincipalID: agent.PrincipalID(principal), SessionID: agent.SessionID(sessionID)}
	session := &browserSessionState{verifier: deriveBrowserVerifier(browserBearerDomain, raw), context: ctx, createdAt: now, lastActivity: now}
	m.state.session = session
	record := &browserSessionTimerRecord{session: session, cancel: make(chan struct{})}
	record.timer = m.clock.NewTimer(BrowserSessionHardTTL)
	m.state.timer = record
	m.state.mu.Unlock()
	if oldTimer != nil {
		oldTimer.timer.Stop()
		close(oldTimer.cancel)
	}
	if old != nil {
		m.revoker.RevokeBrowserSession(context.Background(), old.context.SessionID)
	}
	go func() {
		select {
		case <-record.timer.C():
			m.expire(record)
		case <-record.cancel:
		}
	}()
	return BrowserBearer{value: raw}, now.Add(BrowserSessionHardTTL), nil
}
func (m *BrowserSessionManager) expire(record *browserSessionTimerRecord) {
	if m == nil || record == nil {
		return
	}
	m.state.mu.Lock()
	if m.state.timer != record || m.state.session != record.session {
		m.state.mu.Unlock()
		return
	}
	old := m.state.session
	m.state.session, m.state.timer = nil, nil
	m.state.mu.Unlock()
	m.revoker.RevokeBrowserSession(context.Background(), old.context.SessionID)
}
func (m *BrowserSessionManager) Authenticate(bearer BrowserBearer) (agent.AuthenticatedCallContext, bool) {
	wanted := deriveBrowserVerifier(browserBearerDomain, bearer.value)
	var revoked *browserSessionState
	m.state.mu.Lock()
	now := m.now()
	if m.state.session != nil && (!now.Before(m.state.session.lastActivity.Add(BrowserSessionIdleTTL)) || !now.Before(m.state.session.createdAt.Add(BrowserSessionHardTTL))) {
		revoked = m.state.session
		m.state.session = nil
		timer := m.state.timer
		m.state.timer = nil
		if timer != nil {
			timer.timer.Stop()
			close(timer.cancel)
		}
	}
	if revoked == nil && m.state.session != nil && subtle.ConstantTimeCompare(m.state.session.verifier.value[:], wanted.value[:]) == 1 {
		c := m.state.session.context
		m.state.mu.Unlock()
		return c, true
	}
	m.state.mu.Unlock()
	if revoked != nil {
		m.revoker.RevokeBrowserSession(context.Background(), revoked.context.SessionID)
	}
	return agent.AuthenticatedCallContext{}, false
}

// withActiveBrowserSession linearizes WebSocket activation with replacement,
// revocation, and exact idle/hard expiry without extending HTTP activity.
func (m *BrowserSessionManager) withActiveBrowserSession(id agent.SessionID, activate func() bool) bool {
	if m == nil {
		return false
	}
	var revoked *browserSessionState
	m.state.mu.Lock()
	now := m.now()
	session := m.state.session
	if session != nil && (!now.Before(session.lastActivity.Add(BrowserSessionIdleTTL)) || !now.Before(session.createdAt.Add(BrowserSessionHardTTL))) {
		revoked = session
		m.state.session = nil
		timer := m.state.timer
		m.state.timer = nil
		if timer != nil {
			timer.timer.Stop()
			close(timer.cancel)
		}
		session = nil
	}
	ok := session != nil && session.context.SessionID == id && activate != nil && activate()
	m.state.mu.Unlock()
	if revoked != nil {
		m.revoker.RevokeBrowserSession(context.Background(), revoked.context.SessionID)
	}
	return ok
}
func (m *BrowserSessionManager) Touch(id agent.SessionID) {
	var revoked *browserSessionState
	m.state.mu.Lock()
	now := m.now()
	if m.state.session != nil && (!now.Before(m.state.session.lastActivity.Add(BrowserSessionIdleTTL)) || !now.Before(m.state.session.createdAt.Add(BrowserSessionHardTTL))) {
		revoked = m.state.session
		m.state.session = nil
		timer := m.state.timer
		m.state.timer = nil
		if timer != nil {
			timer.timer.Stop()
			close(timer.cancel)
		}
	} else if m.state.session != nil && m.state.session.context.SessionID == id {
		m.state.session.lastActivity = now
	}
	m.state.mu.Unlock()
	if revoked != nil {
		m.revoker.RevokeBrowserSession(context.Background(), revoked.context.SessionID)
	}
}
func (m *BrowserSessionManager) Revoke(ctx context.Context) {
	m.clear(ctx)
}

// RevokeAll clears the outstanding ticket and active browser session, waiting
// for the registry revocation after the state has been made unavailable.
func (m *BrowserSessionManager) RevokeAll(ctx context.Context) {
	m.clear(ctx)
}

func (m *BrowserSessionManager) clear(ctx context.Context) {
	if m == nil {
		return
	}
	m.state.mu.Lock()
	old := m.state.session
	timer := m.state.timer
	m.state.session = nil
	m.state.ticket = nil
	m.state.timer = nil
	m.state.mu.Unlock()
	if timer != nil {
		timer.timer.Stop()
		close(timer.cancel)
	}
	if old != nil {
		m.revoker.RevokeBrowserSession(ctx, old.context.SessionID)
	}
}

type browserStatusRecorder struct {
	http.ResponseWriter
	finalStatus int
}

func (w *browserStatusRecorder) WriteHeader(status int) {
	if status >= 200 && w.finalStatus == 0 {
		w.finalStatus = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *browserStatusRecorder) Write(b []byte) (int, error) {
	if w.finalStatus == 0 {
		w.finalStatus = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func CreatorOrBrowserGuard(creator *CreatorAuthenticator, sessions *BrowserSessionManager) ProtectedRouteGuard {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			credential, ok := strictBearer(r)
			if !ok {
				writeAcceleratorError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			creatorMatch := creator != nil && creator.matchesBearer(credential)
			var browserCall agent.AuthenticatedCallContext
			browserMatch := false
			bearer, err := ParseBrowserBearer(credential)
			if err == nil && sessions != nil {
				browserCall, browserMatch = sessions.Authenticate(bearer)
			}
			if creatorMatch {
				next.ServeHTTP(w, r.WithContext(authenticatedContext(r.Context(), creator.context, credentialKindCreator)))
				return
			}
			if browserMatch {
				recorder := &browserStatusRecorder{ResponseWriter: w}
				next.ServeHTTP(recorder, r.WithContext(authenticatedContext(r.Context(), browserCall, credentialKindBrowser)))
				if recorder.finalStatus == 0 || recorder.finalStatus >= 200 && recorder.finalStatus < 400 {
					sessions.Touch(browserCall.SessionID)
				}
				return
			}
			writeAcceleratorError(w, http.StatusUnauthorized, "unauthorized")
		})
	}
}
