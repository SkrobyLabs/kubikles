package acceleratorprovision

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kubikles/pkg/acceleratorrelease"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type fakeSnapshot struct {
	identity, namespace string
	client              kubernetes.Interface
}

func (s fakeSnapshot) Identity() string                { return s.identity }
func (s fakeSnapshot) Namespace() string               { return s.namespace }
func (s fakeSnapshot) RESTConfig() *rest.Config        { return &rest.Config{} }
func (s fakeSnapshot) Clientset() kubernetes.Interface { return s.client }

type fakeContexts struct {
	mu            sync.Mutex
	current       string
	snapshot      ContextSnapshot
	err           error
	calls         int
	snapshotCalls chan int
}

func (p *fakeContexts) SnapshotCurrentContext(name string) (ContextSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.snapshotCalls != nil {
		select {
		case p.snapshotCalls <- p.calls:
		default:
		}
	}
	if p.err != nil || name != p.current {
		return nil, p.err
	}
	return p.snapshot, nil
}
func (p *fakeContexts) CurrentContext() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}
func (p *fakeContexts) setCurrent(name string) {
	p.mu.Lock()
	p.current = name
	p.mu.Unlock()
}
func (p *fakeContexts) setSnapshot(snapshot ContextSnapshot) {
	p.mu.Lock()
	p.snapshot = snapshot
	p.mu.Unlock()
}

type fakeCharts struct {
	mu                sync.Mutex
	calls             []string
	prepareReason     UnavailableReason
	installReason     UnavailableReason
	installOwned      bool
	missingJobUID     bool
	ownershipUnproven bool
	cleanupStatus     CleanupStatus
	prepareEntered    chan struct{}
	prepareRelease    chan struct{}
	onPrepare         func()
	onInstall         func()
	cleanupCtxErr     error
	ignoreBoundary    bool
	prepareInputs     []error
	installInputs     []error
	installMessages   []string
	cleanupInputs     []error
	consumedInputs    []string
}

func (f *fakeCharts) record(call string) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}
func hostileInputDigest(rawErrors []error, rawMessages []string) string {
	hash := sha256.New()
	for _, rawError := range rawErrors {
		if rawError != nil {
			_, _ = io.WriteString(hash, rawError.Error())
		}
		_, _ = hash.Write([]byte{0})
	}
	for _, rawMessage := range rawMessages {
		_, _ = io.WriteString(hash, rawMessage)
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
func (f *fakeCharts) consume(stage string, rawErrors []error, rawMessages []string) {
	if len(rawErrors) == 0 && len(rawMessages) == 0 {
		return
	}
	f.mu.Lock()
	f.consumedInputs = append(f.consumedInputs, stage+":"+hostileInputDigest(rawErrors, rawMessages))
	f.mu.Unlock()
}
func (f *fakeCharts) safeOutput() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fmt.Sprint(f.calls, f.consumedInputs)
}
func (f *fakeCharts) consumedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.consumedInputs)
}
func (f *fakeCharts) Prepare(_ context.Context, attempt chartAttempt, boundary func() UnavailableReason) (*preparedChart, UnavailableReason) {
	f.record("prepare")
	f.consume("prepare", f.prepareInputs, nil)
	if f.prepareEntered != nil {
		select {
		case f.prepareEntered <- struct{}{}:
		default:
		}
	}
	if f.prepareRelease != nil {
		<-f.prepareRelease
	}
	if f.onPrepare != nil {
		f.onPrepare()
	}
	if reason := boundary(); reason != "" && !f.ignoreBoundary {
		return nil, reason
	}
	if f.prepareReason != "" {
		return nil, f.prepareReason
	}
	return &preparedChart{jobName: attempt.ReleaseName + "-job", implementation: "prepared"}, ""
}
func (f *fakeCharts) Install(_ context.Context, _ ContextSnapshot, _ *preparedChart) (*ownedRelease, UnavailableReason, bool) {
	f.record("install")
	f.consume("install", f.installInputs, f.installMessages)
	if f.onInstall != nil {
		f.onInstall()
	}
	if f.installOwned {
		jobUID := "job-uid"
		if f.missingJobUID {
			jobUID = ""
		}
		return &ownedRelease{implementation: "owned", jobUID: jobUID}, f.installReason, false
	}
	return nil, f.installReason, f.ownershipUnproven
}
func (f *fakeCharts) Cleanup(ctx context.Context, _ ContextSnapshot, _ *preparedChart, _ *ownedRelease) CleanupStatus {
	f.record("cleanup")
	f.consume("cleanup", f.cleanupInputs, nil)
	f.cleanupCtxErr = ctx.Err()
	if f.cleanupStatus == "" {
		return CleanupSucceeded
	}
	return f.cleanupStatus
}

type fakeObserver struct {
	reason         UnavailableReason
	calls          atomic.Int32
	onCall         func()
	ignoreBoundary bool
	hostileInputs  []error
	mu             sync.Mutex
	consumedInputs []string
}

func (o *fakeObserver) consume() {
	if len(o.hostileInputs) == 0 {
		return
	}
	o.mu.Lock()
	o.consumedInputs = append(o.consumedInputs, "observe:"+hostileInputDigest(o.hostileInputs, nil))
	o.mu.Unlock()
}
func (o *fakeObserver) safeOutput() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return fmt.Sprint(o.calls.Load(), o.consumedInputs)
}
func (o *fakeObserver) consumedCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.consumedInputs)
}

func (o *fakeObserver) Observe(_ context.Context, _ chartAttempt, _ ContextSnapshot, boundary func() UnavailableReason) (ObjectIdentity, ObjectIdentity, UnavailableReason) {
	o.calls.Add(1)
	o.consume()
	if o.onCall != nil {
		o.onCall()
	}
	if reason := boundary(); reason != "" && !o.ignoreBoundary {
		return ObjectIdentity{}, ObjectIdentity{}, reason
	}
	if o.reason != "" {
		return ObjectIdentity{}, ObjectIdentity{}, o.reason
	}
	return ObjectIdentity{Name: "job", UID: "job-uid"}, ObjectIdentity{Name: "pod", UID: "pod-uid"}, ""
}

func TestProvisionPhaseBoundaryWinsConcurrentFailure(t *testing.T) {
	tests := []struct {
		name        string
		charts      *fakeCharts
		observer    *fakeObserver
		wantCleanup CleanupStatus
	}{
		{name: "prepare pull error", charts: &fakeCharts{prepareReason: ChartPullFailed, ignoreBoundary: true}, observer: &fakeObserver{}, wantCleanup: CleanupNotNeeded},
		{name: "install error retains receipt", charts: &fakeCharts{installOwned: true, installReason: InstallFailed}, observer: &fakeObserver{}, wantCleanup: CleanupSucceeded},
		{name: "cancelled ambiguous storage", charts: &fakeCharts{installReason: InstallFailed, ownershipUnproven: true}, observer: &fakeObserver{}, wantCleanup: CleanupOwnershipUnproven},
		{name: "terminal observer error", charts: &fakeCharts{installOwned: true}, observer: &fakeObserver{reason: PodFailed, ignoreBoundary: true}, wantCleanup: CleanupSucceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contexts := &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity", namespace: "default"}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			test.charts.onPrepare = func() {
				if test.name == "prepare pull error" {
					cancel()
				}
			}
			test.charts.onInstall = func() {
				if test.name == "install error retains receipt" || test.name == "cancelled ambiguous storage" {
					cancel()
				}
			}
			test.observer.onCall = func() {
				if test.name == "terminal observer error" {
					cancel()
				}
			}
			service := New(contexts, test.charts, test.observer)
			service.entropy = bytes.NewReader(vectorEntropy())
			result := service.Provision(ctx, validRequest())
			if result.Reason != Cancelled || result.Cleanup != test.wantCleanup {
				t.Fatalf("result=%#v", result)
			}
			if test.wantCleanup == CleanupSucceeded && strings.Count(strings.Join(test.charts.calls, ","), "cleanup") != 1 {
				t.Fatalf("cleanup was not exactly once: %v", test.charts.calls)
			}
		})
	}
}

type repeatReader struct {
	mu sync.Mutex
	n  byte
}

func (r *repeatReader) Read(buffer []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range buffer {
		buffer[i] = r.n
		r.n++
	}
	return len(buffer), nil
}

type countingReader struct {
	reader io.Reader
	reads  atomic.Int32
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	r.reads.Add(1)
	return r.reader.Read(buffer)
}

func TestProvisionSerializesExactContextIdentity(t *testing.T) {
	contexts := &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity-a", namespace: "default"}}
	charts := &fakeCharts{installOwned: true, prepareEntered: make(chan struct{}, 2), prepareRelease: make(chan struct{})}
	service := New(contexts, charts, &fakeObserver{})
	service.entropy = &repeatReader{}
	firstResult := make(chan Result, 1)
	go func() { firstResult <- service.Provision(context.Background(), validRequest()) }()
	select {
	case <-charts.prepareEntered:
	case <-time.After(time.Second):
		t.Fatal("first attempt did not enter prepare")
	}
	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiterResult := make(chan Result, 1)
	go func() { waiterResult <- service.Provision(waiterCtx, validRequest()) }()
	time.Sleep(30 * time.Millisecond)
	charts.mu.Lock()
	prepareCalls := 0
	for _, call := range charts.calls {
		if call == "prepare" {
			prepareCalls++
		}
	}
	charts.mu.Unlock()
	if prepareCalls != 1 {
		t.Fatalf("same identity was not serialized: %d prepares", prepareCalls)
	}
	cancelWaiter()
	if result := <-waiterResult; result.Reason != Cancelled || result.Cleanup != CleanupNotNeeded {
		t.Fatalf("waiter result=%#v", result)
	}
	close(charts.prepareRelease)
	if result := <-firstResult; result.Availability != Available {
		t.Fatalf("first result=%#v", result)
	}
	service.gates.mu.Lock()
	remaining := len(service.gates.m)
	service.gates.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("idle gates retained: %d", remaining)
	}

	t.Run("different keys proceed independently", func(t *testing.T) {
		var gates gateSet
		releaseA := gates.acquire(context.Background(), "a")
		if releaseA == nil {
			t.Fatal("key a")
		}
		acquiredB := make(chan func(), 1)
		go func() { acquiredB <- gates.acquire(context.Background(), "b") }()
		select {
		case releaseB := <-acquiredB:
			if releaseB == nil {
				t.Fatal("key b")
			}
			releaseB()
		case <-time.After(time.Second):
			t.Fatal("different key blocked")
		}
		releaseA()
	})
}

func TestProvisionWaiterRefreshRequiresSameCompleteMutationGateKey(t *testing.T) {
	for _, test := range []struct {
		name          string
		refresh       fakeSnapshot
		wantAvailable bool
	}{
		{name: "namespace changed", refresh: fakeSnapshot{identity: "identity-a", namespace: "ns-b"}},
		{name: "same key", refresh: fakeSnapshot{identity: "identity-a", namespace: "ns-a"}, wantAvailable: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := make(chan int, 8)
			contexts := &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity-a", namespace: "ns-a"}, snapshotCalls: calls}
			charts := &fakeCharts{installOwned: true, prepareEntered: make(chan struct{}, 2), prepareRelease: make(chan struct{})}
			observer := &fakeObserver{}
			service := New(contexts, charts, observer)
			service.entropy = &repeatReader{}

			first := make(chan Result, 1)
			go func() { first <- service.Provision(context.Background(), validRequest()) }()
			for want := 1; want <= 2; want++ {
				if got := <-calls; got != want {
					t.Fatalf("snapshot call=%d want=%d", got, want)
				}
			}
			<-charts.prepareEntered

			waiter := make(chan Result, 1)
			go func() { waiter <- service.Provision(context.Background(), validRequest()) }()
			if got := <-calls; got != 3 {
				t.Fatalf("waiter initial snapshot call=%d", got)
			}
			select {
			case <-charts.prepareEntered:
				t.Fatal("waiter crossed the acquired mutation gate")
			default:
			}

			contexts.setSnapshot(test.refresh)
			close(charts.prepareRelease)
			if result := <-first; result.Availability != Available {
				t.Fatalf("first result=%#v", result)
			}
			result := <-waiter
			if test.wantAvailable {
				if result.Availability != Available || result.Workload == nil || result.Workload.ReleaseNamespace != "ns-a" {
					t.Fatalf("same-key result=%#v", result)
				}
				if observer.calls.Load() != 2 {
					t.Fatalf("same-key observations=%d", observer.calls.Load())
				}
			} else {
				if result.Availability != Unavailable || result.Reason != ContextChanged || result.Cleanup != CleanupNotNeeded {
					t.Fatalf("changed-key result=%#v", result)
				}
				charts.mu.Lock()
				gotCalls := strings.Join(charts.calls, ",")
				charts.mu.Unlock()
				if gotCalls != "prepare,install" || observer.calls.Load() != 1 {
					t.Fatalf("changed-key crossed provision mutation: chart calls=%s observations=%d", gotCalls, observer.calls.Load())
				}
			}
		})
	}
}

func TestProvisionOutcomeMatrix(t *testing.T) {
	tests := []struct {
		name              string
		mutateResolution  func(*Request)
		contextErr        error
		entropy           io.Reader
		prepareReason     UnavailableReason
		installReason     UnavailableReason
		installOwned      bool
		missingJobUID     bool
		ownershipUnproven bool
		observeReason     UnavailableReason
		wantReason        UnavailableReason
		wantCleanup       CleanupStatus
		wantAvailable     bool
	}{
		{name: "success", installOwned: true, wantAvailable: true, wantCleanup: CleanupNotNeeded},
		{name: "successful install missing receipt job uid", installOwned: true, missingJobUID: true, wantReason: InstallFailed, wantCleanup: CleanupSucceeded},
		{name: "artifact unavailable", mutateResolution: func(request *Request) { request.Resolution.Availability = acceleratorrelease.Unavailable }, wantReason: ArtifactUnavailable, wantCleanup: CleanupNotNeeded},
		{name: "malformed release", mutateResolution: func(request *Request) { request.Resolution.Release.SourceCommit = strings.Repeat("X", 40) }, wantReason: ArtifactUnavailable, wantCleanup: CleanupNotNeeded},
		{name: "context unavailable", contextErr: errors.New("secret kubeconfig path"), wantReason: ContextUnavailable, wantCleanup: CleanupNotNeeded},
		{name: "entropy", entropy: bytes.NewReader(make([]byte, 31)), wantReason: EntropyUnavailable, wantCleanup: CleanupNotNeeded},
		{name: "pull", prepareReason: ChartPullFailed, wantReason: ChartPullFailed, wantCleanup: CleanupNotNeeded},
		{name: "integrity", prepareReason: ChartIntegrityFailed, wantReason: ChartIntegrityFailed, wantCleanup: CleanupNotNeeded},
		{name: "render", prepareReason: RenderFailed, wantReason: RenderFailed, wantCleanup: CleanupNotNeeded},
		{name: "conflict", installReason: ReleaseConflict, wantReason: ReleaseConflict, wantCleanup: CleanupNotNeeded},
		{name: "permission", installReason: PermissionDenied, wantReason: PermissionDenied, wantCleanup: CleanupNotNeeded},
		{name: "install ambiguous", installReason: InstallFailed, ownershipUnproven: true, wantReason: InstallFailed, wantCleanup: CleanupOwnershipUnproven},
		{name: "owned install failure", installReason: InstallFailed, installOwned: true, wantReason: InstallFailed, wantCleanup: CleanupSucceeded},
		{name: "job", installOwned: true, observeReason: JobFailed, wantReason: JobFailed, wantCleanup: CleanupSucceeded},
		{name: "pod", installOwned: true, observeReason: PodFailed, wantReason: PodFailed, wantCleanup: CleanupSucceeded},
		{name: "image", installOwned: true, observeReason: ImagePullFailed, wantReason: ImagePullFailed, wantCleanup: CleanupSucceeded},
		{name: "timeout", installOwned: true, observeReason: TimedOut, wantReason: TimedOut, wantCleanup: CleanupSucceeded},
		{name: "cancel", installOwned: true, observeReason: Cancelled, wantReason: Cancelled, wantCleanup: CleanupSucceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			if test.mutateResolution != nil {
				test.mutateResolution(&request)
			}
			contexts := &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity", namespace: "default"}, err: test.contextErr}
			charts := &fakeCharts{prepareReason: test.prepareReason, installReason: test.installReason, installOwned: test.installOwned, missingJobUID: test.missingJobUID, ownershipUnproven: test.ownershipUnproven}
			observer := &fakeObserver{reason: test.observeReason}
			service := New(contexts, charts, observer)
			if test.entropy != nil {
				service.entropy = test.entropy
			} else {
				service.entropy = bytes.NewReader(vectorEntropy())
			}
			result := service.Provision(context.Background(), request)
			if test.wantAvailable {
				if result.Availability != Available || result.Workload == nil || result.Workload.ContextName != "ctx" || result.Workload.ReleaseNamespace != "default" || result.Workload.BuildVersion != "v1.2.3" || result.Workload.ImageDigest != "sha256:"+strings.Repeat("a", 64) || result.Workload.ChartDigest != "sha256:"+strings.Repeat("b", 64) || result.Workload.credential == nil {
					t.Fatalf("invalid success: %#v", result)
				}
			} else if result.Availability != Unavailable || result.Reason != test.wantReason || result.Cleanup != test.wantCleanup || result.Workload != nil {
				t.Fatalf("result=%#v want reason=%s cleanup=%s", result, test.wantReason, test.wantCleanup)
			}
			if (test.wantReason == ArtifactUnavailable) && contexts.calls != 0 {
				t.Fatal("artifact rejection performed context IO")
			}
		})
	}
}

func TestProvisionRollbackOwnershipAndRaces(t *testing.T) {
	t.Run("context change after ownership", func(t *testing.T) {
		contexts := &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity", namespace: "default"}}
		charts := &fakeCharts{installOwned: true}
		charts.onInstall = func() { contexts.setCurrent("other") }
		service := New(contexts, charts, &fakeObserver{})
		service.entropy = bytes.NewReader(vectorEntropy())
		result := service.Provision(context.Background(), validRequest())
		if result.Reason != ContextChanged || result.Cleanup != CleanupSucceeded || charts.cleanupCtxErr != nil {
			t.Fatalf("result=%#v cleanup ctx=%v", result, charts.cleanupCtxErr)
		}
		if strings.Join(charts.calls, ",") != "prepare,install,cleanup" {
			t.Fatalf("calls=%v", charts.calls)
		}
	})

	t.Run("cleanup failure preserves primary", func(t *testing.T) {
		contexts := &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity", namespace: "default"}}
		charts := &fakeCharts{installOwned: true, cleanupStatus: CleanupFailed}
		service := New(contexts, charts, &fakeObserver{reason: ImagePullFailed})
		service.entropy = bytes.NewReader(vectorEntropy())
		result := service.Provision(context.Background(), validRequest())
		if result.Reason != ImagePullFailed || result.Cleanup != CleanupFailed || result.Workload != nil {
			t.Fatalf("result=%#v", result)
		}
	})

	t.Run("success never uninstalls", func(t *testing.T) {
		contexts := &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity", namespace: "default"}}
		charts := &fakeCharts{installOwned: true}
		service := New(contexts, charts, &fakeObserver{})
		service.entropy = bytes.NewReader(vectorEntropy())
		result := service.Provision(context.Background(), validRequest())
		if result.Availability != Available || strings.Contains(strings.Join(charts.calls, ","), "cleanup") {
			t.Fatalf("success cleanup: %#v calls=%v", result, charts.calls)
		}
	})
}

func TestProvisionSecretCorpusNeverEscapes(t *testing.T) {
	secrets := []string{
		"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", "w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM",
		"Bearer registry-secret-T11", "/secret/kubeconfig/path-T11", "raw-registry-error-T11", "raw-helm-error-T11",
		"raw-kubernetes-status-T11", "raw-cleanup-error-T11", "raw helm manifest T11", "raw helm value T11",
	}
	type hostileCase struct {
		name             string
		contexts         *fakeContexts
		charts           *fakeCharts
		observer         *fakeObserver
		entropy          io.Reader
		wantReason       UnavailableReason
		wantCleanup      CleanupStatus
		wantFakeConsumed int
		wantEntropyReads int32
	}
	cases := []hostileCase{
		{name: "kubeconfig path and bearer", contexts: &fakeContexts{current: "ctx", err: errors.New(secrets[2] + " " + secrets[3])}, charts: &fakeCharts{}, observer: &fakeObserver{}, entropy: bytes.NewReader(vectorEntropy()), wantReason: ContextUnavailable, wantCleanup: CleanupNotNeeded},
		{name: "entropy", contexts: &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity", namespace: "default"}}, charts: &fakeCharts{}, observer: &fakeObserver{}, entropy: &hostileErrorReader{err: errors.New(secrets[4])}, wantReason: EntropyUnavailable, wantCleanup: CleanupNotNeeded, wantEntropyReads: 1},
		{name: "registry", contexts: &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity", namespace: "default"}}, charts: &fakeCharts{prepareReason: ChartPullFailed, prepareInputs: []error{errors.New(secrets[2] + " " + secrets[4])}}, observer: &fakeObserver{}, entropy: bytes.NewReader(vectorEntropy()), wantReason: ChartPullFailed, wantCleanup: CleanupNotNeeded, wantFakeConsumed: 1},
		{name: "helm", contexts: &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity", namespace: "default"}}, charts: &fakeCharts{installReason: InstallFailed, ownershipUnproven: true, installInputs: []error{errors.New(secrets[5])}, installMessages: []string{secrets[8], secrets[9]}}, observer: &fakeObserver{}, entropy: bytes.NewReader(vectorEntropy()), wantReason: InstallFailed, wantCleanup: CleanupOwnershipUnproven, wantFakeConsumed: 1},
		{name: "kubernetes and cleanup", contexts: &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity", namespace: "default"}}, charts: &fakeCharts{installOwned: true, cleanupStatus: CleanupFailed, cleanupInputs: []error{errors.New(secrets[7])}}, observer: &fakeObserver{reason: PodFailed, hostileInputs: []error{apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "hostile", errors.New(secrets[6]))}}, entropy: bytes.NewReader(vectorEntropy()), wantReason: PodFailed, wantCleanup: CleanupFailed, wantFakeConsumed: 2},
	}
	var logOutput bytes.Buffer
	priorLogWriter := log.Writer()
	log.SetOutput(&logOutput)
	defer log.SetOutput(priorLogWriter)
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logOutput.Reset()
			service := New(test.contexts, test.charts, test.observer)
			service.entropy = test.entropy
			result := service.Provision(context.Background(), validRequest())
			if result.Availability != Unavailable || result.Reason != test.wantReason || result.Cleanup != test.wantCleanup || result.Workload != nil {
				t.Fatalf("closed result=%#v want reason=%s cleanup=%s", result, test.wantReason, test.wantCleanup)
			}
			if consumed := test.charts.consumedCount() + test.observer.consumedCount(); consumed != test.wantFakeConsumed {
				t.Fatalf("hostile fake inputs consumed=%d want=%d", consumed, test.wantFakeConsumed)
			}
			if reader, ok := test.entropy.(*hostileErrorReader); ok && reader.reads.Load() != test.wantEntropyReads {
				t.Fatalf("hostile entropy reads=%d want=%d", reader.reads.Load(), test.wantEntropyReads)
			}
			if test.contexts.calls == 0 {
				t.Fatal("hostile context seam was not consumed")
			}
			outputs := []string{
				fmt.Sprintf("%v", result), fmt.Sprintf("%+v", result), fmt.Sprintf("%#v", result),
				fmt.Sprintf("%s", result), fmt.Sprintf("%q", result), fmt.Sprintf("%x", result),
				test.charts.safeOutput(), test.observer.safeOutput(), logOutput.String(),
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			outputs = append(outputs, string(encoded))
			for _, output := range outputs {
				for _, secret := range secrets {
					if strings.Contains(output, secret) {
						t.Fatal("secret/raw adapter error escaped through result or fake output")
					}
				}
			}
			if logOutput.Len() != 0 {
				t.Fatal("provisioning emitted a standard-library log entry")
			}
		})
	}
	for _, name := range []string{"service.go", "helm_adapter.go", "observer.go", "credential.go"} {
		source, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"pkg/debug", "pkg/crashlog", "pkg/events", "log.", "slog."} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("provisioning source acquired forbidden log/event boundary %q", forbidden)
			}
		}
	}
}

func TestKindHarnessScansSuccessfulOutputForCredentialCorpus(t *testing.T) {
	sourceBytes, err := os.ReadFile(filepath.Join("..", "..", "scripts", "test-accelerator-desktop-provision-kind.sh"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(sourceBytes)
	for _, required := range []string{
		"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
		"MDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSElKS0xNTk8",
		"w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM",
		"66-MHiCJFjcS0tgtsrGBc-19KNhsQvlV7hXKV3AF2Mo",
		`grep -Fq -- "$sensitive" "$capture"`,
		`fail "go-service-output-sensitive"`,
		"[REDACTED_TEST_CORPUS]",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("Kind output gate missing required static proof %q", required)
		}
	}
	testCapture := strings.Index(source, `>"$tmp/go-test" 2>&1`)
	outputScan := strings.Index(source, `captured_output_sensitive "$tmp/go-test" "$integrated_output"`)
	passed := strings.Index(source, `echo "accelerator-desktop-provision-kind: passed"`)
	if testCapture < 0 || outputScan <= testCapture || passed <= outputScan {
		t.Fatal("Kind output scan does not gate success after captured go test output")
	}
	if strings.Contains(source, `echo "$sensitive"`) || strings.Contains(source, `fail "$sensitive"`) {
		t.Fatal("Kind output gate would disclose the matched sensitive value")
	}
}

func TestKindHarnessIntegratedRoutingDiagnosticExtractorIsClosed(t *testing.T) {
	helper := filepath.Join("..", "..", "scripts", "extract-accelerator-kind-diagnostic.sh")
	harnessBytes, err := os.ReadFile(filepath.Join("..", "..", "scripts", "test-accelerator-desktop-provision-kind.sh"))
	if err != nil {
		t.Fatal(err)
	}
	harness := string(harnessBytes)
	testName := "TestAcceleratorIntegratedRoutingKind"
	phaseCodes := []string{
		"initial-missing", "initial-sweeping", "initial-resolving-zero", "initial-resolving-after-provision",
		"initial-provisioning", "initial-connecting", "initial-active-client-bind", "initial-active-ready-path",
		"initial-unavailable", "initial-terminal", "initial-unknown", "initial-count-invalid", "initial-client-repeat",
		"initial-provision-retry", "initial-provision-context-input", "initial-provision-chart-pull",
		"initial-provision-chart-integrity-render", "initial-provision-install-conflict-permission", "initial-provision-image-pull",
		"initial-provision-job-pod", "initial-provision-timeout-cancel", "initial-provision-mixed", "initial-provision-unknown",
		"initial-connect-not-entered", "initial-connect-tunnel",
		"initial-connect-accelerator", "initial-connect-version", "initial-connect-authoritative", "initial-connect-cancelled",
		"initial-session-client-bind", "initial-session-ready-path", "initial-stage-mixed",
		"stage-setup", "stage-direct", "stage-pre-ready", "stage-ready", "stage-list", "stage-cancel", "stage-detail",
		"stage-watch", "stage-loss", "stage-resume", "stage-mismatch", "stage-isolation", "stage-release", "stage-final-verification",
	}
	recordAt := func(timestamp, testName, output string) string {
		encoded, err := json.Marshal(struct {
			Time, Action, Package, Test, Output string
		}{Time: timestamp, Action: "output", Package: "kubikles", Test: testName, Output: output})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded) + "\n"
	}
	record := func(testName, output string) string {
		return recordAt("2026-08-03T00:00:00Z", testName, output)
	}
	run := func(stream string) string {
		t.Helper()
		capture := filepath.Join(t.TempDir(), "go-test.json")
		if err := os.WriteFile(capture, []byte(stream), 0o600); err != nil {
			t.Fatal(err)
		}
		output, err := exec.Command("bash", helper, capture, testName).CombinedOutput()
		if err != nil {
			t.Fatalf("diagnostic extractor failed: %v", err)
		}
		return strings.TrimSpace(string(output))
	}
	for _, code := range phaseCodes {
		t.Run(code, func(t *testing.T) {
			stream := record(testName, "    accelerator_integrated_routing_kind_test.go:470: accelerator-kind-diagnostic:"+code+"\n")
			if got, want := run(stream), "go-service-test-"+code; got != want {
				t.Fatalf("diagnostic = %q, want %q", got, want)
			}
			if !strings.Contains(harness, "go-service-test-"+code) {
				t.Fatal("Kind harness diagnostic allowlist is missing the fixed code")
			}
		})
	}
	markerOutput := "    accelerator_integrated_routing_kind_test.go:470: accelerator-kind-diagnostic:initial-connecting\n"
	for _, test := range []struct {
		name, timestamp string
	}{
		{name: "UTC", timestamp: "2026-08-03T00:00:00Z"},
		{name: "positive offset", timestamp: "2024-02-29T23:59:59.1+05:30"},
		{name: "negative offset", timestamp: "2000-02-29T00:00:00.123456789-04:00"},
		{name: "calendar edge", timestamp: "2026-04-30T12:30:45+23:59"},
		{name: "unknown local offset", timestamp: "2026-12-31T23:59:59-00:00"},
	} {
		t.Run("timestamp "+test.name, func(t *testing.T) {
			if got, want := run(recordAt(test.timestamp, testName, markerOutput)), "go-service-test-initial-connecting"; got != want {
				t.Fatalf("timestamp diagnostic = %q, want %q", got, want)
			}
		})
	}
	test2jsonInput := "=== RUN   " + testName + "\n" + markerOutput
	for _, test := range []struct {
		name, zone string
		sign       byte
	}{
		{name: "test2json positive offset", zone: "Pacific/Auckland", sign: '+'},
		{name: "test2json negative offset", zone: "America/New_York", sign: '-'},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("go", "tool", "test2json", "-t", "-p", "kubikles")
			for _, variable := range os.Environ() {
				if !strings.HasPrefix(variable, "TZ=") {
					command.Env = append(command.Env, variable)
				}
			}
			command.Env = append(command.Env, "TZ="+test.zone)
			command.Stdin = strings.NewReader(test2jsonInput)
			stream, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("test2json failed: %v", err)
			}
			var timestamp string
			for _, line := range strings.Split(strings.TrimSpace(string(stream)), "\n") {
				var event struct{ Time, Output string }
				if json.Unmarshal([]byte(line), &event) == nil && event.Output == markerOutput {
					timestamp = event.Time
				}
			}
			if len(timestamp) < 6 || timestamp[len(timestamp)-6] != test.sign {
				t.Fatal("test2json did not emit the requested non-UTC zone")
			}
			if got, want := run(string(stream)), "go-service-test-initial-connecting"; got != want {
				t.Fatalf("test2json diagnostic = %q, want %q", got, want)
			}
		})
	}
	for _, superseded := range []string{"initial-count-fallback", "initial-provision-unavailable"} {
		if strings.Contains(harness, "go-service-test-"+superseded) {
			t.Fatal("Kind harness retained a superseded generic diagnostic code")
		}
	}
	credentialCorpus := "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	exact := record(testName, "    accelerator_integrated_routing_kind_test.go:470: accelerator-kind-diagnostic:initial-connecting\n")
	hostileBeforeOutput := strings.Replace(exact, `,"Output":`, `,"Hostile":"private","Output":`, 1)
	hostileJSON := strings.TrimSpace(exact)[:len(strings.TrimSpace(exact))-1] + `,"hostile":"private"}` + "\n"
	hostileMarker := record(testName, "hostile accelerator-kind-diagnostic:initial-connecting\n")
	legacyMarker := record(testName, "    accelerator_integrated_routing_kind_test.go:470: accelerator-kind-initial-diagnostic:initial-connecting\n")
	stageMarker := record(testName, "    accelerator_integrated_routing_kind_test.go:470: accelerator-kind-diagnostic:stage-list\n")
	noncanonicalTime := strings.Replace(exact, `"Time":"2026-08-03T00:00:00Z"`, `"Time":"fixed"`, 1)
	for _, test := range []struct {
		name, stream string
	}{
		{name: "arbitrary", stream: record(testName, "hostile-private-marker\n")},
		{name: "zero marker", stream: record(testName, "ordinary fixed failure\n")},
		{name: "unknown code", stream: record(testName, "    accelerator_integrated_routing_kind_test.go:470: accelerator-kind-diagnostic:stage-hostile\n")},
		{name: "superseded code", stream: record(testName, "    accelerator_integrated_routing_kind_test.go:470: accelerator-kind-diagnostic:initial-count-fallback\n")},
		{name: "superseded provision code", stream: record(testName, "    accelerator_integrated_routing_kind_test.go:470: accelerator-kind-diagnostic:initial-provision-unavailable\n")},
		{name: "legacy marker", stream: legacyMarker},
		{name: "wrong test", stream: record("TestOther", "    accelerator_integrated_routing_kind_test.go:470: accelerator-kind-diagnostic:initial-connecting\n")},
		{name: "credential prefix", stream: record(testName, credentialCorpus+" accelerator-kind-diagnostic:initial-connecting\n")},
		{name: "credential suffix", stream: record(testName, "    accelerator_integrated_routing_kind_test.go:470: accelerator-kind-diagnostic:initial-connecting "+credentialCorpus+"\n")},
		{name: "duplicate", stream: exact + exact},
		{name: "mixed initial and stage", stream: exact + stageMarker},
		{name: "mixed unified and legacy", stream: exact + legacyMarker},
		{name: "mixed canonical and hostile", stream: exact + hostileMarker},
		{name: "extra JSON field before output", stream: hostileBeforeOutput},
		{name: "extra JSON field", stream: hostileJSON},
		{name: "non JSON prefix", stream: "hostile-private-prefix" + exact},
		{name: "non JSON suffix", stream: strings.TrimSuffix(exact, "\n") + "hostile-private-suffix\n"},
		{name: "noncanonical time", stream: noncanonicalTime},
		{name: "non JSON", stream: "accelerator-kind-diagnostic:initial-connecting\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := run(test.stream); got != "go-service-test" {
				t.Fatalf("hostile diagnostic escaped as %q", got)
			}
		})
	}
	for _, test := range []struct {
		name, timestamp string
	}{
		{name: "non leap February", timestamp: "2023-02-29T00:00:00Z"},
		{name: "century non leap February", timestamp: "1900-02-29T00:00:00Z"},
		{name: "short April", timestamp: "2024-04-31T00:00:00Z"},
		{name: "month zero", timestamp: "2024-00-01T00:00:00Z"},
		{name: "month thirteen", timestamp: "2024-13-01T00:00:00Z"},
		{name: "day zero", timestamp: "2024-01-00T00:00:00Z"},
		{name: "hour twenty four", timestamp: "2024-01-01T24:00:00Z"},
		{name: "minute sixty", timestamp: "2024-01-01T00:60:00Z"},
		{name: "second sixty", timestamp: "2024-01-01T00:00:60Z"},
		{name: "empty fraction", timestamp: "2024-01-01T00:00:00.Z"},
		{name: "long fraction", timestamp: "2024-01-01T00:00:00.1234567890Z"},
		{name: "lowercase zone", timestamp: "2024-01-01T00:00:00z"},
		{name: "missing zone", timestamp: "2024-01-01T00:00:00"},
		{name: "offset hour twenty four", timestamp: "2024-01-01T00:00:00+24:00"},
		{name: "offset minute sixty", timestamp: "2024-01-01T00:00:00-04:60"},
		{name: "compact offset", timestamp: "2024-01-01T00:00:00+0530"},
		{name: "short offset hour", timestamp: "2024-01-01T00:00:00+5:30"},
	} {
		t.Run("timestamp "+test.name, func(t *testing.T) {
			if got := run(recordAt(test.timestamp, testName, markerOutput)); got != "go-service-test" {
				t.Fatalf("invalid timestamp escaped as %q", got)
			}
		})
	}
}

func TestKindHarnessIntegratedRoutingTimeoutBudgetsAreExplicit(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	testSource, err := os.ReadFile(filepath.Join(repoRoot, "accelerator_integrated_routing_kind_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	harnessSource, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "test-accelerator-desktop-provision-kind.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(testSource), "integratedRoutingKindInitialReadinessTimeout = acceleratorprovision.SweepTimeout + acceleratorprovision.ActivationAttemptTimeout + time.Minute") {
		t.Fatal("integrated routing initial readiness is not tied to production sweep and activation bounds")
	}
	if !strings.Contains(string(testSource), "integratedRoutingKindSequentialTimeout = integratedRoutingKindFixtureSetupTimeout") {
		t.Fatal("integrated routing sequential bound is not assembled from its inner phases")
	}
	if !strings.Contains(string(testSource), "client.SetAPITimeout(integratedRoutingKindDesktopAPITimeout)") {
		t.Fatal("integrated routing Direct Kubernetes calls do not have an explicit harness deadline")
	}
	teardownRegistration := strings.Index(string(testSource), "// Register bounded teardown first.")
	reporterRegistration := strings.Index(string(testSource), "diagnostic.report(t.Failed()")
	bodyStart := strings.Index(string(testSource), `chartDigest := requiredIntegratedRoutingKindEnv`)
	if teardownRegistration < 0 || reporterRegistration <= teardownRegistration || bodyStart <= reporterRegistration {
		t.Fatal("integrated routing diagnostic reporter is not registered after teardown and before the body")
	}
	if strings.Count(string(testSource), "integratedRoutingKindDiagnosticMarker") != 2 {
		t.Fatal("integrated routing test can emit a diagnostic marker outside the single reporter")
	}
	for _, required := range []string{
		"installIntegratedRoutingKindOperationBounds(app)",
		"return context.WithTimeout(parent, timeout)",
		"integratedRoutingKindFailureSequentialTimeout = integratedRoutingKindFixtureSetupTimeout",
		"c.probe.success(integratedRoutingKindList)",
		"c.probe.success(integratedRoutingKindData)",
		"c.probe.success(integratedRoutingKindYAML)",
	} {
		if !strings.Contains(string(testSource), required) {
			t.Fatalf("integrated routing bounded remote-success proof missing %q", required)
		}
	}
	for _, unbounded := range []string{
		`.Create(context.Background()`,
		`.Get(context.Background()`,
		`.Update(context.Background()`,
		`.Delete(context.Background()`,
		`.List(context.Background()`,
		`helmClient.ListReleases(`,
	} {
		if strings.Contains(string(testSource), unbounded) {
			t.Fatalf("integrated routing source retained unbounded API call %q", unbounded)
		}
	}
	if !strings.Contains(string(harnessSource), "go_test_timeout=34m") {
		t.Fatal("integrated routing outer timeout does not cover its sequential bounded phases")
	}
	privacy := strings.Index(string(harnessSource), `captured_output_sensitive "$tmp/go-test" 1`)
	extraction := strings.Index(string(harnessSource), `extract-accelerator-kind-diagnostic.sh" "$tmp/go-test"`)
	if privacy < 0 || extraction <= privacy {
		t.Fatal("integrated routing diagnostic extraction is not gated by the privacy scan")
	}
}

func TestKindHarnessMandatoryTestDiscoveryIsExactAndSilent(t *testing.T) {
	helper := filepath.Join("..", "..", "scripts", "check-accelerator-kind-test-discovery.sh")
	testName := "TestAcceleratorIntegratedRoutingKind"
	run := `{"Time":"fixed","Action":"run","Package":"kubikles","Test":"` + testName + `"}` + "\n"
	pass := `{"Time":"fixed","Action":"pass","Package":"kubikles","Test":"` + testName + `","Elapsed":0}` + "\n"
	skip := `{"Time":"fixed","Action":"skip","Package":"kubikles","Test":"` + testName + `","Elapsed":0}` + "\n"
	for _, test := range []struct {
		name, stream string
		accepted     bool
	}{
		{name: "exact", stream: run + `{"Action":"output","Package":"kubikles","Test":"` + testName + `","Output":"hostile-private-marker\\n"}` + "\n" + pass, accepted: true},
		{name: "zero", stream: `{"Action":"pass","Package":"kubikles"}` + "\n"},
		{name: "duplicate run", stream: run + run + pass},
		{name: "duplicate pass", stream: run + pass + pass},
		{name: "skip", stream: run + skip},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "go-test.json")
			if err := os.WriteFile(capture, []byte(test.stream), 0o600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("bash", helper, capture, testName)
			output, err := command.CombinedOutput()
			if (err == nil) != test.accepted {
				t.Fatalf("discovery acceptance=%v want=%v", err == nil, test.accepted)
			}
			if len(output) != 0 {
				t.Fatal("discovery helper disclosed captured test output")
			}
		})
	}
}

func TestKindHarnessRejectsStaleSourceImageBeforeMutation(t *testing.T) {
	sourceBytes, err := os.ReadFile(filepath.Join("..", "..", "scripts", "test-accelerator-desktop-provision-kind.sh"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(sourceBytes)
	revisionRead := strings.Index(source, `revision_json="$(docker image inspect "$source_image" --format '{{json (index .Config.Labels "org.opencontainers.image.revision")}}' 2>/dev/null)"`)
	pattern := strings.Index(source, `revision_json_pattern='^"([0-9a-f]{40})"$'`)
	matchAndCapture := `[[ "$revision_json" =~ $revision_json_pattern ]] || fail "source-image-revision-invalid"` + "\n" + `source_revision="${BASH_REMATCH[1]}"`
	capture := strings.Index(source, matchAndCapture)
	headRead := strings.Index(source, `expected_revision="$(git -C "$root" rev-parse HEAD 2>/dev/null)"`)
	rejection := strings.Index(source, `test "$source_revision" = "$expected_revision" || fail "source-image-revision-mismatch"`)
	mutableSetup := strings.Index(source, `tmp="$(mktemp -d`)
	firstContainer := strings.Index(source, `docker run --detach`)
	if revisionRead < 0 || pattern < 0 || capture < 0 || headRead < 0 || rejection < 0 {
		t.Fatal("Kind harness does not bind its source image revision to the exact checkout")
	}
	if !(revisionRead < pattern && pattern < capture && capture < headRead && headRead < rejection && rejection < mutableSetup && rejection < firstContainer) {
		t.Fatalf("source image revision gate ordering read=%d pattern=%d capture=%d head=%d rejection=%d setup=%d container=%d", revisionRead, pattern, capture, headRead, rejection, mutableSetup, firstContainer)
	}
	if strings.Contains(source, `--format '{{index .Config.Labels "org.opencontainers.image.revision"}}'`) {
		t.Fatal("Kind harness retained raw Docker label inspection")
	}
	for _, line := range strings.Split(source[revisionRead:mutableSetup], "\n") {
		failure := strings.Index(line, "fail ")
		if failure < 0 {
			continue
		}
		message := line[failure:]
		for _, variable := range []string{"$revision_json", "$source_revision", "$expected_revision", "${revision_json}", "${source_revision}", "${expected_revision}"} {
			if strings.Contains(message, variable) {
				t.Fatalf("source image revision failure interpolates private state: %s", message)
			}
		}
	}
}

func TestKindHarnessCanonicalRevisionFixtures(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(realGit) {
		realGit, err = filepath.Abs(realGit)
		if err != nil {
			t.Fatal(err)
		}
	}
	canonical := strings.Repeat("a", 40)
	different := strings.Repeat("b", 40)
	malformedCheckout := "checkout-malformed-secret"
	unavailableCheckout := "checkout-unavailable-secret"
	tests := []struct {
		name, revisionJSON, checkoutRevision, gitExit, wantFailure string
		accepted                                                   bool
	}{
		{name: "exact", revisionJSON: `"` + canonical + `"`, checkoutRevision: canonical, gitExit: "0", wantFailure: "registry-start", accepted: true},
		{name: "image trailing newline", revisionJSON: `"` + canonical + `\n"`, checkoutRevision: canonical, gitExit: "0", wantFailure: "source-image-revision-invalid"},
		{name: "image embedded nul", revisionJSON: `"` + canonical + `\u0000"`, checkoutRevision: canonical, gitExit: "0", wantFailure: "source-image-revision-invalid"},
		{name: "image missing", revisionJSON: "", checkoutRevision: canonical, gitExit: "0", wantFailure: "source-image-revision-invalid"},
		{name: "image missing null", revisionJSON: "null", checkoutRevision: canonical, gitExit: "0", wantFailure: "source-image-revision-invalid"},
		{name: "image malformed", revisionJSON: `"ABC"`, checkoutRevision: canonical, gitExit: "0", wantFailure: "source-image-revision-invalid"},
		{name: "image mismatch", revisionJSON: `"` + different + `"`, checkoutRevision: canonical, gitExit: "0", wantFailure: "source-image-revision-mismatch"},
		{name: "checkout malformed", revisionJSON: `"` + canonical + `"`, checkoutRevision: malformedCheckout, gitExit: "0", wantFailure: "checkout-revision-invalid"},
		{name: "checkout unavailable", revisionJSON: `"` + canonical + `"`, checkoutRevision: unavailableCheckout, gitExit: "42", wantFailure: "checkout-revision-unavailable"},
		{name: "checkout mismatch", revisionJSON: `"` + canonical + `"`, checkoutRevision: different, gitExit: "0", wantFailure: "source-image-revision-mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := t.TempDir()
			bin := filepath.Join(fixture, "bin")
			if err := os.Mkdir(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			writeTool := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nset -eu\n"+body), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			writeTool("docker", `case "${1:-}:${2:-}" in
info:) exit 0 ;;
image:inspect)
  case " $* " in
    *" --format "*) printf '%s\n' "$KIND_IMAGE_REVISION_JSON" ;;
    *) exit 0 ;;
  esac ;;
container:inspect) test -e "$KIND_MUTATION_MARKER" ;;
run:--detach) : > "$KIND_MUTATION_REACHED"; : > "$KIND_MUTATION_MARKER"; exit 23 ;;
rm:-f) rm -f "$KIND_MUTATION_MARKER"; : > "$KIND_CLEANUP_MARKER"; exit 0 ;;
*) exit 0 ;;
esac
`)
			writeTool("git", `if [ "$#" -eq 4 ] && [ "$1" = -C ] && [ "$2" = "$KIND_REPO_ROOT" ] && [ "$3" = rev-parse ] && [ "$4" = HEAD ]; then
  : > "$KIND_GIT_HEAD_MARKER"
  if [ "$KIND_GIT_EXIT" != 0 ]; then
    exit "$KIND_GIT_EXIT"
  fi
  printf '%s\n' "$KIND_CHECKOUT_REVISION"
  exit 0
fi
: > "$KIND_GIT_PROXY_MARKER"
exec "$KIND_REAL_GIT" "$@"
`)
			writeTool("kind", `test "${1:-}" = version && printf '%s\n' 'kind v0.32.0 go1.24 linux/amd64'
`)
			writeTool("helm", `test "${1:-}" = version && printf '%s\n' 'v3.21.3+gfixture'
`)
			writeTool("oras", `test "${1:-}" = version && printf '%s\n' 'Version:        1.3.3'
`)
			for _, name := range []string{"kubectl", "go", "curl", "openssl"} {
				writeTool(name, "exit 0\n")
			}
			marker := filepath.Join(fixture, "partial-container")
			mutationReached := filepath.Join(fixture, "mutation-reached")
			cleanupMarker := filepath.Join(fixture, "partial-cleanup")
			headMarker := filepath.Join(fixture, "git-head")
			proxyMarker := filepath.Join(fixture, "git-proxy")
			command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "test-accelerator-desktop-provision-kind.sh"))
			command.Dir = repoRoot
			command.Env = append(os.Environ(),
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"TMPDIR="+fixture,
				"KIND_IMAGE_REVISION_JSON="+test.revisionJSON,
				"KIND_MUTATION_MARKER="+marker,
				"KIND_MUTATION_REACHED="+mutationReached,
				"KIND_CLEANUP_MARKER="+cleanupMarker,
				"KIND_REAL_GIT="+realGit,
				"KIND_REPO_ROOT="+repoRoot,
				"KIND_CHECKOUT_REVISION="+test.checkoutRevision,
				"KIND_GIT_EXIT="+test.gitExit,
				"KIND_GIT_HEAD_MARKER="+headMarker,
				"KIND_GIT_PROXY_MARKER="+proxyMarker,
			)
			output, runErr := command.CombinedOutput()
			if runErr == nil || !strings.Contains(string(output), "accelerator-desktop-provision-kind: "+test.wantFailure) {
				t.Fatalf("result err=%v output=%q", runErr, output)
			}
			_, markerErr := os.Stat(marker)
			_, reachedErr := os.Stat(mutationReached)
			_, cleanupErr := os.Stat(cleanupMarker)
			if test.accepted && (reachedErr != nil || !os.IsNotExist(markerErr) || cleanupErr != nil) {
				t.Fatalf("partial create cleanup reached=%v residue=%v cleanup=%v", reachedErr, markerErr, cleanupErr)
			}
			if !test.accepted && (!os.IsNotExist(markerErr) || !os.IsNotExist(reachedErr) || !os.IsNotExist(cleanupErr)) {
				t.Fatalf("rejected revision crossed the mutation fence: residue=%v reached=%v cleanup=%v", markerErr, reachedErr, cleanupErr)
			}
			_, headErr := os.Stat(headMarker)
			if test.wantFailure == "source-image-revision-invalid" && !os.IsNotExist(headErr) {
				t.Fatalf("invalid image revision reached checkout Git: %v", headErr)
			}
			if test.wantFailure != "source-image-revision-invalid" && headErr != nil {
				t.Fatalf("checkout revision invocation bypassed fake Git: %v", headErr)
			}
			if _, err := os.Stat(proxyMarker); err != nil {
				t.Fatalf("scope-check invocation did not proxy to real Git: %v", err)
			}
			for _, secret := range []string{canonical, different, malformedCheckout, unavailableCheckout, test.revisionJSON, strings.Trim(test.revisionJSON, `"`), test.checkoutRevision} {
				if secret != "" && secret != "null" && strings.Contains(string(output), secret) {
					t.Fatal("revision fixture escaped through harness output")
				}
			}
		})
	}
}

type hostileErrorReader struct {
	err   error
	reads atomic.Int32
}

func (r *hostileErrorReader) Read([]byte) (int, error) {
	r.reads.Add(1)
	return 0, r.err
}

func validRequest() Request {
	return Request{ContextName: "ctx", Resolution: acceleratorrelease.Resolution{
		Availability: acceleratorrelease.Available, Source: acceleratorrelease.SourceNetwork,
		Release: acceleratorrelease.VerifiedRelease{
			BuildVersion: "v1.2.3", SourceCommit: strings.Repeat("c", 40), DescriptorSHA256: strings.Repeat("d", 64),
			ImageReference: imageRepository + "@sha256:" + strings.Repeat("a", 64), ChartReference: chartRepository + "@sha256:" + strings.Repeat("b", 64),
		},
	}}
}

func vectorEntropy() []byte {
	data := make([]byte, 48)
	for i := range data {
		data[i] = byte(i)
	}
	return data
}
