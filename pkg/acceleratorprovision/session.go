package acceleratorprovision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

var errSessionCloseTimeout = errors.New("accelerator session close timeout")

type sessionSocket interface {
	SetWriteDeadline(time.Time) error
	WriteControl(int, []byte, time.Time) error
	SetReadLimit(int64)
	ReadMessage() (int, []byte, error)
	Close() error
}

type ConnectedSession struct {
	identity     SessionIdentity
	capabilities []agent.Capability
	socket       sessionSocket
	tunnel       tunnel
	done         chan struct{}
	readerDone   chan struct{}
	terminalOnce sync.Once
	mu           sync.RWMutex
	reason       SessionEndReason
	frames       chan []byte
}

func newConnectedSession(receipt *workloadReceipt, info server.AuthenticatedAcceleratorInfo, connected connectedIdentity, socket sessionSocket, activeTunnel tunnel) *ConnectedSession {
	session := &ConnectedSession{
		identity: SessionIdentity{
			ContextName: receipt.contextName, ReleaseNamespace: receipt.releaseNamespace, ReleaseName: receipt.releaseName,
			WorkloadSessionID: receipt.workloadSessionID, Job: receipt.job, Pod: receipt.pod,
			BuildVersion: receipt.buildVersion, ImageDigest: receipt.imageDigest, ChartDigest: receipt.chartDigest,
			InstanceID: connected.instanceID, SessionID: connected.sessionID, Generation: connected.generation,
		},
		capabilities: append([]agent.Capability(nil), info.Capabilities...),
		socket:       socket, tunnel: activeTunnel, done: make(chan struct{}), readerDone: make(chan struct{}), frames: make(chan []byte, 64),
	}
	go session.pump()
	go func() {
		select {
		case <-activeTunnel.Done():
			session.terminate(SessionTunnelClosed, false)
		case <-session.done:
		}
	}()
	return session
}

func (s *ConnectedSession) String() string { return "<accelerator connected session>" }
func (s *ConnectedSession) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator connected session>")
}

func (s *ConnectedSession) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}
	type safe struct {
		Identity     SessionIdentity    `json:"identity"`
		Capabilities []agent.Capability `json:"capabilities"`
		EndReason    SessionEndReason   `json:"endReason,omitempty"`
	}
	return json.Marshal(safe{Identity: s.Identity(), Capabilities: s.Capabilities(), EndReason: s.EndReason()})
}

func (s *ConnectedSession) Identity() SessionIdentity {
	if s == nil {
		return SessionIdentity{}
	}
	return s.identity
}

func (s *ConnectedSession) Capabilities() []agent.Capability {
	if s == nil {
		return nil
	}
	return append([]agent.Capability(nil), s.capabilities...)
}

func (s *ConnectedSession) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.done
}

func (s *ConnectedSession) EndReason() SessionEndReason {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reason
}

func (s *ConnectedSession) terminate(reason SessionEndReason, normal bool) {
	if s == nil {
		return
	}
	s.terminalOnce.Do(func() {
		s.mu.Lock()
		s.reason = reason
		s.mu.Unlock()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		if s.socket != nil {
			if normal {
				deadline := time.Now().Add(closeTimeout)
				if contextDeadline, ok := cleanupCtx.Deadline(); ok && contextDeadline.Before(deadline) {
					deadline = contextDeadline
				}
				_ = s.socket.SetWriteDeadline(deadline)
				_ = s.socket.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), deadline)
			}
			_ = s.socket.Close()
		}
		select {
		case <-s.readerDone:
		case <-cleanupCtx.Done():
		}
		if s.tunnel != nil {
			s.tunnel.Stop()
			_ = s.tunnel.Wait(cleanupCtx)
		}
		close(s.done)
	})
}

func (s *ConnectedSession) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	go s.terminate(SessionClosed, true)
	timer := time.NewTimer(closeTimeout)
	defer timer.Stop()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return errSessionCloseTimeout
	case <-timer.C:
		return errSessionCloseTimeout
	}
}

func (s *ConnectedSession) pump() {
	defer close(s.readerDone)
	s.socket.SetReadLimit(server.AcceleratorSocketReadLimit)
	for {
		messageType, payload, err := s.socket.ReadMessage()
		if err != nil {
			go s.terminate(SessionPeerClosed, false)
			return
		}
		if messageType != websocket.TextMessage || len(payload) > int(server.AcceleratorSocketReadLimit) || !json.Valid(payload) {
			go s.terminate(SessionProtocolFailed, false)
			return
		}
		select {
		case s.frames <- append([]byte(nil), payload...):
		default:
			go s.terminate(SessionProtocolFailed, false)
			return
		}
	}
}
