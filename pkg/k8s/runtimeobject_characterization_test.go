package k8s

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// canonicalJSON renders v as key-sorted, whitespace-free JSON in which numbers
// keep their textual form. v may be a map, a slice, a typed Kubernetes object,
// a json.RawMessage or a []byte holding JSON. Because the input is serialised
// and re-parsed, an int64 and a float64 holding the same integer compare equal,
// which is the property a map-returning and a RawMessage-returning
// RuntimeObjectToMap must share.
func canonicalJSON(t testing.TB, v interface{}) string {
	t.Helper()
	var raw []byte
	switch x := v.(type) {
	case json.RawMessage:
		raw = x
	case []byte:
		raw = x
	default:
		var err error
		raw, err = json.Marshal(v)
		if err != nil {
			t.Fatalf("canonicalJSON: marshal %T: %v", v, err)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic interface{}
	if err := dec.Decode(&generic); err != nil {
		t.Fatalf("canonicalJSON: decode: %v\n%s", err, raw)
	}
	out, err := json.Marshal(generic)
	if err != nil {
		t.Fatalf("canonicalJSON: re-marshal: %v", err)
	}
	return string(out)
}

// assertJSONEquivalentToTyped checks the output of RuntimeObjectToMap against
// json.Marshal of the typed object, which is what the frontend would have
// seen if the object had been sent as-is.
func assertJSONEquivalentToTyped(t *testing.T, obj interface{}) map[string]interface{} {
	t.Helper()
	got, err := RuntimeObjectToMap(obj)
	if err != nil {
		t.Fatalf("RuntimeObjectToMap(%T): %v", obj, err)
	}
	want := canonicalJSON(t, obj)
	have := canonicalJSON(t, got)
	if want != have {
		t.Fatalf("RuntimeObjectToMap(%T) is not JSON-equivalent to json.Marshal\nwant: %s\n got: %s", obj, want, have)
	}
	return got
}

func charBool(b bool) *bool    { return &b }
func charInt32(i int32) *int32 { return &i }
func charInt64(i int64) *int64 { return &i }

func mustQuantity(s string) resource.Quantity { return resource.MustParse(s) }

// newRichPod builds a pod resembling what a real API server returns: owner
// references, managed fields, resource quantities, timestamps and status.
func newRichPod(i int) *corev1.Pod {
	created := metav1.NewTime(time.Date(2026, 3, 14, 9, 26, 53, 0, time.UTC))
	started := metav1.NewTime(time.Date(2026, 3, 14, 9, 27, 1, 0, time.UTC))
	name := fmt.Sprintf("web-%d-7d9f8c6b5d-x%04d", i, i)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "production",
			UID:               "6f0c2d58-7c3b-4c1e-9f3a-0123456789ab",
			ResourceVersion:   strconv.Itoa(1000000 + i),
			Generation:        1,
			CreationTimestamp: created,
			Labels: map[string]string{
				"app":                          "web",
				"pod-template-hash":            "7d9f8c6b5d",
				"app.kubernetes.io/managed-by": "helm",
			},
			Annotations: map[string]string{
				"kubectl.kubernetes.io/restartedAt": "2026-03-14T09:26:53Z",
				"prometheus.io/scrape":              "true",
				"prometheus.io/port":                "9090",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "apps/v1",
				Kind:               "ReplicaSet",
				Name:               "web-7d9f8c6b5d",
				UID:                "11111111-2222-3333-4444-555555555555",
				Controller:         charBool(true),
				BlockOwnerDeletion: charBool(true),
			}},
			ManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:    "kube-controller-manager",
				Operation:  metav1.ManagedFieldsOperationUpdate,
				APIVersion: "v1",
				Time:       &created,
				FieldsType: "FieldsV1",
				FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:metadata":{"f:generateName":{},"f:labels":{".":{},"f:app":{}}}}`)},
			}},
		},
		Spec: corev1.PodSpec{
			NodeName:                      "node-" + strconv.Itoa(i%7),
			RestartPolicy:                 corev1.RestartPolicyAlways,
			TerminationGracePeriodSeconds: charInt64(30),
			Containers: []corev1.Container{{
				Name:  "web",
				Image: "registry.example.com/web:1.2.3",
				Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP}},
				Env: []corev1.EnvVar{
					{Name: "LOG_LEVEL", Value: "info"},
					{Name: "FEATURE_FLAG", Value: "true"},
					{Name: "RETRIES", Value: "3"},
					{Name: "DB_PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "db"}, Key: "password"}}},
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    mustQuantity("250m"),
						corev1.ResourceMemory: mustQuantity("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    mustQuantity("1"),
						corev1.ResourceMemory: mustQuantity("1Gi"),
					},
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler:  corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intstr.FromInt32(8080)}},
					PeriodSeconds: 10,
				},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/live", Port: intstr.FromString("http")}},
				},
			}},
			Volumes: []corev1.Volume{{
				Name:         "cfg",
				VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "web"}, DefaultMode: charInt32(420)}},
			}},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			HostIP:    "10.0.0.5",
			PodIP:     "10.244.1.17",
			PodIPs:    []corev1.PodIP{{IP: "10.244.1.17"}},
			QOSClass:  corev1.PodQOSBurstable,
			StartTime: &started,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: started},
				{Type: corev1.ContainersReady, Status: corev1.ConditionTrue, LastProbeTime: metav1.Time{}},
			},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "web",
				Ready:        true,
				RestartCount: 2,
				Image:        "registry.example.com/web:1.2.3",
				ImageID:      "registry.example.com/web@sha256:abcdef",
				ContainerID:  "containerd://0123",
				Started:      charBool(true),
				State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: started}},
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
					ExitCode: 137, Reason: "OOMKilled", StartedAt: created, FinishedAt: started}},
			}},
		},
	}
}

func TestRuntimeObjectToMapCharacterizationTypedObjects(t *testing.T) {
	deployCreated := metav1.NewTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	maxSurge := intstr.FromString("25%")
	maxUnavail := intstr.FromInt32(1)

	cases := []struct {
		name string
		obj  interface{}
	}{
		{"pod", newRichPod(1)},
		{"deployment", &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "production", ResourceVersion: "42", CreationTimestamp: deployCreated, Generation: 7},
			Spec: appsv1.DeploymentSpec{
				Replicas: charInt32(3),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{MaxSurge: &maxSurge, MaxUnavailable: &maxUnavail}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
					Spec:       newRichPod(0).Spec,
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 3, ReadyReplicas: 3, AvailableReplicas: 3, ObservedGeneration: 7,
				Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue, LastUpdateTime: deployCreated, LastTransitionTime: deployCreated}}},
		}},
		{"node", &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1", ResourceVersion: "9", CreationTimestamp: deployCreated,
				Labels: map[string]string{"kubernetes.io/arch": "arm64", "kubernetes.io/os": "linux"}},
			Spec: corev1.NodeSpec{PodCIDR: "10.244.1.0/24", Taints: []corev1.Taint{{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule}}},
			Status: corev1.NodeStatus{
				Capacity:    corev1.ResourceList{corev1.ResourceCPU: mustQuantity("8"), corev1.ResourceMemory: mustQuantity("32780748Ki"), corev1.ResourcePods: mustQuantity("110"), corev1.ResourceEphemeralStorage: mustQuantity("102626232Ki")},
				Allocatable: corev1.ResourceList{corev1.ResourceCPU: mustQuantity("7910m"), corev1.ResourceMemory: mustQuantity("31" + "Gi")},
				Addresses:   []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.5"}, {Type: corev1.NodeHostName, Address: "node-1"}},
				NodeInfo:    corev1.NodeSystemInfo{KernelVersion: "6.1.0", KubeletVersion: "v1.30.2", Architecture: "arm64"},
				Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: deployCreated, LastTransitionTime: deployCreated}},
			},
		}},
		{"service", &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "production", ResourceVersion: "5", CreationTimestamp: deployCreated},
			Spec: corev1.ServiceSpec{
				Type:       corev1.ServiceTypeClusterIP,
				ClusterIP:  "10.96.0.12",
				ClusterIPs: []string{"10.96.0.12"},
				Selector:   map[string]string{"app": "web"},
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: corev1.ProtocolTCP},
					{Name: "named", Port: 8443, TargetPort: intstr.FromString("https"), Protocol: corev1.ProtocolTCP},
				},
			},
		}},
		{"secret with binary data", &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "production", ResourceVersion: "6", CreationTimestamp: deployCreated},
			Type:       corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"password": []byte("hunter2"),
				"binary":   {0x00, 0x01, 0x02, 0xff, 0xfe, 0x80},
				"empty":    {},
				"utf8":     []byte("café €"),
			},
			StringData: map[string]string{"note": "plain"},
		}},
		{"event", &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "web.17abc", Namespace: "production", ResourceVersion: "77", CreationTimestamp: deployCreated},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "web-0", Namespace: "production", UID: "abc"},
			Reason:         "BackOff", Message: "Back-off restarting failed container", Type: corev1.EventTypeWarning,
			Count:          12,
			FirstTimestamp: deployCreated,
			LastTimestamp:  deployCreated,
			EventTime:      metav1.NewMicroTime(time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC)),
			Source:         corev1.EventSource{Component: "kubelet", Host: "node-1"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertJSONEquivalentToTyped(t, tc.obj) })
	}
}

// Strings that YAML or a number-aware decoder could reinterpret. Any
// implementation of RuntimeObjectToMap must keep them as strings.
func TestRuntimeObjectToMapCharacterizationAmbiguousStrings(t *testing.T) {
	values := []string{
		"true", "false", "yes", "no", "on", "off", "null", "~", "123", "0123", "0x1f", "1e3", "1.0", "-1", ".5", "1_000",
		"2020-01-01", "2020-01-01T00:00:00Z", "1:30", "", " leading", "trailing ", "multi\nline", "tab\there",
		"unicode €", "'quoted'", "\"dq\"", "#hash", "- dash", ": colon", "a: b", "{brace}", "[bracket]", "*star", "&amp", "!tag", "%pct", "@at", "`tick",
	}
	labels := map[string]string{}
	annotations := map[string]string{}
	for i, v := range values {
		labels[fmt.Sprintf("k%d", i)] = v
		annotations[v+"key"] = v
	}
	// Keys that look like other scalar types.
	for _, k := range []string{"true", "123", "null", "1e3"} {
		labels[k] = "v-" + k
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Labels: labels, Annotations: annotations}}
	assertJSONEquivalentToTyped(t, pod)
}

func TestRuntimeObjectToMapCharacterizationSecretBytes(t *testing.T) {
	bin := []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x80}
	m := assertJSONEquivalentToTyped(t, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
		Data:       map[string][]byte{"binary": bin, "empty": {}},
	})
	data, ok := m["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data has type %T, want map", m["data"])
	}
	if got, want := data["binary"], base64.StdEncoding.EncodeToString(bin); got != want {
		t.Fatalf("binary = %v, want base64 %q", got, want)
	}
	if got := data["empty"]; got != "" {
		t.Fatalf("empty []byte should encode as empty string, got %#v", got)
	}
}

func TestRuntimeObjectToMapCharacterizationQuantityIsString(t *testing.T) {
	m := assertJSONEquivalentToTyped(t, newRichPod(1))
	spec := m["spec"].(map[string]interface{})
	c := spec["containers"].([]interface{})[0].(map[string]interface{})
	limits := c["resources"].(map[string]interface{})["limits"].(map[string]interface{})
	// "1" is a string, never a number, even though it looks numeric.
	if v, ok := limits["cpu"].(string); !ok || v != "1" {
		t.Fatalf("limits.cpu = %#v, want string \"1\"", limits["cpu"])
	}
	if v, ok := limits["memory"].(string); !ok || v != "1Gi" {
		t.Fatalf("limits.memory = %#v, want string \"1Gi\"", limits["memory"])
	}
	requests := c["resources"].(map[string]interface{})["requests"].(map[string]interface{})
	if v := requests["cpu"]; v != "250m" {
		t.Fatalf("requests.cpu = %#v, want \"250m\"", v)
	}
}

// Documents the Go type of numbers inside the returned map. The current
// implementation round-trips through YAML/JSON so every number is float64.
// A faster implementation may change this, but must then update every
// consumer that type-asserts on numbers (the watch loop only reads strings).
func TestRuntimeObjectToMapCharacterizationNumbersAreFloat64(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Generation: 9},
		Spec:       appsv1.DeploymentSpec{Replicas: charInt32(3)},
	}
	m := assertJSONEquivalentToTyped(t, dep)
	if v, ok := m["spec"].(map[string]interface{})["replicas"].(float64); !ok || v != 3 {
		t.Fatalf("spec.replicas = %#v (%T), want float64(3)", m["spec"].(map[string]interface{})["replicas"], m["spec"].(map[string]interface{})["replicas"])
	}
	if v, ok := m["metadata"].(map[string]interface{})["generation"].(float64); !ok || v != 9 {
		t.Fatalf("metadata.generation = %#v, want float64(9)", m["metadata"].(map[string]interface{})["generation"])
	}
}

func TestRuntimeObjectToMapCharacterizationIntOrString(t *testing.T) {
	svc := &corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080)},
		{Port: 81, TargetPort: intstr.FromString("http")},
	}}}
	m := assertJSONEquivalentToTyped(t, svc)
	ports := m["spec"].(map[string]interface{})["ports"].([]interface{})
	if v, ok := ports[0].(map[string]interface{})["targetPort"].(float64); !ok || v != 8080 {
		t.Fatalf("numeric targetPort = %#v, want float64(8080)", ports[0].(map[string]interface{})["targetPort"])
	}
	if v, ok := ports[1].(map[string]interface{})["targetPort"].(string); !ok || v != "http" {
		t.Fatalf("named targetPort = %#v, want \"http\"", ports[1].(map[string]interface{})["targetPort"])
	}
}

// Zero metav1.Time handling differs by location: ObjectMeta.creationTimestamp
// is omitted entirely when zero (omitzero in current apimachinery), whereas
// other metav1.Time fields such as PodCondition.lastProbeTime serialise as an
// explicit null. A faster encoder must keep both behaviours; assertJSONEquivalentToTyped
// already guarantees it matches json.Marshal, these assertions make the
// distinction visible.
func TestRuntimeObjectToMapCharacterizationZeroTimestamps(t *testing.T) {
	m := assertJSONEquivalentToTyped(t, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{
			{Type: corev1.PodReady, Status: corev1.ConditionTrue},
		}},
	})
	md := m["metadata"].(map[string]interface{})
	if _, present := md["creationTimestamp"]; present {
		t.Fatalf("zero creationTimestamp should be omitted, got %#v", md["creationTimestamp"])
	}
	cond := m["status"].(map[string]interface{})["conditions"].([]interface{})[0].(map[string]interface{})
	v, present := cond["lastProbeTime"]
	if !present || v != nil {
		t.Fatalf("zero lastProbeTime = %#v (present=%v), want key present with nil", v, present)
	}
}

func TestRuntimeObjectToMapCharacterizationTimestampFormat(t *testing.T) {
	ts := metav1.NewTime(time.Date(2026, 3, 14, 9, 26, 53, 999, time.FixedZone("x", 3600)))
	m := assertJSONEquivalentToTyped(t, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", CreationTimestamp: ts}})
	got := m["metadata"].(map[string]interface{})["creationTimestamp"]
	// metav1.Time serialises in UTC with second precision.
	if got != "2026-03-14T08:26:53Z" {
		t.Fatalf("creationTimestamp = %#v", got)
	}
}

// Empty structs that json.Marshal does not omit must appear as empty maps.
func TestRuntimeObjectToMapCharacterizationEmptyStructsAreKept(t *testing.T) {
	m := assertJSONEquivalentToTyped(t, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d"}})
	for _, key := range []string{"spec", "status"} {
		v, ok := m[key].(map[string]interface{})
		if !ok {
			t.Fatalf("%s = %#v, want map", key, m[key])
		}
		if key == "status" && len(v) != 0 {
			t.Fatalf("empty status should be an empty map, got %#v", v)
		}
	}
	// nil slices marshal as null and must stay null, not an empty list.
	tmplSpec := m["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})
	if v, present := tmplSpec["containers"]; !present || v != nil {
		t.Fatalf("nil containers = %#v (present=%v), want nil", v, present)
	}
}

// Watch decoders strip TypeMeta from typed objects, so kind and apiVersion are
// absent unless the caller set them.
func TestRuntimeObjectToMapCharacterizationTypeMeta(t *testing.T) {
	m := assertJSONEquivalentToTyped(t, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"}})
	if _, ok := m["kind"]; ok {
		t.Fatal("kind present on typed object without TypeMeta")
	}
	if _, ok := m["apiVersion"]; ok {
		t.Fatal("apiVersion present on typed object without TypeMeta")
	}

	m = assertJSONEquivalentToTyped(t, &corev1.Pod{TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"}, ObjectMeta: metav1.ObjectMeta{Name: "p"}})
	if m["kind"] != "Pod" || m["apiVersion"] != "v1" {
		t.Fatalf("explicit TypeMeta lost: kind=%v apiVersion=%v", m["kind"], m["apiVersion"])
	}
}

func TestRuntimeObjectToMapCharacterizationUnstructured(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "example.com/v1",
		"kind":       "Widget",
		"metadata": map[string]interface{}{
			"name":              "w1",
			"namespace":         "default",
			"resourceVersion":   "123",
			"generation":        int64(4),
			"creationTimestamp": "2026-03-14T09:26:53Z",
			"labels":            map[string]interface{}{"a": "b"},
		},
		"spec": map[string]interface{}{
			"replicas": int64(3),
			"ratio":    0.25,
			"enabled":  true,
			"nothing":  nil,
			"list":     []interface{}{"x", int64(1), map[string]interface{}{"deep": []interface{}{}}},
		},
		"status": map[string]interface{}{},
	}}
	got, err := RuntimeObjectToMap(u)
	if err != nil {
		t.Fatal(err)
	}
	if canonicalJSON(t, got) != canonicalJSON(t, u.Object) {
		t.Fatalf("unstructured content changed:\n%s\n%s", canonicalJSON(t, got), canonicalJSON(t, u.Object))
	}
	// Unstructured objects are returned without copying today. Callers must
	// treat the map as read-only if that stays true; if an implementation
	// starts copying, this assertion should be relaxed deliberately.
	if reflect.ValueOf(got).Pointer() != reflect.ValueOf(u.Object).Pointer() {
		t.Fatal("expected the unstructured map to be returned without a copy")
	}
	// Because no round trip happens, int64 is preserved here (unlike typed objects).
	if v, ok := got["spec"].(map[string]interface{})["replicas"].(int64); !ok || v != 3 {
		t.Fatalf("replicas = %#v, want int64(3)", got["spec"].(map[string]interface{})["replicas"])
	}
	if got["kind"] != "Widget" || got["apiVersion"] != "example.com/v1" {
		t.Fatal("unstructured kind/apiVersion must be preserved")
	}
}

func TestRuntimeObjectToMapCharacterizationNilUnstructuredObject(t *testing.T) {
	got, err := RuntimeObjectToMap(&unstructured.Unstructured{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("empty unstructured produced %#v", got)
	}
}

func TestRuntimeObjectToMapCharacterizationUnmarshalableInput(t *testing.T) {
	if _, err := RuntimeObjectToMap(make(chan int)); err == nil {
		t.Fatal("expected an error for a value that cannot be marshalled")
	}
}

// Output must be independent of the input: mutating the returned map must not
// change the typed object (and vice versa) because the coalescer holds the
// map while the informer may reuse the object.
func TestRuntimeObjectToMapCharacterizationTypedOutputIsDetached(t *testing.T) {
	pod := newRichPod(1)
	m, err := RuntimeObjectToMap(pod)
	if err != nil {
		t.Fatal(err)
	}
	m["metadata"].(map[string]interface{})["labels"].(map[string]interface{})["app"] = "mutated"
	if pod.Labels["app"] != "web" {
		t.Fatal("mutating the returned map changed the typed object")
	}
}
