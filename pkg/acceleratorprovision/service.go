package acceleratorprovision

import (
	"context"
	"crypto/rand"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

const (
	ProvisionTimeout    = 3 * time.Minute
	ContextPollInterval = 250 * time.Millisecond
	CleanupTimeout      = 45 * time.Second
)

var digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var versionRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var commitRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
var hex64RE = regexp.MustCompile(`^[0-9a-f]{64}$`)

const imageRepository = "ghcr.io/skrobylabs/kubikles-accelerator"
const chartRepository = "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator"

// ContextProvider is intentionally narrower than the desktop client.
type ContextProvider interface {
	SnapshotCurrentContext(string) (ContextSnapshot, error)
	CurrentContext() string
}

type ContextSnapshot interface {
	Identity() string
	Namespace() string
	RESTConfig() *rest.Config
	Clientset() kubernetes.Interface
}

type chartAttempt struct {
	ReleaseName     string
	Namespace       string
	Session         string
	BuildVersion    string
	ImageRepository string
	ImageDigest     string
	ChartReference  string
	ChartDigest     string
	Verifier        string
	JobName         string
	JobUID          string
}

type preparedChart struct {
	jobName        string
	implementation interface{}
}

type ownedRelease struct {
	implementation interface{}
	jobUID         string
}

type ChartInstaller interface {
	Prepare(context.Context, chartAttempt, func() UnavailableReason) (*preparedChart, UnavailableReason)
	Install(context.Context, ContextSnapshot, *preparedChart) (*ownedRelease, UnavailableReason, bool)
	Cleanup(context.Context, ContextSnapshot, *preparedChart, *ownedRelease) CleanupStatus
}

type WorkloadObserver interface {
	Observe(context.Context, chartAttempt, ContextSnapshot, func() UnavailableReason) (ObjectIdentity, ObjectIdentity, UnavailableReason)
}

type desktopContexts struct{ client *k8s.Client }

func (p desktopContexts) SnapshotCurrentContext(name string) (ContextSnapshot, error) {
	return p.client.SnapshotCurrentContext(name)
}
func (p desktopContexts) CurrentContext() string {
	if p.client == nil {
		return ""
	}
	return p.client.GetCurrentContext()
}

type Service struct {
	contexts ContextProvider
	charts   ChartInstaller
	observer WorkloadObserver
	entropy  io.Reader
	gates    gateSet
}

func New(contexts ContextProvider, charts ChartInstaller, observer WorkloadObserver) *Service {
	return &Service{contexts: contexts, charts: charts, observer: observer, entropy: rand.Reader}
}

// NewDesktopService composes the dormant production adapters. Construction is
// side-effect free; provisioning occurs only after an explicit Provision call.
func NewDesktopService(k8sClient *k8s.Client, helmClient *helm.Client) *Service {
	return New(desktopContexts{client: k8sClient}, &helmChartInstaller{client: helmClient, pollWait: realDisposalPollWait}, KubernetesObserver{})
}

func (s *Service) Provision(ctx context.Context, request Request) Result {
	if !validRelease(request.Resolution) {
		return unavailable(ArtifactUnavailable, CleanupNotNeeded)
	}
	if request.ContextName == "" || strings.TrimSpace(request.ContextName) != request.ContextName {
		return unavailable(ContextUnavailable, CleanupNotNeeded)
	}
	if request.NamespaceOverride != "" && len(validation.IsDNS1123Label(request.NamespaceOverride)) != 0 {
		return unavailable(ContextUnavailable, CleanupNotNeeded)
	}
	if ctx == nil || s == nil || s.contexts == nil || s.charts == nil || s.observer == nil || s.entropy == nil {
		return unavailable(ContextUnavailable, CleanupNotNeeded)
	}
	opCtx, cancel := context.WithTimeout(ctx, ProvisionTimeout)
	defer cancel()
	snapshot, err := s.contexts.SnapshotCurrentContext(request.ContextName)
	if err == nil && request.NamespaceOverride != "" {
		snapshot = namespaceOverrideSnapshot{ContextSnapshot: snapshot, namespace: request.NamespaceOverride}
	}
	initialGateKey := releaseMutationGateKey(snapshot)
	if err != nil || initialGateKey == "" {
		return unavailable(ContextUnavailable, CleanupNotNeeded)
	}
	releaseGate := s.gates.acquire(opCtx, initialGateKey)
	if releaseGate == nil {
		return unavailable(contextReason(opCtx), CleanupNotNeeded)
	}
	defer releaseGate()

	// Refresh after gate entry so a waiter never reuses the preceding attempt's
	// snapshot or current-context decision.
	snapshot, err = s.contexts.SnapshotCurrentContext(request.ContextName)
	if err == nil && request.NamespaceOverride != "" {
		snapshot = namespaceOverrideSnapshot{ContextSnapshot: snapshot, namespace: request.NamespaceOverride}
	}
	if err != nil || releaseMutationGateKey(snapshot) != initialGateKey {
		return unavailable(ContextChanged, CleanupNotNeeded)
	}
	boundary := func() UnavailableReason { return checkBoundary(opCtx, s.contexts, request.ContextName) }
	if reason := boundary(); reason != "" {
		return unavailable(reason, CleanupNotNeeded)
	}
	credential, err := generateCreatorCredential(s.entropy)
	if err != nil {
		return unavailable(EntropyUnavailable, CleanupNotNeeded)
	}
	release := request.Resolution.Release
	attempt := chartAttempt{
		ReleaseName: credential.releaseName(), Namespace: snapshot.Namespace(), Session: credential.session,
		BuildVersion: release.BuildVersion, ImageRepository: imageRepository,
		ImageDigest:    strings.TrimPrefix(release.ImageReference, imageRepository+"@"),
		ChartReference: release.ChartReference,
		ChartDigest:    strings.TrimPrefix(release.ChartReference, chartRepository+"@"), Verifier: credential.verifier,
	}
	prepared, reason := s.charts.Prepare(opCtx, attempt, boundary)
	// A phase result is not publishable until its cancellation/current-context
	// boundary has been settled.  In particular, a concurrent pull/render
	// failure must not hide a cancellation or context switch.
	if boundaryReason := boundary(); boundaryReason != "" {
		return unavailable(boundaryReason, CleanupNotNeeded)
	}
	if reason != "" {
		return unavailable(reason, CleanupNotNeeded)
	}
	if prepared == nil || prepared.jobName == "" {
		return unavailable(RenderFailed, CleanupNotNeeded)
	}
	attempt.JobName = prepared.jobName
	if reason = boundary(); reason != "" {
		return unavailable(reason, CleanupNotNeeded)
	}
	owned, reason, ownershipUnproven := s.charts.Install(opCtx, snapshot, prepared)
	// Preserve a receipt while giving the boundary precedence over a concurrent
	// install error; the receipt is what makes the resulting rollback exact.
	if boundaryReason := boundary(); boundaryReason != "" {
		if owned != nil {
			return s.rollback(ctx, boundaryReason, snapshot, prepared, owned)
		}
		if ownershipUnproven {
			return unavailable(boundaryReason, CleanupOwnershipUnproven)
		}
		return unavailable(boundaryReason, CleanupNotNeeded)
	}
	if reason != "" {
		if owned != nil {
			return s.rollback(ctx, reason, snapshot, prepared, owned)
		}
		if ownershipUnproven {
			return unavailable(reason, CleanupOwnershipUnproven)
		}
		return unavailable(reason, CleanupNotNeeded)
	}
	if owned == nil {
		return unavailable(InstallFailed, CleanupOwnershipUnproven)
	}
	if owned.jobUID == "" {
		return s.rollback(ctx, InstallFailed, snapshot, prepared, owned)
	}
	attempt.JobUID = owned.jobUID
	if reason = boundary(); reason != "" {
		return s.rollback(ctx, reason, snapshot, prepared, owned)
	}
	job, pod, reason := s.observer.Observe(opCtx, attempt, snapshot, boundary)
	if boundaryReason := boundary(); boundaryReason != "" {
		reason = boundaryReason
	}
	if reason != "" {
		return s.rollback(ctx, reason, snapshot, prepared, owned)
	}
	workload := &ProvisionedWorkload{
		ContextName: request.ContextName, ReleaseNamespace: attempt.Namespace, ReleaseName: attempt.ReleaseName,
		WorkloadSessionID: attempt.Session, Job: job, Pod: pod, BuildVersion: attempt.BuildVersion,
		ImageDigest: attempt.ImageDigest, ChartDigest: attempt.ChartDigest, credential: credential, snapshot: snapshot,
		connectorState: &workloadConnectorState{},
	}
	workload.connectorState.receipt = &workloadReceipt{contextName: workload.ContextName, releaseNamespace: workload.ReleaseNamespace, releaseName: workload.ReleaseName, workloadSessionID: workload.WorkloadSessionID, job: workload.Job, pod: workload.Pod, buildVersion: workload.BuildVersion, imageDigest: workload.ImageDigest, chartDigest: workload.ChartDigest, snapshot: snapshot, prepared: prepared, owned: owned, owner: workload}
	// This is the publication boundary. No cleanup path exists after this return.
	if reason = boundary(); reason != "" {
		return s.rollback(ctx, reason, snapshot, prepared, owned)
	}
	credential = nil
	return available(workload)
}

// namespaceOverrideSnapshot changes only the release target. Credentials and
// connection identity remain those of the immutable current-context snapshot.
type namespaceOverrideSnapshot struct {
	ContextSnapshot
	namespace string
}

func (s namespaceOverrideSnapshot) Namespace() string { return s.namespace }

func releaseMutationGateKey(snapshot ContextSnapshot) string {
	if snapshot == nil || snapshot.Identity() == "" || snapshot.Namespace() == "" {
		return ""
	}
	return snapshot.Identity() + "\x00" + snapshot.Namespace()
}

func (s *Service) rollback(parent context.Context, reason UnavailableReason, snapshot ContextSnapshot, prepared *preparedChart, owned *ownedRelease) Result {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), CleanupTimeout)
	defer cancel()
	cleanup := s.charts.Cleanup(cleanupCtx, snapshot, prepared, owned)
	if cleanup != CleanupSucceeded && cleanup != CleanupOwnershipUnproven {
		cleanup = CleanupFailed
	}
	return unavailable(reason, cleanup)
}

func contextReason(ctx context.Context) UnavailableReason {
	if ctx != nil && ctx.Err() == context.DeadlineExceeded {
		return TimedOut
	}
	return Cancelled
}

func checkBoundary(ctx context.Context, contexts ContextProvider, name string) UnavailableReason {
	if ctx == nil || ctx.Err() != nil {
		return contextReason(ctx)
	}
	if contexts == nil || contexts.CurrentContext() != name {
		return ContextChanged
	}
	return ""
}

func validRelease(resolution acceleratorrelease.Resolution) bool {
	release := resolution.Release
	if resolution.Availability != acceleratorrelease.Available || (resolution.Source != acceleratorrelease.SourceNetwork && resolution.Source != acceleratorrelease.SourceCache) {
		return false
	}
	if !versionRE.MatchString(release.BuildVersion) || !commitRE.MatchString(release.SourceCommit) || !hex64RE.MatchString(release.DescriptorSHA256) {
		return false
	}
	imageDigest, imageOK := strings.CutPrefix(release.ImageReference, imageRepository+"@")
	chartDigest, chartOK := strings.CutPrefix(release.ChartReference, chartRepository+"@")
	return imageOK && chartOK && digestRE.MatchString(imageDigest) && digestRE.MatchString(chartDigest)
}

type gateEntry struct {
	token chan struct{}
	refs  int
}

type gateSet struct {
	mu sync.Mutex
	m  map[string]*gateEntry
}

func (g *gateSet) acquire(ctx context.Context, key string) func() {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*gateEntry)
	}
	entry := g.m[key]
	if entry == nil {
		entry = &gateEntry{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		g.m[key] = entry
	}
	entry.refs++
	g.mu.Unlock()
	select {
	case <-ctx.Done():
		g.dropReference(key, entry)
		return nil
	case <-entry.token:
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			entry.token <- struct{}{}
			g.dropReference(key, entry)
		})
	}
}

func (g *gateSet) dropReference(key string, entry *gateEntry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && g.m[key] == entry {
		delete(g.m, key)
	}
}
