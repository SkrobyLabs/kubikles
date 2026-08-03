//go:build helm && accelerator_provision_kind

package acceleratorprovision

import (
	"context"
	"reflect"
	"testing"
)

type integratedRoutingKindProvisionerStub struct{ result Result }

func (s integratedRoutingKindProvisionerStub) Provision(context.Context, Request) Result {
	return s.result
}

type integratedRoutingKindConnectorStub struct{ result ConnectResult }

func (s integratedRoutingKindConnectorStub) Connect(context.Context, *ProvisionedWorkload) ConnectResult {
	return s.result
}

func TestIntegratedRoutingKindProbeSnapshotCarriesOnlyClosedBuckets(t *testing.T) {
	snapshotType := reflect.TypeOf(IntegratedRoutingKindCoordinatorProbeSnapshot{})
	if snapshotType.NumField() != 17 || snapshotType.Field(0).Name != "ProvisionAttempts" || snapshotType.Field(0).Type.Kind() != reflect.Int {
		t.Fatal("probe snapshot changed its single cumulative count")
	}
	for index := 1; index < snapshotType.NumField(); index++ {
		if snapshotType.Field(index).Type.Kind() != reflect.Bool {
			t.Fatal("probe outcome escaped its closed value-free bucket")
		}
	}
}

func TestIntegratedRoutingKindProvisionerRecordsClosedOutcomeBuckets(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   IntegratedRoutingKindCoordinatorProbeSnapshot
	}{
		{name: "available", result: Result{Availability: Available, Workload: &ProvisionedWorkload{}}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionAvailable: true}},
		{name: "artifact unavailable", result: Result{Availability: Unavailable, Reason: ArtifactUnavailable}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionContextInput: true}},
		{name: "context unavailable", result: Result{Availability: Unavailable, Reason: ContextUnavailable}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionContextInput: true}},
		{name: "context changed", result: Result{Availability: Unavailable, Reason: ContextChanged}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionContextInput: true}},
		{name: "entropy unavailable", result: Result{Availability: Unavailable, Reason: EntropyUnavailable}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionContextInput: true}},
		{name: "chart pull", result: Result{Availability: Unavailable, Reason: ChartPullFailed}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionChartPull: true}},
		{name: "chart integrity", result: Result{Availability: Unavailable, Reason: ChartIntegrityFailed}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionChartIntegrityRender: true}},
		{name: "render", result: Result{Availability: Unavailable, Reason: RenderFailed}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionChartIntegrityRender: true}},
		{name: "release conflict", result: Result{Availability: Unavailable, Reason: ReleaseConflict}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionInstallPolicy: true}},
		{name: "permission denied", result: Result{Availability: Unavailable, Reason: PermissionDenied}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionInstallPolicy: true}},
		{name: "install", result: Result{Availability: Unavailable, Reason: InstallFailed}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionInstallPolicy: true}},
		{name: "job", result: Result{Availability: Unavailable, Reason: JobFailed}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionJobPod: true}},
		{name: "pod", result: Result{Availability: Unavailable, Reason: PodFailed}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionJobPod: true}},
		{name: "image pull", result: Result{Availability: Unavailable, Reason: ImagePullFailed}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionImagePull: true}},
		{name: "timeout", result: Result{Availability: Unavailable, Reason: TimedOut}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionTimeoutCancel: true}},
		{name: "cancelled", result: Result{Availability: Unavailable, Reason: Cancelled}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1, ProvisionUnavailable: true, ProvisionTimeoutCancel: true}},
	}
	closedReasons := []UnavailableReason{
		ArtifactUnavailable, ContextUnavailable, ContextChanged, EntropyUnavailable,
		ChartPullFailed, ChartIntegrityFailed, RenderFailed, ReleaseConflict, PermissionDenied,
		InstallFailed, JobFailed, PodFailed, ImagePullFailed, TimedOut, Cancelled,
	}
	closed := make(map[UnavailableReason]struct{}, len(closedReasons))
	for _, reason := range closedReasons {
		closed[reason] = struct{}{}
	}
	covered := make(map[UnavailableReason]int, len(closedReasons))
	for _, test := range tests {
		if _, known := closed[test.result.Reason]; known && test.result.Availability == Unavailable && test.result.Workload == nil {
			covered[test.result.Reason]++
		}
	}
	if len(covered) != len(closedReasons) {
		t.Fatal("provision reason regression table does not match the closed enum")
	}
	for _, reason := range closedReasons {
		if covered[reason] != 1 {
			t.Fatal("closed provision reason does not have exactly one bucket regression")
		}
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &IntegratedRoutingKindCoordinatorProbe{}
			wrapper := integratedRoutingKindProvisioner{delegate: integratedRoutingKindProvisionerStub{result: test.result}, probe: probe}
			if got := wrapper.Provision(context.Background(), Request{}); !reflect.DeepEqual(got, test.result) {
				t.Fatal("provision wrapper changed the production result")
			}
			if got := probe.Snapshot(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("provision stage snapshot = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestIntegratedRoutingKindProvisionerClassifiesResultShapeCrossProduct(t *testing.T) {
	availabilities := []struct {
		name  string
		value Availability
	}{
		{name: "available", value: Available},
		{name: "unavailable", value: Unavailable},
		{name: "future", value: Availability("future")},
	}
	workloads := []struct {
		name    string
		present bool
	}{
		{name: "nil"},
		{name: "nonnil", present: true},
	}
	reasons := []struct {
		name  string
		value UnavailableReason
	}{
		{name: "empty"},
		{name: "known", value: ChartPullFailed},
		{name: "future", value: UnavailableReason("future")},
	}
	for _, availability := range availabilities {
		for _, workload := range workloads {
			for _, reason := range reasons {
				t.Run(availability.name+"/"+workload.name+"/"+reason.name, func(t *testing.T) {
					result := Result{Availability: availability.value, Reason: reason.value}
					if workload.present {
						result.Workload = &ProvisionedWorkload{}
					}
					want := IntegratedRoutingKindCoordinatorProbeSnapshot{ProvisionAttempts: 1}
					switch {
					case availability.value == Available && workload.present:
						want.ProvisionAvailable = true
					case availability.value == Unavailable && !workload.present && reason.value == ChartPullFailed:
						want.ProvisionUnavailable = true
						want.ProvisionChartPull = true
					default:
						want.ProvisionUnavailable = true
						want.ProvisionUnknown = true
					}
					probe := &IntegratedRoutingKindCoordinatorProbe{}
					wrapper := integratedRoutingKindProvisioner{delegate: integratedRoutingKindProvisionerStub{result: result}, probe: probe}
					if got := wrapper.Provision(context.Background(), Request{}); !reflect.DeepEqual(got, result) {
						t.Fatal("provision wrapper changed the production result")
					}
					if got := probe.Snapshot(); !reflect.DeepEqual(got, want) {
						t.Fatalf("provision shape snapshot = %#v, want %#v", got, want)
					}
				})
			}
		}
	}
}

func TestIntegratedRoutingKindConnectorRecordsClosedOutcomeBuckets(t *testing.T) {
	tests := []struct {
		name   string
		result ConnectResult
		want   IntegratedRoutingKindCoordinatorProbeSnapshot
	}{
		{name: "available", result: ConnectResult{Availability: Available, Session: &ConnectedSession{}}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectAvailable: true}},
		{name: "available with reason", result: ConnectResult{Availability: Available, Reason: TunnelUnavailable, Session: &ConnectedSession{}}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectAvailable: true}},
		{name: "available without session", result: ConnectResult{Availability: Available}, want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectAuthoritative: true}},
		{name: "tunnel unavailable", result: unavailableConnect(TunnelUnavailable), want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectTunnelUnavailable: true}},
		{name: "accelerator unavailable", result: unavailableConnect(AcceleratorUnavailable), want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectAcceleratorUnavailable: true}},
		{name: "version mismatch", result: unavailableConnect(ConnectVersionMismatch), want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectVersionMismatch: true}},
		{name: "invalid workload", result: unavailableConnect(InvalidWorkload), want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectAuthoritative: true}},
		{name: "workload unavailable", result: unavailableConnect(WorkloadUnavailable), want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectAuthoritative: true}},
		{name: "workload changed", result: unavailableConnect(WorkloadChanged), want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectAuthoritative: true}},
		{name: "cancelled", result: unavailableConnect(ConnectCancelled), want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectCancelled: true}},
		{name: "workload disposing", result: unavailableConnect(WorkloadDisposing), want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectCancelled: true}},
		{name: "unknown reason", result: unavailableConnect(ConnectUnavailableReason("future")), want: IntegratedRoutingKindCoordinatorProbeSnapshot{ConnectAuthoritative: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &IntegratedRoutingKindCoordinatorProbe{}
			wrapper := integratedRoutingKindConnector{delegate: integratedRoutingKindConnectorStub{result: test.result}, probe: probe}
			if got := wrapper.Connect(context.Background(), &ProvisionedWorkload{}); !reflect.DeepEqual(got, test.result) {
				t.Fatal("connector wrapper changed the production result")
			}
			if got := probe.Snapshot(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("connect stage snapshot = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestIntegratedRoutingKindProbeRetainsMixedHistoricalBuckets(t *testing.T) {
	probe := &IntegratedRoutingKindCoordinatorProbe{}
	provisioner := integratedRoutingKindProvisioner{delegate: integratedRoutingKindProvisionerStub{result: Result{Availability: Unavailable, Reason: TimedOut}}, probe: probe}
	_ = provisioner.Provision(context.Background(), Request{})
	provisioner.delegate = integratedRoutingKindProvisionerStub{result: Result{Availability: Unavailable, Reason: ChartPullFailed}}
	_ = provisioner.Provision(context.Background(), Request{})
	provisioner.delegate = integratedRoutingKindProvisionerStub{result: Result{Availability: Available, Workload: &ProvisionedWorkload{}}}
	_ = provisioner.Provision(context.Background(), Request{})
	connector := integratedRoutingKindConnector{delegate: integratedRoutingKindConnectorStub{result: unavailableConnect(TunnelUnavailable)}, probe: probe}
	_ = connector.Connect(context.Background(), &ProvisionedWorkload{})
	connector.delegate = integratedRoutingKindConnectorStub{result: unavailableConnect(AcceleratorUnavailable)}
	_ = connector.Connect(context.Background(), &ProvisionedWorkload{})
	want := IntegratedRoutingKindCoordinatorProbeSnapshot{
		ProvisionAttempts:             3,
		ProvisionAvailable:            true,
		ProvisionUnavailable:          true,
		ProvisionChartPull:            true,
		ProvisionTimeoutCancel:        true,
		ConnectTunnelUnavailable:      true,
		ConnectAcceleratorUnavailable: true,
	}
	if got := probe.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed stage snapshot = %#v, want %#v", got, want)
	}
}

func TestAcceptanceLiteralVersionMatrixUsesExactAuthenticatedConnectorAndBoundedCoordinator(t *testing.T) {
	proof, err := ProveAcceptanceLiteralVersionMatrix()
	if err != nil {
		t.Fatal("literal acceptance matrix failed")
	}
	if !proof.ExactAccepted || proof.AdjacentRejected != 2 || !proof.DirectBeforeCleanup || proof.OneReplacement != 2 || proof.RepeatedMismatch != 2 || !proof.NoThirdWorkload {
		t.Fatalf("literal acceptance proof = %#v", proof)
	}
}
