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
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

var errSessionCloseTimeout = errors.New("accelerator session close timeout")

type sessionSocket interface {
	SetWriteDeadline(time.Time) error
	WriteMessage(int, []byte) error
	WriteControl(int, []byte, time.Time) error
	SetPongHandler(func(string) error)
	SetReadLimit(int64)
	ReadMessage() (int, []byte, error)
	Close() error
}

type ConnectedSession struct {
	self            *ConnectedSession
	identity        SessionIdentity
	capabilities    []agent.Capability
	versionMismatch bool
	receipt         *workloadReceipt
	clock           resumeClock
	socket          sessionSocket
	tunnel          tunnel
	done            chan struct{}
	terminalStart   chan struct{}
	readerDone      chan struct{}
	writerDone      chan struct{}
	terminalOnce    sync.Once
	mu              sync.RWMutex
	reason          SessionEndReason
	disconnect      *disconnectRecord
	resumeClaimed   bool
	closeRequested  bool
	resumeStop      chan struct{}
	arbiter         *sessionFrameArbiter
	outbound        chan []byte
	candidateNonce  string
	candidatePong   <-chan struct{}
	ownerState      *workloadConnectorState
	idleToken       *coordinatorIdleToken
}

type disconnectRecord struct {
	at            time.Time
	reason        SessionEndReason
	workloadNonce *workloadReceipt
	instanceID    string
	sessionID     string
	generation    int
	idleToken     *coordinatorIdleToken
}

// coordinatorIdleToken is deliberately unexported and identity-based. It
// authorizes only the coordinator-owned return from an idle release.
type coordinatorIdleToken struct {
	deadline time.Time
}

type coordinatorIdleRelease struct {
	prior    *ConnectedSession
	token    *coordinatorIdleToken
	deadline time.Time
}

func newConnectedSession(receipt *workloadReceipt, info server.AuthenticatedAcceleratorInfo, connected connectedIdentity, socket sessionSocket, activeTunnel tunnel, clock resumeClock) *ConnectedSession {
	return newConnectedSessionWithCandidateFence(receipt, info, connected, socket, activeTunnel, clock, "", nil)
}

func newConnectedSessionWithCandidateFence(receipt *workloadReceipt, info server.AuthenticatedAcceleratorInfo, connected connectedIdentity, socket sessionSocket, activeTunnel tunnel, clock resumeClock, candidateNonce string, candidatePong <-chan struct{}) *ConnectedSession {
	if clock == nil {
		clock = processResumeClock{}
	}
	session := &ConnectedSession{
		identity: SessionIdentity{
			ContextName: receipt.contextName, ReleaseNamespace: receipt.releaseNamespace, ReleaseName: receipt.releaseName,
			WorkloadSessionID: receipt.workloadSessionID, Job: receipt.job, Pod: receipt.pod,
			BuildVersion: receipt.buildVersion, ImageDigest: receipt.imageDigest, ChartDigest: receipt.chartDigest,
			InstanceID: connected.instanceID, SessionID: connected.sessionID, Generation: connected.generation,
		},
		capabilities:    append([]agent.Capability(nil), info.Capabilities...),
		versionMismatch: info.Build.BuildVersion != receipt.buildVersion,
		receipt:         receipt,
		clock:           clock,
		socket:          socket, tunnel: activeTunnel, done: make(chan struct{}), terminalStart: make(chan struct{}), readerDone: make(chan struct{}), writerDone: make(chan struct{}), outbound: make(chan []byte, acceleratorsecret.OutboundSocketSlots),
		candidateNonce: candidateNonce, candidatePong: candidatePong,
	}
	session.self = session
	session.arbiter = newSessionFrameArbiter(session)
	if receipt != nil && receipt.owner != nil {
		session.ownerState = receipt.owner.connectorState
	}
	go session.pump()
	go session.writePump()
	go func() {
		select {
		case <-activeTunnel.Done():
			session.beginTermination(SessionTunnelClosed, false)
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

func (s *ConnectedSession) terminalStarted() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.terminalStart
}

func (s *ConnectedSession) transportReconnectDeadline() (time.Time, bool) {
	if s == nil {
		return time.Time{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.disconnect == nil || (s.disconnect.reason != SessionPeerClosed && s.disconnect.reason != SessionTunnelClosed) {
		return time.Time{}, false
	}
	return s.disconnect.at.Add(agent.AcceleratorIdleReconnectGrace), true
}

func (s *ConnectedSession) EndReason() SessionEndReason {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reason
}

func (s *ConnectedSession) beginTermination(reason SessionEndReason, normal bool) {
	s.beginTerminationWithIdle(reason, normal, nil)
}

func (s *ConnectedSession) beginTerminationWithIdle(reason SessionEndReason, normal bool, idle *coordinatorIdleToken) {
	if s == nil {
		return
	}
	s.terminalOnce.Do(func() {
		disconnectedAt := s.clock.Now()
		s.mu.Lock()
		s.reason = reason
		if idle != nil {
			idle.deadline = disconnectedAt.Add(agent.AcceleratorIdleReconnectGrace)
			s.idleToken = idle
		}
		s.disconnect = &disconnectRecord{at: disconnectedAt, reason: reason, workloadNonce: s.receipt, instanceID: s.identity.InstanceID, sessionID: s.identity.SessionID, generation: s.identity.Generation, idleToken: idle}
		s.mu.Unlock()
		close(s.terminalStart)
		go s.cleanup(normal)
	})
}

func (s *ConnectedSession) releaseForIdle(ctx context.Context) *coordinatorIdleRelease {
	if s == nil || s.self != s {
		return nil
	}
	token := &coordinatorIdleToken{}
	s.beginTerminationWithIdle(SessionIdleReleased, true, token)
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-s.done:
	case <-ctx.Done():
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.disconnect == nil || s.disconnect.reason != SessionIdleReleased || s.disconnect.idleToken != token || s.idleToken != token || token.deadline.IsZero() {
		return nil
	}
	return &coordinatorIdleRelease{prior: s, token: token, deadline: token.deadline}
}

func (s *ConnectedSession) cleanup(normal bool) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	if s.socket != nil {
		if normal {
			deadline := time.Now().Add(closeTimeout)
			if contextDeadline, ok := cleanupCtx.Deadline(); ok && contextDeadline.Before(deadline) {
				deadline = contextDeadline
			}
			_ = s.socket.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), deadline)
		}
		_ = s.socket.Close()
	}
	select {
	case <-s.readerDone:
	case <-cleanupCtx.Done():
	}
	select {
	case <-s.writerDone:
	case <-cleanupCtx.Done():
	}
	if s.tunnel != nil {
		s.tunnel.Stop()
		_ = s.tunnel.Wait(cleanupCtx)
	}
	if s.ownerState != nil {
		s.ownerState.mu.Lock()
		if s.ownerState.currentSession == s {
			s.ownerState.currentSession = nil
		}
		s.ownerState.signalChangedLocked()
		s.ownerState.mu.Unlock()
	}
	close(s.done)
}

func (s *ConnectedSession) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.closeRequested = true
	if s.resumeStop != nil {
		select {
		case <-s.resumeStop:
		default:
			close(s.resumeStop)
		}
	}
	s.mu.Unlock()
	s.beginTermination(SessionClosed, true)
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

func (s *ConnectedSession) claimResume(workload *ProvisionedWorkload) (disconnectRecord, <-chan struct{}, ResumeReason) {
	return s.claimResumeWithIdle(workload, nil)
}

func (s *ConnectedSession) claimResumeWithIdle(workload *ProvisionedWorkload, idle *coordinatorIdleToken) (disconnectRecord, <-chan struct{}, ResumeReason) {
	if s == nil || s.self != s || workload == nil || s.receipt == nil || !workload.matchesResumeHandle(s.receipt) {
		return disconnectRecord{}, nil, ResumeInvalid
	}
	select {
	case <-s.done:
	default:
		return disconnectRecord{}, nil, ResumeSessionIneligible
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resumeClaimed {
		return disconnectRecord{}, nil, ResumeSuperseded
	}
	if s.disconnect == nil || s.disconnect.workloadNonce != s.receipt || s.disconnect.instanceID != s.identity.InstanceID || s.disconnect.sessionID != s.identity.SessionID || s.disconnect.generation != s.identity.Generation || s.closeRequested {
		return disconnectRecord{}, nil, ResumeSessionIneligible
	}
	eligibleTransport := idle == nil && (s.disconnect.reason == SessionPeerClosed || s.disconnect.reason == SessionTunnelClosed)
	eligibleIdle := idle != nil && s.disconnect.reason == SessionIdleReleased && s.disconnect.idleToken == idle && s.idleToken == idle
	if !eligibleTransport && !eligibleIdle {
		return disconnectRecord{}, nil, ResumeSessionIneligible
	}
	s.resumeClaimed = true
	s.resumeStop = make(chan struct{})
	return *s.disconnect, s.resumeStop, ""
}

func (s *ConnectedSession) pump() {
	defer close(s.readerDone)
	defer s.arbiter.close()
	s.socket.SetReadLimit(int64(acceleratorsecret.MaxCreatorResponseFrameBytes))
	for {
		messageType, payload, err := s.socket.ReadMessage()
		if err != nil {
			s.beginTermination(SessionPeerClosed, false)
			return
		}
		if messageType != websocket.TextMessage || len(payload) > acceleratorsecret.MaxCreatorResponseFrameBytes {
			s.beginTermination(SessionProtocolFailed, false)
			return
		}
		frame, err := acceleratorsecret.DecodeServerFrame(payload)
		clear(payload)
		if err != nil {
			s.beginTermination(SessionProtocolFailed, false)
			return
		}
		if frame.Event != nil {
			switch frame.Event.Name {
			case acceleratorsecret.EventResource, acceleratorsecret.EventWatcherStatus, acceleratorsecret.EventWatcherError:
			default:
				clearServerFrame(&frame)
				s.beginTermination(SessionProtocolFailed, false)
				return
			}
		}
		if !s.arbiter.route(frame) {
			s.beginTermination(SessionProtocolFailed, false)
			return
		}
	}
}

func (s *ConnectedSession) writePump() {
	defer close(s.writerDone)
	defer s.clearOutbound()
	for {
		select {
		case <-s.terminalStart:
			return
		default:
		}
		select {
		case <-s.terminalStart:
			return
		case payload := <-s.outbound:
			deadline := time.Now().Add(server.AcceleratorSocketWriteTimeout)
			if s.socket.SetWriteDeadline(deadline) != nil || s.socket.WriteMessage(websocket.TextMessage, payload) != nil {
				clear(payload)
				s.beginTermination(SessionPeerClosed, false)
				return
			}
			clear(payload)
		}
	}
}

func (s *ConnectedSession) clearOutbound() {
	for {
		select {
		case payload := <-s.outbound:
			clear(payload)
		default:
			return
		}
	}
}

func (s *ConnectedSession) sendApplicationFrame(payload []byte) acceleratorsecret.SecretClientReason {
	if s == nil || len(payload) == 0 || len(payload) > acceleratorsecret.MaxCreatorRequestFrameBytes || !json.Valid(payload) {
		return acceleratorsecret.ReasonProtocol
	}
	select {
	case <-s.terminalStart:
		return acceleratorsecret.ReasonSessionUnavailable
	default:
	}
	owned := append([]byte(nil), payload...)
	select {
	case <-s.terminalStart:
		clear(owned)
		return acceleratorsecret.ReasonSessionUnavailable
	case s.outbound <- owned:
		return ""
	default:
		clear(owned)
		return acceleratorsecret.ReasonCapacity
	}
}

func (s *ConnectedSession) attachSecretFrameHandler(handler sessionFrameHandler) (func(), bool) {
	if s == nil || s.self != s || s.arbiter == nil {
		return func() {}, false
	}
	return s.arbiter.attach(handler)
}
