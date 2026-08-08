//go:build helm && accelerator_provision_kind

package acceleratorprovision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

// AcceptanceLiteralVersionProof is deliberately value-free. It exists only
// in the Kind test build and records the closed observations needed by 60A.
type AcceptanceLiteralVersionProof struct {
	ExactAccepted       bool
	AdjacentRejected    int
	DirectBeforeCleanup bool
	OneReplacement      int
	RepeatedMismatch    int
	NoThirdWorkload     bool
}

// ProveAcceptanceLiteralVersionMatrix drives the opaque build labels through
// the real authenticated HTTP/WebSocket connector and the production
// coordinator. It does not parse, compare, or otherwise interpret the labels.
func ProveAcceptanceLiteralVersionMatrix() (AcceptanceLiteralVersionProof, error) {
	proof := AcceptanceLiteralVersionProof{}
	exact, calls, closeExact, err := acceptanceConnectLiteral("build-N", "build-N")
	if err != nil {
		return proof, err
	}
	defer closeExact()
	if exact.Availability != Available || exact.Session == nil || calls != 3 {
		return proof, errors.New("acceptance literal matrix failed")
	}
	proof.ExactAccepted = true
	if exact.Session.Close(context.Background()) != nil {
		return proof, errors.New("acceptance literal matrix failed")
	}

	for _, runtimeVersion := range []string{"build-N-1", "build-N+1"} {
		result, authenticatedCalls, closeMismatch, connectErr := acceptanceConnectLiteral("build-N", runtimeVersion)
		if connectErr != nil {
			return proof, connectErr
		}
		closeMismatch()
		if result.Reason != ConnectVersionMismatch || result.Session != nil || authenticatedCalls != 1 {
			return proof, errors.New("acceptance literal matrix failed")
		}
		proof.AdjacentRejected++

		accepted, coordinatorErr := acceptanceRunMismatchCoordinator(runtimeVersion, []string{runtimeVersion, "build-N"})
		if coordinatorErr != nil || !accepted {
			return proof, errors.New("acceptance mismatch coordinator failed")
		}
		proof.OneReplacement++

		accepted, coordinatorErr = acceptanceRunMismatchCoordinator(runtimeVersion, []string{runtimeVersion, runtimeVersion})
		if coordinatorErr != nil || accepted {
			return proof, errors.New("acceptance mismatch coordinator failed")
		}
		proof.RepeatedMismatch++
	}
	proof.DirectBeforeCleanup = true
	proof.NoThirdWorkload = proof.OneReplacement == 2 && proof.RepeatedMismatch == 2
	return proof, nil
}

type acceptanceLiteralSnapshot struct{}

func (acceptanceLiteralSnapshot) Identity() string                { return "acceptance" }
func (acceptanceLiteralSnapshot) Namespace() string               { return "acceptance" }
func (acceptanceLiteralSnapshot) RESTConfig() *rest.Config        { return &rest.Config{} }
func (acceptanceLiteralSnapshot) Clientset() kubernetes.Interface { return nil }

type acceptanceLiteralTunnel struct {
	port int
	done chan struct{}
	once sync.Once
}

func (t *acceptanceLiteralTunnel) Port() int                { return t.port }
func (t *acceptanceLiteralTunnel) Done() <-chan struct{}    { return t.done }
func (t *acceptanceLiteralTunnel) Stop()                    { t.once.Do(func() { close(t.done) }) }
func (*acceptanceLiteralTunnel) Wait(context.Context) error { return nil }

func acceptanceConnectLiteral(desktopVersion, runtimeVersion string) (ConnectResult, int, func(), error) {
	protocol, calls, err := acceptanceLiteralProtocol(runtimeVersion)
	if err != nil {
		return ConnectResult{}, 0, func() {}, errors.New("acceptance literal protocol failed")
	}
	closed := false
	closeProtocol := func() {
		if !closed {
			closed = true
			protocol.Close()
		}
	}
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(protocol.URL, "http://"))
	if err != nil {
		closeProtocol()
		return ConnectResult{}, 0, func() {}, errors.New("acceptance literal protocol failed")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		closeProtocol()
		return ConnectResult{}, 0, func() {}, errors.New("acceptance literal protocol failed")
	}
	connector := &Connector{
		buildVersion: desktopVersion,
		clock:        processResumeClock{},
		validate: func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason {
			return ""
		},
		startTunnel: func(context.Context, ContextSnapshot, string, string) (tunnel, error) {
			return &acceptanceLiteralTunnel{port: port, done: make(chan struct{})}, nil
		},
	}
	workload, err := acceptanceLiteralWorkload(desktopVersion)
	if err != nil {
		closeProtocol()
		return ConnectResult{}, 0, func() {}, errors.New("acceptance literal workload failed")
	}
	result := connector.Connect(context.Background(), workload)
	if result.Session == nil {
		closeProtocol()
		return result, int(calls.Load()), func() {}, nil
	}
	return result, int(calls.Load()), closeProtocol, nil
}

func acceptanceLiteralProtocol(runtimeVersion string) (*httptest.Server, *atomic.Int32, error) {
	if runtimeVersion == "" {
		return nil, nil, errors.New("acceptance literal protocol failed")
	}
	entropy := make([]byte, 48)
	for index := range entropy {
		entropy[index] = byte(index)
	}
	token := base64.RawURLEncoding.EncodeToString(entropy[:32])
	var calls atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+token {
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte("{\"error\":\"unauthorized\"}\n"))
			return
		}
		calls.Add(1)
		switch request.URL.Path {
		case "/api/accelerator-info":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(writer).Encode(server.AuthenticatedAcceleratorInfo{
				Runtime: "accelerator", Build: agent.BuildIdentity{BuildVersion: runtimeVersion},
				InstanceID: "acceptance-instance", Capabilities: agent.V1Capabilities(),
				CapabilityDiagnostics: []agent.CapabilityDiagnostic{},
			})
		case "/api/call":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte("{\"error\":\"forbidden\"}\n"))
		case "/ws":
			connection, upgradeErr := upgrader.Upgrade(writer, request, nil)
			if upgradeErr != nil {
				return
			}
			defer connection.Close()
			_ = connection.WriteJSON(map[string]interface{}{
				"type": "event", "name": "connected",
				"data": map[string]interface{}{"sessionId": "acceptance-session", "instanceId": "acceptance-instance", "generation": 1, "resumed": false},
			})
			for {
				if _, _, readErr := connection.ReadMessage(); readErr != nil {
					return
				}
			}
		default:
			http.NotFound(writer, request)
		}
	})
	return httptest.NewServer(handler), &calls, nil
}

func acceptanceLiteralWorkload(buildVersion string) (*ProvisionedWorkload, error) {
	entropy := make([]byte, 48)
	for index := range entropy {
		entropy[index] = byte(index)
	}
	credential, err := generateCreatorCredential(bytes.NewReader(entropy))
	if err != nil {
		return nil, err
	}
	workload := &ProvisionedWorkload{
		ContextName: "acceptance", ReleaseNamespace: "acceptance", ReleaseName: "acceptance-release",
		WorkloadSessionID: "acceptance-workload", Job: ObjectIdentity{Name: "acceptance-job", UID: "acceptance-job-uid"},
		Pod: ObjectIdentity{Name: "acceptance-pod", UID: "acceptance-pod-uid"}, BuildVersion: buildVersion,
		ImageDigest: "sha256:" + strings.Repeat("a", 64), ChartDigest: "sha256:" + strings.Repeat("b", 64),
		credential: credential, snapshot: acceptanceLiteralSnapshot{}, connectorState: &workloadConnectorState{},
	}
	receipt := &workloadReceipt{
		contextName: workload.ContextName, releaseNamespace: workload.ReleaseNamespace, releaseName: workload.ReleaseName,
		workloadSessionID: workload.WorkloadSessionID, job: workload.Job, pod: workload.Pod,
		buildVersion: workload.BuildVersion, imageDigest: workload.ImageDigest, chartDigest: workload.ChartDigest,
		snapshot: workload.snapshot, owner: workload,
	}
	workload.connectorState.receipt = receipt
	return workload, nil
}

type acceptanceLiteralContexts struct{ snapshot ContextSnapshot }

func (c acceptanceLiteralContexts) CurrentContext() string { return "acceptance" }
func (c acceptanceLiteralContexts) SnapshotCurrentContext(name string) (ContextSnapshot, error) {
	if name != "acceptance" {
		return nil, errors.New("acceptance context unavailable")
	}
	return c.snapshot, nil
}

type acceptanceLiteralResolver struct{ calls atomic.Int32 }

func (r *acceptanceLiteralResolver) Resolve(context.Context) acceleratorrelease.Resolution {
	r.calls.Add(1)
	return acceleratorrelease.Resolution{Availability: acceleratorrelease.Available, Source: acceleratorrelease.SourceNetwork, Release: acceleratorrelease.VerifiedRelease{
		BuildVersion: "build-N", SourceCommit: strings.Repeat("c", 40), DescriptorSHA256: strings.Repeat("d", 64),
		ImageReference: imageRepository + "@sha256:" + strings.Repeat("a", 64),
		ChartReference: chartRepository + "@sha256:" + strings.Repeat("b", 64),
	}}
}

type acceptanceLiteralProvisioner struct{ calls atomic.Int32 }

func (p *acceptanceLiteralProvisioner) Provision(context.Context, Request) Result {
	p.calls.Add(1)
	workload, err := acceptanceLiteralWorkload("build-N")
	if err != nil {
		return unavailable(EntropyUnavailable, CleanupNotNeeded)
	}
	return available(workload)
}

type acceptanceLiteralConnector struct {
	runtimes []string
	calls    atomic.Int32
	servers  []func()
	mu       sync.Mutex
}

func (c *acceptanceLiteralConnector) Connect(context.Context, *ProvisionedWorkload) ConnectResult {
	index := int(c.calls.Add(1)) - 1
	if index < 0 || index >= len(c.runtimes) {
		return unavailableConnect(AcceleratorUnavailable)
	}
	result, calls, closeProtocol, err := acceptanceConnectLiteral("build-N", c.runtimes[index])
	if err != nil || (result.Session == nil && calls != 1) || (result.Session != nil && calls != 3) {
		closeProtocol()
		return unavailableConnect(AcceleratorUnavailable)
	}
	if result.Session != nil {
		c.mu.Lock()
		c.servers = append(c.servers, closeProtocol)
		c.mu.Unlock()
	}
	return result
}

func (c *acceptanceLiteralConnector) close() {
	c.mu.Lock()
	servers := append([]func(){}, c.servers...)
	c.servers = nil
	c.mu.Unlock()
	for _, closeServer := range servers {
		closeServer()
	}
}

type acceptanceLiteralReconnector struct{}

func (acceptanceLiteralReconnector) Resume(context.Context, ResumeRequest) ResumeResult {
	return unavailableResume(ResumeVersionMismatch)
}
func (acceptanceLiteralReconnector) resumeIdle(context.Context, ResumeRequest, *coordinatorIdleToken) ResumeResult {
	return unavailableResume(ResumeVersionMismatch)
}

type acceptanceLiteralDisposer struct {
	sweeps              atomic.Int32
	disposes            atomic.Int32
	coordinator         *Coordinator
	directBeforeCleanup atomic.Bool
}

func (d *acceptanceLiteralDisposer) SweepInert(context.Context, ContextSnapshot) SweepResult {
	d.sweeps.Add(1)
	return SweepResult{Status: SweepCompleted}
}
func (d *acceptanceLiteralDisposer) DisposeNow(ctx context.Context, workload *ProvisionedWorkload) DisposalResult {
	d.disposes.Add(1)
	if d.coordinator != nil && !d.coordinator.Snapshot("acceptance").Available {
		d.directBeforeCleanup.Store(true)
	}
	acceptanceCloseLiteralWorkload(ctx, workload)
	return DisposalResult{Requested: DisposalImmediate, Effective: DisposalImmediate}
}
func (d *acceptanceLiteralDisposer) DrainAndDispose(ctx context.Context, workload *ProvisionedWorkload) DisposalResult {
	return d.DisposeNow(ctx, workload)
}

func acceptanceCloseLiteralWorkload(ctx context.Context, workload *ProvisionedWorkload) {
	if workload == nil || workload.connectorState == nil {
		return
	}
	state := workload.connectorState
	state.mu.Lock()
	session := state.currentSession
	state.closed = true
	state.mu.Unlock()
	if session != nil {
		_ = session.Close(context.Background())
	}
	if workload.credential != nil {
		_ = workload.credential.closeAndDestroy(ctx)
	}
}

func acceptanceRunMismatchCoordinator(mismatchVersion string, runtimes []string) (bool, error) {
	if len(runtimes) != 2 || runtimes[0] != mismatchVersion || (mismatchVersion != "build-N-1" && mismatchVersion != "build-N+1") {
		return false, errors.New("acceptance mismatch coordinator failed")
	}
	snapshot := acceptanceLiteralSnapshot{}
	resolver := &acceptanceLiteralResolver{}
	provisioner := &acceptanceLiteralProvisioner{}
	connector := &acceptanceLiteralConnector{runtimes: append([]string(nil), runtimes...)}
	defer connector.close()
	disposer := &acceptanceLiteralDisposer{}
	coordinator := newCoordinator(acceptanceLiteralContexts{snapshot: snapshot}, resolver, provisioner, connector, acceptanceLiteralReconnector{}, disposer, processResumeClock{})
	disposer.coordinator = coordinator
	demand := coordinator.AcquireSecretDemand(context.Background(), "acceptance")
	if !demand.Accepted || demand.Lease == nil {
		return false, errors.New("acceptance mismatch coordinator failed")
	}
	// Demand proves that Secret reads remain Direct; only the connection owner
	// may authorize the two mismatch provisioning attempts exercised below.
	coordinator.Enable("acceptance", "")
	deadline := time.Now().Add(15 * time.Second)
	wantAccepted := runtimes[1] == "build-N"
	for time.Now().Before(deadline) {
		state := coordinator.Snapshot("acceptance")
		if (wantAccepted && state.State == CoordinatorActive && state.Available) || (!wantAccepted && state.State == CoordinatorUnavailable && !state.Available) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	snapshotState := coordinator.Snapshot("acceptance")
	accepted := snapshotState.State == CoordinatorActive && snapshotState.Available
	if resolver.calls.Load() != 2 || provisioner.calls.Load() != 2 || connector.calls.Load() != 2 ||
		(disposer.disposes.Load() != 1 && wantAccepted) || (disposer.disposes.Load() != 2 && !wantAccepted) ||
		disposer.sweeps.Load() != 1 || !disposer.directBeforeCleanup.Load() {
		coordinator.Quiesce(context.Background())
		coordinator.Close(context.Background())
		return false, errors.New("acceptance mismatch coordinator failed")
	}
	if wantAccepted {
		lease, ok := demand.Lease.TrySession()
		if !accepted || !ok || lease == nil {
			coordinator.Quiesce(context.Background())
			coordinator.Close(context.Background())
			return false, errors.New("acceptance mismatch coordinator failed")
		}
		lease.Close()
	} else if accepted || snapshotState.State != CoordinatorUnavailable {
		coordinator.Quiesce(context.Background())
		coordinator.Close(context.Background())
		return false, errors.New("acceptance mismatch coordinator failed")
	}
	coordinator.Quiesce(context.Background())
	coordinator.StopProducers(context.Background())
	coordinator.Close(context.Background())
	return accepted, nil
}
