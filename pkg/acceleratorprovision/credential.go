package acceleratorprovision

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"kubikles/pkg/server"
)

var errEntropy = errors.New("accelerator credential entropy unavailable")

type creatorCredential struct {
	encoded   [43]byte
	verifier  string
	session   string
	gate      chan struct{}
	mu        sync.Mutex
	closing   bool
	destroyed bool
	active    int
	changed   chan struct{}
}

func (c creatorCredential) String() string             { return "<redacted>" }
func (c creatorCredential) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "<redacted>") }
func generateCreatorCredential(entropy io.Reader) (*creatorCredential, error) {
	var tokenBytes [32]byte
	if _, err := io.ReadFull(entropy, tokenBytes[:]); err != nil {
		return nil, errEntropy
	}
	token, err := server.ParseCreatorToken(base64.RawURLEncoding.EncodeToString(tokenBytes[:]))
	if err != nil {
		return nil, errEntropy
	}
	var sessionBytes [16]byte
	if _, err = io.ReadFull(entropy, sessionBytes[:]); err != nil {
		return nil, errEntropy
	}
	encoded := base64.RawURLEncoding.EncodeToString(tokenBytes[:])
	var fixed [43]byte
	copy(fixed[:], encoded)
	credential := &creatorCredential{encoded: fixed, verifier: server.DeriveCreatorVerifier(token).Encoded(), session: hex.EncodeToString(sessionBytes[:]), gate: make(chan struct{}, 1), changed: make(chan struct{})}
	credential.gate <- struct{}{}
	return credential, nil
}
func (c *creatorCredential) releaseName() string {
	if c == nil {
		return ""
	}
	return "kubikles-accelerator-" + c.session
}

// withCreatorAuthorization is the sole narrow bearer boundary.  The callback
// gets an unexported lease and cannot obtain a token or header value.
func (c *creatorCredential) withCreatorAuthorization(ctx context.Context, fn func(context.Context, creatorAuthorizationLease) error) error {
	if c == nil || ctx == nil || fn == nil {
		return errors.New("credential unavailable")
	}
	if c.gate == nil {
		return errors.New("credential unavailable")
	}
	c.mu.Lock()
	unavailable := c.closing || c.destroyed
	c.mu.Unlock()
	if unavailable {
		return errors.New("credential unavailable")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.gate:
	}
	defer func() { c.gate <- struct{}{} }()
	c.mu.Lock()
	if c.closing || c.destroyed {
		c.mu.Unlock()
		return errors.New("credential unavailable")
	}
	c.active++
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.active--
		c.signalChangedLocked()
		c.mu.Unlock()
	}()
	var raw [43]byte
	copy(raw[:], c.encoded[:])
	state := &creatorAuthorizationState{raw: raw}
	state.active.Store(true)
	err := fn(ctx, creatorAuthorizationLease{state: state})
	state.active.Store(false)
	for i := range state.raw {
		state.raw[i] = 0
	}
	for i := range raw {
		raw[i] = 0
	}
	return err
}

func (c *creatorCredential) signalChangedLocked() {
	if c.changed == nil {
		c.changed = make(chan struct{})
		return
	}
	close(c.changed)
	c.changed = make(chan struct{})
}

// closeAndDestroy irreversibly refuses new leases, waits for already-issued
// authorization scratch to be released, and best-effort clears owned material.
func (c *creatorCredential) closeAndDestroy(ctx context.Context) bool {
	if c == nil {
		return true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	if c.destroyed {
		c.mu.Unlock()
		return true
	}
	c.closing = true
	c.signalChangedLocked()
	for c.active != 0 {
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			c.mu.Lock()
			clear(c.encoded[:])
			c.verifier = ""
			c.session = ""
			c.destroyed = true
			c.signalChangedLocked()
			c.mu.Unlock()
			return false
		}
		c.mu.Lock()
	}
	clear(c.encoded[:])
	c.verifier = ""
	c.session = ""
	c.destroyed = true
	c.signalChangedLocked()
	c.mu.Unlock()
	return true
}

type creatorAuthorizationState struct {
	raw    [43]byte
	active atomic.Bool
}

type creatorAuthorizationLease struct{ state *creatorAuthorizationState }

func (l creatorAuthorizationLease) String() string { return "<redacted>" }
func (l creatorAuthorizationLease) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "<redacted>")
}

func (l creatorAuthorizationLease) apply(request *http.Request) error {
	if l.state == nil || !l.state.active.Load() {
		return errors.New("credential unavailable")
	}
	if request == nil {
		return errors.New("credential unavailable")
	}
	request.Header["Authorization"] = []string{"Bearer " + string(l.state.raw[:])}
	return nil
}

func (l creatorAuthorizationLease) DoHTTP(client *http.Client, request *http.Request) (*http.Response, error) {
	if client == nil || request == nil {
		return nil, errors.New("credential unavailable")
	}
	if err := l.apply(request); err != nil {
		return nil, err
	}
	defer request.Header.Del("Authorization")
	return client.Do(request)
}

func (l creatorAuthorizationLease) DialCreatorWebSocket(ctx context.Context, dialer *websocket.Dialer, target string) (*websocket.Conn, *http.Response, error) {
	if ctx == nil || dialer == nil {
		return nil, nil, errors.New("credential unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := l.apply(request); err != nil {
		return nil, nil, err
	}
	header := request.Header
	defer header.Del("Authorization")
	return dialer.DialContext(ctx, target, header)
}
