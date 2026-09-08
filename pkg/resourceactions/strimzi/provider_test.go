package strimzi

import (
	"context"
	"testing"

	"kubikles/pkg/resourceactions"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
)

func obj(api, kind, name, uid string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": api, "kind": kind, "metadata": map[string]interface{}{"name": name, "namespace": "ns", "uid": uid, "resourceVersion": "1"}}}
}
func owner(o *unstructured.Unstructured, api, kind, name, uid string) {
	o.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: api, Kind: kind, Name: name, UID: types.UID(uid)}})
}
func setup() (*fake.FakeDynamicClient, resourceactions.ResourceRef) {
	kafka := obj("kafka.strimzi.io/v1beta2", "Kafka", "demo", "kafka-uid")
	pool := obj("kafka.strimzi.io/v1beta2", "KafkaNodePool", "brokers", "pool-uid")
	pool.SetLabels(map[string]string{"strimzi.io/cluster": "demo"})
	set := obj("core.strimzi.io/v1beta2", "StrimziPodSet", "demo-brokers", "set-uid")
	owner(set, "kafka.strimzi.io/v1beta2", "KafkaNodePool", "brokers", "pool-uid")
	labels := map[string]string{"strimzi.io/cluster": "demo", "strimzi.io/kind": "Kafka", "strimzi.io/name": "demo-kafka"}
	set.SetLabels(labels)
	pod := obj("v1", "Pod", "demo-brokers-0", "pod-uid")
	pod.SetLabels(labels)
	owner(pod, "core.strimzi.io/v1beta2", "StrimziPodSet", "demo-brokers", "set-uid")
	unrelated := obj("v1", "Pod", "unrelated", "unrelated-uid")
	unrelated.SetLabels(labels)
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{podSets: "StrimziPodSetList", pods: "PodList"}, kafka, pool, set, pod, unrelated)
	return client, resourceactions.ResourceRef{Context: "dev", Group: "kafka.strimzi.io", Version: "v1beta2", Resource: "kafkas", Namespace: "ns", Name: "demo", UID: "kafka-uid"}
}
func TestNodePoolDiscoveryAndSelectedPodExecution(t *testing.T) {
	c, ref := setup()
	r := resourceactions.New(Provider{})
	for _, mode := range []string{"cluster", "pods"} {
		plan, err := r.Prepare(context.Background(), c, ref, restartID, mode)
		if err != nil || len(plan.Targets) != 1 {
			t.Fatalf("%s: %+v %v", mode, plan, err)
		}
		if mode == "pods" {
			if plan.Targets[0].Name != "demo-brokers-0" {
				t.Fatal(plan.Targets)
			}
			result, err := r.Execute(context.Background(), c, plan)
			if err != nil || result[0].Status != "requested" {
				t.Fatalf("%+v %v", result, err)
			}
		}
	}
	set, _ := c.Resource(podSets).Namespace("ns").Get(context.Background(), "demo-brokers", metav1.GetOptions{})
	if set.GetAnnotations()[rollingAnnotation] != "" {
		t.Fatal("pod scope modified pod set")
	}
}
func TestPausedReplacedAndUnsupportedKafkaAreRejected(t *testing.T) {
	for _, scenario := range []string{"paused", "replaced", "version", "owner"} {
		t.Run(scenario, func(t *testing.T) {
			c, ref := setup()
			switch scenario {
			case "paused":
				k, _ := c.Resource(ref.GVR()).Namespace("ns").Get(context.Background(), ref.Name, metav1.GetOptions{})
				k.SetAnnotations(map[string]string{"strimzi.io/pause-reconciliation": "true"})
				c.Resource(ref.GVR()).Namespace("ns").Update(context.Background(), k, metav1.UpdateOptions{})
			case "replaced":
				ref.UID = "old"
			case "version":
				ref.Version = "v1alpha1"
			case "owner":
				set, _ := c.Resource(podSets).Namespace("ns").Get(context.Background(), "demo-brokers", metav1.GetOptions{})
				owner(set, "kafka.strimzi.io/v1beta2", "KafkaNodePool", "brokers", "other")
				c.Resource(podSets).Namespace("ns").Update(context.Background(), set, metav1.UpdateOptions{})
			}
			if _, err := resourceactions.New(Provider{}).Prepare(context.Background(), c, ref, restartID, "cluster"); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}
func TestParentRestartMakesPodRequestPending(t *testing.T) {
	c, ref := setup()
	set, _ := c.Resource(podSets).Namespace("ns").Get(context.Background(), "demo-brokers", metav1.GetOptions{})
	set.SetAnnotations(map[string]string{rollingAnnotation: "true"})
	c.Resource(podSets).Namespace("ns").Update(context.Background(), set, metav1.UpdateOptions{})
	r := resourceactions.New(Provider{})
	p, err := r.Prepare(context.Background(), c, ref, restartID, "pods")
	if err != nil {
		t.Fatal(err)
	}
	result, err := r.Execute(context.Background(), c, p)
	if err != nil || result[0].Status != "pending" {
		t.Fatalf("%+v %v", result, err)
	}
	for _, a := range c.Actions() {
		if a.GetVerb() == "patch" {
			t.Fatal("unexpected restart")
		}
	}
}
