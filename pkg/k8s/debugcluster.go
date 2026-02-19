//go:build debugcluster

package k8s

import (
	"fmt"
	"log"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// DebugClusterContextName is the virtual context name shown in the context picker.
const DebugClusterContextName = "debug-cluster"

// IsDebugClusterContext returns true when the given context is the virtual debug cluster.
func IsDebugClusterContext(contextName string) bool {
	return contextName == DebugClusterContextName
}

// DefaultDebugClusterConfig returns sensible defaults for the debug cluster.
func DefaultDebugClusterConfig() DebugClusterConfig {
	return DebugClusterConfig{
		Namespaces:   5,
		Pods:         100,
		Deployments:  10,
		Services:     10,
		ConfigMaps:   10,
		Secrets:      5,
		Nodes:        5,
		StatefulSets: 3,
		DaemonSets:   2,
		Jobs:         5,
		ReplicaSets:  10,
	}
}

var (
	debugClusterMu     sync.Mutex
	debugClusterConfig DebugClusterConfig
	debugClientset     kubernetes.Interface
)

func init() {
	debugClusterConfig = DefaultDebugClusterConfig()
}

// GetDebugClusterConfig returns the current debug cluster configuration.
func GetDebugClusterConfig() DebugClusterConfig {
	debugClusterMu.Lock()
	defer debugClusterMu.Unlock()
	return debugClusterConfig
}

// GetDebugClusterClientset returns the cached fake clientset, creating it on first call.
func GetDebugClusterClientset() (kubernetes.Interface, error) {
	debugClusterMu.Lock()
	defer debugClusterMu.Unlock()
	if debugClientset == nil {
		cs, err := buildFakeClientset(debugClusterConfig)
		if err != nil {
			return nil, err
		}
		debugClientset = cs
	}
	return debugClientset, nil
}

// RegenerateDebugCluster recreates the fake clientset with the given config.
func RegenerateDebugCluster(config DebugClusterConfig) error {
	debugClusterMu.Lock()
	defer debugClusterMu.Unlock()
	debugClusterConfig = config
	cs, err := buildFakeClientset(config)
	if err != nil {
		return err
	}
	debugClientset = cs
	return nil
}

// switchToDebugCluster swaps the client's clientset to the fake debug cluster.
func (c *Client) switchToDebugCluster() error {
	cs, err := GetDebugClusterClientset()
	if err != nil {
		return fmt.Errorf("failed to create debug cluster clientset: %w", err)
	}

	c.mu.Lock()
	c.clientset = cs
	c.metricsClient = nil
	c.clientPool = nil
	c.currentContext = DebugClusterContextName
	c.mu.Unlock()

	log.Printf("[Debug Cluster] Switched to debug cluster (config: %+v)", GetDebugClusterConfig())
	return nil
}

// ---------------------------------------------------------------------------
// Fake clientset builder
// ---------------------------------------------------------------------------

// resourceVersion is a sequential counter for generating unique resource versions.
var rvCounter int64

func nextRV() string {
	rvCounter++
	return fmt.Sprintf("%d", rvCounter)
}

func buildFakeClientset(cfg DebugClusterConfig) (kubernetes.Interface, error) {
	rvCounter = 0
	start := time.Now()

	var objects []runtime.Object

	// --- Nodes (cluster-scoped) ---
	nodeNames := make([]string, cfg.Nodes)
	for i := range cfg.Nodes {
		n := generateNode(i)
		nodeNames[i] = n.Name
		objects = append(objects, n)
	}

	// --- Namespaces ---
	nsNames := make([]string, cfg.Namespaces)
	for i := range cfg.Namespaces {
		ns := generateNamespace(i)
		nsNames[i] = ns.Name
		objects = append(objects, ns)
	}

	// --- Per-namespace resources ---
	for _, nsName := range nsNames {
		// Deployments → ReplicaSets → Pods (owned)
		for i := range cfg.Deployments {
			dep, rs, depPods := generateDeploymentChain(nsName, i, nodeNames)
			objects = append(objects, dep, rs)
			for _, p := range depPods {
				objects = append(objects, p)
			}
		}

		// Extra standalone ReplicaSets
		for i := range cfg.ReplicaSets {
			rs := generateStandaloneReplicaSet(nsName, i)
			objects = append(objects, rs)
		}

		// StatefulSets
		for i := range cfg.StatefulSets {
			ss, ssPods := generateStatefulSet(nsName, i, nodeNames)
			objects = append(objects, ss)
			for _, p := range ssPods {
				objects = append(objects, p)
			}
		}

		// DaemonSets
		for i := range cfg.DaemonSets {
			ds := generateDaemonSet(nsName, i)
			objects = append(objects, ds)
		}

		// Standalone Pods (not owned by deployments/statefulsets)
		for i := range cfg.Pods {
			objects = append(objects, generatePod(nsName, i, nodeNames))
		}

		// Services
		for i := range cfg.Services {
			objects = append(objects, generateService(nsName, i))
		}

		// ConfigMaps
		for i := range cfg.ConfigMaps {
			objects = append(objects, generateConfigMap(nsName, i))
		}

		// Secrets
		for i := range cfg.Secrets {
			objects = append(objects, generateSecret(nsName, i))
		}

		// Jobs
		for i := range cfg.Jobs {
			objects = append(objects, generateJob(nsName, i))
		}

		// Events (a handful per namespace)
		for i := range 10 {
			objects = append(objects, generateEvent(nsName, i))
		}
	}

	log.Printf("[Debug Cluster] Generated %d objects in %v", len(objects), time.Since(start))

	// Build a node→pods index so the field-selector reactor can avoid
	// querying the tracker (which doesn't support field selectors).
	podsByNode := make(map[string][]corev1.Pod)
	for _, obj := range objects {
		pod, ok := obj.(*corev1.Pod)
		if !ok || pod.Spec.NodeName == "" {
			continue
		}
		podsByNode[pod.Spec.NodeName] = append(podsByNode[pod.Spec.NodeName], *pod)
	}

	cs := fake.NewSimpleClientset(objects...)

	// The fake clientset ignores field selectors, so prepend a reactor that
	// intercepts pod list calls with a spec.nodeName selector and returns
	// only pods scheduled on that node (used by ListPodsForNode).
	cs.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		la, ok := action.(k8stesting.ListAction)
		if !ok {
			return false, nil, nil
		}
		fs := la.GetListRestrictions().Fields
		if fs == nil || fs.Empty() {
			return false, nil, nil // no field selector — fall through to default
		}
		val, ok := fs.RequiresExactMatch("spec.nodeName")
		if !ok {
			return false, nil, nil
		}
		return true, &corev1.PodList{Items: podsByNode[val]}, nil
	})

	return cs, nil
}

// ---------------------------------------------------------------------------
// Resource generators
// ---------------------------------------------------------------------------

func uid(kind, ns, name string) types.UID {
	return types.UID(fmt.Sprintf("%s-%s-%s", kind, ns, name))
}

func generateNamespace(idx int) *corev1.Namespace {
	name := fmt.Sprintf("debug-ns-%d", idx)
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			UID:               uid("ns", "", name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            map[string]string{"kubernetes.io/metadata.name": name},
		},
		Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
}

func generateNode(idx int) *corev1.Node {
	name := fmt.Sprintf("debug-node-%d", idx)
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			UID:               uid("node", "", name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels: map[string]string{
				"kubernetes.io/hostname": name,
				"kubernetes.io/os":       "linux",
				"kubernetes.io/arch":     "amd64",
				"node-role.kubernetes.io/worker": "",
			},
		},
		Status: corev1.NodeStatus{
			Phase: corev1.NodeRunning,
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: metav1.Now()},
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("8"),
				corev1.ResourceMemory: resource.MustParse("32Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("8"),
				corev1.ResourceMemory: resource.MustParse("32Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: fmt.Sprintf("10.0.%d.%d", idx/256, idx%256)},
				{Type: corev1.NodeHostName, Address: name},
			},
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion:          "v1.31.0",
				ContainerRuntimeVersion: "containerd://1.7.0",
				OSImage:                 "Ubuntu 22.04 LTS",
				OperatingSystem:         "linux",
				Architecture:            "amd64",
			},
		},
	}
}

func podPhase(idx int) (corev1.PodPhase, []corev1.ContainerStatus) {
	containerName := "app"
	switch {
	case idx%100 < 80: // 80% Running
		return corev1.PodRunning, []corev1.ContainerStatus{{
			Name:  containerName,
			Ready: true,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}},
			RestartCount: int32(idx % 3),
		}}
	case idx%100 < 90: // 10% Pending
		return corev1.PodPending, []corev1.ContainerStatus{{
			Name:  containerName,
			Ready: false,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
		}}
	case idx%100 < 95: // 5% Failed
		return corev1.PodFailed, []corev1.ContainerStatus{{
			Name:  containerName,
			Ready: false,
			State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 1,
				Reason:   "Error",
			}},
		}}
	case idx%100 < 98: // 3% CrashLoopBackOff
		return corev1.PodRunning, []corev1.ContainerStatus{{
			Name:         containerName,
			Ready:        false,
			RestartCount: int32(5 + idx%10),
			State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 137,
				Reason:   "OOMKilled",
			}},
		}}
	default: // 2% Succeeded
		return corev1.PodSucceeded, []corev1.ContainerStatus{{
			Name:  containerName,
			Ready: false,
			State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 0,
				Reason:   "Completed",
			}},
		}}
	}
}

func generatePod(ns string, idx int, nodeNames []string) *corev1.Pod {
	name := fmt.Sprintf("pod-%d", idx)
	phase, containerStatuses := podPhase(idx)
	nodeName := ""
	if len(nodeNames) > 0 && phase != corev1.PodPending {
		nodeName = nodeNames[idx%len(nodeNames)]
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			UID:               uid("pod", ns, name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            map[string]string{"app": name, "debug": "true"},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{{
				Name:  "app",
				Image: fmt.Sprintf("debug-image:%d", idx%5),
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase:             phase,
			ContainerStatuses: containerStatuses,
			PodIP:             fmt.Sprintf("10.%d.%d.%d", idx/65536%256, idx/256%256, idx%256),
			HostIP:            "10.0.0.1",
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: conditionForPhase(phase)},
				{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
			},
		},
	}
}

func conditionForPhase(phase corev1.PodPhase) corev1.ConditionStatus {
	if phase == corev1.PodRunning {
		return corev1.ConditionTrue
	}
	return corev1.ConditionFalse
}

func generateDeploymentChain(ns string, idx int, nodeNames []string) (*appsv1.Deployment, *appsv1.ReplicaSet, []*corev1.Pod) {
	depName := fmt.Sprintf("deploy-%d", idx)
	rsName := fmt.Sprintf("deploy-%d-rs", idx)
	replicas := int32(3)

	depUID := uid("deploy", ns, depName)
	rsUID := uid("rs", ns, rsName)

	labels := map[string]string{"app": depName, "debug": "true"}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              depName,
			Namespace:         ns,
			UID:               depUID,
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: fmt.Sprintf("deploy-image:%d", idx%3),
					}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:          replicas,
			ReadyReplicas:     replicas,
			AvailableReplicas: replicas,
			UpdatedReplicas:   replicas,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
			},
		},
	}

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              rsName,
			Namespace:         ns,
			UID:               rsUID,
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            labels,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       depName,
				UID:        depUID,
				Controller: ptr(true),
			}},
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: dep.Spec.Template,
		},
		Status: appsv1.ReplicaSetStatus{
			Replicas:          replicas,
			ReadyReplicas:     replicas,
			AvailableReplicas: replicas,
		},
	}

	pods := make([]*corev1.Pod, replicas)
	for i := range replicas {
		podName := fmt.Sprintf("%s-%d", rsName, i)
		phase, cs := podPhase(idx*10 + int(i)) // varied but deterministic
		nodeName := ""
		if len(nodeNames) > 0 {
			nodeName = nodeNames[int(i)%len(nodeNames)]
		}
		pods[i] = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              podName,
				Namespace:         ns,
				UID:               uid("pod", ns, podName),
				ResourceVersion:   nextRV(),
				CreationTimestamp: metav1.Now(),
				Labels:            labels,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       rsName,
					UID:        rsUID,
					Controller: ptr(true),
				}},
			},
			Spec: corev1.PodSpec{
				NodeName: nodeName,
				Containers: []corev1.Container{{
					Name:  "app",
					Image: fmt.Sprintf("deploy-image:%d", idx%3),
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				}},
			},
			Status: corev1.PodStatus{
				Phase:             phase,
				ContainerStatuses: cs,
				PodIP:             fmt.Sprintf("10.%d.%d.%d", idx, int(i), 1),
				HostIP:            "10.0.0.1",
			},
		}
	}

	return dep, rs, pods
}

func generateStandaloneReplicaSet(ns string, idx int) *appsv1.ReplicaSet {
	name := fmt.Sprintf("standalone-rs-%d", idx)
	replicas := int32(2)
	labels := map[string]string{"app": name}

	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			UID:               uid("rs", ns, name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            labels,
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
		},
		Status: appsv1.ReplicaSetStatus{
			Replicas:      replicas,
			ReadyReplicas: replicas,
		},
	}
}

func generateStatefulSet(ns string, idx int, nodeNames []string) (*appsv1.StatefulSet, []*corev1.Pod) {
	name := fmt.Sprintf("sts-%d", idx)
	replicas := int32(3)
	labels := map[string]string{"app": name}
	stsUID := uid("sts", ns, name)

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			UID:               stsUID,
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: name,
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: fmt.Sprintf("sts-image:%d", idx),
					}},
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:      replicas,
			ReadyReplicas: replicas,
		},
	}

	pods := make([]*corev1.Pod, replicas)
	for i := range replicas {
		podName := fmt.Sprintf("%s-%d", name, i)
		nodeName := ""
		if len(nodeNames) > 0 {
			nodeName = nodeNames[int(i)%len(nodeNames)]
		}
		pods[i] = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              podName,
				Namespace:         ns,
				UID:               uid("pod", ns, podName),
				ResourceVersion:   nextRV(),
				CreationTimestamp: metav1.Now(),
				Labels:            labels,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       name,
					UID:        stsUID,
					Controller: ptr(true),
				}},
			},
			Spec: corev1.PodSpec{
				NodeName: nodeName,
				Containers: []corev1.Container{{
					Name:  "app",
					Image: fmt.Sprintf("sts-image:%d", idx),
				}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  "app",
					Ready: true,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}},
				}},
				PodIP:  fmt.Sprintf("10.%d.%d.%d", idx+100, int(i), 1),
				HostIP: "10.0.0.1",
			},
		}
	}

	return ss, pods
}

func generateDaemonSet(ns string, idx int) *appsv1.DaemonSet {
	name := fmt.Sprintf("ds-%d", idx)
	labels := map[string]string{"app": name}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			UID:               uid("ds", ns, name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "daemon",
						Image: fmt.Sprintf("daemon-image:%d", idx),
					}},
				},
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 5,
			CurrentNumberScheduled: 5,
			NumberReady:            5,
			NumberAvailable:        5,
		},
	}
}

func generateService(ns string, idx int) *corev1.Service {
	name := fmt.Sprintf("svc-%d", idx)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			UID:               uid("svc", ns, name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            map[string]string{"app": name},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{{
				Name:     "http",
				Port:     int32(80 + idx%100),
				Protocol: corev1.ProtocolTCP,
			}},
			ClusterIP: fmt.Sprintf("10.96.%d.%d", idx/256, idx%256+1),
		},
	}
}

func generateConfigMap(ns string, idx int) *corev1.ConfigMap {
	name := fmt.Sprintf("cm-%d", idx)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			UID:               uid("cm", ns, name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            map[string]string{"app": name},
		},
		Data: map[string]string{
			"config.yaml": fmt.Sprintf("key: value-%d\nenv: debug", idx),
			"debug":       "true",
		},
	}
}

func generateSecret(ns string, idx int) *corev1.Secret {
	name := fmt.Sprintf("secret-%d", idx)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			UID:               uid("secret", ns, name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            map[string]string{"app": name},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password": []byte(fmt.Sprintf("debug-pass-%d", idx)),
			"token":    []byte("debug-token"),
		},
	}
}

func generateJob(ns string, idx int) *batchv1.Job {
	name := fmt.Sprintf("job-%d", idx)
	completions := int32(1)
	var succeeded int32
	var failed int32

	switch {
	case idx%3 == 0:
		succeeded = 1 // completed
	case idx%3 == 1:
		failed = 1 // failed
	default:
		// active
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			UID:               uid("job", ns, name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            map[string]string{"app": name},
		},
		Spec: batchv1.JobSpec{
			Completions: &completions,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "worker",
						Image:   fmt.Sprintf("job-image:%d", idx),
						Command: []string{"echo", "hello"},
					}},
				},
			},
		},
		Status: batchv1.JobStatus{
			Succeeded: succeeded,
			Failed:    failed,
		},
	}
}

func generateEvent(ns string, idx int) *corev1.Event {
	name := fmt.Sprintf("event-%d", idx)
	reasons := []string{"Pulled", "Created", "Started", "Scheduled", "FailedScheduling", "Unhealthy", "BackOff", "Killing", "ScalingReplicaSet", "SuccessfulCreate"}
	types := []string{"Normal", "Normal", "Normal", "Normal", "Warning", "Warning", "Warning", "Normal", "Normal", "Normal"}

	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			UID:               uid("event", ns, name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      fmt.Sprintf("pod-%d", idx),
			Namespace: ns,
		},
		Reason:  reasons[idx%len(reasons)],
		Message: fmt.Sprintf("Debug event message %d: %s", idx, reasons[idx%len(reasons)]),
		Type:    types[idx%len(types)],
		Count:   int32(1 + idx%5),
		FirstTimestamp: metav1.Now(),
		LastTimestamp:  metav1.Now(),
	}
}
