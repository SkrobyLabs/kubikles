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
	mu       sync.Mutex
	current  string
	snapshot ContextSnapshot
	err      error
	calls    int
}

func (p *fakeContexts) SnapshotCurrentContext(name string) (ContextSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
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
		`grep -Fq -- "$sensitive" "$tmp/go-test"`,
		`fail "go-service-output-sensitive"`,
		"[REDACTED_TEST_CORPUS]",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("Kind output gate missing required static proof %q", required)
		}
	}
	testCapture := strings.Index(source, `>"$tmp/go-test" 2>&1`)
	outputScan := strings.Index(source, `grep -Fq -- "$sensitive" "$tmp/go-test"`)
	passed := strings.Index(source, `echo "accelerator-desktop-provision-kind: passed"`)
	if testCapture < 0 || outputScan <= testCapture || passed <= outputScan {
		t.Fatal("Kind output scan does not gate success after captured go test output")
	}
	if strings.Contains(source, `echo "$sensitive"`) || strings.Contains(source, `fail "$sensitive"`) {
		t.Fatal("Kind output gate would disclose the matched sensitive value")
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
