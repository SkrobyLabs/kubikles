//go:build helm && accelerator_provision_kind && !headless && !accelerator

package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"kubikles/pkg/acceleratorprovision"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/k8s"
)

type integratedRoutingKindBoundedRouterRecorder struct {
	unavailableSecretReadRouter
	retained      context.Context
	retainedName  string
	releases      int
	fencedName    string
	switchedName  string
	switchedReady bool
	quiesced      context.Context
	stopped       context.Context
	closed        context.Context
	hadDeadline   [integratedRoutingKindOrigins]bool
}

func (r *integratedRoutingKindBoundedRouterRecorder) record(ctx context.Context, origin integratedRoutingKindOrigin) {
	_, r.hadDeadline[origin] = ctx.Deadline()
}
func (r *integratedRoutingKindBoundedRouterRecorder) Retain(ctx context.Context, contextName string) {
	r.retained, r.retainedName = ctx, contextName
}
func (r *integratedRoutingKindBoundedRouterRecorder) Release() { r.releases++ }
func (r *integratedRoutingKindBoundedRouterRecorder) FenceContextSwitch(contextName string) {
	r.fencedName = contextName
}
func (r *integratedRoutingKindBoundedRouterRecorder) ContextSwitched(contextName string, ready bool) {
	r.switchedName, r.switchedReady = contextName, ready
}
func (r *integratedRoutingKindBoundedRouterRecorder) ListSecretsMetadata(ctx context.Context, _ SecretReadSourceToken, _, _ string, _ bool) ([]k8s.SecretListItem, error) {
	r.record(ctx, integratedRoutingKindList)
	return nil, nil
}
func (r *integratedRoutingKindBoundedRouterRecorder) GetSecretData(ctx context.Context, _ SecretReadSourceToken, _, _ string) ([]k8s.DataEntry, error) {
	r.record(ctx, integratedRoutingKindData)
	return nil, nil
}
func (r *integratedRoutingKindBoundedRouterRecorder) GetSecretYaml(ctx context.Context, _ SecretReadSourceToken, _, _ string) (string, error) {
	r.record(ctx, integratedRoutingKindYAML)
	return "", nil
}
func (r *integratedRoutingKindBoundedRouterRecorder) CancelListRequest(ctx context.Context, _ SecretReadSourceToken, _ string) (bool, error) {
	r.record(ctx, integratedRoutingKindCancel)
	return false, nil
}
func (r *integratedRoutingKindBoundedRouterRecorder) SubscribeSecretWatcher(ctx context.Context, _ SecretReadSourceToken, _ string, _ bool) (acceleratorsecret.SecretWatchSubscription, error) {
	r.record(ctx, integratedRoutingKindSubscribe)
	return acceleratorsecret.SecretWatchSubscription{WatcherSpecID: "fixed"}, nil
}
func (r *integratedRoutingKindBoundedRouterRecorder) UnsubscribeSecretWatcher(ctx context.Context, _ SecretReadSourceToken, _ acceleratorsecret.SecretWatchSpecID) error {
	r.record(ctx, integratedRoutingKindUnsubscribe)
	return nil
}
func (r *integratedRoutingKindBoundedRouterRecorder) Quiesce(ctx context.Context) {
	r.quiesced = ctx
}
func (r *integratedRoutingKindBoundedRouterRecorder) StopProducers(ctx context.Context) {
	r.stopped = ctx
}
func (r *integratedRoutingKindBoundedRouterRecorder) Close(ctx context.Context) { r.closed = ctx }

func TestIntegratedRoutingKindInitialReadinessBudgetCoversProductionBounds(t *testing.T) {
	minimum := acceleratorprovision.SweepTimeout + acceleratorprovision.ActivationAttemptTimeout
	if integratedRoutingKindInitialReadinessTimeout < minimum+time.Minute {
		t.Fatalf("initial readiness timeout = %s, want at least %s", integratedRoutingKindInitialReadinessTimeout, minimum+time.Minute)
	}
}

func TestIntegratedRoutingKindResolutionPassesProductionArtifactValidation(t *testing.T) {
	chartDigest := "sha256:" + strings.Repeat("b", 64)
	resolution := integratedRoutingKindResolution("v1.2.3", chartDigest, "sha256:"+strings.Repeat("a", 64))
	if resolution.Release.ChartReference != integratedRoutingKindChartRepository+"@"+chartDigest {
		t.Fatal("integrated routing chart reference is not the exact production OCI reference")
	}

	result := acceleratorprovision.New(nil, nil, nil).Provision(context.Background(), acceleratorprovision.Request{
		ContextName: "ctx",
		Resolution:  resolution,
	})
	if result.Availability != acceleratorprovision.Unavailable || result.Reason != acceleratorprovision.ContextUnavailable || result.Cleanup != acceleratorprovision.CleanupNotNeeded || result.Workload != nil {
		t.Fatal("integrated routing resolution did not pass production artifact validation")
	}
}

func TestIntegratedRoutingKindDiagnosticReporterIsClosedAndSingleShot(t *testing.T) {
	tests := []struct {
		name string
		code integratedRoutingKindDiagnosticCode
	}{
		{name: "early setup", code: integratedRoutingKindStageSetup},
		{name: "initial detailed timeout", code: integratedRoutingKindInitialConnecting},
		{name: "first post-ready list", code: integratedRoutingKindStageList},
		{name: "resume readiness", code: integratedRoutingKindStageResume},
		{name: "final release", code: integratedRoutingKindStageRelease},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostic := newIntegratedRoutingKindDiagnostic(integratedRoutingKindStageSetup)
			diagnostic.set(test.code)
			var records []string
			emit := func(record string) { records = append(records, record) }
			diagnostic.report(true, emit)
			diagnostic.report(true, emit)
			if len(records) != 1 || records[0] != integratedRoutingKindDiagnosticMarker+string(test.code) {
				t.Fatal("diagnostic reporter did not emit exactly one canonical value-free marker")
			}
		})
	}

	t.Run("pass emits nothing", func(t *testing.T) {
		diagnostic := newIntegratedRoutingKindDiagnostic(integratedRoutingKindStageWatch)
		var records []string
		diagnostic.report(false, func(record string) { records = append(records, record) })
		if len(records) != 0 {
			t.Fatal("passing diagnostic reporter emitted a marker")
		}
	})

	t.Run("LIFO reporter precedes teardown", func(t *testing.T) {
		var order []string
		t.Run("registered order", func(t *testing.T) {
			diagnostic := newIntegratedRoutingKindDiagnostic(integratedRoutingKindStageList)
			t.Cleanup(func() {
				diagnostic.set(integratedRoutingKindStageRelease)
				order = append(order, "teardown")
			})
			t.Cleanup(func() {
				diagnostic.report(true, func(record string) { order = append(order, record) })
			})
		})
		if len(order) != 2 || order[0] != integratedRoutingKindDiagnosticMarker+string(integratedRoutingKindStageList) || order[1] != "teardown" {
			t.Fatal("diagnostic reporter did not snapshot the body stage before teardown")
		}
	})

	t.Run("race safe stage state", func(t *testing.T) {
		diagnostic := newIntegratedRoutingKindDiagnostic(integratedRoutingKindStageResume)
		var writers sync.WaitGroup
		for range 8 {
			writers.Add(1)
			go func() {
				defer writers.Done()
				diagnostic.set(integratedRoutingKindStageResume)
			}()
		}
		writers.Wait()
		var records []string
		diagnostic.report(true, func(record string) { records = append(records, record) })
		if len(records) != 1 || records[0] != integratedRoutingKindDiagnosticMarker+string(integratedRoutingKindStageResume) {
			t.Fatal("concurrent diagnostic stage state was not stable")
		}
	})

	t.Run("unknown code panics closed", func(t *testing.T) {
		diagnostic := newIntegratedRoutingKindDiagnostic(integratedRoutingKindStageSetup)
		deferred := false
		func() {
			defer func() { deferred = recover() != nil }()
			diagnostic.set(integratedRoutingKindDiagnosticCode("stage-hostile"))
		}()
		if !deferred {
			t.Fatal("unknown diagnostic stage was accepted")
		}
	})
}

func TestIntegratedRoutingKindSequentialBudgetDominatesInnerBounds(t *testing.T) {
	if integratedRoutingKindDesktopAPITimeout < k8s.DefaultAPITimeout {
		t.Fatalf("desktop API timeout = %s, want at least %s", integratedRoutingKindDesktopAPITimeout, k8s.DefaultAPITimeout)
	}
	for _, test := range []struct {
		operation acceleratorsecret.Operation
		budget    time.Duration
	}{
		{operation: acceleratorsecret.OperationListSecretsMetadata, budget: integratedRoutingKindListOperationTimeout},
		{operation: acceleratorsecret.OperationGetSecretData, budget: integratedRoutingKindDataOperationTimeout},
		{operation: acceleratorsecret.OperationGetSecretYAML, budget: integratedRoutingKindYAMLOperationTimeout},
		{operation: acceleratorsecret.OperationCancelListRequest, budget: integratedRoutingKindCancelOperationTimeout},
		{operation: acceleratorsecret.OperationSubscribeSecretWatcher, budget: integratedRoutingKindSubscribeOperationTimeout},
		{operation: acceleratorsecret.OperationUnsubscribeSecretWatcher, budget: integratedRoutingKindUnsubscribeOperationTimeout},
	} {
		if inner := acceleratorsecret.OperationTimeout(test.operation); test.budget < inner {
			t.Fatalf("%s budget = %s, want at least inner timeout %s", test.operation, test.budget, inner)
		}
	}
	for _, test := range []struct {
		origin    integratedRoutingKindOrigin
		operation acceleratorsecret.Operation
	}{
		{origin: integratedRoutingKindList, operation: acceleratorsecret.OperationListSecretsMetadata},
		{origin: integratedRoutingKindData, operation: acceleratorsecret.OperationGetSecretData},
		{origin: integratedRoutingKindYAML, operation: acceleratorsecret.OperationGetSecretYAML},
		{origin: integratedRoutingKindCancel, operation: acceleratorsecret.OperationCancelListRequest},
		{origin: integratedRoutingKindSubscribe, operation: acceleratorsecret.OperationSubscribeSecretWatcher},
		{origin: integratedRoutingKindUnsubscribe, operation: acceleratorsecret.OperationUnsubscribeSecretWatcher},
	} {
		if total, inner := integratedRoutingKindOperationTotalTimeout(test.origin), acceleratorsecret.OperationTimeout(test.operation); total < inner {
			t.Fatalf("origin %d total timeout = %s, want at least %s", test.origin, total, inner)
		}
	}
	if integratedRoutingKindDataTotalTimeout != integratedRoutingKindDataOperationTimeout+integratedRoutingKindDesktopAPITimeout || integratedRoutingKindYAMLTotalTimeout != integratedRoutingKindYAMLOperationTimeout+integratedRoutingKindDesktopAPITimeout {
		t.Fatal("detail total operation timeouts do not cover remote plus Direct fallback")
	}
	if integratedRoutingKindSequentialTimeout != 33*time.Minute+17*time.Second {
		t.Fatalf("sequential timeout = %s, want machine-checked 33m17s", integratedRoutingKindSequentialTimeout)
	}
	if integratedRoutingKindGoTestTimeout != 34*time.Minute {
		t.Fatalf("go test timeout = %s, want 34m", integratedRoutingKindGoTestTimeout)
	}
	if margin := integratedRoutingKindGoTestTimeout - integratedRoutingKindSequentialTimeout; margin != 43*time.Second {
		t.Fatalf("outer timeout margin = %s, want 43s", margin)
	}
	if integratedRoutingKindFailureSequentialTimeout != 21*time.Minute+40*time.Second {
		t.Fatalf("fallback failure timeout = %s, want machine-checked 21m40s", integratedRoutingKindFailureSequentialTimeout)
	}
	if integratedRoutingKindFailureSequentialTimeout >= integratedRoutingKindGoTestTimeout {
		t.Fatal("fallback plus cleanup failure path is not dominated by the outer timeout")
	}
}

func TestIntegratedRoutingKindOperationProxyBoundsOnlyCalls(t *testing.T) {
	recorder := &integratedRoutingKindBoundedRouterRecorder{}
	proxy := integratedRoutingKindBoundedSecretReadRouter{delegate: recorder}
	background := context.Background()
	proxy.Retain(background, "retained")
	proxy.Release()
	proxy.FenceContextSwitch("fenced")
	proxy.ContextSwitched("switched", true)
	lifecycle := context.WithValue(background, struct{}{}, "fixed")
	proxy.Quiesce(lifecycle)
	proxy.StopProducers(lifecycle)
	proxy.Close(lifecycle)
	token := SecretReadSourceToken("opaque")
	_, _ = proxy.ListSecretsMetadata(background, token, "owner", "ns", false)
	_, _ = proxy.GetSecretData(background, token, "ns", "name")
	_, _ = proxy.GetSecretYaml(background, token, "ns", "name")
	_, _ = proxy.CancelListRequest(background, token, "owner")
	_, _ = proxy.SubscribeSecretWatcher(background, token, "ns", false)
	_ = proxy.UnsubscribeSecretWatcher(background, token, "fixed")
	if recorder.retained != background || recorder.retainedName != "retained" || recorder.releases != 1 || recorder.fencedName != "fenced" || recorder.switchedName != "switched" || !recorder.switchedReady {
		t.Fatal("operation proxy changed demand or context lifecycle delegation")
	}
	if recorder.quiesced != lifecycle || recorder.stopped != lifecycle || recorder.closed != lifecycle {
		t.Fatal("operation proxy changed runtime lifecycle contexts")
	}
	for origin, bounded := range recorder.hadDeadline {
		if !bounded {
			t.Fatalf("operation origin %d did not receive a total deadline", origin)
		}
	}
}

func TestIntegratedRoutingKindFallbackCannotProveRemoteSuccess(t *testing.T) {
	operations := []struct {
		name   string
		origin integratedRoutingKindOrigin
		call   func(integratedRoutingKindBoundedSecretReadRouter, SecretReadSourceToken) bool
	}{
		{name: "list", origin: integratedRoutingKindList, call: func(router integratedRoutingKindBoundedSecretReadRouter, token SecretReadSourceToken) bool {
			items, err := router.ListSecretsMetadata(context.Background(), token, "owner", "ns", false)
			return err == nil && len(items) == 1 && items[0].DataKeys == 2
		}},
		{name: "data", origin: integratedRoutingKindData, call: func(router integratedRoutingKindBoundedSecretReadRouter, token SecretReadSourceToken) bool {
			data, err := router.GetSecretData(context.Background(), token, "ns", "name")
			return err == nil && len(data) == 1 && data[0].Key == "direct"
		}},
		{name: "yaml", origin: integratedRoutingKindYAML, call: func(router integratedRoutingKindBoundedSecretReadRouter, token SecretReadSourceToken) bool {
			value, err := router.GetSecretYaml(context.Background(), token, "ns", "name")
			return err == nil && value == "direct"
		}},
	}
	for _, reason := range []acceleratorsecret.SecretClientReason{acceleratorsecret.ReasonCapacity, acceleratorsecret.ReasonForbidden, acceleratorsecret.ReasonRemoteUnavailable} {
		for _, operation := range operations {
			t.Run(string(reason)+"-"+operation.name, func(t *testing.T) {
				remote := newFakeRouterClient()
				failure := fakeSecretReason(reason)
				remote.list = func(context.Context, string, string, bool) ([]k8s.SecretListItem, error) { return nil, failure }
				remote.data = func(context.Context, string, string) ([]k8s.DataEntry, error) { return nil, failure }
				remote.yaml = func(context.Context, string, string) (string, error) { return "", failure }
				probe := &integratedRoutingKindOriginProbe{}
				client := integratedRoutingKindSecretClient{client: remote, probe: probe}
				demand := newFakeRouterDemand(newFakeRouterSession(1, client))
				production, ready, _ := routerFixture(t, demand)
				bounded := integratedRoutingKindBoundedSecretReadRouter{delegate: production}
				bounded.Retain(context.Background(), "ctx")
				token := waitRouterSignal(t, ready).SourceToken
				if !operation.call(bounded, token) {
					t.Fatal("Direct fallback fixture did not return its valid result")
				}
				got := probe.snapshot()
				if got.attempts[operation.origin] != 1 || got.successes[operation.origin] != 0 {
					t.Fatal("fallback origin counters did not preserve attempt without success")
				}
				oldAttemptOnlyWant := integratedRoutingKindOriginSnapshot{}
				oldAttemptOnlyWant.attempts[operation.origin] = 1
				if got.attempts != oldAttemptOnlyWant.attempts {
					t.Fatal("fallback fixture would not have passed the former attempt-only proof")
				}
				newSuccessWant := oldAttemptOnlyWant
				newSuccessWant.successes[operation.origin] = 1
				if integratedRoutingKindOriginProofMatches(got, newSuccessWant) {
					t.Fatal("Direct fallback satisfied remote success proof")
				}
			})
		}
	}
}

func TestIntegratedRoutingKindListFallbackUsesOneTotalDeadline(t *testing.T) {
	remote := newFakeRouterClient()
	remote.list = func(context.Context, string, string, bool) ([]k8s.SecretListItem, error) {
		return nil, fakeSecretReason(acceleratorsecret.ReasonRemoteUnavailable)
	}
	deadline := make(chan error, 1)
	probe := &integratedRoutingKindOriginProbe{}
	client := integratedRoutingKindSecretClient{client: remote, probe: probe}
	demand := newFakeRouterDemand(newFakeRouterSession(1, client))
	production, ready, _ := routerFixtureWith(t, demand, func(deps *integratedSecretRouterDependencies) {
		deps.directList = func(ctx context.Context, _, _ string, _ bool) ([]k8s.SecretListItem, error) {
			<-ctx.Done()
			deadline <- ctx.Err()
			return nil, ctx.Err()
		}
	})
	bounded := integratedRoutingKindBoundedSecretReadRouter{delegate: production}
	bounded.overrides[integratedRoutingKindList] = 20 * time.Millisecond
	bounded.Retain(context.Background(), "ctx")
	token := waitRouterSignal(t, ready).SourceToken
	started := time.Now()
	if _, err := bounded.ListSecretsMetadata(context.Background(), token, "owner", "ns", false); err == nil {
		t.Fatal("bounded Direct List fallback succeeded")
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatal("bounded Direct List fallback exceeded its total deadline")
	}
	select {
	case err := <-deadline:
		if err != context.DeadlineExceeded {
			t.Fatal("Direct List fallback received the wrong terminal context")
		}
	default:
		t.Fatal("Direct List fallback did not observe the total deadline")
	}
	got := probe.snapshot()
	if got.attempts[integratedRoutingKindList] != 1 || got.successes[integratedRoutingKindList] != 0 {
		t.Fatal("bounded List fallback incorrectly proved remote success")
	}
}

func TestIntegratedRoutingKindInitialDiagnosticClassifier(t *testing.T) {
	tests := []struct {
		name         string
		state        acceleratorprovision.CoordinatorState
		stages       acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot
		clients      int32
		want         integratedRoutingKindInitialPhase
		missingInput bool
	}{
		{name: "missing", want: integratedRoutingKindInitialMissing, missingInput: true},
		{name: "sweeping", state: acceleratorprovision.CoordinatorSweeping, want: integratedRoutingKindInitialSweeping},
		{name: "resolving zero", state: acceleratorprovision.CoordinatorResolving, want: integratedRoutingKindInitialResolvingZero},
		{name: "resolving after provision", state: acceleratorprovision.CoordinatorResolving, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1}, want: integratedRoutingKindInitialResolvingAfterProvision},
		{name: "provisioning", state: acceleratorprovision.CoordinatorProvisioning, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1}, want: integratedRoutingKindInitialProvisioning},
		{name: "connecting", state: acceleratorprovision.CoordinatorConnecting, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1}, want: integratedRoutingKindInitialConnecting},
		{name: "active client bind", state: acceleratorprovision.CoordinatorActive, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1}, want: integratedRoutingKindInitialActiveClientBind},
		{name: "active ready path", state: acceleratorprovision.CoordinatorActive, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1}, clients: 1, want: integratedRoutingKindInitialStageMixed},
		{name: "unavailable", state: acceleratorprovision.CoordinatorUnavailable, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 3}, want: integratedRoutingKindInitialUnavailable},
		{name: "direct terminal", state: acceleratorprovision.CoordinatorDirectOnly, want: integratedRoutingKindInitialTerminal},
		{name: "reconnecting terminal", state: acceleratorprovision.CoordinatorReconnecting, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1}, want: integratedRoutingKindInitialTerminal},
		{name: "draining terminal", state: acceleratorprovision.CoordinatorDraining, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1}, want: integratedRoutingKindInitialTerminal},
		{name: "disposing terminal", state: acceleratorprovision.CoordinatorDisposing, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1}, want: integratedRoutingKindInitialTerminal},
		{name: "closed terminal", state: acceleratorprovision.CoordinatorClosed, want: integratedRoutingKindInitialTerminal},
		{name: "unknown", state: acceleratorprovision.CoordinatorState("future"), want: integratedRoutingKindInitialUnknown},
		{name: "retry cycles", state: acceleratorprovision.CoordinatorResolving, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: acceleratorprovision.MaxTransientActivationAttempts + 1}, want: integratedRoutingKindInitialProvisionRetry},
		{name: "client repeat", state: acceleratorprovision.CoordinatorActive, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true, ConnectAvailable: true, ConnectTunnelUnavailable: true}, clients: 2, want: integratedRoutingKindInitialClientRepeat},
		{name: "negative provision count", state: acceleratorprovision.CoordinatorActive, stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: -1}, want: integratedRoutingKindInitialCountInvalid},
		{name: "negative client count", state: acceleratorprovision.CoordinatorActive, clients: -1, want: integratedRoutingKindInitialCountInvalid},
		{name: "provision context input", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionContextInput: true}, want: integratedRoutingKindInitialProvisionContextInput},
		{name: "provision chart pull", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionChartPull: true}, want: integratedRoutingKindInitialProvisionChartPull},
		{name: "provision chart integrity render", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionChartIntegrityRender: true}, want: integratedRoutingKindInitialProvisionChartRender},
		{name: "provision install conflict permission", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionInstallPolicy: true}, want: integratedRoutingKindInitialProvisionInstallPolicy},
		{name: "provision image pull", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionImagePull: true}, want: integratedRoutingKindInitialProvisionImagePull},
		{name: "provision job pod", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionJobPod: true}, want: integratedRoutingKindInitialProvisionJobPod},
		{name: "provision timeout cancel", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionTimeoutCancel: true}, want: integratedRoutingKindInitialProvisionTimeoutCancel},
		{name: "provision unknown", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionUnknown: true}, want: integratedRoutingKindInitialProvisionUnknown},
		{name: "provision missing reason family", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true}, want: integratedRoutingKindInitialProvisionUnknown},
		{name: "provision mixed families", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 2, ProvisionUnavailable: true, ProvisionChartPull: true, ProvisionTimeoutCancel: true}, want: integratedRoutingKindInitialProvisionMixed},
		{name: "provision known and unknown mixed", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 2, ProvisionUnavailable: true, ProvisionChartPull: true, ProvisionUnknown: true}, want: integratedRoutingKindInitialProvisionMixed},
		{name: "provision family without unavailable", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionChartPull: true}, want: integratedRoutingKindInitialStageMixed},
		{name: "connect not entered", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true}, want: integratedRoutingKindInitialConnectNotEntered},
		{name: "connect tunnel", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true, ConnectTunnelUnavailable: true}, want: integratedRoutingKindInitialConnectTunnel},
		{name: "connect accelerator", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true, ConnectAcceleratorUnavailable: true}, want: integratedRoutingKindInitialConnectAccelerator},
		{name: "connect version", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true, ConnectVersionMismatch: true}, want: integratedRoutingKindInitialConnectVersion},
		{name: "connect authoritative", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true, ConnectAuthoritative: true}, want: integratedRoutingKindInitialConnectAuthoritative},
		{name: "connect cancelled", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true, ConnectCancelled: true}, want: integratedRoutingKindInitialConnectCancelled},
		{name: "session client bind", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true, ConnectAvailable: true}, want: integratedRoutingKindInitialSessionClientBind},
		{name: "session ready path", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true, ConnectAvailable: true}, clients: 1, want: integratedRoutingKindInitialSessionReadyPath},
		{name: "mixed provision outcomes", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 2, ProvisionAvailable: true, ProvisionUnavailable: true, ProvisionChartPull: true}, want: integratedRoutingKindInitialStageMixed},
		{name: "mixed connect outcomes", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 2, ProvisionAvailable: true, ConnectTunnelUnavailable: true, ConnectAcceleratorUnavailable: true}, want: integratedRoutingKindInitialStageMixed},
		{name: "connect without provision", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ConnectTunnelUnavailable: true}, want: integratedRoutingKindInitialStageMixed},
		{name: "client without available connect", stages: acceleratorprovision.IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true, ConnectTunnelUnavailable: true}, clients: 1, want: integratedRoutingKindInitialStageMixed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyIntegratedRoutingKindInitialSnapshot(test.state, test.stages, test.clients)
			if test.missingInput {
				got = classifyIntegratedRoutingKindInitialPhase(nil, nil, "", nil)
			}
			if got != test.want {
				t.Fatalf("phase = %q, want %q", got, test.want)
			}
		})
	}
}
