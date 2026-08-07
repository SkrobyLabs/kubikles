package acceleratorprovision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"kubikles/pkg/helm"
	localk8s "kubikles/pkg/k8s"
)

type fakeInertSweeper struct {
	lists                int
	names                []string
	listOK               bool
	inspects, uninstalls int
	proof                helm.AcceleratorSweepProofStatus
	onInspect            func(context.Context)
	onCleanup            func(context.Context)
}

func (f *fakeInertSweeper) ListAcceleratorSweepReleaseNames(context.Context, *rest.Config, string) ([]string, bool) {
	f.lists++
	return append([]string(nil), f.names...), f.listOK
}
func (f *fakeInertSweeper) InspectAcceleratorSweepCandidate(ctx context.Context, _ *rest.Config, _, _ string) (*helm.AcceleratorSweepCandidate, helm.AcceleratorSweepProofStatus) {
	f.inspects++
	if f.onInspect != nil {
		f.onInspect(ctx)
	}
	return &helm.AcceleratorSweepCandidate{}, f.proof
}
func (f *fakeInertSweeper) CleanupAcceleratorSweepCandidate(ctx context.Context, _ *rest.Config, _ *helm.AcceleratorSweepCandidate) helm.AcceleratorSweepProofStatus {
	f.uninstalls++
	if f.onCleanup != nil {
		f.onCleanup(ctx)
	}
	return f.proof
}

func TestSweepBoundsBeforeAnyCandidateMutation(t *testing.T) {
	snapshot := fakeSnapshot{identity: "identity", namespace: "default", client: fakeClientset(observedJob(observerAttempt()), observedPod(observerAttempt(), observedJob(observerAttempt())))}
	snapshotConfig := snapshot.RESTConfig()
	_ = snapshotConfig
	for _, test := range []struct {
		name  string
		names []string
	}{
		{name: "release limit", names: make([]string, 101)},
		{name: "candidate limit", names: make([]string, 21)},
	} {
		t.Run(test.name, func(t *testing.T) {
			for index := range test.names {
				if test.name == "release limit" {
					test.names[index] = fmt.Sprintf("other-%03d", index)
				} else {
					test.names[index] = fmt.Sprintf("kubikles-accelerator-%032x", index)
				}
			}
			fake := &fakeInertSweeper{names: test.names, listOK: true, proof: helm.AcceleratorSweepEligible}
			service := &DisposalService{gates: &gateSet{}, sweeper: fake, acceptSweepSnapshot: func(ContextSnapshot) bool { return true }}
			result := service.SweepInert(context.Background(), snapshot)
			if result.Status != SweepBoundedLimit || fake.inspects != 0 || fake.uninstalls != 0 || len(result.Candidates) != 0 {
				t.Fatalf("result=%#v inspect=%d uninstall=%d", result, fake.inspects, fake.uninstalls)
			}
		})
	}
}

func TestSweepRejectsUntrustedSnapshotBeforeIO(t *testing.T) {
	fake := &fakeInertSweeper{listOK: true}
	service := NewDisposalService(nil)
	service.sweeper = fake
	result := service.SweepInert(context.Background(), fakeSnapshot{identity: "identity", namespace: "default"})
	if result.Status != SweepInvalidSnapshot || fake.lists != 0 || fake.inspects != 0 || fake.uninstalls != 0 {
		t.Fatalf("result=%v calls=%d/%d/%d", result.Status, fake.lists, fake.inspects, fake.uninstalls)
	}
}

func TestSweepEligibleCandidatesAreSequentiallyProvedAndCleaned(t *testing.T) {
	snapshot := fakeSnapshot{identity: "identity", namespace: "default", client: fakeClientset(observedJob(observerAttempt()), observedPod(observerAttempt(), observedJob(observerAttempt())))}
	fake := &fakeInertSweeper{names: []string{"kubikles-accelerator-00000000000000000000000000000001", "unrelated"}, listOK: true, proof: helm.AcceleratorSweepEligible}
	service := &DisposalService{gates: &gateSet{}, sweeper: fake, acceptSweepSnapshot: func(ContextSnapshot) bool { return true }}
	result := service.SweepInert(context.Background(), snapshot)
	if result.Status != SweepCompleted || len(result.Candidates) != 1 || result.Candidates[0].Status != SweepCleaned || fake.inspects != 1 || fake.uninstalls != 1 {
		t.Fatalf("result=%#v inspect=%d uninstall=%d", result, fake.inspects, fake.uninstalls)
	}
}

func TestSweepCancellationStopsBeforeMutationOrNextCandidate(t *testing.T) {
	snapshot := fakeSnapshot{identity: "identity", namespace: "default", client: fakeClientset(observedJob(observerAttempt()), observedPod(observerAttempt(), observedJob(observerAttempt())))}
	const first = "kubikles-accelerator-00000000000000000000000000000001"
	const second = "kubikles-accelerator-00000000000000000000000000000002"

	t.Run("while waiting for release gate", func(t *testing.T) {
		fake := &fakeInertSweeper{names: []string{first}, listOK: true, proof: helm.AcceleratorSweepEligible}
		service := &DisposalService{gates: &gateSet{}, sweeper: fake, acceptSweepSnapshot: func(ContextSnapshot) bool { return true }}
		release := service.gates.acquire(context.Background(), releaseMutationGateKey(snapshot))
		if release == nil {
			t.Fatal("failed to hold release gate")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result := service.SweepInert(ctx, snapshot)
		release()
		if result.Status != SweepTimedOut || fake.inspects != 0 || fake.uninstalls != 0 || len(result.Candidates) != 0 {
			t.Fatalf("result=%#v inspect=%d cleanup=%d", result, fake.inspects, fake.uninstalls)
		}
	})

	t.Run("during first proof", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		fake := &fakeInertSweeper{
			names: []string{first}, listOK: true, proof: helm.AcceleratorSweepEligible,
			onInspect: func(proofCtx context.Context) {
				if proofCtx.Err() != nil {
					t.Fatal("first proof did not start live")
				}
				cancel()
			},
		}
		service := &DisposalService{gates: &gateSet{}, sweeper: fake, acceptSweepSnapshot: func(ContextSnapshot) bool { return true }}
		result := service.SweepInert(ctx, snapshot)
		if result.Status != SweepTimedOut || fake.inspects != 1 || fake.uninstalls != 0 || len(result.Candidates) != 0 {
			t.Fatalf("result=%#v inspect=%d cleanup=%d", result, fake.inspects, fake.uninstalls)
		}
	})

	t.Run("after second proof does not start another candidate", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		fake := &fakeInertSweeper{
			names: []string{first, second}, listOK: true, proof: helm.AcceleratorSweepEligible,
			onCleanup: func(cleanupCtx context.Context) {
				if cleanupCtx.Err() != nil {
					t.Fatal("cleanup did not receive the live outer operation context")
				}
				cancel()
			},
		}
		service := &DisposalService{gates: &gateSet{}, sweeper: fake, acceptSweepSnapshot: func(ContextSnapshot) bool { return true }}
		result := service.SweepInert(ctx, snapshot)
		if result.Status != SweepTimedOut || fake.inspects != 1 || fake.uninstalls != 1 || len(result.Candidates) != 1 || result.Candidates[0].Status != SweepCleaned {
			t.Fatalf("result=%#v inspect=%d cleanup=%d", result, fake.inspects, fake.uninstalls)
		}
	})
}

func waitForGateReferences(t *testing.T, gates *gateSet, key string, want int) {
	t.Helper()
	for attempt := 0; attempt < 1_000_000; attempt++ {
		gates.mu.Lock()
		refs := 0
		if entry := gates.m[key]; entry != nil {
			refs = entry.refs
		}
		gates.mu.Unlock()
		if refs >= want {
			return
		}
		runtime.Gosched()
	}
	gates.mu.Lock()
	refs := make(map[string]int, len(gates.m))
	for held, entry := range gates.m {
		refs[held] = entry.refs
	}
	gates.mu.Unlock()
	t.Fatalf("gate %q never reached %d references: %v", key, want, refs)
}

func TestSweepProvisionAndOwnedDisposalShareExactReleaseMutationGate(t *testing.T) {
	const candidateName = "kubikles-accelerator-00000000000000000000000000000001"
	snapshot := fakeSnapshot{identity: "snapshot", namespace: "team-a", client: fakeClientset(observedJob(observerAttempt()), observedPod(observerAttempt(), observedJob(observerAttempt())))}
	key := releaseMutationGateKey(snapshot)

	t.Run("provision before sweep", func(t *testing.T) {
		contexts := &fakeContexts{current: "ctx", snapshot: snapshot}
		charts := &fakeCharts{installOwned: true, prepareEntered: make(chan struct{}, 1), prepareRelease: make(chan struct{})}
		provisioner := New(contexts, charts, &fakeObserver{})
		provisioner.entropy = &repeatReader{}
		fake := &fakeInertSweeper{names: []string{candidateName}, listOK: true, proof: helm.AcceleratorSweepEligible}
		disposer := NewDisposalService(provisioner)
		disposer.sweeper = fake
		disposer.acceptSweepSnapshot = func(ContextSnapshot) bool { return true }

		provisioned := make(chan Result, 1)
		go func() { provisioned <- provisioner.Provision(context.Background(), validRequest()) }()
		<-charts.prepareEntered
		swept := make(chan SweepResult, 1)
		go func() { swept <- disposer.SweepInert(context.Background(), snapshot) }()
		waitForGateReferences(t, &provisioner.gates, key, 2)
		if fake.inspects != 0 || fake.uninstalls != 0 {
			t.Fatalf("sweep crossed provision gate: inspect=%d cleanup=%d", fake.inspects, fake.uninstalls)
		}
		close(charts.prepareRelease)
		if result := <-provisioned; result.Availability != Available {
			t.Fatalf("provision availability=%s reason=%s", result.Availability, result.Reason)
		}
		if result := <-swept; result.Status != SweepCompleted || fake.inspects != 1 || fake.uninstalls != 1 {
			t.Fatalf("sweep status=%s inspect=%d cleanup=%d", result.Status, fake.inspects, fake.uninstalls)
		}
	})

	t.Run("sweep before provision", func(t *testing.T) {
		contexts := &fakeContexts{current: "ctx", snapshot: snapshot}
		charts := &fakeCharts{installOwned: true}
		provisioner := New(contexts, charts, &fakeObserver{})
		provisioner.entropy = &repeatReader{}
		cleanupEntered, cleanupRelease := make(chan struct{}), make(chan struct{})
		fake := &fakeInertSweeper{
			names: []string{candidateName}, listOK: true, proof: helm.AcceleratorSweepEligible,
			onCleanup: func(context.Context) { close(cleanupEntered); <-cleanupRelease },
		}
		disposer := NewDisposalService(provisioner)
		disposer.sweeper = fake
		disposer.acceptSweepSnapshot = func(ContextSnapshot) bool { return true }

		swept := make(chan SweepResult, 1)
		go func() { swept <- disposer.SweepInert(context.Background(), snapshot) }()
		<-cleanupEntered
		provisioned := make(chan Result, 1)
		go func() { provisioned <- provisioner.Provision(context.Background(), validRequest()) }()
		waitForGateReferences(t, &provisioner.gates, key, 2)
		charts.mu.Lock()
		calls := len(charts.calls)
		charts.mu.Unlock()
		if calls != 0 {
			t.Fatalf("provision crossed sweep gate: calls=%d", calls)
		}
		close(cleanupRelease)
		if result := <-swept; result.Status != SweepCompleted {
			t.Fatalf("sweep status=%s", result.Status)
		}
		if result := <-provisioned; result.Availability != Available {
			t.Fatalf("provision availability=%s reason=%s", result.Availability, result.Reason)
		}
	})

	t.Run("owned disposal before sweep", func(t *testing.T) {
		provisioner := New(&fakeContexts{current: "ctx", snapshot: snapshot}, &fakeCharts{installOwned: true}, &fakeObserver{})
		disposer := NewDisposalService(provisioner)
		cleanupEntered, cleanupRelease := make(chan struct{}), make(chan struct{})
		disposer.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
			close(cleanupEntered)
			<-cleanupRelease
			return OwnershipAlreadyGone, UninstallNotNeeded, DisappearanceSucceeded
		}
		fake := &fakeInertSweeper{names: []string{candidateName}, listOK: true, proof: helm.AcceleratorSweepEligible}
		disposer.sweeper = fake
		disposer.acceptSweepSnapshot = func(ContextSnapshot) bool { return true }

		workload := connectorWorkload(t)
		disposed := make(chan DisposalResult, 1)
		go func() { disposed <- disposer.DisposeNow(context.Background(), workload) }()
		<-cleanupEntered
		swept := make(chan SweepResult, 1)
		go func() { swept <- disposer.SweepInert(context.Background(), snapshot) }()
		waitForGateReferences(t, &provisioner.gates, key, 2)
		if fake.inspects != 0 || fake.uninstalls != 0 {
			t.Fatalf("sweep crossed owned-disposal gate: inspect=%d cleanup=%d", fake.inspects, fake.uninstalls)
		}
		close(cleanupRelease)
		if result := <-disposed; result.Ownership != OwnershipAlreadyGone {
			t.Fatalf("ownership=%s", result.Ownership)
		}
		if result := <-swept; result.Status != SweepCompleted || fake.inspects != 1 || fake.uninstalls != 1 {
			t.Fatalf("sweep status=%s inspect=%d cleanup=%d", result.Status, fake.inspects, fake.uninstalls)
		}
	})

	t.Run("sweep before owned disposal", func(t *testing.T) {
		provisioner := New(&fakeContexts{current: "ctx", snapshot: snapshot}, &fakeCharts{installOwned: true}, &fakeObserver{})
		disposer := NewDisposalService(provisioner)
		cleanupEntered, cleanupRelease := make(chan struct{}), make(chan struct{})
		fake := &fakeInertSweeper{
			names: []string{candidateName}, listOK: true, proof: helm.AcceleratorSweepEligible,
			onCleanup: func(context.Context) { close(cleanupEntered); <-cleanupRelease },
		}
		disposer.sweeper = fake
		disposer.acceptSweepSnapshot = func(ContextSnapshot) bool { return true }
		cleanupCalls := 0
		disposer.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
			cleanupCalls++
			return OwnershipAlreadyGone, UninstallNotNeeded, DisappearanceSucceeded
		}

		swept := make(chan SweepResult, 1)
		go func() { swept <- disposer.SweepInert(context.Background(), snapshot) }()
		<-cleanupEntered
		workload := connectorWorkload(t)
		disposed := make(chan DisposalResult, 1)
		go func() { disposed <- disposer.DisposeNow(context.Background(), workload) }()
		waitForWorkloadDisposing(workload)
		waitForGateReferences(t, &provisioner.gates, key, 2)
		if cleanupCalls != 0 {
			t.Fatal("owned cleanup crossed sweep gate")
		}
		close(cleanupRelease)
		if result := <-swept; result.Status != SweepCompleted {
			t.Fatalf("sweep status=%s", result.Status)
		}
		if result := <-disposed; result.Ownership != OwnershipAlreadyGone || cleanupCalls != 1 {
			t.Fatalf("ownership=%s cleanup calls=%d", result.Ownership, cleanupCalls)
		}
	})

	t.Run("owned disposal acquisition", func(t *testing.T) {
		contexts := &fakeContexts{current: "ctx", snapshot: snapshot}
		charts := &fakeCharts{installOwned: true, prepareEntered: make(chan struct{}, 1), prepareRelease: make(chan struct{})}
		provisioner := New(contexts, charts, &fakeObserver{})
		provisioner.entropy = &repeatReader{}
		disposer := NewDisposalService(provisioner)
		cleanupPhase := make(chan struct{})
		phaseCalls := 0
		disposer.phaseContext = func(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
			phaseCalls++
			if phaseCalls == 2 {
				close(cleanupPhase)
			}
			return context.WithCancel(parent)
		}
		cleanupCalls := 0
		disposer.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
			cleanupCalls++
			return OwnershipAlreadyGone, UninstallNotNeeded, DisappearanceSucceeded
		}

		provisioned := make(chan Result, 1)
		go func() { provisioned <- provisioner.Provision(context.Background(), validRequest()) }()
		<-charts.prepareEntered
		workload := connectorWorkload(t)
		disposed := make(chan DisposalResult, 1)
		go func() { disposed <- disposer.DisposeNow(context.Background(), workload) }()
		waitForWorkloadDisposing(workload)
		<-cleanupPhase
		waitForGateReferences(t, &provisioner.gates, key, 2)
		if cleanupCalls != 0 {
			t.Fatal("owned cleanup crossed provision gate")
		}
		close(charts.prepareRelease)
		<-provisioned
		if result := <-disposed; result.Ownership != OwnershipAlreadyGone || cleanupCalls != 1 {
			t.Fatalf("ownership=%s cleanup calls=%d", result.Ownership, cleanupCalls)
		}
	})
}

func TestSweepResultRedactionAndDirectBoundaryTripwires(t *testing.T) {
	hostile := []string{
		"raw watch verifier error",
		"Bearer cleanup-secret-token",
	}
	var logs bytes.Buffer
	prior := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prior)

	snapshot := fakeSnapshot{identity: "identity", namespace: "default", client: fakeClientset(observedJob(observerAttempt()), observedPod(observerAttempt(), observedJob(observerAttempt())))}
	sweeper := &fakeInertSweeper{names: []string{"kubikles-accelerator-00000000000000000000000000000001"}, listOK: true, proof: helm.AcceleratorSweepUnsupportedMalformed}
	sweepService := &DisposalService{gates: &gateSet{}, sweeper: sweeper, acceptSweepSnapshot: func(ContextSnapshot) bool { return true }}
	sweepResult := sweepService.SweepInert(context.Background(), snapshot)
	if sweepResult.Status != SweepCompleted || len(sweepResult.Candidates) != 1 || sweepResult.Candidates[0].Status != SweepUnsupportedMalformed {
		t.Fatalf("sweep status=%s candidates=%d", sweepResult.Status, len(sweepResult.Candidates))
	}

	workload := connectorWorkload(t)
	client := workload.snapshot.Clientset().(*k8sfake.Clientset)
	watchErrors, cleanupErrors := 0, 0
	client.PrependWatchReactor("jobs", func(k8stesting.Action) (bool, watch.Interface, error) {
		watchErrors++
		return true, nil, errors.New(hostile[0])
	})
	client.PrependReactor("get", "pods", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		cleanupErrors++
		return true, nil, errors.New(hostile[1])
	})
	disposalService := NewDisposalService(nil)
	disposalService.observeDrain = observeExactDrainJobSettled
	disposalService.cleanupOwned = func(ctx context.Context, receipt *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
		disappearance := waitDisposedAcceleratorResources(ctx, receipt.snapshot, &helm.AcceleratorOwnershipReceipt{}, receipt.pod, func(context.Context, time.Duration) bool { return false })
		return OwnershipProven, UninstallFailed, disappearance
	}
	disposalResult := disposalService.DrainAndDispose(context.Background(), workload)
	if disposalResult.Observation != DrainReadError || disposalResult.Ownership != OwnershipProven || disposalResult.Uninstall != UninstallFailed || disposalResult.Disappearance != DisappearanceResourcesRemaining || watchErrors != 1 || cleanupErrors != 1 {
		t.Fatalf("result=%#v watch errors=%d cleanup errors=%d", disposalResult, watchErrors, cleanupErrors)
	}

	encoded, err := json.Marshal(struct {
		Sweep    SweepResult    `json:"sweep"`
		Disposal DisposalResult `json:"disposal"`
	}{sweepResult, disposalResult})
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{fmt.Sprintf("%v", sweepResult), fmt.Sprintf("%#v", sweepResult), fmt.Sprintf("%v", disposalResult), fmt.Sprintf("%#v", disposalResult), string(encoded), logs.String()} {
		for _, raw := range hostile {
			if strings.Contains(output, raw) {
				t.Fatal("disposal/sweep output exposed hostile corpus")
			}
		}
	}
}

func TestInvalidOrMalformedSweepLeavesDirectKubernetesMethodCallable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/namespaces/default/secrets" {
			http.NotFound(writer, request)
			return
		}
		table := metav1.Table{
			ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}},
			Rows: []metav1.TableRow{{
				Cells:  []interface{}{"direct-secret", "Opaque", int64(1)},
				Object: k8sruntime.RawExtension{Raw: []byte(`{"metadata":{"name":"direct-secret","namespace":"default","uid":"direct-uid","creationTimestamp":"2026-08-02T00:00:00Z"}}`)},
			}},
		}
		_ = json.NewEncoder(writer).Encode(table)
	}))
	defer server.Close()
	direct, err := localk8s.NewClientForRESTConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	assertDirect := func() {
		items, listErr := direct.ListSecretsMetadataWithContext(context.Background(), "default")
		if listErr != nil || len(items) != 1 || items[0].Metadata.Name != "direct-secret" {
			t.Fatalf("direct list items=%d err=%v", len(items), listErr)
		}
	}

	invalid := NewDisposalService(nil).SweepInert(context.Background(), nil)
	if invalid.Status != SweepInvalidSnapshot {
		t.Fatalf("invalid status=%s", invalid.Status)
	}
	assertDirect()

	job := observedJob(observerAttempt())
	snapshot := fakeSnapshot{identity: "identity", namespace: "default", client: fakeClientset(job, observedPod(observerAttempt(), job))}
	sweeper := &fakeInertSweeper{names: []string{"kubikles-accelerator-00000000000000000000000000000001"}, listOK: true, proof: helm.AcceleratorSweepUnsupportedMalformed}
	service := &DisposalService{gates: &gateSet{}, sweeper: sweeper, acceptSweepSnapshot: func(ContextSnapshot) bool { return true }}
	malformed := service.SweepInert(context.Background(), snapshot)
	if malformed.Status != SweepCompleted || len(malformed.Candidates) != 1 || malformed.Candidates[0].Status != SweepUnsupportedMalformed {
		t.Fatalf("malformed status=%s candidates=%d", malformed.Status, len(malformed.Candidates))
	}
	assertDirect()
}
