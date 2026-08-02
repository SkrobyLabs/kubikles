package acceleratorprovision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
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
	ConnectCancelled       ConnectUnavailableReason = "cancelled"
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

type SessionEndReason string

const (
	SessionClosed         SessionEndReason = "closed"
	SessionPeerClosed     SessionEndReason = "peer_closed"
	SessionTunnelClosed   SessionEndReason = "tunnel_closed"
	SessionProtocolFailed SessionEndReason = "protocol_failed"
)

type tunnel interface {
	Port() int
	Done() <-chan struct{}
	Stop()
	Wait(context.Context) error
}

type connectorTunnelStarter func(context.Context, ContextSnapshot, string, string) (tunnel, error)
type connectorValidator func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason

type Connector struct {
	buildVersion    string
	startTunnel     connectorTunnelStarter
	validate        connectorValidator
	observeEndpoint func(string)
	beforePublish   func()
}

// sealedContext preserves cancellation and deadlines while ensuring caller
// context values can never reach Kubernetes or creator transports.
type sealedContext struct{ context.Context }

func (sealedContext) Value(any) any { return nil }

func NewConnector(buildVersion string) *Connector {
	return &Connector{
		buildVersion: buildVersion,
		validate:     revalidateWorkload,
		startTunnel: func(ctx context.Context, source ContextSnapshot, namespace, pod string) (tunnel, error) {
			snapshot, ok := source.(*localk8s.AcceleratorContextSnapshot)
			if !ok {
				return nil, localk8s.ErrAcceleratorContextUnavailable
			}
			return localk8s.StartAcceleratorPodTunnel(ctx, snapshot, namespace, pod)
		},
	}
}

func unavailableConnect(reason ConnectUnavailableReason) ConnectResult {
	return ConnectResult{Availability: Unavailable, Reason: reason}
}

func (c *Connector) Connect(ctx context.Context, workload *ProvisionedWorkload) ConnectResult {
	if c == nil || ctx == nil || c.buildVersion == "" || c.startTunnel == nil || c.validate == nil || !validWorkloadHandle(workload, c.buildVersion) {
		return unavailableConnect(InvalidWorkload)
	}
	lease, ok := workload.connectorLease()
	if !ok {
		return unavailableConnect(InvalidWorkload)
	}
	defer lease.release()
	exactWorkload := lease.receipt.workload()
	// Keep cancellation directly parented to the caller so a standard cancel
	// function propagates synchronously. Only the derived I/O view seals values.
	op, cancel := context.WithTimeout(ctx, connectorTimeout)
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
		return unavailableConnect(ConnectCancelled)
	}
	if reason := c.validate(sealedOp, exactWorkload, lease.snapshot); op.Err() != nil {
		return unavailableConnect(ConnectCancelled)
	} else if reason != "" {
		return unavailableConnect(mapWorkload(reason))
	}
	ready, readyCancel := context.WithTimeout(sealedOp, tunnelReadyTimeout)
	activeTunnel, err := c.startTunnel(ready, lease.snapshot, lease.receipt.releaseNamespace, lease.receipt.pod.Name)
	readyCancel()
	var socket *websocket.Conn
	published := false
	if activeTunnel != nil {
		defer func() {
			if !published {
				closeConnectTransports(socket, activeTunnel)
			}
		}()
	}
	if op.Err() != nil {
		return unavailableConnect(ConnectCancelled)
	}
	if err != nil || activeTunnel == nil || activeTunnel.Port() < 1 || activeTunnel.Port() > 65535 {
		return unavailableConnect(TunnelUnavailable)
	}
	if op.Err() != nil {
		return unavailableConnect(ConnectCancelled)
	}
	if tunnelEnded(activeTunnel) {
		return unavailableConnect(TunnelUnavailable)
	}
	if reason := c.validate(sealedOp, exactWorkload, lease.snapshot); op.Err() != nil {
		return unavailableConnect(ConnectCancelled)
	} else if reason != "" {
		return unavailableConnect(mapWorkload(reason))
	}
	endpoint := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", activeTunnel.Port()))
	if c.observeEndpoint != nil {
		c.observeEndpoint(endpoint)
	}
	if op.Err() != nil {
		return unavailableConnect(ConnectCancelled)
	}
	var info server.AuthenticatedAcceleratorInfo
	err = lease.credential.withCreatorAuthorization(sealedOp, func(authCtx context.Context, authorization creatorAuthorizationLease) error {
		var fetchErr error
		info, fetchErr = fetchInfo(authCtx, endpoint, authorization)
		if fetchErr != nil {
			return fetchErr
		}
		if !validInfo(info, c.buildVersion, exactWorkload.BuildVersion) {
			return errors.New("accelerator info unavailable")
		}
		policyErr := policyCanary(authCtx, endpoint, authorization)
		return policyErr
	})
	if op.Err() != nil {
		return unavailableConnect(ConnectCancelled)
	}
	if err != nil {
		return unavailableConnect(AcceleratorUnavailable)
	}
	var connected connectedIdentity
	err = lease.credential.withCreatorAuthorization(sealedOp, func(authCtx context.Context, authorization creatorAuthorizationLease) error {
		var dialErr error
		socket, dialErr = dialCreator(authCtx, endpoint, authorization)
		if dialErr != nil {
			return dialErr
		}
		connected, dialErr = validateConnected(authCtx, socket, info.InstanceID)
		return dialErr
	})
	if op.Err() != nil {
		return unavailableConnect(ConnectCancelled)
	}
	if err != nil {
		return unavailableConnect(AcceleratorUnavailable)
	}
	if reason := c.validate(sealedOp, exactWorkload, lease.snapshot); op.Err() != nil {
		return unavailableConnect(ConnectCancelled)
	} else if reason != "" {
		return unavailableConnect(mapWorkload(reason))
	}
	if op.Err() != nil {
		return unavailableConnect(ConnectCancelled)
	}
	if tunnelEnded(activeTunnel) {
		return unavailableConnect(TunnelUnavailable)
	}
	if c.beforePublish != nil {
		c.beforePublish()
	}
	// Stopping the cancellation callback is the publication linearization
	// point. If cancellation won before this point, wait for its fixed signal
	// and synchronously tear down instead of returning a stale live session.
	if !stopPublicationCancellation() {
		publicationPending = false
		<-cancelledBeforePublication
		return unavailableConnect(ConnectCancelled)
	}
	publicationPending = false
	session := newConnectedSession(lease.receipt, info, connected, socket, activeTunnel)
	socket = nil
	published = true
	return ConnectResult{Availability: Available, Session: session}
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
		return server.AuthenticatedAcceleratorInfo{}, errors.New("accelerator info unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strictJSONResponse(response) {
		return server.AuthenticatedAcceleratorInfo{}, errors.New("accelerator info unavailable")
	}
	info, err := decodeExactInfo(response.Body)
	if err != nil {
		return server.AuthenticatedAcceleratorInfo{}, errors.New("accelerator info unavailable")
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
		return errors.New("accelerator policy unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden || !strictJSONResponse(response) {
		return errors.New("accelerator policy unavailable")
	}
	payload, readErr := io.ReadAll(&io.LimitedReader{R: response.Body, N: policyBodyLimit + 1})
	if readErr != nil || len(payload) > policyBodyLimit || string(payload) != "{\"error\":\"forbidden\"}\n" {
		return errors.New("accelerator policy unavailable")
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
	wanted := agent.V1Capabilities()
	if info.Runtime != "accelerator" || info.Build.BuildVersion == "" || info.Build.BuildVersion != localVersion || info.Build.BuildVersion != handleVersion || info.InstanceID == "" || len(info.Capabilities) != len(wanted) {
		return false
	}
	for index := range wanted {
		if info.Capabilities[index] != wanted[index] {
			return false
		}
	}
	return true
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
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, errors.New("accelerator websocket unavailable")
	}
	if response == nil || response.StatusCode != http.StatusSwitchingProtocols || socket.Subprotocol() != "" || len(response.Header.Values("Sec-WebSocket-Protocol")) != 0 || len(response.Header.Values("Set-Cookie")) != 0 {
		_ = socket.Close()
		return nil, errors.New("accelerator websocket unavailable")
	}
	return socket, nil
}

func validateConnected(ctx context.Context, socket connectedFrameSocket, instanceID string) (connectedIdentity, error) {
	if socket == nil || ctx == nil {
		return connectedIdentity{}, errors.New("accelerator connected event unavailable")
	}
	deadline := time.Now().Add(connectedFrameTimeout)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	socket.SetReadLimit(connectedFrameLimit)
	if socket.SetReadDeadline(deadline) != nil {
		return connectedIdentity{}, errors.New("accelerator connected event unavailable")
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
		return connectedIdentity{}, errors.New("accelerator connected event unavailable")
	}
	messageType, payload, err := socket.ReadMessage()
	if !stopWake() {
		<-wakeDone
	}
	if ctx.Err() != nil || err != nil || messageType != websocket.TextMessage || len(payload) > connectedFrameLimit {
		return connectedIdentity{}, errors.New("accelerator connected event unavailable")
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
	if err = decodeStrictJSON(strings.NewReader(string(payload)), connectedFrameLimit, &event); err != nil || event.Type == nil || *event.Type != "event" || event.Name == nil || *event.Name != "connected" || event.Data == nil || event.Data.SessionID == nil || *event.Data.SessionID == "" || event.Data.InstanceID == nil || *event.Data.InstanceID != instanceID || event.Data.Generation == nil || *event.Data.Generation != 1 || event.Data.Resumed == nil || *event.Data.Resumed {
		return connectedIdentity{}, errors.New("accelerator connected event unavailable")
	}
	if socket.SetReadDeadline(time.Time{}) != nil {
		return connectedIdentity{}, errors.New("accelerator connected event unavailable")
	}
	return connectedIdentity{sessionID: *event.Data.SessionID, instanceID: *event.Data.InstanceID, generation: *event.Data.Generation}, nil
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
