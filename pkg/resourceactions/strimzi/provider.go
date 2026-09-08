// Package strimzi contains Strimzi-specific matching, discovery and restart rules.
package strimzi

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"kubikles/pkg/resourceactions"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const restartID = "strimzi.kafka.rolling-restart"
const rollingAnnotation = "strimzi.io/manual-rolling-update"

var podSets = schema.GroupVersionResource{Group: "core.strimzi.io", Version: "v1beta2", Resource: "strimzipodsets"}
var pods = schema.GroupVersionResource{Version: "v1", Resource: "pods"}

type Provider struct{}

func (Provider) Actions(group, resource string) []resourceactions.Action {
	if group != "kafka.strimzi.io" || resource != "kafkas" {
		return nil
	}
	return []resourceactions.Action{{ID: restartID, Label: "Rolling restart…", Description: "Ask Strimzi to restart Kafka brokers and controllers during reconciliation. Availability depends on your Kafka replication and cluster health.", Modes: []resourceactions.Mode{
		{ID: "cluster", Label: "Whole Kafka cluster"},
		{ID: "pods", Label: "Selected Kafka pods", SelectTargets: true},
	}}}
}

func ownedBy(obj *unstructured.Unstructured, kind, group, name, uid string) bool {
	for _, owner := range obj.GetOwnerReferences() {
		if owner.Kind == kind && strings.Split(owner.APIVersion, "/")[0] == group && owner.Name == name && string(owner.UID) == uid {
			return true
		}
	}
	return false
}
func target(ref resourceactions.ResourceRef, obj *unstructured.Unstructured, gvr schema.GroupVersionResource, kind string) resourceactions.Target {
	return resourceactions.Target{ResourceRef: resourceactions.ResourceRef{Context: ref.Context, Group: gvr.Group, Version: gvr.Version, Resource: gvr.Resource, Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID())}, Kind: kind, Pending: obj.GetAnnotations()[rollingAnnotation] == "true"}
}

// Support pod-set based Strimzi installations. Legacy StatefulSet installations
// are intentionally unavailable until covered by a separate compatibility path.
// Protocol: https://strimzi.io/docs/operators/latest/deploying#assembly-rolling-updates-str
func (Provider) Prepare(ctx context.Context, client dynamic.Interface, ref resourceactions.ResourceRef, id, mode string) (resourceactions.Plan, error) {
	plan := resourceactions.Plan{Source: ref, ActionID: id, Mode: mode, Targets: []resourceactions.Target{}, Annotations: map[string]string{rollingAnnotation: "true"}}
	if ref.Namespace == "" {
		return plan, fmt.Errorf("Kafka namespace is required")
	}
	if ref.Version != "v1beta2" && ref.Version != "v1" {
		return plan, fmt.Errorf("Kafka API version %s is not supported for rolling restarts", ref.Version)
	}
	kafka, err := client.Resource(ref.GVR()).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return plan, err
	}
	if string(kafka.GetUID()) != ref.UID {
		return plan, fmt.Errorf("Kafka was replaced; reopen the action")
	}
	if kafka.GetDeletionTimestamp() != nil {
		return plan, fmt.Errorf("Kafka is being deleted")
	}
	if kafka.GetAnnotations()["strimzi.io/pause-reconciliation"] == "true" {
		return plan, fmt.Errorf("Strimzi reconciliation is paused for this Kafka cluster")
	}
	conditions, _, _ := unstructured.NestedSlice(kafka.Object, "status", "conditions")
	for _, c := range conditions {
		if condition, ok := c.(map[string]interface{}); ok && condition["type"] == "ReconciliationPaused" && condition["status"] == "True" {
			return plan, fmt.Errorf("Strimzi reconciliation is paused for this Kafka cluster")
		}
	}
	selector := labels.Set{"strimzi.io/cluster": ref.Name, "strimzi.io/kind": "Kafka", "strimzi.io/name": ref.Name + "-kafka"}.AsSelector().String()
	sets, err := client.Resource(podSets).Namespace(ref.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return plan, fmt.Errorf("cannot discover Kafka pod sets (requires StrimziPodSet support and list permission): %w", err)
	}
	verified := map[string]*unstructured.Unstructured{}
	for i := range sets.Items {
		set := &sets.Items[i]
		belongs := ownedBy(set, "Kafka", ref.Group, ref.Name, ref.UID)
		for _, owner := range set.GetOwnerReferences() {
			if belongs {
				break
			}
			gv, parseErr := schema.ParseGroupVersion(owner.APIVersion)
			if parseErr != nil || gv.Group != ref.Group || owner.Kind != "KafkaNodePool" || (gv.Version != "v1beta2" && gv.Version != "v1") {
				continue
			}
			pool, getErr := client.Resource(gv.WithResource("kafkanodepools")).Namespace(ref.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
			if getErr != nil {
				return plan, fmt.Errorf("cannot verify Kafka node pool %s: %w", owner.Name, getErr)
			}
			belongs = pool.GetUID() == owner.UID && pool.GetLabels()["strimzi.io/cluster"] == ref.Name && pool.GetDeletionTimestamp() == nil
		}
		if !belongs {
			continue
		}
		if set.GetDeletionTimestamp() != nil {
			return plan, fmt.Errorf("Kafka pod set %s is being deleted", set.GetName())
		}
		verified[string(set.GetUID())] = set
		if mode == "cluster" {
			plan.Targets = append(plan.Targets, target(ref, set, podSets, "StrimziPodSet"))
		}
	}
	if len(verified) == 0 {
		return plan, fmt.Errorf("no supported Kafka pod sets found; rolling restart requires pod sets owned by this Kafka cluster")
	}
	if mode == "pods" {
		list, err := client.Resource(pods).Namespace(ref.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return plan, fmt.Errorf("cannot discover Kafka pods: %w", err)
		}
		for i := range list.Items {
			pod := &list.Items[i]
			if pod.GetDeletionTimestamp() != nil {
				continue
			}
			for uid, set := range verified {
				if ownedBy(pod, "StrimziPodSet", podSets.Group, set.GetName(), uid) {
					podTarget := target(ref, pod, pods, "Pod")
					podTarget.Pending = podTarget.Pending || set.GetAnnotations()[rollingAnnotation] == "true"
					plan.Targets = append(plan.Targets, podTarget)
					break
				}
			}
		}
	}
	if len(plan.Targets) == 0 {
		return plan, fmt.Errorf("no supported Kafka pods found")
	}
	sort.Slice(plan.Targets, func(i, j int) bool { return plan.Targets[i].Name < plan.Targets[j].Name })
	plan.Summary = "Strimzi will perform the rolling restart during reconciliation. A successful request does not mean the restart has completed."
	return plan, nil
}
