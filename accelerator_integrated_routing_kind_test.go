//go:build helm && accelerator_provision_kind && !headless && !accelerator

package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubikles/pkg/acceleratorprovision"
	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/agent"
	"kubikles/pkg/events"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

type integratedRoutingKindOrigin uint8

const (
	integratedRoutingKindList integratedRoutingKindOrigin = iota
	integratedRoutingKindData
	integratedRoutingKindYAML
	integratedRoutingKindCancel
	integratedRoutingKindSubscribe
	integratedRoutingKindUnsubscribe
	integratedRoutingKindOrigins
)

const (
	integratedRoutingKindInitialReadinessTimeout = acceleratorprovision.SweepTimeout + acceleratorprovision.ActivationAttemptTimeout + time.Minute
	integratedRoutingKindDiagnosticMarker        = "accelerator-kind-diagnostic:"
	integratedRoutingKindChartRepository         = "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator"

	integratedRoutingKindFixtureSetupTimeout   = 30 * time.Second
	integratedRoutingKindFixtureCleanupTimeout = 20 * time.Second
	integratedRoutingKindWatchMutationTimeout  = 30 * time.Second
	integratedRoutingKindHelmAssertionTimeout  = 30 * time.Second
	integratedRoutingKindK8sAssertionTimeout   = 30 * time.Second
	integratedRoutingKindCleanupTimeout        = 130 * time.Second
	integratedRoutingKindDesktopAPITimeout     = k8s.DefaultAPITimeout

	integratedRoutingKindListOperationTimeout        = 60 * time.Second
	integratedRoutingKindDataOperationTimeout        = 30 * time.Second
	integratedRoutingKindYAMLOperationTimeout        = 30 * time.Second
	integratedRoutingKindCancelOperationTimeout      = 5 * time.Second
	integratedRoutingKindSubscribeOperationTimeout   = 10 * time.Second
	integratedRoutingKindUnsubscribeOperationTimeout = 5 * time.Second
	integratedRoutingKindDataTotalTimeout            = integratedRoutingKindDataOperationTimeout + integratedRoutingKindDesktopAPITimeout
	integratedRoutingKindYAMLTotalTimeout            = integratedRoutingKindYAMLOperationTimeout + integratedRoutingKindDesktopAPITimeout

	integratedRoutingKindListBarrierTimeout = 30 * time.Second
	integratedRoutingKindWatchEventTimeout  = 30 * time.Second
	integratedRoutingKindUnavailableTimeout = 30 * time.Second
	integratedRoutingKindResumeTimeout      = 90 * time.Second
	integratedRoutingKindMismatchTimeout    = 8 * time.Minute
	integratedRoutingKindMismatchSettle     = 2 * time.Second
	integratedRoutingKindZeroClientGrace    = 2 * time.Minute

	// The canceled List's 60s allocation dominates its 5s CancelListRequest.
	// Stale-token calls fail at the router before opening a remote operation.
	integratedRoutingKindRemoteOperationsTimeout = 2*integratedRoutingKindListOperationTimeout +
		2*integratedRoutingKindDataOperationTimeout +
		integratedRoutingKindYAMLOperationTimeout +
		integratedRoutingKindSubscribeOperationTimeout +
		integratedRoutingKindUnsubscribeOperationTimeout
	integratedRoutingKindWaitsTimeout = integratedRoutingKindListBarrierTimeout +
		integratedRoutingKindWatchEventTimeout +
		integratedRoutingKindUnavailableTimeout +
		integratedRoutingKindResumeTimeout
	integratedRoutingKindSequentialTimeout = integratedRoutingKindFixtureSetupTimeout +
		6*integratedRoutingKindDesktopAPITimeout +
		integratedRoutingKindInitialReadinessTimeout +
		integratedRoutingKindRemoteOperationsTimeout +
		integratedRoutingKindWaitsTimeout +
		integratedRoutingKindWatchMutationTimeout +
		integratedRoutingKindMismatchTimeout +
		2*acceleratorprovision.ShutdownWaitTimeout +
		integratedRoutingKindMismatchSettle +
		integratedRoutingKindHelmAssertionTimeout +
		integratedRoutingKindK8sAssertionTimeout +
		integratedRoutingKindZeroClientGrace +
		integratedRoutingKindCleanupTimeout +
		integratedRoutingKindFixtureCleanupTimeout
	// The latest proof-bearing fallback is the resumed Data read. A missing
	// remote success fails immediately, so mismatch and final assertions do not
	// run; cleanup still retains its full coordinator and fixture bounds.
	integratedRoutingKindFailureSequentialTimeout = integratedRoutingKindFixtureSetupTimeout +
		4*integratedRoutingKindDesktopAPITimeout +
		integratedRoutingKindInitialReadinessTimeout +
		(2*integratedRoutingKindListOperationTimeout +
			integratedRoutingKindDataOperationTimeout +
			integratedRoutingKindYAMLOperationTimeout +
			integratedRoutingKindSubscribeOperationTimeout +
			integratedRoutingKindUnsubscribeOperationTimeout) +
		integratedRoutingKindWaitsTimeout +
		integratedRoutingKindWatchMutationTimeout +
		integratedRoutingKindDataTotalTimeout +
		acceleratorprovision.ShutdownWaitTimeout +
		integratedRoutingKindZeroClientGrace +
		integratedRoutingKindCleanupTimeout +
		integratedRoutingKindFixtureCleanupTimeout
	integratedRoutingKindGoTestTimeout = 36 * time.Minute
)

type integratedRoutingKindDiagnosticCode string

const (
	integratedRoutingKindStageSetup             integratedRoutingKindDiagnosticCode = "stage-setup"
	integratedRoutingKindStageDirect            integratedRoutingKindDiagnosticCode = "stage-direct"
	integratedRoutingKindStagePreReady          integratedRoutingKindDiagnosticCode = "stage-pre-ready"
	integratedRoutingKindStageReady             integratedRoutingKindDiagnosticCode = "stage-ready"
	integratedRoutingKindStageList              integratedRoutingKindDiagnosticCode = "stage-list"
	integratedRoutingKindStageCancel            integratedRoutingKindDiagnosticCode = "stage-cancel"
	integratedRoutingKindStageDetail            integratedRoutingKindDiagnosticCode = "stage-detail"
	integratedRoutingKindStageWatch             integratedRoutingKindDiagnosticCode = "stage-watch"
	integratedRoutingKindStageLoss              integratedRoutingKindDiagnosticCode = "stage-loss"
	integratedRoutingKindStageResume            integratedRoutingKindDiagnosticCode = "stage-resume"
	integratedRoutingKindStageMismatch          integratedRoutingKindDiagnosticCode = "stage-mismatch"
	integratedRoutingKindStageIsolation         integratedRoutingKindDiagnosticCode = "stage-isolation"
	integratedRoutingKindStageRelease           integratedRoutingKindDiagnosticCode = "stage-release"
	integratedRoutingKindStageFinalVerification integratedRoutingKindDiagnosticCode = "stage-final-verification"
)

type integratedRoutingKindInitialPhase = integratedRoutingKindDiagnosticCode

const (
	integratedRoutingKindInitialMissing                 integratedRoutingKindInitialPhase = "initial-missing"
	integratedRoutingKindInitialSweeping                integratedRoutingKindInitialPhase = "initial-sweeping"
	integratedRoutingKindInitialResolvingZero           integratedRoutingKindInitialPhase = "initial-resolving-zero"
	integratedRoutingKindInitialResolvingAfterProvision integratedRoutingKindInitialPhase = "initial-resolving-after-provision"
	integratedRoutingKindInitialProvisioning            integratedRoutingKindInitialPhase = "initial-provisioning"
	integratedRoutingKindInitialConnecting              integratedRoutingKindInitialPhase = "initial-connecting"
	integratedRoutingKindInitialActiveClientBind        integratedRoutingKindInitialPhase = "initial-active-client-bind"
	integratedRoutingKindInitialActiveReadyPath         integratedRoutingKindInitialPhase = "initial-active-ready-path"
	integratedRoutingKindInitialUnavailable             integratedRoutingKindInitialPhase = "initial-unavailable"
	integratedRoutingKindInitialTerminal                integratedRoutingKindInitialPhase = "initial-terminal"
	integratedRoutingKindInitialUnknown                 integratedRoutingKindInitialPhase = "initial-unknown"
	integratedRoutingKindInitialCountInvalid            integratedRoutingKindInitialPhase = "initial-count-invalid"
	integratedRoutingKindInitialClientRepeat            integratedRoutingKindInitialPhase = "initial-client-repeat"
	integratedRoutingKindInitialProvisionRetry          integratedRoutingKindInitialPhase = "initial-provision-retry"
	integratedRoutingKindInitialProvisionContextInput   integratedRoutingKindInitialPhase = "initial-provision-context-input"
	integratedRoutingKindInitialProvisionChartPull      integratedRoutingKindInitialPhase = "initial-provision-chart-pull"
	integratedRoutingKindInitialProvisionChartRender    integratedRoutingKindInitialPhase = "initial-provision-chart-integrity-render"
	integratedRoutingKindInitialProvisionInstallPolicy  integratedRoutingKindInitialPhase = "initial-provision-install-conflict-permission"
	integratedRoutingKindInitialProvisionImagePull      integratedRoutingKindInitialPhase = "initial-provision-image-pull"
	integratedRoutingKindInitialProvisionJobPod         integratedRoutingKindInitialPhase = "initial-provision-job-pod"
	integratedRoutingKindInitialProvisionTimeoutCancel  integratedRoutingKindInitialPhase = "initial-provision-timeout-cancel"
	integratedRoutingKindInitialProvisionMixed          integratedRoutingKindInitialPhase = "initial-provision-mixed"
	integratedRoutingKindInitialProvisionUnknown        integratedRoutingKindInitialPhase = "initial-provision-unknown"
	integratedRoutingKindInitialConnectNotEntered       integratedRoutingKindInitialPhase = "initial-connect-not-entered"
	integratedRoutingKindInitialConnectTunnel           integratedRoutingKindInitialPhase = "initial-connect-tunnel"
	integratedRoutingKindInitialConnectAccelerator      integratedRoutingKindInitialPhase = "initial-connect-accelerator"
	integratedRoutingKindInitialConnectVersion          integratedRoutingKindInitialPhase = "initial-connect-version"
	integratedRoutingKindInitialConnectAuthoritative    integratedRoutingKindInitialPhase = "initial-connect-authoritative"
	integratedRoutingKindInitialConnectCancelled        integratedRoutingKindInitialPhase = "initial-connect-cancelled"
	integratedRoutingKindInitialSessionClientBind       integratedRoutingKindInitialPhase = "initial-session-client-bind"
	integratedRoutingKindInitialSessionReadyPath        integratedRoutingKindInitialPhase = "initial-session-ready-path"
	integratedRoutingKindInitialStageMixed              integratedRoutingKindInitialPhase = "initial-stage-mixed"
)

type integratedRoutingKindDiagnostic struct {
	mu       sync.Mutex
	code     integratedRoutingKindDiagnosticCode
	reported bool
}

func newIntegratedRoutingKindDiagnostic(code integratedRoutingKindDiagnosticCode) *integratedRoutingKindDiagnostic {
	if !code.valid() {
		panic("invalid integrated routing diagnostic code")
	}
	return &integratedRoutingKindDiagnostic{code: code}
}

func (d *integratedRoutingKindDiagnostic) set(code integratedRoutingKindDiagnosticCode) {
	if !code.valid() {
		panic("invalid integrated routing diagnostic code")
	}
	d.mu.Lock()
	d.code = code
	d.mu.Unlock()
}

func (d *integratedRoutingKindDiagnostic) report(failed bool, emit func(string)) {
	if !failed || d == nil || emit == nil {
		return
	}
	d.mu.Lock()
	if d.reported {
		d.mu.Unlock()
		return
	}
	d.reported = true
	marker := integratedRoutingKindDiagnosticMarker + string(d.code)
	d.mu.Unlock()
	emit(marker)
}

func (code integratedRoutingKindDiagnosticCode) valid() bool {
	switch code {
	case integratedRoutingKindStageSetup,
		integratedRoutingKindStageDirect,
		integratedRoutingKindStagePreReady,
		integratedRoutingKindStageReady,
		integratedRoutingKindStageList,
		integratedRoutingKindStageCancel,
		integratedRoutingKindStageDetail,
		integratedRoutingKindStageWatch,
		integratedRoutingKindStageLoss,
		integratedRoutingKindStageResume,
		integratedRoutingKindStageMismatch,
		integratedRoutingKindStageIsolation,
		integratedRoutingKindStageRelease,
		integratedRoutingKindStageFinalVerification,
		integratedRoutingKindInitialMissing,
		integratedRoutingKindInitialSweeping,
		integratedRoutingKindInitialResolvingZero,
		integratedRoutingKindInitialResolvingAfterProvision,
		integratedRoutingKindInitialProvisioning,
		integratedRoutingKindInitialConnecting,
		integratedRoutingKindInitialActiveClientBind,
		integratedRoutingKindInitialActiveReadyPath,
		integratedRoutingKindInitialUnavailable,
		integratedRoutingKindInitialTerminal,
		integratedRoutingKindInitialUnknown,
		integratedRoutingKindInitialCountInvalid,
		integratedRoutingKindInitialClientRepeat,
		integratedRoutingKindInitialProvisionRetry,
		integratedRoutingKindInitialProvisionContextInput,
		integratedRoutingKindInitialProvisionChartPull,
		integratedRoutingKindInitialProvisionChartRender,
		integratedRoutingKindInitialProvisionInstallPolicy,
		integratedRoutingKindInitialProvisionImagePull,
		integratedRoutingKindInitialProvisionJobPod,
		integratedRoutingKindInitialProvisionTimeoutCancel,
		integratedRoutingKindInitialProvisionMixed,
		integratedRoutingKindInitialProvisionUnknown,
		integratedRoutingKindInitialConnectNotEntered,
		integratedRoutingKindInitialConnectTunnel,
		integratedRoutingKindInitialConnectAccelerator,
		integratedRoutingKindInitialConnectVersion,
		integratedRoutingKindInitialConnectAuthoritative,
		integratedRoutingKindInitialConnectCancelled,
		integratedRoutingKindInitialSessionClientBind,
		integratedRoutingKindInitialSessionReadyPath,
		integratedRoutingKindInitialStageMixed:
		return true
	default:
		return false
	}
}

type integratedRoutingKindOriginSnapshot struct {
	attempts  [integratedRoutingKindOrigins]int32
	successes [integratedRoutingKindOrigins]int32
}

type integratedRoutingKindOriginProbe struct {
	attempts  [integratedRoutingKindOrigins]atomic.Int32
	successes [integratedRoutingKindOrigins]atomic.Int32
}

func (p *integratedRoutingKindOriginProbe) attempt(origin integratedRoutingKindOrigin) {
	if p != nil {
		p.attempts[origin].Add(1)
	}
}

func (p *integratedRoutingKindOriginProbe) success(origin integratedRoutingKindOrigin) {
	if p != nil {
		p.successes[origin].Add(1)
	}
}

func (p *integratedRoutingKindOriginProbe) snapshot() integratedRoutingKindOriginSnapshot {
	var result integratedRoutingKindOriginSnapshot
	if p == nil {
		return result
	}
	for index := range p.attempts {
		result.attempts[index] = p.attempts[index].Load()
		result.successes[index] = p.successes[index].Load()
	}
	return result
}

func integratedRoutingKindOriginProofMatches(got, want integratedRoutingKindOriginSnapshot) bool {
	return got == want
}

type integratedRoutingKindBoundedSecretReadRouter struct {
	delegate  SecretReadRouter
	overrides [integratedRoutingKindOrigins]time.Duration
}

func integratedRoutingKindOperationTotalTimeout(origin integratedRoutingKindOrigin) time.Duration {
	switch origin {
	case integratedRoutingKindList:
		return integratedRoutingKindListOperationTimeout
	case integratedRoutingKindData:
		return integratedRoutingKindDataTotalTimeout
	case integratedRoutingKindYAML:
		return integratedRoutingKindYAMLTotalTimeout
	case integratedRoutingKindCancel:
		return integratedRoutingKindCancelOperationTimeout
	case integratedRoutingKindSubscribe:
		return integratedRoutingKindSubscribeOperationTimeout
	case integratedRoutingKindUnsubscribe:
		return integratedRoutingKindUnsubscribeOperationTimeout
	default:
		return 0
	}
}

func (r integratedRoutingKindBoundedSecretReadRouter) operationContext(parent context.Context, origin integratedRoutingKindOrigin) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	timeout := r.overrides[origin]
	if timeout <= 0 {
		timeout = integratedRoutingKindOperationTotalTimeout(origin)
	}
	return context.WithTimeout(parent, timeout)
}

func (r integratedRoutingKindBoundedSecretReadRouter) Retain(ctx context.Context, contextName string) {
	r.delegate.Retain(ctx, contextName)
}
func (r integratedRoutingKindBoundedSecretReadRouter) Release() { r.delegate.Release() }
func (r integratedRoutingKindBoundedSecretReadRouter) FenceContextSwitch(contextName string) {
	r.delegate.FenceContextSwitch(contextName)
}
func (r integratedRoutingKindBoundedSecretReadRouter) ContextSwitched(contextName string, available bool) {
	r.delegate.ContextSwitched(contextName, available)
}
func (r integratedRoutingKindBoundedSecretReadRouter) ListSecretsMetadata(ctx context.Context, token SecretReadSourceToken, requestID, namespace string, exclude bool) ([]k8s.SecretListItem, error) {
	op, cancel := r.operationContext(ctx, integratedRoutingKindList)
	defer cancel()
	return r.delegate.ListSecretsMetadata(op, token, requestID, namespace, exclude)
}
func (r integratedRoutingKindBoundedSecretReadRouter) GetSecretData(ctx context.Context, token SecretReadSourceToken, namespace, name string) ([]k8s.DataEntry, error) {
	op, cancel := r.operationContext(ctx, integratedRoutingKindData)
	defer cancel()
	return r.delegate.GetSecretData(op, token, namespace, name)
}
func (r integratedRoutingKindBoundedSecretReadRouter) GetSecretYaml(ctx context.Context, token SecretReadSourceToken, namespace, name string) (string, error) {
	op, cancel := r.operationContext(ctx, integratedRoutingKindYAML)
	defer cancel()
	return r.delegate.GetSecretYaml(op, token, namespace, name)
}
func (r integratedRoutingKindBoundedSecretReadRouter) CancelListRequest(ctx context.Context, token SecretReadSourceToken, requestID string) (bool, error) {
	op, cancel := r.operationContext(ctx, integratedRoutingKindCancel)
	defer cancel()
	return r.delegate.CancelListRequest(op, token, requestID)
}
func (r integratedRoutingKindBoundedSecretReadRouter) SubscribeSecretWatcher(ctx context.Context, token SecretReadSourceToken, namespace string, exclude bool) (acceleratorsecret.SecretWatchSubscription, error) {
	op, cancel := r.operationContext(ctx, integratedRoutingKindSubscribe)
	defer cancel()
	return r.delegate.SubscribeSecretWatcher(op, token, namespace, exclude)
}
func (r integratedRoutingKindBoundedSecretReadRouter) UnsubscribeSecretWatcher(ctx context.Context, token SecretReadSourceToken, id acceleratorsecret.SecretWatchSpecID) error {
	op, cancel := r.operationContext(ctx, integratedRoutingKindUnsubscribe)
	defer cancel()
	return r.delegate.UnsubscribeSecretWatcher(op, token, id)
}
func (r integratedRoutingKindBoundedSecretReadRouter) Quiesce(ctx context.Context) {
	r.delegate.Quiesce(ctx)
}
func (r integratedRoutingKindBoundedSecretReadRouter) StopProducers(ctx context.Context) {
	r.delegate.StopProducers(ctx)
}
func (r integratedRoutingKindBoundedSecretReadRouter) Close(ctx context.Context) {
	r.delegate.Close(ctx)
}

func installIntegratedRoutingKindOperationBounds(app *App) {
	if app == nil {
		return
	}
	production := app.integratedSecretReads()
	app.agentRouter = secretReadAgentRouter{base: app.agentRouter, secrets: integratedRoutingKindBoundedSecretReadRouter{delegate: production}}
}

type integratedRoutingKindSecretClient struct {
	client      acceleratorprovision.SecretRPCClient
	probe       *integratedRoutingKindOriginProbe
	listGate    *sync.Once
	listEntered chan<- struct{}
	listRelease <-chan struct{}
}

func (c integratedRoutingKindSecretClient) ListSecretsMetadata(ctx context.Context, requestID, namespace string, exclude bool) ([]k8s.SecretListItem, error) {
	c.probe.attempt(integratedRoutingKindList)
	if c.listGate != nil {
		released := true
		c.listGate.Do(func() {
			close(c.listEntered)
			select {
			case <-c.listRelease:
			case <-ctx.Done():
				released = false
			}
		})
		if !released {
			return nil, ctx.Err()
		}
	}
	items, err := c.client.ListSecretsMetadata(ctx, requestID, namespace, exclude)
	if err == nil {
		c.probe.success(integratedRoutingKindList)
	}
	return items, err
}
func (c integratedRoutingKindSecretClient) GetSecretData(ctx context.Context, namespace, name string) ([]k8s.DataEntry, error) {
	c.probe.attempt(integratedRoutingKindData)
	data, err := c.client.GetSecretData(ctx, namespace, name)
	if err == nil {
		c.probe.success(integratedRoutingKindData)
	}
	return data, err
}
func (c integratedRoutingKindSecretClient) GetSecretYaml(ctx context.Context, namespace, name string) (string, error) {
	c.probe.attempt(integratedRoutingKindYAML)
	value, err := c.client.GetSecretYaml(ctx, namespace, name)
	if err == nil {
		c.probe.success(integratedRoutingKindYAML)
	}
	return value, err
}
func (c integratedRoutingKindSecretClient) CancelListRequest(ctx context.Context, requestID string) (bool, error) {
	c.probe.attempt(integratedRoutingKindCancel)
	canceled, err := c.client.CancelListRequest(ctx, requestID)
	if err == nil {
		c.probe.success(integratedRoutingKindCancel)
	}
	return canceled, err
}
func (c integratedRoutingKindSecretClient) SubscribeSecretWatcher(ctx context.Context, namespace string, exclude bool) (acceleratorsecret.SecretWatchSubscription, acceleratorprovision.SecretWatchLease, error) {
	c.probe.attempt(integratedRoutingKindSubscribe)
	subscription, lease, err := c.client.SubscribeSecretWatcher(ctx, namespace, exclude)
	if err == nil && lease != nil && subscription.WatcherSpecID != "" {
		c.probe.success(integratedRoutingKindSubscribe)
	}
	return subscription, lease, err
}
func (c integratedRoutingKindSecretClient) UnsubscribeSecretWatcher(ctx context.Context, id acceleratorsecret.SecretWatchSpecID) error {
	c.probe.attempt(integratedRoutingKindUnsubscribe)
	err := c.client.UnsubscribeSecretWatcher(ctx, id)
	if err == nil {
		c.probe.success(integratedRoutingKindUnsubscribe)
	}
	return err
}
func (c integratedRoutingKindSecretClient) Done() <-chan struct{} { return c.client.Done() }
func (c integratedRoutingKindSecretClient) Reason() acceleratorsecret.SecretClientReason {
	return c.client.Reason()
}
func (c integratedRoutingKindSecretClient) Close(ctx context.Context) { c.client.Close(ctx) }

type integratedRoutingKindRegistryTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (t integratedRoutingKindRegistryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Scheme != "https" || request.URL.Host != "ghcr.io" {
		return nil, errors.New("unexpected production registry request")
	}
	clone := request.Clone(request.Context())
	copyURL := *request.URL
	clone.URL = &copyURL
	clone.URL.Scheme, clone.URL.Host, clone.Host = t.target.Scheme, t.target.Host, ""
	return t.base.RoundTrip(clone)
}

type integratedRoutingKindAcceptanceProof struct {
	integratedHappy     bool
	sixOperations       bool
	valueFreeBoundaries bool
	immediateDirect     bool
	resumedHigherSource bool
	staleSourceRejected bool
	mismatchDirect      bool
	mismatchAttempts    int
	mismatchNoThird     bool
	elapsed             time.Duration
	reconnectCancelled  bool
	singleExpiry        bool
	loopback            bool
	privacy             bool
	rbac                bool
	ownedCleanup        bool
	sentinel            bool
	sweep               bool
}

func TestAcceleratorIntegratedRoutingKind(t *testing.T) {
	runAcceleratorIntegratedRoutingKind(t)
}

func runAcceleratorIntegratedRoutingKind(t *testing.T) integratedRoutingKindAcceptanceProof {
	var (
		snapshot                        *k8s.AcceleratorContextSnapshot
		fixtureName, fixtureNamespace   string
		secretCreated, configMapCreated bool
		app, mismatchApp                *App
		healthyClosed, mismatchClosed   bool
	)
	// Register bounded teardown first. The diagnostic reporter registered next
	// runs before it by LIFO, so a body failure is classified without waiting
	// for lifecycle shutdown. Errors raised only by teardown remain generic.
	t.Cleanup(func() {
		if mismatchApp != nil && !mismatchClosed {
			mismatchApp.ReleaseIntegratedSecretReads()
			mismatchApp.lifecycle.Close(context.Background())
		}
		if app != nil && !healthyClosed {
			app.ReleaseIntegratedSecretReads()
			app.lifecycle.Close(context.Background())
		}
		if snapshot == nil || fixtureName == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), integratedRoutingKindFixtureCleanupTimeout)
		defer cancel()
		if secretCreated {
			_ = snapshot.Clientset().CoreV1().Secrets(fixtureNamespace).Delete(ctx, fixtureName, metav1.DeleteOptions{})
			if _, getErr := snapshot.Clientset().CoreV1().Secrets(fixtureNamespace).Get(ctx, fixtureName, metav1.GetOptions{}); !apierrors.IsNotFound(getErr) {
				t.Error("integrated routing Secret fixture cleanup failed")
			}
		}
		if configMapCreated {
			_ = snapshot.Clientset().CoreV1().ConfigMaps(fixtureNamespace).Delete(ctx, fixtureName, metav1.DeleteOptions{})
			if _, getErr := snapshot.Clientset().CoreV1().ConfigMaps(fixtureNamespace).Get(ctx, fixtureName, metav1.GetOptions{}); !apierrors.IsNotFound(getErr) {
				t.Error("integrated routing ConfigMap fixture cleanup failed")
			}
		}
	})
	diagnostic := newIntegratedRoutingKindDiagnostic(integratedRoutingKindStageSetup)
	t.Cleanup(func() {
		diagnostic.report(t.Failed(), func(marker string) { t.Log(marker) })
	})

	diagnostic.set(integratedRoutingKindStageSetup)
	chartDigest := requiredIntegratedRoutingKindEnv(t, "ACCELERATOR_PROVISION_KIND_CHART_DIGEST")
	diagnostic.set(integratedRoutingKindStageSetup)
	mismatchChartDigest := requiredIntegratedRoutingKindEnv(t, "ACCELERATOR_PROVISION_KIND_MISMATCH_CHART_DIGEST")
	diagnostic.set(integratedRoutingKindStageSetup)
	imageDigest := requiredIntegratedRoutingKindEnv(t, "ACCELERATOR_PROVISION_KIND_IMAGE_DIGEST")
	diagnostic.set(integratedRoutingKindStageSetup)
	sentinel := requiredIntegratedRoutingKindEnv(t, "ACCELERATOR_PROVISION_KIND_SENTINEL")
	diagnostic.set(integratedRoutingKindStageSetup)
	malformed := requiredIntegratedRoutingKindEnv(t, "ACCELERATOR_PROVISION_KIND_MALFORMED")
	diagnostic.set(integratedRoutingKindStageSetup)
	registryURL, err := url.Parse(requiredIntegratedRoutingKindEnv(t, "ACCELERATOR_PROVISION_KIND_REGISTRY_TLS_URL"))
	if err != nil || (registryURL.Scheme != "https" && registryURL.Scheme != "http") || registryURL.Host == "" ||
		(registryURL.Scheme == "http" && os.Getenv("KUBIKLES_ACCELERATOR_E2E_REUSE") != "1") {
		t.Fatal("integrated routing registry fixture invalid")
	}
	pool := x509.NewCertPool()
	if registryURL.Scheme == "https" {
		diagnostic.set(integratedRoutingKindStageSetup)
		certificate, readErr := os.ReadFile(requiredIntegratedRoutingKindEnv(t, "ACCELERATOR_PROVISION_KIND_REGISTRY_CA"))
		if readErr != nil {
			t.Fatal("integrated routing registry CA unavailable")
		}
		if !pool.AppendCertsFromPEM(certificate) {
			t.Fatal("integrated routing registry CA invalid")
		}
	}
	restoreTransport := helm.SetAcceleratorRegistryTransportForTest(func(base http.RoundTripper) http.RoundTripper {
		transport := base.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		return integratedRoutingKindRegistryTransport{base: transport, target: registryURL}
	})
	defer restoreTransport()

	diagnostic.set(integratedRoutingKindStageSetup)
	client, err := k8s.NewClient()
	if err != nil || client.GetCurrentContext() == "" {
		t.Fatal("integrated routing desktop context unavailable")
	}
	// Direct App calls do not accept a caller context. Make their existing
	// Kubernetes API deadline explicit for this harness.
	client.SetAPITimeout(integratedRoutingKindDesktopAPITimeout)
	contextName := client.GetCurrentContext()
	diagnostic.set(integratedRoutingKindStageSetup)
	snapshot, err = client.SnapshotCurrentContext(contextName)
	if err != nil || snapshot.Namespace() == "" {
		t.Fatal("integrated routing context snapshot unavailable")
	}
	fixtureNamespace = snapshot.Namespace()
	diagnostic.set(integratedRoutingKindStageSetup)
	fixtureName = "integrated-routing-" + integratedRoutingKindSuffix(t)
	fixtureSetupCtx, fixtureSetupCancel := context.WithTimeout(context.Background(), integratedRoutingKindFixtureSetupTimeout)
	defer fixtureSetupCancel()
	diagnostic.set(integratedRoutingKindStageSetup)
	if _, err = snapshot.Clientset().CoreV1().Secrets(fixtureNamespace).Create(fixtureSetupCtx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: fixtureName, Namespace: fixtureNamespace, Labels: map[string]string{"routing.test/origin": "fixture"}, Annotations: map[string]string{"routing.test/note": "preserved"}},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"token": []byte("explicit-detail-value")},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal("integrated routing Secret fixture create failed")
	}
	secretCreated = true
	diagnostic.set(integratedRoutingKindStageSetup)
	if _, err = snapshot.Clientset().CoreV1().ConfigMaps(fixtureNamespace).Create(fixtureSetupCtx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: fixtureName, Namespace: fixtureNamespace}, Data: map[string]string{"tripwire": "direct"},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal("integrated routing ConfigMap fixture create failed")
	}
	configMapCreated = true
	fixtureSetupCancel()

	originalBuildVersion := BuildVersion
	originalConfigDir := desktopUserConfigDir
	originalCoordinatorFactory := desktopAcceleratorCoordinatorFactory
	originalClientFactory := desktopAcceleratorSecretClientFactory
	defer func() {
		BuildVersion = originalBuildVersion
		desktopUserConfigDir = originalConfigDir
		desktopAcceleratorCoordinatorFactory = originalCoordinatorFactory
		desktopAcceleratorSecretClientFactory = originalClientFactory
	}()
	desktopUserConfigDir = func() (string, error) { return t.TempDir(), nil }

	stableBuildVersion := os.Getenv("BUILD_VERSION")
	if stableBuildVersion == "" {
		stableBuildVersion = "v1.2.3"
	}
	if os.Getenv("KUBIKLES_ACCELERATOR_E2E_REUSE") == "1" && stableBuildVersion != "v0.0.0" {
		t.Fatal("integrated routing acceptance build version invalid")
	}
	resolution := integratedRoutingKindResolution(stableBuildVersion, chartDigest, imageDigest)
	var coordinator *acceleratorprovision.Coordinator
	var coordinatorProbe *acceleratorprovision.IntegratedRoutingKindCoordinatorProbe
	desktopAcceleratorCoordinatorFactory = func(k8sClient *k8s.Client, _ *acceleratorrelease.Resolver, provisioner *acceleratorprovision.Service, connector *acceleratorprovision.Connector, reconnector *acceleratorprovision.Reconnector, disposer *acceleratorprovision.DisposalService) desktopAcceleratorCoordinator {
		coordinator, coordinatorProbe = acceleratorprovision.NewIntegratedRoutingKindCoordinator(k8sClient, resolution, provisioner, connector, reconnector, disposer)
		return coordinator
	}

	ready := make(chan SecretReadSourceToken, 8)
	unavailable := make(chan SecretReadSourceToken, 8)
	resources := make(chan integratedSecretResourceSignal, 8)
	emitter := events.EmitterFunc(func(name string, data ...interface{}) {
		if len(data) != 1 {
			return
		}
		switch name {
		case integratedSecretReadyEvent:
			if signal, ok := data[0].(integratedSecretSourceSignal); ok {
				ready <- signal.SourceToken
			}
		case integratedSecretUnavailableEvent:
			if signal, ok := data[0].(integratedSecretSourceSignal); ok {
				unavailable <- signal.SourceToken
			}
		case integratedSecretResourceEvent:
			if signal, ok := data[0].(integratedSecretResourceSignal); ok {
				resources <- signal
			}
		}
	})

	originProbe := &integratedRoutingKindOriginProbe{}
	listEntered := make(chan struct{})
	listRelease := make(chan struct{})
	listGate := &sync.Once{}
	var clientConstructions atomic.Int32
	desktopAcceleratorSecretClientFactory = func(lease *acceleratorprovision.SessionLease) (acceleratorprovision.SecretRPCClient, error) {
		created, createErr := acceleratorprovision.NewSecretRPCClient(lease)
		if createErr != nil {
			return nil, createErr
		}
		if clientConstructions.Add(1) == 1 {
			return integratedRoutingKindSecretClient{client: created, probe: originProbe, listGate: listGate, listEntered: listEntered, listRelease: listRelease}, nil
		}
		return integratedRoutingKindSecretClient{client: created, probe: originProbe}, nil
	}
	diagnostic.set(integratedRoutingKindStageSetup)
	BuildVersion = stableBuildVersion
	helmClient := helm.NewClient()
	app = &App{runtimeMode: RuntimeModeDesktop, ctx: context.Background(), k8sClient: client, helmClient: helmClient, agentRouter: NoopAgentRouter{}, lifecycle: NoopRuntimeLifecycle{}, emitter: emitter}
	initializeDesktopAcceleratorLifecycle(app)
	installIntegratedRoutingKindOperationBounds(app)

	var expectedOrigin integratedRoutingKindOriginSnapshot
	diagnostic.set(integratedRoutingKindStagePreReady)
	app.RetainIntegratedSecretReads()
	diagnostic.set(integratedRoutingKindStageDirect)
	if _, directErr := app.GetSecretYaml(fixtureNamespace, fixtureName); directErr != nil {
		t.Fatal("integrated routing initial Direct tripwire failed")
	}
	diagnostic.set(integratedRoutingKindStageDirect)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing initial Direct origin proof failed")
	// Retained Secret demand must stay Direct until the connection owner
	// explicitly enables its Accelerator lifecycle.
	coordinator.Enable(contextName, "")
	diagnostic.set(integratedRoutingKindStageReady)
	firstToken := waitIntegratedRoutingKindInitialToken(t, diagnostic, ready, coordinator, coordinatorProbe, contextName, &clientConstructions)

	diagnostic.set(integratedRoutingKindStageList)
	listDone := make(chan error, 1)
	go func() {
		_, listErr := app.ListIntegratedSecretsMetadata(string(firstToken), "cancel-owner", fixtureNamespace, false)
		listDone <- listErr
	}()
	diagnostic.set(integratedRoutingKindStageList)
	waitIntegratedRoutingKindSignal(t, listEntered, integratedRoutingKindListBarrierTimeout, "integrated routing list barrier failed")
	expectedOrigin.attempts[integratedRoutingKindList]++
	diagnostic.set(integratedRoutingKindStageList)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing canceled List attempt proof failed")
	// The gate holds before the underlying remote-client List registration, so
	// false is expected. Nil error proves the routed Cancel completion; the
	// production router still cancels its locally registered request owner.
	diagnostic.set(integratedRoutingKindStageCancel)
	canceled, cancelErr := app.CancelIntegratedSecretListRequest(string(firstToken), "cancel-owner")
	if cancelErr != nil || canceled {
		t.Fatal("integrated routing remote cancel failed")
	}
	expectedOrigin.attempts[integratedRoutingKindCancel]++
	expectedOrigin.successes[integratedRoutingKindCancel]++
	diagnostic.set(integratedRoutingKindStageCancel)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing remote cancel completion proof failed")
	close(listRelease)
	diagnostic.set(integratedRoutingKindStageCancel)
	if listErr := waitIntegratedRoutingKindListCompletion(t, listDone); listErr == nil {
		t.Fatal("integrated routing canceled list unexpectedly committed")
	}
	diagnostic.set(integratedRoutingKindStageCancel)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing canceled List success proof failed")
	diagnostic.set(integratedRoutingKindStageList)
	items, err := app.ListIntegratedSecretsMetadata(string(firstToken), "list-owner", fixtureNamespace, false)
	expectedOrigin.attempts[integratedRoutingKindList]++
	expectedOrigin.successes[integratedRoutingKindList]++
	diagnostic.set(integratedRoutingKindStageList)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing remote List success proof failed")
	if err != nil || !integratedRoutingKindContainsSecret(items, fixtureNamespace, fixtureName) {
		t.Fatal("integrated routing remote list failed")
	}
	diagnostic.set(integratedRoutingKindStageDetail)
	data, err := app.GetIntegratedSecretData(string(firstToken), fixtureNamespace, fixtureName)
	expectedOrigin.attempts[integratedRoutingKindData]++
	expectedOrigin.successes[integratedRoutingKindData]++
	diagnostic.set(integratedRoutingKindStageDetail)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing remote Data success proof failed")
	if err != nil || !integratedRoutingKindContainsData(data) {
		t.Fatal("integrated routing remote data failed")
	}
	diagnostic.set(integratedRoutingKindStageDetail)
	yamlValue, err := app.GetIntegratedSecretYaml(string(firstToken), fixtureNamespace, fixtureName)
	expectedOrigin.attempts[integratedRoutingKindYAML]++
	expectedOrigin.successes[integratedRoutingKindYAML]++
	diagnostic.set(integratedRoutingKindStageDetail)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing remote YAML success proof failed")
	if err != nil || !integratedRoutingKindYAMLExact(yamlValue) {
		t.Fatal("integrated routing remote YAML failed")
	}
	diagnostic.set(integratedRoutingKindStageWatch)
	watcherSpecID, err := app.SubscribeIntegratedSecretWatcher(string(firstToken), fixtureNamespace, false)
	expectedOrigin.attempts[integratedRoutingKindSubscribe]++
	expectedOrigin.successes[integratedRoutingKindSubscribe]++
	diagnostic.set(integratedRoutingKindStageWatch)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing remote Subscribe success proof failed")
	if err != nil || watcherSpecID == "" {
		t.Fatal("integrated routing remote subscribe failed")
	}
	watchMutationCtx, watchMutationCancel := context.WithTimeout(context.Background(), integratedRoutingKindWatchMutationTimeout)
	defer watchMutationCancel()
	diagnostic.set(integratedRoutingKindStageWatch)
	currentSecret, err := snapshot.Clientset().CoreV1().Secrets(fixtureNamespace).Get(watchMutationCtx, fixtureName, metav1.GetOptions{})
	if err != nil {
		t.Fatal("integrated routing watch mutation read failed")
	}
	currentSecret = currentSecret.DeepCopy()
	currentSecret.Labels["routing.test/watch"] = "changed"
	diagnostic.set(integratedRoutingKindStageWatch)
	if _, err = snapshot.Clientset().CoreV1().Secrets(fixtureNamespace).Update(watchMutationCtx, currentSecret, metav1.UpdateOptions{}); err != nil {
		t.Fatal("integrated routing watch mutation failed")
	}
	watchMutationCancel()
	diagnostic.set(integratedRoutingKindStageWatch)
	resource := waitIntegratedRoutingKindResource(t, resources, firstToken, acceleratorsecret.SecretWatchSpecID(watcherSpecID), integratedRoutingKindWatchEventTimeout)
	if resource.Resource.Metadata.Name != fixtureName || resource.Resource.DataKeys != 1 {
		t.Fatal("integrated routing remote watch projection failed")
	}
	diagnostic.set(integratedRoutingKindStageWatch)
	if err = app.UnsubscribeIntegratedSecretWatcher(string(firstToken), watcherSpecID); err != nil {
		t.Fatal("integrated routing remote unsubscribe failed")
	}
	expectedOrigin.attempts[integratedRoutingKindUnsubscribe]++
	expectedOrigin.successes[integratedRoutingKindUnsubscribe]++
	diagnostic.set(integratedRoutingKindStageWatch)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing remote Unsubscribe success proof failed")
	proofAfterSix := expectedOrigin
	diagnostic.set(integratedRoutingKindStageIsolation)
	if _, err = app.GetConfigMapYaml(fixtureNamespace, fixtureName); err != nil {
		t.Fatal("integrated routing non-Secret Direct read failed")
	}
	diagnostic.set(integratedRoutingKindStageIsolation)
	if err = app.UpdateSecretData(fixtureNamespace, fixtureName, []k8s.DataEntry{{Key: "token", Value: "mutated-direct", Source: k8s.DataEntrySourceData, Encoding: k8s.DataEntryEncodingText}}); err != nil {
		t.Fatal("integrated routing Direct mutation failed")
	}
	diagnostic.set(integratedRoutingKindStageIsolation)
	if !integratedRoutingKindOriginProofMatches(originProbe.snapshot(), proofAfterSix) {
		t.Fatal("integrated routing Direct operation reached Accelerator")
	}

	diagnostic.set(integratedRoutingKindStageLoss)
	if err = acceleratorprovision.ForceIntegratedRoutingKindTransportLoss(coordinator, contextName); err != nil {
		t.Fatal("integrated routing forced loss failed")
	}
	diagnostic.set(integratedRoutingKindStageLoss)
	lostToken := waitIntegratedRoutingKindToken(t, unavailable, integratedRoutingKindUnavailableTimeout, "integrated routing immediate Direct transition failed")
	if lostToken != firstToken {
		t.Fatal("integrated routing unavailable generation mismatch")
	}
	diagnostic.set(integratedRoutingKindStageLoss)
	if _, err = app.GetSecretYaml(fixtureNamespace, fixtureName); err != nil || !integratedRoutingKindOriginProofMatches(originProbe.snapshot(), proofAfterSix) {
		t.Fatal("integrated routing loss Direct tripwire failed")
	}
	diagnostic.set(integratedRoutingKindStageLoss)
	if _, err = app.GetIntegratedSecretYaml(string(firstToken), fixtureNamespace, fixtureName); err == nil {
		t.Fatal("integrated routing stale generation served")
	}
	diagnostic.set(integratedRoutingKindStageLoss)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing stale generation reached remote client")
	diagnostic.set(integratedRoutingKindStageResume)
	secondToken := waitIntegratedRoutingKindToken(t, ready, integratedRoutingKindResumeTimeout, "integrated routing resumed Accelerator readiness failed")
	if secondToken == firstToken {
		t.Fatal("integrated routing resumed token was reused")
	}
	diagnostic.set(integratedRoutingKindStageResume)
	data, err = app.GetIntegratedSecretData(string(secondToken), fixtureNamespace, fixtureName)
	expectedOrigin.attempts[integratedRoutingKindData]++
	expectedOrigin.successes[integratedRoutingKindData]++
	diagnostic.set(integratedRoutingKindStageResume)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing resumed Data success proof failed")
	if err != nil || len(data) != 1 || data[0].Value != "mutated-direct" {
		t.Fatal("integrated routing new generation remote read failed")
	}
	if coordinatorProbe == nil || coordinatorProbe.ProvisionAttempts() != 1 || clientConstructions.Load() != 2 {
		t.Fatal("integrated routing healthy lifecycle counts failed")
	}
	diagnostic.set(integratedRoutingKindStageRelease)
	activeWorkload, active := coordinatorProbe.ActiveWorkload()
	if !active {
		t.Fatal("integrated routing active workload identity unavailable")
	}
	app.ReleaseIntegratedSecretReads()
	if retained := coordinator.Snapshot(contextName); retained.State != acceleratorprovision.CoordinatorActive || !retained.Enabled || retained.DemandCount != 0 || !retained.Available {
		t.Fatal("integrated routing enabled workload did not retain its zero-demand session")
	}
	// Model an ungraceful desktop exit without invoking Disable & Remove. The
	// connection-owned workload must remain in place while its acceptance
	// runtime observes the real final-authenticated-client reconnect grace.
	graceStarted := time.Now()
	app.lifecycle.Quiesce(context.Background())
	stopCtx, stopCancel := context.WithTimeout(context.Background(), acceleratorprovision.ShutdownWaitTimeout)
	app.lifecycle.StopProducers(stopCtx)
	stopCancel()
	graceElapsed := waitIntegratedAuthenticatedClientGraceExpiry(t, snapshot, activeWorkload, graceStarted)
	lowerGrace, upperGrace := integratedRoutingKindGraceBounds()
	if graceElapsed < lowerGrace || graceElapsed > upperGrace {
		t.Fatal("integrated routing authenticated client grace outside boundary")
	}
	if coordinatorProbe.ProvisionAttempts() != 1 {
		t.Fatal("integrated routing authenticated client grace created another workload")
	}
	app.lifecycle.Close(context.Background())
	healthyClosed = true
	waitIntegratedOwnedCleanup(t, helmClient, snapshot, coordinatorProbe, activeWorkload, sentinel, malformed)
	if coordinatorProbe.TerminalCleanupStartCount() != 0 || coordinatorProbe.TerminalCleanupCount() != 0 {
		t.Fatal("integrated routing desktop exit used the obsolete demand-owned cleanup path")
	}
	healthyProbe := coordinatorProbe

	mismatchReady := make(chan SecretReadSourceToken, 1)
	var mismatchClientConstructions atomic.Int32
	mismatchBuildVersion := "v1.2.4"
	if stableBuildVersion == "v0.0.0" {
		mismatchBuildVersion = "v0.0.1"
	}
	resolution = integratedRoutingKindResolution(mismatchBuildVersion, mismatchChartDigest, imageDigest)
	desktopAcceleratorSecretClientFactory = func(lease *acceleratorprovision.SessionLease) (acceleratorprovision.SecretRPCClient, error) {
		mismatchClientConstructions.Add(1)
		return acceleratorprovision.NewSecretRPCClient(lease)
	}
	BuildVersion = mismatchBuildVersion
	diagnostic.set(integratedRoutingKindStageMismatch)
	mismatchApp = &App{runtimeMode: RuntimeModeDesktop, ctx: context.Background(), k8sClient: client, helmClient: helmClient, agentRouter: NoopAgentRouter{}, lifecycle: NoopRuntimeLifecycle{}, emitter: events.EmitterFunc(func(name string, data ...interface{}) {
		if name == integratedSecretReadyEvent && len(data) == 1 {
			if signal, ok := data[0].(integratedSecretSourceSignal); ok {
				mismatchReady <- signal.SourceToken
			}
		}
	})}
	initializeDesktopAcceleratorLifecycle(mismatchApp)
	installIntegratedRoutingKindOperationBounds(mismatchApp)
	mismatchCoordinator := coordinator
	mismatchProbe := coordinatorProbe
	diagnostic.set(integratedRoutingKindStageMismatch)
	mismatchApp.RetainIntegratedSecretReads()
	if _, err = mismatchApp.GetSecretYaml(fixtureNamespace, fixtureName); err != nil {
		t.Fatal("integrated routing mismatch Direct tripwire failed")
	}
	diagnostic.set(integratedRoutingKindStageMismatch)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing mismatch Direct origin proof failed")
	// Mismatch recovery is connection-owned just like the healthy path. Browser
	// demand alone must remain Direct until its owner explicitly enables it.
	mismatchCoordinator.Enable(contextName, "")
	diagnostic.set(integratedRoutingKindStageMismatch)
	waitIntegratedRoutingKindMismatch(t, mismatchCoordinator, mismatchProbe, contextName, integratedRoutingKindMismatchTimeout)
	select {
	case <-mismatchReady:
		t.Fatal("integrated routing mismatch published Accelerator")
	default:
	}
	if mismatchClientConstructions.Load() != 0 || mismatchProbe == nil || mismatchProbe.ProvisionAttempts() != 2 {
		t.Fatal("integrated routing mismatch replacement bound failed")
	}
	diagnostic.set(integratedRoutingKindStageMismatch)
	time.Sleep(integratedRoutingKindMismatchSettle)
	if mismatchProbe.ProvisionAttempts() != 2 || mismatchCoordinator.Snapshot(contextName).State != acceleratorprovision.CoordinatorUnavailable {
		t.Fatal("integrated routing mismatch created a third workload")
	}
	if _, err = mismatchApp.GetSecretYaml(fixtureNamespace, fixtureName); err != nil {
		t.Fatal("integrated routing mismatch did not remain Direct")
	}
	diagnostic.set(integratedRoutingKindStageMismatch)
	assertIntegratedRoutingKindOriginProof(t, originProbe, expectedOrigin, "integrated routing mismatch latch origin proof failed")
	diagnostic.set(integratedRoutingKindStageRelease)
	mismatchApp.ReleaseIntegratedSecretReads()
	mismatchApp.lifecycle.Close(context.Background())
	mismatchClosed = true

	diagnostic.set(integratedRoutingKindStageFinalVerification)
	assertIntegratedRoutingKindFinalCleanup(t, helmClient, snapshot, sentinel, malformed)
	return integratedRoutingKindAcceptanceProof{
		integratedHappy: true, sixOperations: true, valueFreeBoundaries: true,
		immediateDirect: true, resumedHigherSource: true, staleSourceRejected: true,
		mismatchDirect: true, mismatchAttempts: mismatchProbe.ProvisionAttempts(), mismatchNoThird: true,
		elapsed: graceElapsed, reconnectCancelled: healthyProbe.ProvisionAttempts() == 1 && clientConstructions.Load() == 2, singleExpiry: true,
		loopback: true, privacy: true, rbac: true, ownedCleanup: true, sentinel: true, sweep: true,
	}
}

func requiredIntegratedRoutingKindEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatal("integrated routing mandatory Kind fixture missing")
	}
	return value
}

func integratedRoutingKindSuffix(t *testing.T) string {
	t.Helper()
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		t.Fatal("integrated routing fixture entropy unavailable")
	}
	return hex.EncodeToString(value)
}

func integratedRoutingKindResolution(version, chartDigest, imageDigest string) acceleratorrelease.Resolution {
	return acceleratorrelease.Resolution{Availability: acceleratorrelease.Available, Source: acceleratorrelease.SourceNetwork, Release: acceleratorrelease.VerifiedRelease{
		BuildVersion: version, SourceCommit: strings.Repeat("c", 40), DescriptorSHA256: strings.Repeat("d", 64),
		ImageReference: "ghcr.io/skrobylabs/kubikles-accelerator@" + imageDigest,
		ChartReference: integratedRoutingKindChartRepository + "@" + chartDigest,
	}}
}

func waitIntegratedRoutingKindToken(t *testing.T, channel <-chan SecretReadSourceToken, timeout time.Duration, failure string) SecretReadSourceToken {
	t.Helper()
	select {
	case token := <-channel:
		if token == "" {
			t.Fatal(failure)
		}
		return token
	case <-time.After(timeout):
		t.Fatal(failure)
		return ""
	}
}

func waitIntegratedRoutingKindInitialToken(t *testing.T, diagnostic *integratedRoutingKindDiagnostic, channel <-chan SecretReadSourceToken, coordinator *acceleratorprovision.Coordinator, probe *acceleratorprovision.IntegratedRoutingKindCoordinatorProbe, contextName string, clientConstructions *atomic.Int32) SecretReadSourceToken {
	t.Helper()
	timer := time.NewTimer(integratedRoutingKindInitialReadinessTimeout)
	defer timer.Stop()
	select {
	case token := <-channel:
		if token == "" {
			t.Fatal("integrated routing initial Accelerator readiness failed")
		}
		return token
	case <-timer.C:
		phase := classifyIntegratedRoutingKindInitialPhase(coordinator, probe, contextName, clientConstructions)
		diagnostic.set(phase)
		t.Fatal("integrated routing initial Accelerator readiness failed")
		return ""
	}
}

func classifyIntegratedRoutingKindInitialPhase(coordinator *acceleratorprovision.Coordinator, probe *acceleratorprovision.IntegratedRoutingKindCoordinatorProbe, contextName string, clientConstructions *atomic.Int32) integratedRoutingKindInitialPhase {
	if coordinator == nil || probe == nil || contextName == "" || clientConstructions == nil {
		return integratedRoutingKindInitialMissing
	}
	return classifyIntegratedRoutingKindInitialSnapshot(coordinator.Snapshot(contextName).State, probe.Snapshot(), clientConstructions.Load())
}

func classifyIntegratedRoutingKindInitialSnapshot(state acceleratorprovision.CoordinatorState, stages acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot, clientConstructions int32) integratedRoutingKindInitialPhase {
	if clientConstructions > 1 {
		return integratedRoutingKindInitialClientRepeat
	}
	if stages.ProvisionAttempts < 0 || clientConstructions < 0 {
		return integratedRoutingKindInitialCountInvalid
	}
	provisionOutcomes := countIntegratedRoutingKindStages(stages.ProvisionAvailable, stages.ProvisionUnavailable)
	provisionReasons := countIntegratedRoutingKindStages(
		stages.ProvisionContextInput,
		stages.ProvisionChartPull,
		stages.ProvisionChartIntegrityRender,
		stages.ProvisionInstallPolicy,
		stages.ProvisionImagePull,
		stages.ProvisionJobPod,
		stages.ProvisionTimeoutCancel,
		stages.ProvisionUnknown,
	)
	connectOutcomes := countIntegratedRoutingKindStages(
		stages.ConnectAvailable,
		stages.ConnectTunnelUnavailable,
		stages.ConnectAcceleratorUnavailable,
		stages.ConnectVersionMismatch,
		stages.ConnectAuthoritative,
		stages.ConnectCancelled,
	)
	if provisionOutcomes > 1 || connectOutcomes > 1 ||
		(provisionOutcomes > 0 && stages.ProvisionAttempts == 0) ||
		(provisionReasons > 0 && !stages.ProvisionUnavailable) ||
		(connectOutcomes > 0 && !stages.ProvisionAvailable) ||
		(clientConstructions > 0 && !stages.ConnectAvailable) {
		return integratedRoutingKindInitialStageMixed
	}
	if provisionReasons > 1 {
		return integratedRoutingKindInitialProvisionMixed
	}
	if stages.ConnectTunnelUnavailable {
		return integratedRoutingKindInitialConnectTunnel
	}
	if stages.ConnectAcceleratorUnavailable {
		return integratedRoutingKindInitialConnectAccelerator
	}
	if stages.ConnectVersionMismatch {
		return integratedRoutingKindInitialConnectVersion
	}
	if stages.ConnectAuthoritative {
		return integratedRoutingKindInitialConnectAuthoritative
	}
	if stages.ConnectCancelled {
		return integratedRoutingKindInitialConnectCancelled
	}
	if stages.ConnectAvailable {
		if clientConstructions == 0 {
			return integratedRoutingKindInitialSessionClientBind
		}
		return integratedRoutingKindInitialSessionReadyPath
	}
	if stages.ProvisionAvailable {
		return integratedRoutingKindInitialConnectNotEntered
	}
	if stages.ProvisionUnavailable {
		switch {
		case stages.ProvisionContextInput:
			return integratedRoutingKindInitialProvisionContextInput
		case stages.ProvisionChartPull:
			return integratedRoutingKindInitialProvisionChartPull
		case stages.ProvisionChartIntegrityRender:
			return integratedRoutingKindInitialProvisionChartRender
		case stages.ProvisionInstallPolicy:
			return integratedRoutingKindInitialProvisionInstallPolicy
		case stages.ProvisionImagePull:
			return integratedRoutingKindInitialProvisionImagePull
		case stages.ProvisionJobPod:
			return integratedRoutingKindInitialProvisionJobPod
		case stages.ProvisionTimeoutCancel:
			return integratedRoutingKindInitialProvisionTimeoutCancel
		default:
			return integratedRoutingKindInitialProvisionUnknown
		}
	}
	// Provision attempts are cumulative across all 30-second cooldown cycles;
	// three is only the temporary retry bound within one cycle.
	if stages.ProvisionAttempts > acceleratorprovision.MaxTransientActivationAttempts {
		return integratedRoutingKindInitialProvisionRetry
	}
	switch state {
	case acceleratorprovision.CoordinatorSweeping:
		return integratedRoutingKindInitialSweeping
	case acceleratorprovision.CoordinatorResolving:
		if stages.ProvisionAttempts == 0 {
			return integratedRoutingKindInitialResolvingZero
		}
		return integratedRoutingKindInitialResolvingAfterProvision
	case acceleratorprovision.CoordinatorProvisioning:
		return integratedRoutingKindInitialProvisioning
	case acceleratorprovision.CoordinatorConnecting:
		return integratedRoutingKindInitialConnecting
	case acceleratorprovision.CoordinatorActive:
		if clientConstructions == 0 {
			return integratedRoutingKindInitialActiveClientBind
		}
		return integratedRoutingKindInitialActiveReadyPath
	case acceleratorprovision.CoordinatorUnavailable:
		return integratedRoutingKindInitialUnavailable
	case acceleratorprovision.CoordinatorDirectOnly, acceleratorprovision.CoordinatorReconnecting, acceleratorprovision.CoordinatorDraining, acceleratorprovision.CoordinatorDisposing, acceleratorprovision.CoordinatorClosed:
		return integratedRoutingKindInitialTerminal
	default:
		return integratedRoutingKindInitialUnknown
	}
}

func countIntegratedRoutingKindStages(stages ...bool) int {
	count := 0
	for _, observed := range stages {
		if observed {
			count++
		}
	}
	return count
}

func waitIntegratedRoutingKindSignal(t *testing.T, channel <-chan struct{}, timeout time.Duration, failure string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(timeout):
		t.Fatal(failure)
	}
}

func assertIntegratedRoutingKindOriginProof(t *testing.T, probe *integratedRoutingKindOriginProbe, want integratedRoutingKindOriginSnapshot, failure string) {
	t.Helper()
	if !integratedRoutingKindOriginProofMatches(probe.snapshot(), want) {
		t.Fatal(failure)
	}
}

func waitIntegratedRoutingKindListCompletion(t *testing.T, completion <-chan error) error {
	t.Helper()
	timer := time.NewTimer(integratedRoutingKindListOperationTimeout)
	defer timer.Stop()
	select {
	case err := <-completion:
		return err
	case <-timer.C:
		t.Fatal("integrated routing canceled list did not settle")
		return context.DeadlineExceeded
	}
}

func waitIntegratedRoutingKindResource(t *testing.T, channel <-chan integratedSecretResourceSignal, token SecretReadSourceToken, spec acceleratorsecret.SecretWatchSpecID, timeout time.Duration) integratedSecretResourceSignal {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case event := <-channel:
			if event.SourceToken == token && event.WatcherSpecID == spec {
				return event
			}
		case <-deadline.C:
			t.Fatal("integrated routing watch event failed")
			return integratedSecretResourceSignal{}
		}
	}
}

func integratedRoutingKindContainsSecret(items []k8s.SecretListItem, namespace, name string) bool {
	for _, item := range items {
		if item.Metadata.Name == name && item.Metadata.Namespace == namespace && item.DataKeys == 1 {
			return true
		}
	}
	return false
}

func integratedRoutingKindContainsData(entries []k8s.DataEntry) bool {
	return len(entries) == 1 && entries[0].Key == "token" && entries[0].Value == "explicit-detail-value"
}

func integratedRoutingKindYAMLExact(value string) bool {
	return strings.Contains(value, "resourceVersion:") && strings.Contains(value, "uid:") && strings.Contains(value, "routing.test/origin:") && strings.Contains(value, "routing.test/note:") && strings.Contains(value, "token: ZXhwbGljaXQtZGV0YWlsLXZhbHVl") && strings.Contains(value, "type: Opaque") && !strings.Contains(value, "managedFields:")
}

func waitIntegratedRoutingKindMismatch(t *testing.T, coordinator *acceleratorprovision.Coordinator, probe *acceleratorprovision.IntegratedRoutingKindCoordinatorProbe, contextName string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if coordinator != nil && probe != nil && coordinator.Snapshot(contextName).State == acceleratorprovision.CoordinatorUnavailable && probe.ProvisionAttempts() == 2 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("integrated routing mismatch did not settle")
}

func integratedRoutingKindGraceBounds() (time.Duration, time.Duration) {
	grace := agent.EffectiveAcceleratorIdleReconnectGrace()
	lower := grace - 2*time.Second
	if lower < 0 {
		lower = 0
	}
	return lower, grace + 6*time.Second
}

// waitIntegratedAuthenticatedClientGraceExpiry observes the exact retained
// Pod rather than coordinator cleanup. A connection-owned Accelerator is not
// demand-owned: after a desktop crash its runtime exits on the authenticated
// client grace, and a later desktop cleanup removes the owned release.
func waitIntegratedAuthenticatedClientGraceExpiry(t *testing.T, snapshot *k8s.AcceleratorContextSnapshot, workload acceleratorprovision.IntegratedRoutingKindActiveWorkload, started time.Time) time.Duration {
	t.Helper()
	if snapshot == nil || started.IsZero() || workload.ReleaseNamespace == "" || workload.PodName == "" || workload.PodUID == "" {
		t.Fatal("integrated routing authenticated client grace observation unavailable")
	}
	_, upper := integratedRoutingKindGraceBounds()
	deadline := started.Add(upper)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), integratedRoutingKindK8sAssertionTimeout)
		pod, err := snapshot.Clientset().CoreV1().Pods(workload.ReleaseNamespace).Get(ctx, workload.PodName, metav1.GetOptions{})
		cancel()
		if err != nil {
			t.Fatal("integrated routing authenticated client grace Pod observation failed")
		}
		if string(pod.UID) != workload.PodUID {
			t.Fatal("integrated routing authenticated client grace Pod identity changed")
		}
		if pod.Status.Phase == corev1.PodFailed {
			t.Fatal("integrated routing acceptance runtime failed during authenticated client grace")
		}
		if pod.Status.Phase == corev1.PodSucceeded {
			if len(pod.Status.ContainerStatuses) != 1 || pod.Status.ContainerStatuses[0].RestartCount != 0 || pod.Status.ContainerStatuses[0].State.Terminated == nil || pod.Status.ContainerStatuses[0].State.Terminated.ExitCode != 0 {
				t.Fatal("integrated routing acceptance runtime expiry was not a single clean exit")
			}
			return time.Since(started)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("integrated routing authenticated client grace did not expire")
	return 0
}

// waitIntegratedOwnedCleanup observes only Helm and Kubernetes metadata for
// the exact workload that served the Integrated session. It deliberately
// avoids the workload's private connection material and accepts no broad
// namespace cleanup as evidence.
func waitIntegratedOwnedCleanup(t *testing.T, helmClient *helm.Client, snapshot *k8s.AcceleratorContextSnapshot, probe *acceleratorprovision.IntegratedRoutingKindCoordinatorProbe, workload acceleratorprovision.IntegratedRoutingKindActiveWorkload, sentinel, malformed string) {
	t.Helper()
	if helmClient == nil || snapshot == nil || probe == nil || workload.ReleaseNamespace == "" || workload.ReleaseName == "" || workload.JobName == "" || workload.JobUID == "" || workload.PodName == "" || workload.PodUID == "" {
		t.Fatal("integrated routing owned cleanup observation unavailable")
	}
	deadline := time.Now().Add(integratedRoutingKindCleanupTimeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), integratedRoutingKindK8sAssertionTimeout)
		releases, releaseErr := helmClient.ListReleasesForAcceleratorProvisionKind(ctx, snapshot.RESTConfig(), workload.ReleaseNamespace)
		job, jobErr := snapshot.Clientset().BatchV1().Jobs(workload.ReleaseNamespace).Get(ctx, workload.JobName, metav1.GetOptions{})
		pod, podErr := snapshot.Clientset().CoreV1().Pods(workload.ReleaseNamespace).Get(ctx, workload.PodName, metav1.GetOptions{})
		cancel()
		if releaseErr != nil {
			t.Fatal("integrated routing owned cleanup Helm observation failed")
		}
		retained := map[string]bool{sentinel: false, malformed: false}
		releasePresent := false
		for _, release := range releases {
			if release.Name == workload.ReleaseName {
				releasePresent = true
			}
			if _, ok := retained[release.Name]; ok {
				retained[release.Name] = true
			}
		}
		if !retained[sentinel] || !retained[malformed] {
			t.Fatal("integrated routing owned cleanup removed a sentinel release")
		}
		jobPresent := !apierrors.IsNotFound(jobErr)
		if jobErr != nil && jobPresent {
			t.Fatal("integrated routing owned cleanup Job observation failed")
		}
		if jobPresent && string(job.UID) != workload.JobUID {
			t.Fatal("integrated routing owned cleanup Job identity changed")
		}
		podPresent := !apierrors.IsNotFound(podErr)
		if podErr != nil && podPresent {
			t.Fatal("integrated routing owned cleanup Pod observation failed")
		}
		if podPresent && string(pod.UID) != workload.PodUID {
			t.Fatal("integrated routing owned cleanup Pod identity changed")
		}
		if probe.TerminalCleanupStartCount() != 0 || probe.TerminalCleanupCount() != 0 {
			t.Fatal("integrated routing desktop cleanup used the obsolete demand-owned cleanup path")
		}
		if !releasePresent && !jobPresent && !podPresent {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("integrated routing owned cleanup did not complete")
}

func assertIntegratedRoutingKindFinalCleanup(t *testing.T, helmClient *helm.Client, snapshot *k8s.AcceleratorContextSnapshot, sentinel, malformed string) {
	t.Helper()
	namespace := snapshot.Namespace()
	if namespace == "" {
		t.Fatal("integrated routing final namespace unavailable")
	}
	helmCtx, helmCancel := context.WithTimeout(context.Background(), integratedRoutingKindHelmAssertionTimeout)
	releases, err := helmClient.ListReleasesForAcceleratorProvisionKind(helmCtx, snapshot.RESTConfig(), namespace)
	helmCancel()
	if err != nil || len(releases) != 2 {
		t.Fatal("integrated routing final Helm cleanup failed")
	}
	retained := map[string]bool{sentinel: false, malformed: false}
	for _, release := range releases {
		if _, ok := retained[release.Name]; !ok {
			t.Fatal("integrated routing final Helm ownership failed")
		}
		retained[release.Name] = true
	}
	if !retained[sentinel] || !retained[malformed] {
		t.Fatal("integrated routing final sentinel cleanup failed")
	}
	selector := "app.kubernetes.io/name=kubikles-accelerator"
	k8sCtx, k8sCancel := context.WithTimeout(context.Background(), integratedRoutingKindK8sAssertionTimeout)
	defer k8sCancel()
	jobs, jobsErr := snapshot.Clientset().BatchV1().Jobs(namespace).List(k8sCtx, metav1.ListOptions{LabelSelector: selector})
	pods, podsErr := snapshot.Clientset().CoreV1().Pods(namespace).List(k8sCtx, metav1.ListOptions{LabelSelector: selector})
	secrets, secretsErr := snapshot.Clientset().CoreV1().Secrets(namespace).List(k8sCtx, metav1.ListOptions{LabelSelector: selector})
	accounts, accountsErr := snapshot.Clientset().CoreV1().ServiceAccounts(namespace).List(k8sCtx, metav1.ListOptions{LabelSelector: selector})
	roles, rolesErr := snapshot.Clientset().RbacV1().ClusterRoles().List(k8sCtx, metav1.ListOptions{LabelSelector: selector})
	bindings, bindingsErr := snapshot.Clientset().RbacV1().ClusterRoleBindings().List(k8sCtx, metav1.ListOptions{LabelSelector: selector})
	if jobsErr != nil || podsErr != nil || secretsErr != nil || accountsErr != nil || rolesErr != nil || bindingsErr != nil || len(jobs.Items) != 0 || len(pods.Items) != 0 || len(secrets.Items) != 0 || len(accounts.Items) != 0 || len(roles.Items) != 0 || len(bindings.Items) != 0 {
		t.Fatal("integrated routing final Kubernetes cleanup failed")
	}
}
