package server

import (
	"github.com/gorilla/websocket"
	"kubikles/pkg/agent"
	"net/http"
)

type AcceleratorWebSocketAuthenticator struct {
	Creator        *CreatorAuthenticator
	Registry       *AcceleratorSessionRegistry
	afterUpgrade   func()
	afterPrepare   func()
	beforeActivate func(*acceleratorRegistration)
}

type acceleratorWebSocketIdentity struct {
	call agent.AuthenticatedCallContext
}

func (a AcceleratorWebSocketAuthenticator) authenticate(r *http.Request) (acceleratorWebSocketIdentity, bool) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery || len(r.Header.Values("Cookie")) != 0 {
		return acceleratorWebSocketIdentity{}, false
	}
	auth := r.Header.Values("Authorization")
	if len(auth) == 1 && len(r.Header.Values("Sec-WebSocket-Protocol")) == 0 && a.Creator != nil {
		v, ok := strictBearer(r)
		if ok && a.Creator.matchesBearer(v) {
			return acceleratorWebSocketIdentity{call: a.Creator.context}, true
		}
		return acceleratorWebSocketIdentity{}, false
	}
	return acceleratorWebSocketIdentity{}, false
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
	a.handleAdmitted(w, r, release, nil)
}

func (a AcceleratorWebSocketAuthenticator) handleAdmitted(w http.ResponseWriter, r *http.Request, release func(), dispatcher *AcceleratorRPCDispatcher) {
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
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		release()
		return
	}
	if a.afterUpgrade != nil {
		a.afterUpgrade()
	}
	var registration *acceleratorRegistration
	registration, prepared := a.Registry.prepareRPCRegistration(identity.call, conn, dispatcher)
	release()
	if !prepared {
		_ = conn.Close()
		return
	}
	_, _ = a.Registry.completeRegistration(registration)
}
