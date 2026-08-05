package acceleratorprovision

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/agent"
	localk8s "kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

const (
	connectorTimeout      = 30 * time.Second
	tunnelReadyTimeout    = 15 * time.Second
	httpTimeout           = 5 * time.Second
	websocketTimeout      = 5 * time.Second
	connectedFrameTimeout = 10 * time.Second
	closeTimeout          = 5 * time.Second
	infoBodyLimit         = 16 << 10
	policyBodyLimit       = 256
	connectedFrameLimit   = 4 << 10
)

type ConnectUnavailableReason string

const (
	InvalidWorkload        ConnectUnavailableReason = "invalid_workload"
	WorkloadChanged        ConnectUnavailableReason = "workload_changed"
	WorkloadUnavailable    ConnectUnavailableReason = "workload_unavailable"
	TunnelUnavailable      ConnectUnavailableReason = "tunnel_unavailable"
	AcceleratorUnavailable ConnectUnavailableReason = "accelerator_unavailable"
	ConnectVersionMismatch ConnectUnavailableReason = "version_mismatch"
	ConnectCancelled       ConnectUnavailableReason = "cancelled"
	WorkloadDisposing      ConnectUnavailableReason = "workload_disposing"
)

type ConnectResult struct {
	Availability Availability             `json:"availability"`
	Reason       ConnectUnavailableReason `json:"reason,omitempty"`
	Session      *ConnectedSession        `json:"session,omitempty"`
}

func (r ConnectResult) String() string { return "<accelerator connect result>" }
func (r ConnectResult) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "<accelerator connect result>")
}
func (r ConnectResult) MarshalJSON() ([]byte, error) {
	type safe struct {
		Availability Availability             `json:"availability"`
		Reason       ConnectUnavailableReason `json:"reason,omitempty"`
		Session      *ConnectedSession        `json:"session,omitempty"`
	}
	return json.Marshal(safe{Availability: r.Availability, Reason: r.Reason, Session: r.Session})
}

type SessionIdentity struct {
	ContextName, ReleaseNamespace, ReleaseName, WorkloadSessionID string
	Job, Pod                                                      ObjectIdentity
	BuildVersion, ImageDigest, ChartDigest, InstanceID, SessionID string
	Generation                                                    int
}

func (i SessionIdentity) String() string { return "<accelerator session identity>" }
func (i SessionIdentity) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "<accelerator session identity>")
}
func (SessionIdentity) MarshalJSON() ([]byte, error) {
	return json.Marshal("<accelerator session identity>")
}

type SessionEndReason string

const (
	SessionClosed         SessionEndReason = "closed"
	SessionPeerClosed     SessionEndReason = "peer_closed"
	SessionTunnelClosed   SessionEndReason = "tunnel_closed"
	SessionProtocolFailed SessionEndReason = "protocol_failed"
	SessionIdleReleased   SessionEndReason = "idle_released"
)

type tunnel interface {
	Port() int
	Done() <-chan struct{}
	Stop()
	Wait(context.Context) error
}

type acceleratorPodTunnel struct{ *localk8s.AcceleratorPodTunnel }

func (t *acceleratorPodTunnel) terminalCause() error {
	return connectorTunnelFailure(localk8s.AcceleratorPodTunnelFailure(t.AcceleratorPodTunnel))
}

var (
	errTunnelUpgradeAttempt          = errors.New("accelerator tunnel upgrade unavailable")
	errTunnelHTTPSProxyAttempt       = errors.New("accelerator tunnel proxy unavailable")
	errTunnelTransientAttempt        = errors.New("accelerator tunnel temporarily unavailable")
	errTunnelGenericAttempt          = errors.New("accelerator tunnel unavailable")
	errTunnelCleanupUnsettledAttempt = errors.New("accelerator tunnel cleanup unsettled")
	errCandidateUnavailable          = errors.New("accelerator candidate unavailable")
)

func connectorTunnelFailure(err error) error {
	if err == nil {
		return nil
	}
	switch localk8s.ClassifyAcceleratorTunnelFailure(err) {
	case localk8s.AcceleratorTunnelFailureUpgrade:
		return errTunnelUpgradeAttempt
	case localk8s.AcceleratorTunnelFailureHTTPSProxy:
		return errTunnelHTTPSProxyAttempt
	case localk8s.AcceleratorTunnelFailureTransient:
		return errTunnelTransientAttempt
	case localk8s.AcceleratorTunnelFailureCleanupUnsettled:
		return errTunnelCleanupUnsettledAttempt
	default:
		return errTunnelGenericAttempt
	}
}

type connectorTunnelStarter func(context.Context, ContextSnapshot, string, string) (tunnel, error)
type connectorValidator func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason
type connectorDetailedValidator func(context.Context, *ProvisionedWorkload, ContextSnapshot) workloadValidationResult

type Connector struct {
	buildVersion         string
	allowVersionMismatch bool
	clock                resumeClock
	startTunnel          connectorTunnelStarter
	validate             connectorValidator
	validateDetailed     connectorDetailedValidator
	observeEndpoint      func(string)
	beforePublish        func()
}

// sealedContext preserves cancellation and deadlines while ensuring caller
// context values can never reach Kubernetes or creator transports.
type sealedContext struct{ context.Context }

func (sealedContext) Value(any) any { return nil }

func NewConnector(buildVersion string) *Connector {
	return &Connector{
		buildVersion:     buildVersion,
		clock:            processResumeClock{},
		validate:         revalidateWorkload,
		validateDetailed: revalidateWorkloadDetailed,
		startTunnel: func(ctx context.Context, source ContextSnapshot, namespace, pod string) (tunnel, error) {
			snapshot, ok := source.(*localk8s.AcceleratorContextSnapshot)
			if !ok {
				return nil, localk8s.ErrAcceleratorContextUnavailable
			}
			started, err := localk8s.StartAcceleratorPodTunnel(ctx, snapshot, namespace, pod)
			if started == nil {
				return nil, connectorTunnelFailure(err)
			}
			return &acceleratorPodTunnel{AcceleratorPodTunnel: started}, connectorTunnelFailure(err)
		},
	}
}

func unavailableConnect(reason ConnectUnavailableReason) ConnectResult {
	return ConnectResult{Availability: Unavailable, Reason: reason}
}

func (c *Connector) Connect(ctx context.Context, workload *ProvisionedWorkload) ConnectResult {
	if c == nil || ctx == nil || c.buildVersion == "" || c.startTunnel == nil || (c.validate == nil && c.validateDetailed == nil) || !validWorkloadHandle(workload, c.buildVersion) {
		return unavailableConnect(InvalidWorkload)
	}
	op, finish, lifecycleReason := workload.beginLifecycleOperation(ctx)
	if lifecycleReason != "" {
		return unavailableConnect(lifecycleReason)
	}
	defer finish()
	lease, ok := workload.connectorLease()
	if !ok {
		if workload.isDisposing() {
			return unavailableConnect(WorkloadDisposing)
		}
		return unavailableConnect(InvalidWorkload)
	}
	defer lease.release()
	op, cancel := context.WithTimeout(op, connectorTimeout)
	defer cancel()
	session, failure := c.connectExactAttempt(op, lease, connectionExpectation{kind: connectionFresh})
	if failure != nil {
		if workload.connectorState != nil {
			workload.connectorState.mu.Lock()
			disposing := workload.connectorState.disposing
			workload.connectorState.mu.Unlock()
			if disposing {
				return unavailableConnect(WorkloadDisposing)
			}
		}
		return unavailableConnect(failure.connectReason)
	}
	if !workload.publishCurrentSession(session) {
		_ = session.Close(context.Background())
		return unavailableConnect(WorkloadDisposing)
	}
	return ConnectResult{Availability: Available, Session: session}
}

// ConnectWithVersionPolicy applies an explicit development target without
// mutating the production connector shared by the coordinator.
func (c *Connector) ConnectWithVersionPolicy(ctx context.Context, workload *ProvisionedWorkload, expectedVersion string, allowMismatch bool) ConnectResult {
	if c == nil || expectedVersion == "" {
		return unavailableConnect(InvalidWorkload)
	}
	configured := *c
	configured.buildVersion = expectedVersion
	configured.allowVersionMismatch = allowMismatch
	return configured.Connect(ctx, workload)
}

type connectionExpectationKind uint8

const (
	connectionFresh connectionExpectationKind = iota + 1
	connectionResume
)

type connectionExpectation struct {
	kind            connectionExpectationKind
	instanceID      string
	sessionID       string
	generationFloor int
	accepted        func(int)
}

type attemptPhase uint8

const (
	attemptPreValidate attemptPhase = iota + 1
	attemptTunnel
	attemptPostValidate
	attemptInfo
	attemptPolicy
	attemptWebSocket
	attemptConnectedFrame
	attemptFinalValidate
	attemptPublication
)

type connectAttemptFailure struct {
	phase         attemptPhase
	connectReason ConnectUnavailableReason
	cause         error
	status        int
	earlyClose    bool
	peerClose     bool
}

func (c *Connector) connectExactAttempt(ctx context.Context, lease connectorLease, expectation connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
	if c == nil || ctx == nil || c.buildVersion == "" || c.startTunnel == nil || (c.validate == nil && c.validateDetailed == nil) || lease.receipt == nil || lease.credential == nil || lease.snapshot == nil || !validExpectation(expectation) {
		return nil, &connectAttemptFailure{phase: attemptPreValidate, connectReason: InvalidWorkload}
	}
	exactWorkload := lease.receipt.workload()
	// Keep cancellation directly parented to the caller so a standard cancel
	// function propagates synchronously. Only the derived I/O view seals values.
	op, cancel := context.WithCancel(ctx)
	defer cancel()
	sealedOp := sealedContext{op}
	cancelledBeforePublication := make(chan struct{})
	stopPublicationCancellation := context.AfterFunc(op, func() { close(cancelledBeforePublication) })
	publicationPending := true
	defer func() {
		if publicationPending {
			stopPublicationCancellation()
		}
	}()
	if op.Err() != nil {
		return nil, &connectAttemptFailure{phase: attemptPreValidate, connectReason: ConnectCancelled, cause: op.Err()}
	}
	if validation := c.validateExact(sealedOp, exactWorkload, lease.snapshot); op.Err() != nil {
		return nil, &connectAttemptFailure{phase: attemptPreValidate, connectReason: ConnectCancelled, cause: op.Err()}
	} else if validation.reason != "" {
		return nil, workloadAttemptFailure(attemptPreValidate, validation)
	}
	ready, readyCancel := context.WithTimeout(sealedOp, tunnelReadyTimeout)
	activeTunnel, err := c.startTunnel(ready, lease.snapshot, lease.receipt.releaseNamespace, lease.receipt.pod.Name)
	readyCancel()
	var socket *websocket.Conn
	var candidate *ConnectedSession
	published := false
	if activeTunnel != nil {
		defer func() {
			if !published {
				if candidate != nil {
					closeCandidateSession(candidate)
				} else {
					closeConnectTransports(socket, activeTunnel)
				}
			}
		}()
	}
	if op.Err() != nil {
		return nil, &connectAttemptFailure{phase: attemptTunnel, connectReason: ConnectCancelled, cause: op.Err()}
	}
	if err != nil || activeTunnel == nil || activeTunnel.Port() < 1 || activeTunnel.Port() > 65535 {
		return nil, &connectAttemptFailure{phase: attemptTunnel, connectReason: TunnelUnavailable, cause: err}
	}
	if op.Err() != nil {
		return nil, &connectAttemptFailure{phase: attemptTunnel, connectReason: ConnectCancelled, cause: op.Err()}
	}
	if tunnelEnded(activeTunnel) {
		return nil, &connectAttemptFailure{phase: attemptTunnel, connectReason: TunnelUnavailable, cause: tunnelTerminalCause(activeTunnel), earlyClose: true}
	}
	if validation := c.validateExact(sealedOp, exactWorkload, lease.snapshot); op.Err() != nil {
		return nil, &connectAttemptFailure{phase: attemptPostValidate, connectReason: ConnectCancelled, cause: op.Err()}
	} else if validation.reason != "" {
		return nil, workloadAttemptFailure(attemptPostValidate, validation)
	}
	endpoint := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", activeTunnel.Port()))
	if c.observeEndpoint != nil {
		c.observeEndpoint(endpoint)
	}
	if op.Err() != nil {
		return nil, &connectAttemptFailure{phase: attemptInfo, connectReason: ConnectCancelled, cause: op.Err()}
	}
	var info server.AuthenticatedAcceleratorInfo
	phase := attemptInfo
	err = lease.credential.withCreatorAuthorization(sealedOp, func(authCtx context.Context, authorization creatorAuthorizationLease) error {
		var fetchErr error
		info, fetchErr = fetchInfo(authCtx, endpoint, authorization)
		if fetchErr != nil {
			return fetchErr
		}
		if infoFailure := authenticatedInfoFailureWithPolicy(info, c.buildVersion, exactWorkload.BuildVersion, c.allowVersionMismatch); infoFailure != nil {
			return infoFailure
		}
		if expectation.kind == connectionResume && info.InstanceID != expectation.instanceID {
			return identityAttemptError{}
		}
		phase = attemptPolicy
		policyErr := policyCanary(authCtx, endpoint, authorization)
		return policyErr
	})
	if op.Err() != nil {
		return nil, &connectAttemptFailure{phase: phase, connectReason: ConnectCancelled, cause: op.Err()}
	}
	if err != nil {
		return nil, failureFromError(phase, AcceleratorUnavailable, err)
	}
	var connected connectedIdentity
	phase = attemptWebSocket
	err = lease.credential.withCreatorAuthorization(sealedOp, func(authCtx context.Context, authorization creatorAuthorizationLease) error {
		var dialErr error
		socket, dialErr = dialCreator(authCtx, endpoint, authorization)
		if dialErr != nil {
			return dialErr
		}
		phase = attemptConnectedFrame
		connected, dialErr = validateConnectedExpectation(authCtx, socket, info.InstanceID, expectation)
		return dialErr
	})
	if op.Err() != nil {
		return nil, &connectAttemptFailure{phase: phase, connectReason: ConnectCancelled, cause: op.Err()}
	}
	if err != nil {
		return nil, failureFromError(phase, AcceleratorUnavailable, err)
	}
	if validation := c.validateExact(sealedOp, exactWorkload, lease.snapshot); op.Err() != nil {
		return nil, &connectAttemptFailure{phase: attemptFinalValidate, connectReason: ConnectCancelled, cause: op.Err()}
	} else if validation.reason != "" {
		return nil, workloadAttemptFailure(attemptFinalValidate, validation)
	}
	if op.Err() != nil {
		return nil, &connectAttemptFailure{phase: attemptPublication, connectReason: ConnectCancelled, cause: op.Err()}
	}
	if tunnelEnded(activeTunnel) {
		return nil, &connectAttemptFailure{phase: attemptPublication, connectReason: TunnelUnavailable, cause: tunnelTerminalCause(activeTunnel), earlyClose: true}
	}
	if c.beforePublish != nil {
		c.beforePublish()
	}
	if op.Err() != nil {
		return nil, &connectAttemptFailure{phase: attemptPublication, connectReason: ConnectCancelled, cause: op.Err()}
	}
	if expectation.kind == connectionResume {
		candidate, err = newConnectedCandidate(lease.receipt, info, connected, socket, activeTunnel, c.clock)
		if err != nil {
			return nil, &connectAttemptFailure{phase: attemptPublication, connectReason: AcceleratorUnavailable, cause: err}
		}
		socket = nil
		if err = probeConnectedCandidate(sealedOp, candidate); err != nil {
			if op.Err() != nil {
				return nil, &connectAttemptFailure{phase: attemptPublication, connectReason: ConnectCancelled, cause: op.Err()}
			}
			return nil, &connectAttemptFailure{phase: attemptPublication, connectReason: AcceleratorUnavailable, cause: err, earlyClose: true, peerClose: true}
		}
	}
	if tunnelEnded(activeTunnel) {
		return nil, &connectAttemptFailure{phase: attemptPublication, connectReason: TunnelUnavailable, cause: tunnelTerminalCause(activeTunnel), earlyClose: true}
	}
	// Stopping the cancellation callback is the publication linearization
	// point. If cancellation won before this point, wait for its fixed signal
	// and synchronously tear down instead of returning a stale live session.
	if !stopPublicationCancellation() {
		publicationPending = false
		<-cancelledBeforePublication
		return nil, &connectAttemptFailure{phase: attemptPublication, connectReason: ConnectCancelled, cause: op.Err()}
	}
	publicationPending = false
	if candidate == nil {
		candidate = newConnectedSession(lease.receipt, info, connected, socket, activeTunnel, c.clock)
		socket = nil
	}
	published = true
	return candidate, nil
}

func validExpectation(expectation connectionExpectation) bool {
	if expectation.kind == connectionFresh {
		return expectation.instanceID == "" && expectation.sessionID == "" && expectation.generationFloor == 0
	}
	return expectation.kind == connectionResume && expectation.instanceID != "" && expectation.sessionID != "" && expectation.generationFloor > 0
}

type workloadValidationError struct {
	reason UnavailableReason
	cause  error
}

func (e workloadValidationError) Error() string { return "accelerator workload unavailable" }
func (e workloadValidationError) Unwrap() error { return e.cause }

func workloadAttemptFailure(phase attemptPhase, validation workloadValidationResult) *connectAttemptFailure {
	return &connectAttemptFailure{phase: phase, connectReason: mapWorkload(validation.reason), cause: workloadValidationError{reason: validation.reason, cause: validation.cause}}
}

func (c *Connector) validateExact(ctx context.Context, workload *ProvisionedWorkload, snapshot ContextSnapshot) workloadValidationResult {
	if c.validateDetailed != nil {
		return c.validateDetailed(ctx, workload, snapshot)
	}
	return workloadValidationResult{reason: c.validate(ctx, workload, snapshot)}
}

type protocolAttemptError struct{}

func (protocolAttemptError) Error() string { return "accelerator protocol unavailable" }

// buildVersionMismatchAttemptError deliberately carries no version labels.
// The authenticated info payload is the only source for this classification,
// and public results expose only the fixed lifecycle reason.
type buildVersionMismatchAttemptError struct{}

func (buildVersionMismatchAttemptError) Error() string { return "accelerator build version mismatch" }

type identityAttemptError struct{}

func (identityAttemptError) Error() string { return "accelerator identity unavailable" }

type httpStatusAttemptError struct {
	status int
	cause  error
}

func (httpStatusAttemptError) Error() string { return "accelerator HTTP unavailable" }

func failureFromError(phase attemptPhase, reason ConnectUnavailableReason, err error) *connectAttemptFailure {
	var mismatch buildVersionMismatchAttemptError
	if errors.As(err, &mismatch) {
		reason = ConnectVersionMismatch
	}
	failure := &connectAttemptFailure{phase: phase, connectReason: reason, cause: err}
	var statusErr httpStatusAttemptError
	if errors.As(err, &statusErr) {
		failure.status = statusErr.status
		failure.cause = statusErr.cause
	}
	return failure
}

func validWorkloadHandle(w *ProvisionedWorkload, buildVersion string) bool {
	return w != nil && w.ContextName != "" && w.ReleaseNamespace != "" && w.ReleaseName != "" && w.WorkloadSessionID != "" &&
		w.Job.Name != "" && w.Job.UID != "" && w.Pod.Name != "" && w.Pod.UID != "" && w.BuildVersion == buildVersion &&
		w.ImageDigest != "" && w.ChartDigest != ""
}

func mapWorkload(reason UnavailableReason) ConnectUnavailableReason {
	switch reason {
	case ContextChanged:
		return WorkloadChanged
	case Cancelled, TimedOut:
		return ConnectCancelled
	default:
		return WorkloadUnavailable
	}
}

func tunnelEnded(active tunnel) bool {
	select {
	case <-active.Done():
		return true
	default:
		return false
	}
}

type tunnelCauseProvider interface{ terminalCause() error }

func tunnelTerminalCause(active tunnel) error {
	if provider, ok := active.(tunnelCauseProvider); ok {
		return provider.terminalCause()
	}
	return nil
}

func newConnectedCandidate(receipt *workloadReceipt, info server.AuthenticatedAcceleratorInfo, connected connectedIdentity, socket *websocket.Conn, active tunnel, clock resumeClock) (*ConnectedSession, error) {
	if socket == nil {
		return nil, protocolAttemptError{}
	}
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, protocolAttemptError{}
	}
	nonce := hex.EncodeToString(nonceBytes)
	pong := make(chan struct{})
	var once sync.Once
	socket.SetPongHandler(func(payload string) error {
		if payload == nonce {
			once.Do(func() { close(pong) })
		}
		return nil
	})
	candidate := newConnectedSessionWithCandidateFence(receipt, info, connected, socket, active, clock, nonce, pong)
	return candidate, nil
}

func probeConnectedCandidate(ctx context.Context, candidate *ConnectedSession) error {
	if ctx == nil || candidate == nil || candidate.socket == nil || candidate.candidateNonce == "" || candidate.candidatePong == nil {
		return protocolAttemptError{}
	}
	deadline := time.Now().Add(websocketTimeout)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	if err := candidate.socket.SetWriteDeadline(deadline); err != nil {
		return err
	}
	if err := candidate.socket.WriteControl(websocket.PingMessage, []byte(candidate.candidateNonce), deadline); err != nil {
		return err
	}
	if err := candidate.socket.SetWriteDeadline(time.Time{}); err != nil {
		return protocolAttemptError{}
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-candidate.candidatePong:
		return nil
	case <-candidate.Done():
		return errCandidateUnavailable
	case <-ctx.Done():
		return errCandidateUnavailable
	case <-timer.C:
		return errCandidateUnavailable
	}
}

func closeCandidateSession(candidate *ConnectedSession) {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	_ = candidate.Close(ctx)
	cancel()
}

func closeConnectTransports(socket *websocket.Conn, active tunnel) {
	if socket != nil {
		_ = socket.Close()
	}
	if active != nil {
		active.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		_ = active.Wait(ctx)
		cancel()
	}
}

func localClient(endpoint string) *http.Client {
	dialer := &net.Dialer{}
	transport := &http.Transport{
		Proxy:                  nil,
		DisableCompression:     true,
		DisableKeepAlives:      true,
		MaxResponseHeaderBytes: 8 << 10,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || address != endpoint {
				return nil, errors.New("accelerator local endpoint rejected")
			}
			return dialer.DialContext(ctx, network, address)
		},
	}
	return &http.Client{
		Timeout:   httpTimeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func newLocalRequest(ctx context.Context, method, endpoint, requestPath string, body io.Reader) (*http.Request, error) {
	target := url.URL{Scheme: "http", Host: endpoint, Path: requestPath}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, errors.New("accelerator request unavailable")
	}
	request.Host = endpoint
	request.Header["Accept-Encoding"] = []string{"identity"}
	request.Header["User-Agent"] = nil
	return request, nil
}

func fetchInfo(ctx context.Context, endpoint string, authorization creatorAuthorizationLease) (server.AuthenticatedAcceleratorInfo, error) {
	request, err := newLocalRequest(ctx, http.MethodGet, endpoint, "/api/accelerator-info", nil)
	if err != nil {
		return server.AuthenticatedAcceleratorInfo{}, err
	}
	response, err := authorization.DoHTTP(localClient(endpoint), request)
	if err != nil {
		return server.AuthenticatedAcceleratorInfo{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return server.AuthenticatedAcceleratorInfo{}, httpStatusAttemptError{status: response.StatusCode}
	}
	if !strictJSONResponse(response) {
		return server.AuthenticatedAcceleratorInfo{}, protocolAttemptError{}
	}
	info, err := decodeExactInfo(response.Body)
	if err != nil {
		return server.AuthenticatedAcceleratorInfo{}, protocolAttemptError{}
	}
	return info, nil
}

const policyCanaryBody = "{\"method\":\"GetVersionInfo\",\"args\":[]}\n"

func policyCanary(ctx context.Context, endpoint string, authorization creatorAuthorizationLease) error {
	request, err := newLocalRequest(ctx, http.MethodPost, endpoint, "/api/call", strings.NewReader(policyCanaryBody))
	if err != nil {
		return err
	}
	request.Header["Content-Type"] = []string{"application/json"}
	response, err := authorization.DoHTTP(localClient(endpoint), request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		return httpStatusAttemptError{status: response.StatusCode}
	}
	if !strictJSONResponse(response) {
		return protocolAttemptError{}
	}
	payload, readErr := io.ReadAll(&io.LimitedReader{R: response.Body, N: policyBodyLimit + 1})
	if readErr != nil || len(payload) > policyBodyLimit || string(payload) != "{\"error\":\"forbidden\"}\n" {
		return protocolAttemptError{}
	}
	return nil
}

func strictJSONResponse(response *http.Response) bool {
	if response == nil || len(response.Header.Values("Cache-Control")) != 1 || response.Header.Values("Cache-Control")[0] != "no-store" || len(response.Header.Values("Set-Cookie")) != 0 || len(response.Header.Values("Content-Encoding")) != 0 || len(response.Header.Values("Content-Type")) != 1 {
		return false
	}
	mediaType, parameters, err := mime.ParseMediaType(response.Header.Values("Content-Type")[0])
	return err == nil && mediaType == "application/json" && len(parameters) == 0
}

func decodeExactInfo(reader io.Reader) (server.AuthenticatedAcceleratorInfo, error) {
	type exactBuild struct {
		BuildVersion *string `json:"buildVersion"`
		Commit       *string `json:"commit"`
		Dirty        *bool   `json:"dirty"`
	}
	type exactInfo struct {
		Runtime               *string                       `json:"runtime"`
		Build                 *exactBuild                   `json:"build"`
		InstanceID            *string                       `json:"instanceId"`
		Capabilities          *[]agent.Capability           `json:"capabilities"`
		CapabilityDiagnostics *[]agent.CapabilityDiagnostic `json:"capabilityDiagnostics"`
	}
	var wire exactInfo
	if err := decodeStrictJSON(reader, infoBodyLimit, &wire); err != nil || wire.Runtime == nil || wire.Build == nil || wire.Build.BuildVersion == nil || wire.Build.Commit == nil || wire.Build.Dirty == nil || wire.InstanceID == nil || wire.Capabilities == nil || wire.CapabilityDiagnostics == nil {
		return server.AuthenticatedAcceleratorInfo{}, errors.New("invalid accelerator info")
	}
	return server.AuthenticatedAcceleratorInfo{
		Runtime:               *wire.Runtime,
		Build:                 agent.BuildIdentity{BuildVersion: *wire.Build.BuildVersion, Commit: *wire.Build.Commit, Dirty: *wire.Build.Dirty},
		InstanceID:            *wire.InstanceID,
		Capabilities:          append([]agent.Capability(nil), (*wire.Capabilities)...),
		CapabilityDiagnostics: append([]agent.CapabilityDiagnostic(nil), (*wire.CapabilityDiagnostics)...),
	}, nil
}

func validInfo(info server.AuthenticatedAcceleratorInfo, localVersion, handleVersion string) bool {
	return validInfoWithPolicy(info, localVersion, handleVersion, false)
}

func validInfoWithPolicy(info server.AuthenticatedAcceleratorInfo, localVersion, handleVersion string, allowMismatch bool) bool {
	wanted := agent.V1Capabilities()
	if info.Runtime != "accelerator" || info.Build.BuildVersion == "" || (!allowMismatch && (info.Build.BuildVersion != localVersion || info.Build.BuildVersion != handleVersion)) || info.InstanceID == "" || len(info.Capabilities) != len(wanted) {
		return false
	}
	for index := range wanted {
		if info.Capabilities[index] != wanted[index] {
			return false
		}
	}
	return true
}

func authenticatedInfoFailure(info server.AuthenticatedAcceleratorInfo, localVersion, handleVersion string) error {
	return authenticatedInfoFailureWithPolicy(info, localVersion, handleVersion, false)
}

func authenticatedInfoFailureWithPolicy(info server.AuthenticatedAcceleratorInfo, localVersion, handleVersion string, allowMismatch bool) error {
	if allowMismatch && validInfoWithPolicy(info, localVersion, handleVersion, true) {
		return nil
	}
	if info.Runtime == "accelerator" && info.Build.BuildVersion != "" &&
		(info.Build.BuildVersion != localVersion || info.Build.BuildVersion != handleVersion) {
		return buildVersionMismatchAttemptError{}
	}
	if !validInfoWithPolicy(info, localVersion, handleVersion, allowMismatch) {
		return protocolAttemptError{}
	}
	return nil
}

type connectedIdentity struct {
	sessionID  string
	instanceID string
	generation int
}

type connectedFrameSocket interface {
	SetReadLimit(int64)
	SetReadDeadline(time.Time) error
	ReadMessage() (int, []byte, error)
	Close() error
}

func dialCreator(ctx context.Context, endpoint string, authorization creatorAuthorizationLease) (*websocket.Conn, error) {
	target := url.URL{Scheme: "ws", Host: endpoint, Path: "/ws"}
	netDialer := &net.Dialer{}
	dialer := &websocket.Dialer{
		Proxy:             nil,
		HandshakeTimeout:  websocketTimeout,
		EnableCompression: false,
		Subprotocols:      nil,
		NetDialContext: func(dialCtx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || address != endpoint {
				return nil, errors.New("accelerator local endpoint rejected")
			}
			return netDialer.DialContext(dialCtx, network, address)
		},
	}
	socket, response, err := authorization.DialCreatorWebSocket(ctx, dialer, target.String())
	if err != nil {
		status := 0
		if response != nil && response.Body != nil {
			status = response.StatusCode
			_ = response.Body.Close()
		}
		if status != 0 {
			return nil, httpStatusAttemptError{status: status, cause: err}
		}
		return nil, err
	}
	if response == nil {
		_ = socket.Close()
		return nil, protocolAttemptError{}
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		_ = socket.Close()
		return nil, httpStatusAttemptError{status: response.StatusCode}
	}
	if socket.Subprotocol() != "" || len(response.Header.Values("Sec-WebSocket-Protocol")) != 0 || len(response.Header.Values("Set-Cookie")) != 0 {
		_ = socket.Close()
		return nil, protocolAttemptError{}
	}
	return socket, nil
}

func validateConnected(ctx context.Context, socket connectedFrameSocket, instanceID string) (connectedIdentity, error) {
	return validateConnectedExpectation(ctx, socket, instanceID, connectionExpectation{kind: connectionFresh})
}

func validateConnectedExpectation(ctx context.Context, socket connectedFrameSocket, instanceID string, expectation connectionExpectation) (connectedIdentity, error) {
	if socket == nil || ctx == nil {
		return connectedIdentity{}, protocolAttemptError{}
	}
	deadline := time.Now().Add(connectedFrameTimeout)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	socket.SetReadLimit(connectedFrameLimit)
	if socket.SetReadDeadline(deadline) != nil {
		return connectedIdentity{}, protocolAttemptError{}
	}
	wakeDone := make(chan struct{})
	stopWake := context.AfterFunc(ctx, func() {
		// Gorilla permits Close concurrently with every method. Read-side
		// methods, including SetReadDeadline, must remain single-threaded.
		_ = socket.Close()
		close(wakeDone)
	})
	if ctx.Err() != nil {
		if !stopWake() {
			<-wakeDone
		}
		return connectedIdentity{}, protocolAttemptError{}
	}
	messageType, payload, err := socket.ReadMessage()
	if !stopWake() {
		<-wakeDone
	}
	if ctx.Err() != nil || err != nil || messageType != websocket.TextMessage || len(payload) > connectedFrameLimit {
		if err != nil {
			return connectedIdentity{}, err
		}
		return connectedIdentity{}, protocolAttemptError{}
	}
	var event struct {
		Type *string `json:"type"`
		Name *string `json:"name"`
		Data *struct {
			SessionID  *string `json:"sessionId"`
			InstanceID *string `json:"instanceId"`
			Generation *int    `json:"generation"`
			Resumed    *bool   `json:"resumed"`
		} `json:"data"`
	}
	if err = decodeStrictJSON(strings.NewReader(string(payload)), connectedFrameLimit, &event); err != nil || event.Type == nil || *event.Type != "event" || event.Name == nil || *event.Name != "connected" || event.Data == nil || event.Data.SessionID == nil || *event.Data.SessionID == "" || event.Data.InstanceID == nil || event.Data.Generation == nil || event.Data.Resumed == nil {
		return connectedIdentity{}, protocolAttemptError{}
	}
	identity := connectedIdentity{sessionID: *event.Data.SessionID, instanceID: *event.Data.InstanceID, generation: *event.Data.Generation}
	if identity.instanceID != instanceID {
		return connectedIdentity{}, identityAttemptError{}
	}
	switch expectation.kind {
	case connectionFresh:
		if identity.generation != 1 || *event.Data.Resumed {
			return connectedIdentity{}, protocolAttemptError{}
		}
	case connectionResume:
		if identity.instanceID != expectation.instanceID || identity.sessionID != expectation.sessionID {
			return connectedIdentity{}, identityAttemptError{}
		}
		if !*event.Data.Resumed || identity.generation <= expectation.generationFloor {
			return connectedIdentity{}, protocolAttemptError{}
		}
		if expectation.accepted != nil {
			expectation.accepted(identity.generation)
		}
	default:
		return connectedIdentity{}, protocolAttemptError{}
	}
	if socket.SetReadDeadline(time.Time{}) != nil {
		return connectedIdentity{}, protocolAttemptError{}
	}
	return identity, nil
}

func decodeStrictJSON(reader io.Reader, limit int64, destination interface{}) error {
	limited := &io.LimitedReader{R: reader, N: limit + 1}
	payload, err := io.ReadAll(limited)
	if err != nil || len(payload) == 0 || int64(len(payload)) > limit || hasDuplicateJSONKey(payload) {
		return errors.New("invalid accelerator JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(destination); err != nil {
		return errors.New("invalid accelerator JSON")
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid accelerator JSON")
	}
	return nil
}

func hasDuplicateJSONKey(payload []byte) bool {
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	var walk func() bool
	walk = func() bool {
		token, err := decoder.Token()
		if err != nil {
			return true
		}
		delimiter, composite := token.(json.Delim)
		if !composite {
			return false
		}
		switch delimiter {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, keyErr := decoder.Token()
				key, keyOK := keyToken.(string)
				if keyErr != nil || !keyOK {
					return true
				}
				if _, exists := seen[key]; exists {
					return true
				}
				seen[key] = struct{}{}
				if walk() {
					return true
				}
			}
			end, endErr := decoder.Token()
			return endErr != nil || end != json.Delim('}')
		case '[':
			for decoder.More() {
				if walk() {
					return true
				}
			}
			end, endErr := decoder.Token()
			return endErr != nil || end != json.Delim(']')
		default:
			return true
		}
	}
	if walk() {
		return true
	}
	_, err := decoder.Token()
	return err != io.EOF
}
