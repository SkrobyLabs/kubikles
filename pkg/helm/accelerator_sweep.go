//go:build helm

package helm

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const AcceleratorSweepReleaseProbeLimit = 101

var acceleratorSweepNameRE = regexp.MustCompile(`^kubikles-accelerator-([0-9a-f]{32})$`)

func (c *Client) ListAcceleratorSweepReleaseNames(ctx context.Context, config *rest.Config, namespace string) ([]string, bool) {
	actionConfig, err := acceleratorActionConfig(ctx, config, namespace)
	if err != nil {
		return nil, false
	}
	list := configureAcceleratorSweepList(actionConfig)
	if list == nil {
		return nil, false
	}
	releases, err := list.Run()
	if err != nil {
		return nil, false
	}
	names := make([]string, 0, len(releases))
	for _, stored := range releases {
		if stored != nil {
			names = append(names, stored.Name)
		}
	}
	sort.Strings(names)
	return names, true
}

func configureAcceleratorSweepList(actionConfig *action.Configuration) *action.List {
	if actionConfig == nil {
		return nil
	}
	list := action.NewList(actionConfig)
	list.All = false
	list.AllNamespaces = false
	list.StateMask = action.ListAll | action.ListUnknown
	list.Limit = AcceleratorSweepReleaseProbeLimit
	list.Offset = 0
	list.SortReverse = false
	list.ByDate = false
	return list
}

func (c *Client) InspectAcceleratorSweepCandidate(ctx context.Context, config *rest.Config, namespace, name string) (*AcceleratorSweepCandidate, AcceleratorSweepProofStatus) {
	actionConfig, err := acceleratorActionConfig(ctx, config, namespace)
	if err != nil {
		return nil, AcceleratorSweepUnsupportedMalformed
	}
	clientset, err := acceleratorClientset(ctx, config)
	if err != nil {
		return nil, AcceleratorSweepUnsupportedMalformed
	}
	return inspectAcceleratorSweepCandidateWith(ctx, actionConfig, clientset, namespace, name)
}

func inspectAcceleratorSweepCandidateWith(ctx context.Context, actionConfig *action.Configuration, client kubernetes.Interface, namespace, name string) (*AcceleratorSweepCandidate, AcceleratorSweepProofStatus) {
	if actionConfig == nil || actionConfig.Releases == nil || client == nil || namespace == "" || !acceleratorSweepNameRE.MatchString(name) {
		return nil, AcceleratorSweepUnsupportedMalformed
	}
	history, err := actionConfig.Releases.History(name)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, AcceleratorSweepAlreadyGone
	}
	if err != nil || len(history) != 1 {
		return nil, AcceleratorSweepUnsupportedMalformed
	}
	stored := history[0]
	closed, stage := proveClosedAcceleratorRelease(stored, namespace, name, nil)
	if stage != "" {
		return nil, AcceleratorSweepUnsupportedMalformed
	}
	request, resources, jobName, hash := closed.request, closed.resources, closed.jobName, closed.renderHash
	storageName := acceleratorStorageName(name)
	storage, err := client.CoreV1().Secrets(namespace).Get(ctx, storageName, metav1.GetOptions{})
	if err != nil || storage.UID == "" {
		if apierrors.IsNotFound(err) {
			return nil, AcceleratorSweepAlreadyGone
		}
		return nil, AcceleratorSweepUnsupportedMalformed
	}
	candidate := &AcceleratorSweepCandidate{name: name, namespace: namespace, session: request.WorkloadSession, renderHash: hash, storage: AcceleratorStorageIdentity{Namespace: namespace, Name: storageName, UID: storage.UID}, authority: trustedAcceleratorSweepAuthority}
	var job *batchv1.Job
	names := make(map[string]string, len(resources))
	for _, identity := range resources {
		names[identity.Kind] = identity.Name
	}
	for _, identity := range resources {
		object, getErr := getAcceleratorLiveObject(ctx, client, identity)
		if apierrors.IsNotFound(getErr) && identity.Kind == "Job" {
			continue
		}
		if getErr != nil || object.GetUID() == "" || !acceleratorClosedLiveObjectContract(object, closed, names) {
			return nil, AcceleratorSweepActiveOrAmbiguous
		}
		candidate.resources = append(candidate.resources, AcceleratorDeletionIdentity{Resource: identity, UID: object.GetUID()})
		if identity.Kind == "Job" {
			job, _ = object.(*batchv1.Job)
		}
	}
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "kubikles.io/workload-session-id=" + request.WorkloadSession})
	if err != nil {
		return nil, AcceleratorSweepActiveOrAmbiguous
	}
	jobTerminal := job == nil
	if job != nil {
		jobTerminal = acceleratorSweepJobTerminal(job)
		if !jobTerminal || job.Name != jobName || job.Labels["kubikles.io/workload-session-id"] != request.WorkloadSession {
			return nil, AcceleratorSweepActiveOrAmbiguous
		}
	}
	var correlatedJobUID types.UID
	if job != nil {
		correlatedJobUID = job.UID
	}
	for index := range pods.Items {
		pod := &pods.Items[index]
		if !acceleratorClosedPodContract(pod, closed, jobName, correlatedJobUID) {
			return nil, AcceleratorSweepActiveOrAmbiguous
		}
		if correlatedJobUID == "" {
			correlatedJobUID = pod.OwnerReferences[0].UID
		}
		if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			return nil, AcceleratorSweepActiveOrAmbiguous
		}
		candidate.pods = append(candidate.pods, AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "v1", Kind: "Pod", Namespace: namespace, Name: pod.Name}, UID: pod.UID})
	}
	if !jobTerminal {
		return nil, AcceleratorSweepActiveOrAmbiguous
	}
	return candidate, AcceleratorSweepEligible
}

func acceleratorClosedLiveObjectContract(object metav1.Object, proof acceleratorClosedReleaseProof, names map[string]string) bool {
	request := proof.request
	if !acceleratorHelmMetadata(object, request) {
		return false
	}
	switch typed := object.(type) {
	case *batchv1.Job:
		return acceleratorClosedJobContract(typed, proof, names)
	case *corev1.Secret:
		if typed.Type != corev1.SecretTypeOpaque || typed.Immutable == nil || !*typed.Immutable || len(typed.Data) != 1 {
			return false
		}
		return sha256.Sum256(typed.Data["creatorVerifier"]) == proof.verifierHash
	case *corev1.ServiceAccount:
		return typed.AutomountServiceAccountToken != nil && !*typed.AutomountServiceAccountToken && len(typed.Secrets) == 0 && len(typed.ImagePullSecrets) == 0
	case *rbacv1.ClusterRole:
		return typed.AggregationRule == nil && len(typed.Rules) == 1 && len(typed.Rules[0].APIGroups) == 1 && typed.Rules[0].APIGroups[0] == "" && len(typed.Rules[0].Resources) == 1 && typed.Rules[0].Resources[0] == "secrets" && sameStrings(typed.Rules[0].Verbs, []string{"get", "list", "watch"}) && len(typed.Rules[0].ResourceNames) == 0 && len(typed.Rules[0].NonResourceURLs) == 0
	case *rbacv1.ClusterRoleBinding:
		return typed.RoleRef.APIGroup == rbacv1.GroupName && typed.RoleRef.Kind == "ClusterRole" && typed.RoleRef.Name == names["ClusterRole"] && len(typed.Subjects) == 1 && typed.Subjects[0].APIGroup == "" && typed.Subjects[0].Kind == "ServiceAccount" && typed.Subjects[0].Name == names["ServiceAccount"] && typed.Subjects[0].Namespace == request.ReleaseNamespace
	default:
		return false
	}
}

func acceleratorClosedJobContract(job *batchv1.Job, proof acceleratorClosedReleaseProof, names map[string]string) bool {
	if job == nil || job.Name != proof.jobName || !acceleratorTemplateMetadataContract(job.Spec.Template.ObjectMeta, proof.request, job.Name, job.UID) {
		return false
	}
	spec := job.Spec.DeepCopy()
	if spec.Selector != nil {
		if len(spec.Selector.MatchExpressions) != 0 || len(spec.Selector.MatchLabels) == 0 {
			return false
		}
		for key, value := range spec.Selector.MatchLabels {
			if (key != "batch.kubernetes.io/controller-uid" && key != "controller-uid") || value != string(job.UID) {
				return false
			}
		}
		spec.Selector = nil
	}
	if spec.ManualSelector != nil && !*spec.ManualSelector {
		spec.ManualSelector = nil
	}
	if spec.CompletionMode != nil && *spec.CompletionMode == batchv1.NonIndexedCompletion {
		spec.CompletionMode = nil
	}
	if spec.Suspend != nil && !*spec.Suspend {
		spec.Suspend = nil
	}
	if spec.PodReplacementPolicy != nil && *spec.PodReplacementPolicy == batchv1.TerminatingOrFailed {
		spec.PodReplacementPolicy = nil
	}
	spec.Template.ObjectMeta = metav1.ObjectMeta{}
	normalizeAcceleratorPodDefaults(&spec.Template.Spec)
	one, zero, ttl := int32(1), int32(0), int32(3600)
	want := batchv1.JobSpec{
		Completions: &one, Parallelism: &one, BackoffLimit: &zero, TTLSecondsAfterFinished: &ttl,
		Template: corev1.PodTemplateSpec{Spec: acceleratorExpectedPodSpec(proof.request, names)},
	}
	return apiequality.Semantic.DeepEqual(*spec, want)
}

type acceleratorClosedReleaseProof struct {
	request      AcceleratorReleaseRequest
	resources    []AcceleratorResourceIdentity
	jobName      string
	renderHash   string
	verifierHash [sha256.Size]byte
}

func proveStoredSweepRelease(stored *release.Release, namespace, name string) (AcceleratorReleaseRequest, []AcceleratorResourceIdentity, string, string, bool) {
	proof, stage := proveClosedAcceleratorRelease(stored, namespace, name, nil)
	return proof.request, proof.resources, proof.jobName, proof.renderHash, stage == ""
}

func proveStoredSweepReleaseStage(stored *release.Release, namespace, name string) (AcceleratorReleaseRequest, []AcceleratorResourceIdentity, string, string, string) {
	proof, stage := proveClosedAcceleratorRelease(stored, namespace, name, nil)
	return proof.request, proof.resources, proof.jobName, proof.renderHash, stage
}

// proveClosedAcceleratorRelease is the single stored-release contract used by
// both receipt-owned cleanup and orphan sweeping. The verifier is reduced to a
// structural digest before returning and is never exposed or used for auth.
func proveClosedAcceleratorRelease(stored *release.Release, namespace, name string, expected *AcceleratorPreparedRelease) (acceleratorClosedReleaseProof, string) {
	var zero acceleratorClosedReleaseProof
	if stored == nil || stored.Name != name || stored.Namespace != namespace {
		return zero, "identity"
	}
	if stored.Version != 1 {
		return zero, "revision"
	}
	if stored.Info == nil {
		return zero, "status_missing"
	}
	if stored.Info.Status != release.StatusPendingInstall {
		switch stored.Info.Status {
		case release.StatusDeployed:
			return zero, "status_deployed"
		case release.StatusFailed:
			return zero, "status_failed"
		case release.StatusUninstalling:
			return zero, "status_uninstalling"
		default:
			return zero, "status_other"
		}
	}
	if stored.Chart == nil || stored.Chart.Metadata == nil || stored.Chart.Metadata.APIVersion != "v2" || stored.Chart.Metadata.Name != "kubikles-accelerator" || stored.Chart.Metadata.Type != "application" {
		return zero, "chart"
	}
	sessionMatch := acceleratorSweepNameRE.FindStringSubmatch(name)
	if len(sessionMatch) != 2 {
		return zero, "identity"
	}
	request, verifierScratch, ok := decodeSweepValues(stored.Config, namespace, name)
	if !ok {
		return zero, "config"
	}
	defer clear(verifierScratch)
	if sessionMatch[1] != request.WorkloadSession {
		return zero, "session"
	}
	if !acceleratorVersion.MatchString(request.BuildVersion) || stored.Chart.Metadata.Version != strings.TrimPrefix(request.BuildVersion, "v") || stored.Chart.Metadata.AppVersion != request.BuildVersion {
		return zero, "version"
	}
	if len(stored.Hooks) != 0 {
		return zero, "hooks"
	}
	request.CreatorVerifier = string(verifierScratch)
	resources, jobName, _, ok := validateAcceleratorManifest(stored.Manifest, request)
	request.CreatorVerifier = ""
	if !ok || len(resources) != 5 {
		return zero, "manifest"
	}
	hash, ok := acceleratorReleaseHash(stored.Manifest, stored.Config)
	if !ok {
		return zero, "hash"
	}
	if expected != nil {
		want := expected.request
		if request.BuildVersion != want.BuildVersion || request.ImageRepository != want.ImageRepository || request.ImageDigest != want.ImageDigest || request.WorkloadSession != want.WorkloadSession || request.ReleaseName != want.ReleaseName || request.ReleaseNamespace != want.ReleaseNamespace || stored.Manifest != expected.manifest || !reflect.DeepEqual(stored.Config, expected.values) || hash != expected.renderHash {
			return zero, "receipt"
		}
	}
	return acceleratorClosedReleaseProof{request: request, resources: resources, jobName: jobName, renderHash: hash, verifierHash: sha256.Sum256(verifierScratch)}, ""
}

func decodeSweepValues(values map[string]interface{}, namespace, name string) (AcceleratorReleaseRequest, []byte, bool) {
	if len(values) != 3 {
		return AcceleratorReleaseRequest{}, nil, false
	}
	accelerator, aok := values["accelerator"].(map[string]interface{})
	auth, authOK := values["auth"].(map[string]interface{})
	image, imageOK := values["image"].(map[string]interface{})
	if !aok || !authOK || !imageOK || len(accelerator) != 1 || len(auth) != 1 || len(image) != 4 {
		return AcceleratorReleaseRequest{}, nil, false
	}
	session, sok := accelerator["workloadSessionId"].(string)
	verifier, vok := auth["creatorVerifier"].(string)
	repository, rok := image["repository"].(string)
	digest, dok := image["digest"].(string)
	version, versionOK := image["version"].(string)
	architecture, architectureOK := image["architecture"].(string)
	if !sok || !vok || !rok || !dok || !versionOK || !architectureOK || architecture != acceleratorRuntimeArchitecture() || !acceleratorSweepNameRE.MatchString("kubikles-accelerator-"+session) || repository != acceleratorImageRepository || !acceleratorDigest.MatchString(digest) || !acceleratorVersion.MatchString(version) || !acceleratorVerifier.MatchString(verifier) {
		return AcceleratorReleaseRequest{}, nil, false
	}
	return AcceleratorReleaseRequest{BuildVersion: version, ImageRepository: repository, ImageDigest: digest, WorkloadSession: session, ReleaseName: name, ReleaseNamespace: namespace}, []byte(verifier), true
}

func acceleratorSweepJobTerminal(job *batchv1.Job) bool {
	for _, condition := range job.Status.Conditions {
		if condition.Status == corev1.ConditionTrue && (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) {
			return true
		}
	}
	return false
}

func acceleratorClosedPodContract(pod *corev1.Pod, proof acceleratorClosedReleaseProof, jobName string, jobUID types.UID) bool {
	request := proof.request
	names := map[string]string{"ServiceAccount": acceleratorFullname(request.ReleaseName), "Secret": acceleratorFullname(request.ReleaseName) + "-verifier"}
	if pod == nil || pod.UID == "" || pod.DeletionTimestamp != nil || pod.Namespace != request.ReleaseNamespace || !acceleratorClosedPodSpecContract(pod.Spec, request, names) || len(pod.OwnerReferences) != 1 {
		return false
	}
	owner := pod.OwnerReferences[0]
	if owner.APIVersion != "batch/v1" || owner.Kind != "Job" || owner.Name != jobName || owner.UID == "" || owner.Controller == nil || !*owner.Controller || owner.BlockOwnerDeletion == nil || !*owner.BlockOwnerDeletion {
		return false
	}
	if jobUID != "" && owner.UID != jobUID {
		return false
	}
	return acceleratorTemplateMetadataContract(pod.ObjectMeta, request, jobName, owner.UID)
}

func acceleratorTemplateMetadataContract(metadata metav1.ObjectMeta, request AcceleratorReleaseRequest, jobName string, jobUID types.UID) bool {
	wantLabels := acceleratorLabels(request.ReleaseName)
	wantLabels["kubikles.io/workload-session-id"] = request.WorkloadSession
	labels := make(map[string]string, len(metadata.Labels))
	for key, value := range metadata.Labels {
		labels[key] = value
	}
	generated := map[string]string{
		"batch.kubernetes.io/controller-uid": string(jobUID),
		"batch.kubernetes.io/job-name":       jobName,
		"controller-uid":                     string(jobUID),
		"job-name":                           jobName,
	}
	for key, want := range generated {
		if got, ok := labels[key]; ok {
			if want == "" || got != want {
				return false
			}
			delete(labels, key)
		}
	}
	return equalStringMap(labels, wantLabels) && equalStringMap(metadata.Annotations, map[string]string{"kubikles.io/build-version": request.BuildVersion})
}

func acceleratorClosedPodSpecContract(spec corev1.PodSpec, request AcceleratorReleaseRequest, names map[string]string) bool {
	actual := spec.DeepCopy()
	if !normalizeAcceleratorLivePodMutations(actual) {
		return false
	}
	normalizeAcceleratorPodDefaults(actual)
	return apiequality.Semantic.DeepEqual(*actual, acceleratorExpectedPodSpec(request, names))
}

// normalizeAcceleratorLivePodMutations requires the complete Pod-only tuple
// before removing it. Priority, preemption, and the two NoExecute tolerations
// are admission results. A nonempty NodeName is the scheduler binding result;
// the strict Job-template proof requires every one of these fields unset.
func normalizeAcceleratorLivePodMutations(spec *corev1.PodSpec) bool {
	if !acceleratorExactLivePodMutationTuple(spec) {
		return false
	}
	spec.Priority = nil
	spec.PreemptionPolicy = nil
	spec.Tolerations = nil
	spec.NodeName = ""
	return true
}

func acceleratorExactLivePodMutationTuple(spec *corev1.PodSpec) bool {
	return spec != nil &&
		spec.Priority != nil && *spec.Priority == 0 &&
		spec.PreemptionPolicy != nil && *spec.PreemptionPolicy == corev1.PreemptLowerPriority &&
		spec.PriorityClassName == "" &&
		acceleratorDefaultNoExecuteTolerations(spec.Tolerations) &&
		spec.NodeName != ""
}

func acceleratorDefaultNoExecuteTolerations(tolerations []corev1.Toleration) bool {
	if len(tolerations) != 2 {
		return false
	}
	want := map[string]bool{corev1.TaintNodeNotReady: false, corev1.TaintNodeUnreachable: false}
	for _, toleration := range tolerations {
		seen, known := want[toleration.Key]
		if !known || seen || toleration.Operator != corev1.TolerationOpExists || toleration.Value != "" || toleration.Effect != corev1.TaintEffectNoExecute || toleration.TolerationSeconds == nil || *toleration.TolerationSeconds != 300 {
			return false
		}
		want[toleration.Key] = true
	}
	return want[corev1.TaintNodeNotReady] && want[corev1.TaintNodeUnreachable]
}

func acceleratorExpectedPodSpec(request AcceleratorReleaseRequest, names map[string]string) corev1.PodSpec {
	automount, grace, user, mode, expiration := false, int64(30), int64(65532), int32(292), int64(3600)
	runNonRoot, allowEscalation, readOnly := true, false, true
	return corev1.PodSpec{
		ServiceAccountName: names["ServiceAccount"], AutomountServiceAccountToken: &automount, RestartPolicy: corev1.RestartPolicyNever, TerminationGracePeriodSeconds: &grace,
		NodeSelector:    map[string]string{"kubernetes.io/arch": acceleratorRuntimeArchitecture()},
		SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: &runNonRoot, RunAsUser: &user, RunAsGroup: &user, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
		Containers: []corev1.Container{{Name: "accelerator", Image: request.ImageRepository + "@" + request.ImageDigest, ImagePullPolicy: corev1.PullIfNotPresent,
			Env:             []corev1.EnvVar{{Name: "KUBIKLES_ACCELERATOR_CREATOR_VERIFIER", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: names["Secret"]}, Key: "creatorVerifier"}}}},
			SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &allowEscalation, ReadOnlyRootFilesystem: &readOnly, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
			Resources:       corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("512Mi")}},
			VolumeMounts:    []corev1.VolumeMount{{Name: "serviceaccount", MountPath: "/var/run/secrets/kubernetes.io/serviceaccount", ReadOnly: true}}}},
		Volumes: []corev1.Volume{{Name: "serviceaccount", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{DefaultMode: &mode, Sources: []corev1.VolumeProjection{
			{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{Path: "token", ExpirationSeconds: &expiration}},
			{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "kube-root-ca.crt"}, Items: []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}}},
			{DownwardAPI: &corev1.DownwardAPIProjection{Items: []corev1.DownwardAPIVolumeFile{{Path: "namespace", FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}}}},
		}}}}},
	}
}

func normalizeAcceleratorPodDefaults(spec *corev1.PodSpec) {
	if spec == nil {
		return
	}
	if spec.DeprecatedServiceAccount == spec.ServiceAccountName {
		spec.DeprecatedServiceAccount = ""
	}
	if spec.DNSPolicy == corev1.DNSClusterFirst {
		spec.DNSPolicy = ""
	}
	if spec.SchedulerName == corev1.DefaultSchedulerName {
		spec.SchedulerName = ""
	}
	if spec.EnableServiceLinks != nil && *spec.EnableServiceLinks == corev1.DefaultEnableServiceLinks {
		spec.EnableServiceLinks = nil
	}
	if len(spec.Containers) == 1 {
		container := &spec.Containers[0]
		if container.TerminationMessagePath == corev1.TerminationMessagePathDefault {
			container.TerminationMessagePath = ""
		}
		if container.TerminationMessagePolicy == corev1.TerminationMessageReadFile {
			container.TerminationMessagePolicy = ""
		}
		if len(container.Env) == 1 && container.Env[0].ValueFrom != nil && container.Env[0].ValueFrom.SecretKeyRef != nil && container.Env[0].ValueFrom.SecretKeyRef.Optional != nil && !*container.Env[0].ValueFrom.SecretKeyRef.Optional {
			container.Env[0].ValueFrom.SecretKeyRef.Optional = nil
		}
	}
	if len(spec.Volumes) == 1 && spec.Volumes[0].Projected != nil && len(spec.Volumes[0].Projected.Sources) == 3 {
		items := spec.Volumes[0].Projected.Sources[2].DownwardAPI
		if items != nil && len(items.Items) == 1 && items.Items[0].FieldRef != nil && items.Items[0].FieldRef.APIVersion == "v1" {
			items.Items[0].FieldRef.APIVersion = ""
		}
	}
}

func acceleratorPodSecurityContract(security *corev1.PodSecurityContext) bool {
	return security != nil && security.RunAsNonRoot != nil && *security.RunAsNonRoot && security.RunAsUser != nil && *security.RunAsUser == 65532 && security.RunAsGroup != nil && *security.RunAsGroup == 65532 && security.SeccompProfile != nil && security.SeccompProfile.Type == corev1.SeccompProfileTypeRuntimeDefault && security.SeccompProfile.LocalhostProfile == nil && security.FSGroup == nil && len(security.SupplementalGroups) == 0 && len(security.Sysctls) == 0
}

func acceleratorContainerSecurityContract(security *corev1.SecurityContext) bool {
	return security != nil && security.AllowPrivilegeEscalation != nil && !*security.AllowPrivilegeEscalation && security.ReadOnlyRootFilesystem != nil && *security.ReadOnlyRootFilesystem && security.Capabilities != nil && len(security.Capabilities.Add) == 0 && sameCapabilities(security.Capabilities.Drop, []corev1.Capability{"ALL"}) && security.Privileged == nil && security.RunAsUser == nil && security.RunAsGroup == nil && security.RunAsNonRoot == nil && security.SeccompProfile == nil
}

func sameCapabilities(actual, expected []corev1.Capability) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range expected {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}

func acceleratorResourceContract(resources corev1.ResourceRequirements) bool {
	wantRequests := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")}
	wantLimits := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("512Mi")}
	return len(resources.Claims) == 0 && reflect.DeepEqual(resources.Requests, wantRequests) && reflect.DeepEqual(resources.Limits, wantLimits)
}

func acceleratorProjectedVolumeContract(volume corev1.Volume) bool {
	if volume.Name != "serviceaccount" || volume.Projected == nil || volume.Secret != nil || volume.ConfigMap != nil || volume.DownwardAPI != nil || volume.EmptyDir != nil || volume.Projected.DefaultMode == nil || *volume.Projected.DefaultMode != 292 || len(volume.Projected.Sources) != 3 {
		return false
	}
	token, ca, namespace := volume.Projected.Sources[0], volume.Projected.Sources[1], volume.Projected.Sources[2]
	if token.ServiceAccountToken == nil || token.ServiceAccountToken.Path != "token" || token.ServiceAccountToken.ExpirationSeconds == nil || *token.ServiceAccountToken.ExpirationSeconds != 3600 || token.ServiceAccountToken.Audience != "" || token.Secret != nil || token.ConfigMap != nil || token.DownwardAPI != nil {
		return false
	}
	if ca.ConfigMap == nil || ca.ConfigMap.Name != "kube-root-ca.crt" || ca.ConfigMap.Optional != nil || len(ca.ConfigMap.Items) != 1 || ca.ConfigMap.Items[0].Key != "ca.crt" || ca.ConfigMap.Items[0].Path != "ca.crt" || ca.ConfigMap.Items[0].Mode != nil || ca.Secret != nil || ca.DownwardAPI != nil || ca.ServiceAccountToken != nil {
		return false
	}
	if namespace.DownwardAPI == nil || len(namespace.DownwardAPI.Items) != 1 || namespace.Secret != nil || namespace.ConfigMap != nil || namespace.ServiceAccountToken != nil {
		return false
	}
	item := namespace.DownwardAPI.Items[0]
	return item.Path == "namespace" && item.Mode == nil && item.FieldRef != nil && (item.FieldRef.APIVersion == "" || item.FieldRef.APIVersion == "v1") && item.FieldRef.FieldPath == "metadata.namespace" && item.ResourceFieldRef == nil
}

func (c *Client) UninstallAcceleratorSweepCandidate(ctx context.Context, config *rest.Config, candidate *AcceleratorSweepCandidate) AcceleratorSweepProofStatus {
	if candidate == nil || candidate.authority != trustedAcceleratorSweepAuthority {
		return AcceleratorSweepOwnershipChanged
	}
	actionConfig, err := acceleratorActionConfig(ctx, config, candidate.namespace)
	if err != nil {
		return AcceleratorSweepCleanupFailed
	}
	clientset, err := acceleratorClientset(ctx, config)
	if err != nil {
		return AcceleratorSweepCleanupFailed
	}
	return uninstallAcceleratorSweepCandidateWith(ctx, actionConfig, clientset, candidate)
}

// CleanupAcceleratorSweepCandidate keeps the caller-bound outer context for
// the explicit second proof. Only a successful, uncancelled proof crosses into
// the detached phase used for the single Helm Run and exact-UID absence wait.
func (c *Client) CleanupAcceleratorSweepCandidate(outer context.Context, config *rest.Config, candidate *AcceleratorSweepCandidate) AcceleratorSweepProofStatus {
	if outer == nil || candidate == nil || candidate.authority != trustedAcceleratorSweepAuthority {
		return AcceleratorSweepOwnershipChanged
	}
	proofConfig, err := acceleratorActionConfig(outer, config, candidate.namespace)
	if err != nil {
		return AcceleratorSweepCleanupFailed
	}
	proofClient, err := acceleratorClientset(outer, config)
	if err != nil {
		return AcceleratorSweepCleanupFailed
	}
	second, status := inspectAcceleratorSweepCandidateWith(outer, proofConfig, proofClient, candidate.namespace, candidate.name)
	if status == AcceleratorSweepAlreadyGone {
		return status
	}
	if status != AcceleratorSweepEligible || !sameSweepCandidate(candidate, second) {
		return AcceleratorSweepOwnershipChanged
	}
	if outer.Err() != nil {
		return AcceleratorSweepCleanupFailed
	}
	cleanupCtx, cancelCleanup := acceleratorDetachedSweepCleanupContext(outer, time.Now, context.WithTimeout)
	defer cancelCleanup()
	cleanupConfig, err := acceleratorActionConfig(cleanupCtx, config, candidate.namespace)
	if err != nil {
		return AcceleratorSweepCleanupFailed
	}
	cleanupClient, err := acceleratorClientset(cleanupCtx, config)
	if err != nil {
		return AcceleratorSweepCleanupFailed
	}
	uninstall := configureAcceleratorOwnedUninstall(cleanupConfig)
	if uninstall == nil {
		return AcceleratorSweepCleanupFailed
	}
	if _, err := uninstall.Run(candidate.name); err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return AcceleratorSweepAlreadyGone
		}
		return AcceleratorSweepCleanupFailed
	}
	if !waitAcceleratorSweepCandidateGoneWith(cleanupCtx, cleanupClient, candidate) {
		return AcceleratorSweepCleanupFailed
	}
	return AcceleratorSweepEligible
}

func acceleratorDetachedSweepCleanupContext(outer context.Context, now func() time.Time, withTimeout func(context.Context, time.Duration) (context.Context, context.CancelFunc)) (context.Context, context.CancelFunc) {
	duration := AcceleratorOwnedCleanupTimeout
	if now == nil {
		now = time.Now
	}
	if deadline, ok := outer.Deadline(); ok {
		if remaining := deadline.Sub(now()); remaining < duration {
			duration = remaining
		}
	}
	if duration <= 0 {
		duration = time.Nanosecond
	}
	if withTimeout == nil {
		withTimeout = context.WithTimeout
	}
	return withTimeout(context.WithoutCancel(outer), duration)
}

func uninstallAcceleratorSweepCandidateWith(ctx context.Context, actionConfig *action.Configuration, clientset kubernetes.Interface, candidate *AcceleratorSweepCandidate) AcceleratorSweepProofStatus {
	if actionConfig == nil || clientset == nil || candidate == nil || candidate.authority != trustedAcceleratorSweepAuthority {
		return AcceleratorSweepOwnershipChanged
	}
	second, status := inspectAcceleratorSweepCandidateWith(ctx, actionConfig, clientset, candidate.namespace, candidate.name)
	if status == AcceleratorSweepAlreadyGone {
		return status
	}
	if status != AcceleratorSweepEligible || !sameSweepCandidate(candidate, second) {
		return AcceleratorSweepOwnershipChanged
	}
	uninstall := configureAcceleratorOwnedUninstall(actionConfig)
	if uninstall == nil {
		return AcceleratorSweepCleanupFailed
	}
	if _, runErr := uninstall.Run(candidate.name); runErr != nil {
		if errors.Is(runErr, driver.ErrReleaseNotFound) {
			return AcceleratorSweepAlreadyGone
		}
		return AcceleratorSweepCleanupFailed
	}
	return AcceleratorSweepEligible
}

func sameSweepCandidate(left, right *AcceleratorSweepCandidate) bool {
	if left == nil || right == nil || left.authority != trustedAcceleratorSweepAuthority || right.authority != trustedAcceleratorSweepAuthority || left.name != right.name || left.namespace != right.namespace || left.session != right.session || left.renderHash != right.renderHash || left.storage != right.storage || len(left.resources) != len(right.resources) || len(left.pods) != len(right.pods) {
		return false
	}
	key := func(item AcceleratorDeletionIdentity) string {
		return item.Resource.APIVersion + "/" + item.Resource.Kind + "/" + item.Resource.Namespace + "/" + item.Resource.Name + "/" + string(item.UID)
	}
	a, b := make([]string, 0, len(left.resources)+len(left.pods)), make([]string, 0, len(right.resources)+len(right.pods))
	for _, item := range append(append([]AcceleratorDeletionIdentity{}, left.resources...), left.pods...) {
		a = append(a, key(item))
	}
	for _, item := range append(append([]AcceleratorDeletionIdentity{}, right.resources...), right.pods...) {
		b = append(b, key(item))
	}
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}

func (c *Client) WaitAcceleratorSweepCandidateGone(ctx context.Context, config *rest.Config, candidate *AcceleratorSweepCandidate) bool {
	if ctx == nil || candidate == nil || candidate.authority != trustedAcceleratorSweepAuthority {
		return false
	}
	client, err := acceleratorClientset(ctx, config)
	if err != nil {
		return false
	}
	return waitAcceleratorSweepCandidateGoneWith(ctx, client, candidate)
}

func waitAcceleratorSweepCandidateGoneWith(ctx context.Context, client kubernetes.Interface, candidate *AcceleratorSweepCandidate) bool {
	return waitAcceleratorSweepCandidateGoneWithPoll(ctx, client, candidate, time.Now, realAcceleratorSweepPollWait)
}

func waitAcceleratorSweepCandidateGoneWithPoll(ctx context.Context, client kubernetes.Interface, candidate *AcceleratorSweepCandidate, now func() time.Time, pollWait func(context.Context, time.Duration) bool) bool {
	if ctx == nil || client == nil || candidate == nil || candidate.authority != trustedAcceleratorSweepAuthority {
		return false
	}
	if now == nil {
		now = time.Now
	}
	if pollWait == nil {
		pollWait = realAcceleratorSweepPollWait
	}
	deadline := now().Add(AcceleratorOwnedCleanupTimeout)
	pending := make(map[string]AcceleratorDeletionIdentity, len(candidate.resources)+len(candidate.pods)+1)
	for _, item := range append(append([]AcceleratorDeletionIdentity{}, candidate.resources...), candidate.pods...) {
		key := item.Resource.APIVersion + "/" + item.Resource.Kind + "/" + item.Resource.Namespace + "/" + item.Resource.Name
		pending[key] = item
	}
	storageKey := "v1/Secret/" + candidate.storage.Namespace + "/" + candidate.storage.Name
	pending[storageKey] = AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "v1", Kind: "Secret", Namespace: candidate.storage.Namespace, Name: candidate.storage.Name}, UID: candidate.storage.UID}
	for {
		if ctx.Err() != nil || !now().Before(deadline) {
			return false
		}
		remaining := false
		for key, item := range pending {
			object, getErr := getAcceleratorLiveObject(ctx, client, item.Resource)
			if getErr == nil && object.GetUID() == item.UID {
				remaining = true
			} else if getErr == nil || apierrors.IsNotFound(getErr) {
				delete(pending, key)
			} else {
				remaining = true
			}
		}
		if !remaining {
			return true
		}
		if !pollWait(ctx, 250*time.Millisecond) {
			return false
		}
	}
}

func realAcceleratorSweepPollWait(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
