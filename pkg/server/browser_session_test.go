package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kubikles/pkg/agent"
)

type lockedReader struct {
	mu sync.Mutex
	r  io.Reader
}

type countingEntropy struct{ reads atomic.Int32 }

func (r *countingEntropy) Read([]byte) (int, error) {
	r.reads.Add(1)
	return 0, io.EOF
}

type firstReadBlockingEntropy struct {
	mu      sync.Mutex
	data    *bytes.Reader
	reads   int
	entered chan struct{}
	release chan struct{}
}

func (r *firstReadBlockingEntropy) Read(p []byte) (int, error) {
	r.mu.Lock()
	r.reads++
	first := r.reads == 1
	r.mu.Unlock()
	if first {
		close(r.entered)
		<-r.release
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.data.Read(p)
}

func (r *lockedReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.r.Read(p)
}

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
func (c *testClock) Set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

type browserRevokerFunc func(context.Context, agent.SessionID)

func (f browserRevokerFunc) RevokeBrowserSession(ctx context.Context, id agent.SessionID) { f(ctx, id) }

func repeatedEntropy(parts ...struct {
	b byte
	n int
}) io.Reader {
	var all []byte
	for _, part := range parts {
		all = append(all, bytes.Repeat([]byte{part.b}, part.n)...)
	}
	return bytes.NewReader(all)
}

func mintAndExchange(t *testing.T, manager *BrowserSessionManager) (BrowserTicket, BrowserBearer, agent.AuthenticatedCallContext) {
	t.Helper()
	ticket, _, err := manager.Mint()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := manager.Exchange(ticket)
	if err != nil {
		t.Fatal(err)
	}
	call, ok := manager.Authenticate(bearer)
	if !ok {
		t.Fatal("new bearer did not authenticate")
	}
	return ticket, bearer, call
}

func TestBrowserCredentialKnownVectors(t *testing.T) {
	const (
		encoded        = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
		ticketDigest   = "3b6535a16329ead0b2e2f5e90bb2976a484acdbfd4de40e86418bc5447c5b3ed"
		bearerDigest   = "cd688cef2ca2a23fdda938019e535bcbc3c9685c3f50fd9c2947d68e21ce651a"
		ticketVerifier = "O2U1oWMp6tCy4vXpC7KXakhKzb_U3kDoZBi8VEfFs-0"
		bearerVerifier = "zWiM7yyioj_dqTgBnlNby8PJaFw_UP2cKUfWjiHOZRo"
	)
	ticket, err := ParseBrowserTicket(encoded)
	if err != nil {
		t.Fatal(err)
	}
	bearer, err := ParseBrowserBearer(encoded)
	if err != nil {
		t.Fatal(err)
	}
	ticketGot := DeriveBrowserTicketVerifier(ticket)
	bearerGot := DeriveBrowserBearerVerifier(bearer)
	if hex.EncodeToString(ticketGot[:]) != ticketDigest || base64.RawURLEncoding.EncodeToString(ticketGot[:]) != ticketVerifier {
		t.Fatalf("ticket verifier = %x", ticketGot)
	}
	if hex.EncodeToString(bearerGot[:]) != bearerDigest || base64.RawURLEncoding.EncodeToString(bearerGot[:]) != bearerVerifier {
		t.Fatalf("bearer verifier = %x", bearerGot)
	}
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	independentTicket := sha256.Sum256(append([]byte(browserTicketDomain), raw...))
	independentBearer := sha256.Sum256(append([]byte(browserBearerDomain), raw...))
	if ticketGot != independentTicket || bearerGot != independentBearer || ticketGot == bearerGot {
		t.Fatal("domain-separated independent vectors did not match")
	}

	malformed := []string{"", " " + encoded, encoded + "=", strings.Repeat("!", 43), base64.RawURLEncoding.EncodeToString(make([]byte, 31)), base64.RawURLEncoding.EncodeToString(make([]byte, 33)), encoded[:42] + "9"}
	for _, value := range malformed {
		if _, err := ParseBrowserTicket(value); !errors.Is(err, ErrInvalidBrowserTicket) {
			t.Errorf("ticket %q error = %v", value, err)
		}
		if _, err := ParseBrowserBearer(value); !errors.Is(err, ErrInvalidBrowserBearer) {
			t.Errorf("bearer %q error = %v", value, err)
		}
	}
	for _, value := range []any{ticket, bearer, deriveBrowserVerifier(browserTicketDomain, ticket.value)} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
			if got := fmt.Sprintf(verb, value); got != "<redacted>" {
				t.Fatalf("%T %s = %q", value, verb, got)
			}
		}
		if _, ok := value.(json.Marshaler); ok {
			t.Fatalf("%T implements json.Marshaler", value)
		}
		if _, ok := value.(encoding.TextMarshaler); ok {
			t.Fatalf("%T implements encoding.TextMarshaler", value)
		}
		marshaled, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{encoded, ticketDigest, bearerDigest, ticketVerifier, bearerVerifier} {
			if strings.Contains(string(marshaled), secret) {
				t.Fatalf("%T JSON exposed credential material: %s", value, marshaled)
			}
		}
	}
}

func TestBrowserTicketMintReplacesOutstanding(t *testing.T) {
	start := time.Date(2026, 8, 1, 1, 2, 3, 4, time.UTC)
	clock := &testClock{now: start}
	entropy := repeatedEntropy(struct {
		b byte
		n int
	}{1, 32}, struct {
		b byte
		n int
	}{2, 32}, struct {
		b byte
		n int
	}{3, 64}, struct {
		b byte
		n int
	}{4, 64}, struct {
		b byte
		n int
	}{5, 64})
	m := newBrowserSessionManager(clock.Now, entropy, NoopBrowserSessionRevoker{})
	first, firstExpiry, err := m.Mint()
	if err != nil || !firstExpiry.Equal(start.Add(time.Minute)) {
		t.Fatalf("first mint = %v %v", firstExpiry, err)
	}
	second, secondExpiry, err := m.Mint()
	if err != nil || !secondExpiry.Equal(start.Add(BrowserTicketLifetime)) {
		t.Fatalf("second mint = %v %v", secondExpiry, err)
	}
	if _, _, err := m.Exchange(first); !errors.Is(err, ErrInvalidBrowserTicket) {
		t.Fatalf("replaced ticket error = %v", err)
	}
	guess := BrowserTicket{value: [32]byte{99}}
	if _, _, err := m.Exchange(guess); !errors.Is(err, ErrInvalidBrowserTicket) {
		t.Fatalf("guess error = %v", err)
	}
	if _, _, err := m.Exchange(second); err != nil {
		t.Fatalf("guess consumed outstanding ticket: %v", err)
	}

	expiring := newBrowserSessionManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{5}, 32+64)), NoopBrowserSessionRevoker{})
	ticket, _, err := expiring.Mint()
	if err != nil {
		t.Fatal(err)
	}
	clock.Set(start.Add(BrowserTicketLifetime))
	if _, _, err := expiring.Exchange(ticket); !errors.Is(err, ErrInvalidBrowserTicket) {
		t.Fatalf("ticket valid at exact expiry: %v", err)
	}
	expiring.state.mu.Lock()
	defer expiring.state.mu.Unlock()
	if expiring.state.ticket != nil || bytes.Contains([]byte(fmt.Sprintf("%#v", expiring)), []byte(ticket.encoded())) {
		t.Fatal("expired/raw ticket retained or formatted")
	}
}

func TestBrowserTicketSingleUseAndSessionReplacement(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	clock := &testClock{now: start}
	entropy := repeatedEntropy(struct {
		b byte
		n int
	}{1, 32}, struct {
		b byte
		n int
	}{2, 32}, struct {
		b byte
		n int
	}{3, 16}, struct {
		b byte
		n int
	}{4, 16}, struct {
		b byte
		n int
	}{5, 32}, struct {
		b byte
		n int
	}{6, 32}, struct {
		b byte
		n int
	}{7, 16}, struct {
		b byte
		n int
	}{8, 16})
	m := newBrowserSessionManager(clock.Now, entropy, NoopBrowserSessionRevoker{})
	firstTicket, firstBearer, firstCall := mintAndExchange(t, m)
	if _, _, err := m.Exchange(firstTicket); !errors.Is(err, ErrInvalidBrowserTicket) {
		t.Fatalf("ticket reuse = %v", err)
	}
	secondTicket, _, err := m.Mint()
	if err != nil {
		t.Fatal(err)
	}
	secondBearer, expires, err := m.Exchange(secondTicket)
	if err != nil || !expires.Equal(start.Add(BrowserSessionHardTTL)) {
		t.Fatalf("replacement = %v %v", expires, err)
	}
	secondCall, ok := m.Authenticate(secondBearer)
	if !ok || secondCall == firstCall || !secondCall.IsAuthenticated() {
		t.Fatalf("replacement context = %#v, first %#v", secondCall, firstCall)
	}
	if _, ok := m.Authenticate(firstBearer); ok {
		t.Fatal("old bearer survived replacement")
	}
	wantBearer := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{6}, 32))
	if secondBearer.encoded() != wantBearer {
		t.Fatalf("bearer = %q, want exact entropy %q", secondBearer.encoded(), wantBearer)
	}
	m.state.mu.Lock()
	state := *m.state.session
	m.state.mu.Unlock()
	if state.verifier.value == secondBearer.value || state.createdAt != start || state.lastActivity != start || state.context != secondCall {
		t.Fatalf("stored state = %#v", state)
	}
}

func TestBrowserEntropyFailuresPreserveState(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	m := newBrowserSessionManager(func() time.Time { return start }, bytes.NewReader(bytes.Repeat([]byte{1}, 32)), NoopBrowserSessionRevoker{})
	ticket, _, err := m.Mint()
	if err != nil {
		t.Fatal(err)
	}
	m.entropy = bytes.NewReader(make([]byte, 31))
	if _, _, err := m.Mint(); err == nil {
		t.Fatal("short mint entropy accepted")
	}
	m.entropy = errReader{}
	if _, _, err := m.Exchange(ticket); err == nil {
		t.Fatal("exchange entropy error accepted")
	}
	m.entropy = bytes.NewReader(bytes.Repeat([]byte{2}, 64))
	if _, _, err := m.Exchange(ticket); err != nil {
		t.Fatalf("entropy failure consumed ticket: %v", err)
	}
}

func TestBrowserExchangeInvalidTicketsReadNoEntropy(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	newManager := func() (*BrowserSessionManager, *testClock) {
		clock := &testClock{now: start}
		return newBrowserSessionManager(clock.Now, repeatedEntropy(
			struct {
				b byte
				n int
			}{1, 32},
			struct {
				b byte
				n int
			}{2, 32},
			struct {
				b byte
				n int
			}{3, 96},
		), NoopBrowserSessionRevoker{}), clock
	}
	for _, test := range []struct {
		name  string
		setup func(*testing.T, *BrowserSessionManager, *testClock) BrowserTicket
	}{
		{name: "absent", setup: func(_ *testing.T, _ *BrowserSessionManager, _ *testClock) BrowserTicket {
			return BrowserTicket{value: [32]byte{9}}
		}},
		{name: "mismatched", setup: func(t *testing.T, m *BrowserSessionManager, _ *testClock) BrowserTicket {
			if _, _, err := m.Mint(); err != nil {
				t.Fatal(err)
			}
			return BrowserTicket{value: [32]byte{9}}
		}},
		{name: "reused", setup: func(t *testing.T, m *BrowserSessionManager, _ *testClock) BrowserTicket {
			ticket, _, err := m.Mint()
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := m.Exchange(ticket); err != nil {
				t.Fatal(err)
			}
			return ticket
		}},
		{name: "replaced", setup: func(t *testing.T, m *BrowserSessionManager, _ *testClock) BrowserTicket {
			ticket, _, err := m.Mint()
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := m.Mint(); err != nil {
				t.Fatal(err)
			}
			return ticket
		}},
		{name: "expired", setup: func(t *testing.T, m *BrowserSessionManager, clock *testClock) BrowserTicket {
			ticket, _, err := m.Mint()
			if err != nil {
				t.Fatal(err)
			}
			clock.Set(start.Add(BrowserTicketLifetime))
			return ticket
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, clock := newManager()
			ticket := test.setup(t, m, clock)
			entropy := &countingEntropy{}
			m.entropy = entropy
			if _, _, err := m.Exchange(ticket); !errors.Is(err, ErrInvalidBrowserTicket) {
				t.Fatalf("Exchange error = %v", err)
			}
			if got := entropy.reads.Load(); got != 0 {
				t.Fatalf("invalid exchange entropy reads = %d, want 0", got)
			}
		})
	}
}

func TestBrowserExchangeRevalidatesAfterEntropy(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	m := newBrowserSessionManager(func() time.Time { return start }, bytes.NewReader(bytes.Repeat([]byte{1}, 32)), NoopBrowserSessionRevoker{})
	oldTicket, _, err := m.Mint()
	if err != nil {
		t.Fatal(err)
	}
	blocking := &firstReadBlockingEntropy{data: bytes.NewReader(bytes.Repeat([]byte{2}, 128)), entered: make(chan struct{}), release: make(chan struct{})}
	m.entropy = blocking
	done := make(chan error, 1)
	go func() { _, _, err := m.Exchange(oldTicket); done <- err }()
	<-blocking.entered
	newTicket, _, err := m.Mint()
	if err != nil {
		t.Fatal(err)
	}
	close(blocking.release)
	if err := <-done; !errors.Is(err, ErrInvalidBrowserTicket) {
		t.Fatalf("old exchange error = %v", err)
	}
	m.entropy = bytes.NewReader(bytes.Repeat([]byte{3}, 64))
	if _, _, err := m.Exchange(newTicket); err != nil {
		t.Fatalf("old exchange consumed replacement ticket: %v", err)
	}
}

func TestBrowserExchangeExpiresWhileEntropyBlocked(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	clock := &testClock{now: start}
	m := newBrowserSessionManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{1}, 32)), NoopBrowserSessionRevoker{})
	ticket, _, err := m.Mint()
	if err != nil {
		t.Fatal(err)
	}
	clock.Set(start.Add(BrowserTicketLifetime - time.Nanosecond))
	blocking := &firstReadBlockingEntropy{data: bytes.NewReader(bytes.Repeat([]byte{2}, 64)), entered: make(chan struct{}), release: make(chan struct{})}
	m.entropy = blocking
	done := make(chan error, 1)
	go func() { _, _, err := m.Exchange(ticket); done <- err }()
	<-blocking.entered
	clock.Set(start.Add(BrowserTicketLifetime))
	close(blocking.release)
	if err := <-done; !errors.Is(err, ErrInvalidBrowserTicket) {
		t.Fatalf("exchange at exact expiry error = %v", err)
	}
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	if m.state.ticket != nil || m.state.session != nil {
		t.Fatalf("expired exchange state ticket/session = %#v/%#v", m.state.ticket, m.state.session)
	}
}

func TestBrowserExchangeUsesPostEntropyTime(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	clock := &testClock{now: start}
	m := newBrowserSessionManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{1}, 32)), NoopBrowserSessionRevoker{})
	ticket, _, err := m.Mint()
	if err != nil {
		t.Fatal(err)
	}
	entryTime := start.Add(10 * time.Second)
	postEntropyTime := start.Add(20 * time.Second)
	clock.Set(entryTime)
	blocking := &firstReadBlockingEntropy{data: bytes.NewReader(bytes.Repeat([]byte{2}, 64)), entered: make(chan struct{}), release: make(chan struct{})}
	m.entropy = blocking
	type result struct {
		bearer  BrowserBearer
		expires time.Time
		err     error
	}
	done := make(chan result, 1)
	go func() {
		bearer, expires, err := m.Exchange(ticket)
		done <- result{bearer: bearer, expires: expires, err: err}
	}()
	<-blocking.entered
	clock.Set(postEntropyTime)
	close(blocking.release)
	got := <-done
	if got.err != nil {
		t.Fatal(got.err)
	}
	if want := postEntropyTime.Add(BrowserSessionHardTTL); !got.expires.Equal(want) {
		t.Fatalf("hard deadline = %v, want %v", got.expires, want)
	}
	m.state.mu.Lock()
	state := m.state.session
	m.state.mu.Unlock()
	if state == nil || state.createdAt != postEntropyTime || state.lastActivity != postEntropyTime {
		t.Fatalf("post-entropy session state = %#v", state)
	}
	if _, ok := m.Authenticate(got.bearer); !ok {
		t.Fatal("post-entropy bearer did not authenticate")
	}
}

func TestConcurrentTicketExchangeAllowsExactlyOne(t *testing.T) {
	const contenders = 48
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	entropy := &lockedReader{r: bytes.NewReader(bytes.Repeat([]byte{7}, 32+contenders*64))}
	m := newBrowserSessionManager(func() time.Time { return start }, entropy, NoopBrowserSessionRevoker{})
	ticket, _, err := m.Mint()
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var invalid atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, exchangeErr := m.Exchange(ticket); exchangeErr == nil {
				successes.Add(1)
			} else if errors.Is(exchangeErr, ErrInvalidBrowserTicket) {
				invalid.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || invalid.Load() != contenders-1 {
		t.Fatalf("success/invalid = %d/%d", successes.Load(), invalid.Load())
	}
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	if m.state.ticket != nil || m.state.session == nil {
		t.Fatalf("bounded state ticket/session = %#v/%#v", m.state.ticket, m.state.session)
	}
}

func TestConcurrentBrowserMintAndExchangeRemainBounded(t *testing.T) {
	const operations = 64
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	rawEntropy := make([]byte, 32+operations*96)
	for block := 0; block*32 < len(rawEntropy); block++ {
		for i := block * 32; i < (block+1)*32 && i < len(rawEntropy); i++ {
			rawEntropy[i] = byte(block)
		}
	}
	entropy := &lockedReader{r: bytes.NewReader(rawEntropy)}
	m := newBrowserSessionManager(func() time.Time { return start }, entropy, NoopBrowserSessionRevoker{})
	ticket, _, err := m.Mint()
	if err != nil {
		t.Fatal(err)
	}
	var exchangeSuccesses atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < operations; i++ {
		wg.Add(1)
		if i%2 == 0 {
			go func() {
				defer wg.Done()
				_, _, _ = m.Mint()
			}()
		} else {
			go func() {
				defer wg.Done()
				if _, _, exchangeErr := m.Exchange(ticket); exchangeErr == nil {
					exchangeSuccesses.Add(1)
				}
			}()
		}
	}
	wg.Wait()
	if exchangeSuccesses.Load() > 1 {
		t.Fatalf("original ticket exchanged %d times", exchangeSuccesses.Load())
	}
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	// The representation itself proves the bound: exactly one optional ticket slot and one optional session slot.
	if m.state.ticket != nil && m.state.ticket.expiresAt != start.Add(BrowserTicketLifetime) {
		t.Fatalf("outstanding ticket expiry = %v", m.state.ticket.expiresAt)
	}
	if m.state.session != nil && (m.state.session.context.SessionID == "" || m.state.session.context.PrincipalID == "") {
		t.Fatalf("active session context = %#v", m.state.session.context)
	}
}

func TestBrowserSessionExpiryBoundaries(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	newSession := func(t *testing.T) (*BrowserSessionManager, *testClock, BrowserBearer, agent.AuthenticatedCallContext) {
		t.Helper()
		clock := &testClock{now: start}
		m := newBrowserSessionManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{9}, 96)), NoopBrowserSessionRevoker{})
		_, bearer, call := mintAndExchange(t, m)
		return m, clock, bearer, call
	}
	t.Run("idle half open", func(t *testing.T) {
		m, clock, bearer, _ := newSession(t)
		clock.Set(start.Add(BrowserSessionIdleTTL - time.Nanosecond))
		if _, ok := m.Authenticate(bearer); !ok {
			t.Fatal("expired before idle boundary")
		}
		clock.Set(start.Add(BrowserSessionIdleTTL))
		if _, ok := m.Authenticate(bearer); ok {
			t.Fatal("valid at idle boundary")
		}
	})
	t.Run("touch advances idle only", func(t *testing.T) {
		m, clock, bearer, call := newSession(t)
		clock.Set(start.Add(14 * time.Minute))
		m.Touch(call.SessionID)
		clock.Set(start.Add(29*time.Minute - time.Nanosecond))
		if _, ok := m.Authenticate(bearer); !ok {
			t.Fatal("touch did not advance idle")
		}
		clock.Set(start.Add(BrowserSessionHardTTL))
		if _, ok := m.Authenticate(bearer); ok {
			t.Fatal("touch extended hard expiry")
		}
	})
	t.Run("hard half open", func(t *testing.T) {
		m, clock, bearer, call := newSession(t)
		for at := start.Add(14 * time.Minute); at.Before(start.Add(BrowserSessionHardTTL)); at = at.Add(14 * time.Minute) {
			clock.Set(at)
			m.Touch(call.SessionID)
		}
		clock.Set(start.Add(BrowserSessionHardTTL - time.Nanosecond))
		if _, ok := m.Authenticate(bearer); !ok {
			t.Fatal("expired before hard boundary")
		}
		clock.Set(start.Add(BrowserSessionHardTTL))
		if _, ok := m.Authenticate(bearer); ok {
			t.Fatal("valid at hard boundary")
		}
	})
}

func TestBrowserActivationSamplesClockAfterStateLock(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	clock := &testClock{now: start}
	var revocations atomic.Int32
	manager := newBrowserSessionManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{23}, 96)), browserRevokerFunc(func(context.Context, agent.SessionID) { revocations.Add(1) }))
	_, _, call := mintAndExchange(t, manager)
	clock.Set(start.Add(BrowserSessionIdleTTL - time.Nanosecond))
	manager.state.mu.Lock()
	started := make(chan struct{})
	result := make(chan bool, 1)
	go func() {
		close(started)
		result <- manager.withActiveBrowserSession(call.SessionID, func() bool { t.Error("expired activation ran"); return true })
	}()
	<-started
	clock.Set(start.Add(BrowserSessionIdleTTL))
	manager.state.mu.Unlock()
	if <-result {
		t.Fatal("session active at exact idle TTL")
	}
	if revocations.Load() != 1 {
		t.Fatalf("revocations=%d, want 1", revocations.Load())
	}
}

func TestBrowserAuthenticateAndTouchSampleClockAfterStateLock(t *testing.T) {
	for _, operation := range []string{"authenticate", "touch"} {
		t.Run(operation, func(t *testing.T) {
			start := time.Unix(1_700_100_000, 0)
			clock := &testClock{now: start}
			var revocations atomic.Int32
			manager := newBrowserSessionManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{24}, 96)), browserRevokerFunc(func(context.Context, agent.SessionID) { revocations.Add(1) }))
			_, bearer, call := mintAndExchange(t, manager)
			clock.Set(start.Add(BrowserSessionIdleTTL - time.Nanosecond))
			manager.state.mu.Lock()
			started := make(chan struct{})
			done := make(chan bool, 1)
			go func() {
				close(started)
				if operation == "authenticate" {
					_, ok := manager.Authenticate(bearer)
					done <- ok
				} else {
					manager.Touch(call.SessionID)
					done <- false
				}
			}()
			<-started
			clock.Set(start.Add(BrowserSessionIdleTTL))
			manager.state.mu.Unlock()
			if <-done {
				t.Fatal("exact-TTL operation accepted session")
			}
			if revocations.Load() != 1 {
				t.Fatalf("revocations=%d", revocations.Load())
			}
		})
	}
}

func TestBrowserSessionActivityOnlyOnSuccessfulProtectedHTTP(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		serve func(http.ResponseWriter)
		touch bool
	}{
		{name: "implicit 200", serve: func(http.ResponseWriter) {}, touch: true},
		{name: "write implicit 200", serve: func(w http.ResponseWriter) { _, _ = io.WriteString(w, "ok") }, touch: true},
		{name: "204", serve: func(w http.ResponseWriter) { w.WriteHeader(204) }, touch: true},
		{name: "399", serve: func(w http.ResponseWriter) { w.WriteHeader(399) }, touch: true},
		{name: "informational then implicit 200", serve: func(w http.ResponseWriter) { w.WriteHeader(103) }, touch: true},
		{name: "informational then 204", serve: func(w http.ResponseWriter) { w.WriteHeader(103); w.WriteHeader(204) }, touch: true},
		{name: "first final success", serve: func(w http.ResponseWriter) { w.WriteHeader(204); w.WriteHeader(500) }, touch: true},
		{name: "400", serve: func(w http.ResponseWriter) { w.WriteHeader(400) }},
		{name: "401", serve: func(w http.ResponseWriter) { w.WriteHeader(401) }},
		{name: "403", serve: func(w http.ResponseWriter) { w.WriteHeader(403) }},
		{name: "404", serve: func(w http.ResponseWriter) { w.WriteHeader(404) }},
		{name: "405", serve: func(w http.ResponseWriter) { w.WriteHeader(405) }},
		{name: "413", serve: func(w http.ResponseWriter) { w.WriteHeader(413) }},
		{name: "500", serve: func(w http.ResponseWriter) { w.WriteHeader(500) }},
		{name: "first final failure", serve: func(w http.ResponseWriter) { w.WriteHeader(500); w.WriteHeader(204) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &testClock{now: start}
			m := newBrowserSessionManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{11}, 96)), NoopBrowserSessionRevoker{})
			_, bearer, _ := mintAndExchange(t, m)
			clock.Set(start.Add(14 * time.Minute))
			handler := CreatorOrBrowserGuard(nil, m)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { test.serve(w) }))
			r := httptest.NewRequest(http.MethodGet, "/protected", nil)
			r.Header.Set("Authorization", "Bearer "+bearer.encoded())
			handler.ServeHTTP(httptest.NewRecorder(), r)
			m.state.mu.Lock()
			last := m.state.session.lastActivity
			m.state.mu.Unlock()
			want := start
			if test.touch {
				want = start.Add(14 * time.Minute)
			}
			if !last.Equal(want) {
				t.Fatalf("last activity = %v, want %v", last, want)
			}
		})
	}
}

func TestCreatorOrBrowserGuardInjectsCorrectContext(t *testing.T) {
	creatorRaw := bytes.Repeat([]byte{0}, 32)
	creatorToken, err := ParseCreatorToken(base64.RawURLEncoding.EncodeToString(creatorRaw))
	if err != nil {
		t.Fatal(err)
	}
	creator, err := newCreatorAuthenticator(DeriveCreatorVerifier(creatorToken).Encoded(), bytes.NewReader(bytes.Repeat([]byte{12}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	// The browser bearer deliberately equals the creator token. Both comparisons must run and creator wins.
	entropy := io.MultiReader(bytes.NewReader(bytes.Repeat([]byte{13}, 32)), bytes.NewReader(creatorRaw), bytes.NewReader(bytes.Repeat([]byte{14}, 32)))
	m := newBrowserSessionManager(time.Now, entropy, NoopBrowserSessionRevoker{})
	_, browserBearer, browserCall := mintAndExchange(t, m)
	if browserBearer.encoded() != base64.RawURLEncoding.EncodeToString(creatorRaw) {
		t.Fatal("overlap fixture did not overlap")
	}
	guard := CreatorOrBrowserGuard(creator, m)
	var got agent.AuthenticatedCallContext
	var kind credentialKind
	h := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = authenticatedCreatorContext(r.Context())
		kind = authenticatedCredentialKind(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "/protected?PrincipalID=forged&SessionID=forged", nil)
	r.Header.Set("Authorization", "Bearer "+browserBearer.encoded())
	r.Header.Set("X-Principal-ID", "forged")
	r.Header.Set("X-Session-ID", "forged")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got != creator.context || kind != credentialKindCreator || got == browserCall {
		t.Fatalf("precedence context/kind = %#v/%v", got, kind)
	}

	// Replace with a distinct browser credential and prove the server-owned browser context is used.
	m.entropy = bytes.NewReader(append(bytes.Repeat([]byte{15}, 32), append(bytes.Repeat([]byte{16}, 32), bytes.Repeat([]byte{17}, 32)...)...))
	_, distinctBearer, distinctCall := mintAndExchange(t, m)
	r = httptest.NewRequest(http.MethodGet, "/protected", nil)
	r.Header.Set("Authorization", "Bearer "+distinctBearer.encoded())
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got != distinctCall || kind != credentialKindBrowser || !got.IsAuthenticated() {
		t.Fatalf("browser context/kind = %#v/%v", got, kind)
	}

	for _, credential := range []string{"", "wrong", strings.Repeat("A", 42), base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{99}, 32))} {
		body := &recordingBody{data: strings.NewReader("must not read")}
		r = httptest.NewRequest(http.MethodPost, "/protected", nil)
		r.Body = body
		if credential != "" {
			r.Header.Set("Authorization", "Bearer "+credential)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		assertUnauthorizedResponse(t, w)
		if body.reads != 0 {
			t.Fatalf("invalid auth read body for %q", credential)
		}
	}
	m.Revoke(context.Background())
	r = httptest.NewRequest(http.MethodGet, "/protected", nil)
	r.Header.Set("Authorization", "Bearer "+distinctBearer.encoded())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assertUnauthorizedResponse(t, w)
}

func TestBrowserSuccessfulRequestCannotResurrectReplacedSession(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	clock := &testClock{now: start}
	m := newBrowserSessionManager(clock.Now, repeatedEntropy(struct {
		b byte
		n int
	}{0x31, 96}, struct {
		b byte
		n int
	}{0x32, 96}), NoopBrowserSessionRevoker{})
	_, oldBearer, oldCall := mintAndExchange(t, m)
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := CreatorOrBrowserGuard(nil, m)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	clock.Set(start.Add(time.Minute))
	done := make(chan struct{})
	go func() {
		r := httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.Header.Set("Authorization", "Bearer "+oldBearer.encoded())
		handler.ServeHTTP(httptest.NewRecorder(), r)
		close(done)
	}()
	<-entered
	_, newBearer, newCall := mintAndExchange(t, m)
	if newCall == oldCall {
		t.Fatal("replacement reused context")
	}
	clock.Set(start.Add(2 * time.Minute))
	close(release)
	<-done
	m.state.mu.Lock()
	last := m.state.session.lastActivity
	active := m.state.session.context
	m.state.mu.Unlock()
	if active != newCall || !last.Equal(start.Add(time.Minute)) {
		t.Fatalf("old completion resurrected/touched state: context=%#v last=%v", active, last)
	}
	if _, ok := m.Authenticate(oldBearer); ok {
		t.Fatal("old bearer resurrected")
	}
	if call, ok := m.Authenticate(newBearer); !ok || call != newCall {
		t.Fatalf("replacement lost: %#v/%v", call, ok)
	}
}

func TestBrowserSessionRevocationCallback(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	t.Run("explicit is synchronous idempotent and reentrant", func(t *testing.T) {
		clock := &testClock{now: start}
		entered := make(chan struct{})
		release := make(chan struct{})
		var calls atomic.Int32
		var manager *BrowserSessionManager
		revoker := browserRevokerFunc(func(context.Context, agent.SessionID) {
			calls.Add(1)
			manager.Touch("reentrant-inspection")
			close(entered)
			<-release
		})
		manager = newBrowserSessionManager(clock.Now, bytes.NewReader(bytes.Repeat([]byte{18}, 96)), revoker)
		_, bearer, _ := mintAndExchange(t, manager)
		done := make(chan struct{})
		go func() { manager.Revoke(context.Background()); close(done) }()
		<-entered
		if _, ok := manager.Authenticate(bearer); ok {
			t.Fatal("session remained active during callback")
		}
		select {
		case <-done:
			t.Fatal("revoke returned before callback")
		default:
		}
		close(release)
		<-done
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); manager.Revoke(context.Background()) }()
		}
		wg.Wait()
		if calls.Load() != 1 {
			t.Fatalf("callback calls = %d", calls.Load())
		}
	})
	t.Run("replacement and lazy expiry once", func(t *testing.T) {
		clock := &testClock{now: start}
		var mu sync.Mutex
		var ids []agent.SessionID
		revoker := browserRevokerFunc(func(_ context.Context, id agent.SessionID) { mu.Lock(); ids = append(ids, id); mu.Unlock() })
		manager := newBrowserSessionManager(clock.Now, repeatedEntropy(struct {
			b byte
			n int
		}{19, 96}, struct {
			b byte
			n int
		}{20, 96}), revoker)
		_, firstBearer, firstCall := mintAndExchange(t, manager)
		_, secondBearer, secondCall := mintAndExchange(t, manager)
		if _, ok := manager.Authenticate(firstBearer); ok {
			t.Fatal("replacement retained old bearer")
		}
		clock.Set(start.Add(BrowserSessionIdleTTL))
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); manager.Authenticate(secondBearer) }()
		}
		wg.Wait()
		mu.Lock()
		defer mu.Unlock()
		if len(ids) != 2 || ids[0] != firstCall.SessionID || ids[1] != secondCall.SessionID {
			t.Fatalf("callbacks = %#v", ids)
		}
	})
}

func TestBrowserSessionManagerRevokeAll(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	clock := &testClock{now: start}
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	var manager *BrowserSessionManager
	revoker := browserRevokerFunc(func(context.Context, agent.SessionID) {
		calls.Add(1)
		manager.state.mu.Lock()
		cleared := manager.state.ticket == nil && manager.state.session == nil
		manager.state.mu.Unlock()
		if !cleared {
			t.Error("browser state was visible to revoker")
		}
		manager.Touch("callback-reentry")
		close(entered)
		<-release
	})
	manager = newBrowserSessionManager(clock.Now, repeatedEntropy(struct {
		b byte
		n int
	}{0x51, 96}, struct {
		b byte
		n int
	}{0x52, 32}), revoker)
	_, bearer, _ := mintAndExchange(t, manager)
	if _, _, err := manager.Mint(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { manager.RevokeAll(context.Background()); close(done) }()
	<-entered
	if _, ok := manager.Authenticate(bearer); ok {
		t.Fatal("active bearer remained valid while revoker blocked")
	}
	manager.state.mu.Lock()
	cleared := manager.state.ticket == nil && manager.state.session == nil
	manager.state.mu.Unlock()
	if !cleared {
		t.Fatal("ticket and session were not atomically cleared")
	}
	select {
	case <-done:
		t.Fatal("RevokeAll returned before synchronous revoker")
	default:
	}
	close(release)
	<-done

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); manager.RevokeAll(context.Background()) }()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("revoker calls = %d", calls.Load())
	}
}

func TestBrowserManagerAndDTOFormattingAlwaysRedacts(t *testing.T) {
	m := newBrowserSessionManager(time.Now, bytes.NewReader(bytes.Repeat([]byte{20}, 96)), NoopBrowserSessionRevoker{})
	ticket, bearer, _ := mintAndExchange(t, m)
	values := []any{
		m, *m, m.state.session.verifier,
		browserTicketResponse{Ticket: ticket.encoded(), ExpiresAt: "secret-time"},
		browserExchangeRequest{Ticket: ticket.encoded()},
		browserBearerResponse{Bearer: bearer.encoded(), ExpiresAt: "secret-time"},
	}
	for _, value := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
			if got := fmt.Sprintf(verb, value); got != "<redacted>" {
				t.Fatalf("%T %s = %q", value, verb, got)
			}
		}
	}
}

func TestBrowserSessionStateFormattingAlwaysRedacts(t *testing.T) {
	ticketRaw := [32]byte([]byte("TICKET-RAW-0123456789-ABCDEFGHIJ"))
	bearerRaw := [32]byte([]byte("BEARER-RAW-0123456789-ABCDEFGHIJ"))
	ticketVerifier := sha256.Sum256(append([]byte(browserTicketDomain), ticketRaw[:]...))
	bearerVerifier := sha256.Sum256(append([]byte(browserBearerDomain), bearerRaw[:]...))
	if got := deriveBrowserVerifier(browserTicketDomain, ticketRaw).value; got != ticketVerifier {
		t.Fatalf("ticket verifier was not independently reproduced: %x != %x", got, ticketVerifier)
	}
	if got := deriveBrowserVerifier(browserBearerDomain, bearerRaw).value; got != bearerVerifier {
		t.Fatalf("bearer verifier was not independently reproduced: %x != %x", got, bearerVerifier)
	}
	const (
		wantTicketVerifierHex = "7ba037739d9d875da41870db74851f590fb8b59dfb3d0abae9fd0f69601100cf"
		wantBearerVerifierHex = "35e6da2135be2409056743c7a69426b2141c4ad377496e2ef79ca70e466c8c46"
	)
	if got := hex.EncodeToString(ticketVerifier[:]); got != wantTicketVerifierHex {
		t.Fatalf("ticket SHA-256 verifier = %s", got)
	}
	if got := hex.EncodeToString(bearerVerifier[:]); got != wantBearerVerifierHex {
		t.Fatalf("bearer SHA-256 verifier = %s", got)
	}
	if ticketRaw == bearerRaw || ticketVerifier == bearerVerifier {
		t.Fatal("credential and domain-separated verifier fixtures must differ")
	}

	ticketState := browserTicketState{
		verifier:  browserVerifier{value: ticketVerifier},
		expiresAt: time.Date(2026, 8, 1, 5, 7, 7, 123, time.UTC),
	}
	sessionState := browserSessionState{
		verifier: browserVerifier{value: bearerVerifier},
		context: agent.AuthenticatedCallContext{
			PrincipalID: "browser-redaction-principal",
			SessionID:   "browser-http-redaction-session",
		},
		createdAt:    time.Date(2026, 8, 1, 5, 6, 7, 123, time.UTC),
		lastActivity: time.Date(2026, 8, 1, 5, 6, 8, 123, time.UTC),
	}
	values := []struct {
		name  string
		value any
	}{
		{name: "ticket value", value: ticketState},
		{name: "ticket pointer", value: &ticketState},
		{name: "session value", value: sessionState},
		{name: "session pointer", value: &sessionState},
		{name: "store value", value: browserSessionStore{ticket: &ticketState, session: &sessionState}},
		{name: "store pointer", value: &browserSessionStore{ticket: &ticketState, session: &sessionState}},
	}
	formats := []string{
		"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%o", "%O", "%b", "%c", "%U",
		"%e", "%E", "%f", "%F", "%g", "%G", "%t", "% 20.3v", "%+#20.3v", "%-#30.9x", "%020q",
	}
	var formatted strings.Builder
	for _, tc := range values {
		if _, ok := tc.value.(fmt.Formatter); !ok {
			t.Fatalf("%s (%T) does not implement fmt.Formatter", tc.name, tc.value)
		}
		for _, format := range formats {
			got := fmt.Sprintf(format, tc.value)
			formatted.WriteString(got)
			formatted.WriteByte('\n')
			if got != "<redacted>" {
				t.Fatalf("%s (%T) format %q = %q", tc.name, tc.value, format, got)
			}
		}
	}

	base64Representations := func(value []byte) []string {
		return []string{
			base64.StdEncoding.EncodeToString(value),
			base64.RawStdEncoding.EncodeToString(value),
			base64.URLEncoding.EncodeToString(value),
			base64.RawURLEncoding.EncodeToString(value),
		}
	}
	ticketVerifierRepresentations := []string{
		string(ticketVerifier[:]),
		fmt.Sprintf("%v", ticketVerifier),
		fmt.Sprintf("%#v", ticketVerifier),
		fmt.Sprintf("%#v", ticketVerifier[:]),
		hex.EncodeToString(ticketVerifier[:]),
		strings.ToUpper(hex.EncodeToString(ticketVerifier[:])),
	}
	bearerVerifierRepresentations := []string{
		string(bearerVerifier[:]),
		fmt.Sprintf("%v", bearerVerifier),
		fmt.Sprintf("%#v", bearerVerifier),
		fmt.Sprintf("%#v", bearerVerifier[:]),
		hex.EncodeToString(bearerVerifier[:]),
		strings.ToUpper(hex.EncodeToString(bearerVerifier[:])),
	}
	ticketVerifierRepresentations = append(ticketVerifierRepresentations, base64Representations(ticketVerifier[:])...)
	bearerVerifierRepresentations = append(bearerVerifierRepresentations, base64Representations(bearerVerifier[:])...)
	credentialRepresentations := append([]string{string(ticketRaw[:]), hex.EncodeToString(ticketRaw[:]), strings.ToUpper(hex.EncodeToString(ticketRaw[:]))}, base64Representations(ticketRaw[:])...)
	credentialRepresentations = append(credentialRepresentations, string(bearerRaw[:]), hex.EncodeToString(bearerRaw[:]), strings.ToUpper(hex.EncodeToString(bearerRaw[:])))
	credentialRepresentations = append(credentialRepresentations, base64Representations(bearerRaw[:])...)
	for i := range ticketVerifierRepresentations {
		if ticketVerifierRepresentations[i] == "" || bearerVerifierRepresentations[i] == "" {
			t.Fatalf("verifier representation %d is vacuous", i)
		}
		if ticketVerifierRepresentations[i] == bearerVerifierRepresentations[i] {
			t.Fatalf("ticket and bearer verifier representation %d did not differ: %q", i, ticketVerifierRepresentations[i])
		}
	}
	needles := append(credentialRepresentations, ticketVerifierRepresentations...)
	needles = append(needles, bearerVerifierRepresentations...)
	formattedCorpus := formatted.String()
	for _, needle := range needles {
		if needle == "" || needle == "<redacted>" {
			t.Fatalf("credential leak needle is vacuous: %q", needle)
		}
		if strings.Contains(formattedCorpus, needle) {
			t.Fatalf("formatted browser state exposed credential/verifier representation %q in %q", needle, formattedCorpus)
		}
	}
}
