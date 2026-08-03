package server

import (
	"github.com/gorilla/websocket"
	"kubikles/pkg/agent"
	"net/http"
	"strings"
)

const AcceleratorWebSocketProtocol = "kubikles-accelerator-v1"
const AcceleratorBrowserCredentialProtocolPrefix = "kubikles-accelerator-browser-bearer."

type AcceleratorWebSocketAuthenticator struct {
	Creator         *CreatorAuthenticator
	BrowserSessions *BrowserSessionManager
	Registry        *AcceleratorSessionRegistry
	afterUpgrade    func()
	afterPrepare    func()
	beforeActivate  func(*acceleratorRegistration)
}

type acceleratorWebSocketIdentity struct {
	call    agent.AuthenticatedCallContext
	browser bool
}

func (a AcceleratorWebSocketAuthenticator) authenticate(r *http.Request) (acceleratorWebSocketIdentity, bool) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery || len(r.Header.Values("Cookie")) != 0 {
		return acceleratorWebSocketIdentity{}, false
	}
	protocolHeaders := r.Header.Values("Sec-WebSocket-Protocol")
	auth := r.Header.Values("Authorization")
	if len(auth) == 1 && len(protocolHeaders) == 0 && a.Creator != nil {
		v, ok := strictBearer(r)
		if ok && a.Creator.matchesBearer(v) {
			return acceleratorWebSocketIdentity{call: a.Creator.context}, true
		}
		return acceleratorWebSocketIdentity{}, false
	}
	if len(auth) != 0 || len(protocolHeaders) != 1 || a.BrowserSessions == nil {
		return acceleratorWebSocketIdentity{}, false
	}
	// A valid browser offer is one HTTP header containing exactly two ordered
	// protocol tokens. Gorilla parses the normal comma-separated handshake; raw
	// cardinality prevents alternate/multiple-header ambiguity.
	parts := strings.Split(protocolHeaders[0], ",")
	if len(parts) != 2 {
		return acceleratorWebSocketIdentity{}, false
	}
	protocols := websocket.Subprotocols(r)
	if len(protocols) != 2 || protocols[0] != strings.TrimSpace(parts[0]) || protocols[1] != strings.TrimSpace(parts[1]) || protocols[0] != AcceleratorWebSocketProtocol || !strings.HasPrefix(protocols[1], AcceleratorBrowserCredentialProtocolPrefix) {
		return acceleratorWebSocketIdentity{}, false
	}
	bearer, err := ParseBrowserBearer(strings.TrimPrefix(protocols[1], AcceleratorBrowserCredentialProtocolPrefix))
	if err != nil {
		return acceleratorWebSocketIdentity{}, false
	}
	call, ok := a.BrowserSessions.Authenticate(bearer)
	return acceleratorWebSocketIdentity{call: call, browser: true}, ok
}
func (a AcceleratorWebSocketAuthenticator) Handler(w http.ResponseWriter, r *http.Request) {
	if a.Registry == nil {
		http.NotFound(w, r)
		return
	}
	release, ok := a.Registry.beginUpgrade()
	if !ok {
		writeAcceleratorError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	a.handleAdmitted(w, r, release)
}

func (a AcceleratorWebSocketAuthenticator) handleAdmitted(w http.ResponseWriter, r *http.Request, release func()) {
	if r.Method != http.MethodGet || a.Registry == nil {
		release()
		http.NotFound(w, r)
		return
	}
	identity, ok := a.authenticate(r)
	if !ok {
		release()
		writeAcceleratorError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }, Subprotocols: []string{AcceleratorWebSocketProtocol}}
	if !identity.browser {
		up.Subprotocols = nil
	}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		release()
		return
	}
	if a.afterUpgrade != nil {
		a.afterUpgrade()
	}
	var registration *acceleratorRegistration
	prepare := func() bool {
		var prepared bool
		if identity.browser {
			registration, prepared = a.Registry.prepareRegistrationWithHook(identity.call, conn, func(generation AcceleratorSocketGeneration) {
				a.BrowserSessions.browserSocketPreparedLocked(identity.call.SessionID, generation)
			})
		} else {
			registration, prepared = a.Registry.prepareRegistration(identity.call, conn)
		}
		return prepared
	}
	if identity.browser {
		// The upgrade deliberately precedes the atomic session recheck. If revoke,
		// replacement, or expiry wins this post-101 race, registration never
		// publishes and the peer receives no connected frame.
		active := a.BrowserSessions.withActiveBrowserSession(identity.call.SessionID, prepare)
		release()
		if !active {
			_ = conn.Close()
			return
		}
		activated := false
		defer func() {
			if !activated {
				a.BrowserSessions.browserSocketActivationFailed(identity.call.SessionID, registration.socket.snapshot.Generation)
			}
		}()
		a.Registry.observeReplacement(registration)
		if a.afterPrepare != nil {
			a.afterPrepare()
		}
		activationBegan := a.BrowserSessions.withActiveBrowserSession(identity.call.SessionID, func() bool {
			if a.beforeActivate != nil {
				a.beforeActivate(registration)
			}
			return a.Registry.beginRegistrationActivation(registration)
		})
		if !activationBegan {
			_ = conn.Close()
			return
		}
		socket, activated := a.Registry.finishRegistrationActivation(registration)
		if !activated {
			_ = conn.Close()
			return
		}
		a.BrowserSessions.browserSocketActivated(identity.call.SessionID, socket.snapshot.Generation)
		activated = true
		go func() {
			<-socket.pumpsDone
			a.BrowserSessions.browserSocketEnded(identity.call.SessionID, socket.snapshot.Generation)
		}()
		return
	}
	prepared := prepare()
	release()
	if !prepared {
		_ = conn.Close()
		return
	}
	_, _ = a.Registry.completeRegistration(registration)
}
