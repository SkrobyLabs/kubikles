// Package certmanager implements the same renewal trigger as cmctl renew.
package certmanager

import (
	"context"
	"fmt"
	"time"

	"kubikles/pkg/resourceactions"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const renewID = "cert-manager.certificate.renew"

type Provider struct{}

func (Provider) Actions(group, resource string) []resourceactions.Action {
	if group != "cert-manager.io" || resource != "certificates" {
		return nil
	}
	return []resourceactions.Action{{ID: renewID, Label: "Renew certificate…", Description: "Request certificate reissuance using cert-manager's manual renewal workflow. Issuance remains subject to issuer policy and rate limits.", Modes: []resourceactions.Mode{{ID: "certificate", Label: "This certificate"}}}}
}
func (Provider) Prepare(ctx context.Context, client dynamic.Interface, ref resourceactions.ResourceRef, id, mode string) (resourceactions.Plan, error) {
	plan := resourceactions.Plan{Source: ref, ActionID: id, Mode: mode, Targets: []resourceactions.Target{}, Summary: "cert-manager will manage the CertificateRequest and any ACME Order and Challenge resources. Each certificate renewal request is an atomic status update."}
	if ref.Version != "v1" {
		return plan, fmt.Errorf("manual renewal requires the cert-manager v1 API")
	}
	obj, err := resourceactions.ReadSource(ctx, client, ref)
	if err != nil {
		return plan, err
	}
	t := resourceactions.ObjectTarget(ref.Context, "certificates", obj)
	t.Pending = issuing(obj)
	plan.Targets = append(plan.Targets, t)
	return plan, nil
}
func issuing(obj *unstructured.Unstructured) bool {
	conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, item := range conditions {
		if c, ok := item.(map[string]interface{}); ok && c["type"] == "Issuing" && c["status"] == "True" {
			return true
		}
	}
	return false
}

// Source: https://github.com/cert-manager/cmctl/blob/main/pkg/renew/renew.go
// A single guarded status patch preserves Ready and other conditions, and lets
// cert-manager coordinate its dependent resources instead of editing timestamps.
func (Provider) Mutation(_ resourceactions.Plan, _ resourceactions.Target, obj *unstructured.Unstructured) (resourceactions.Mutation, error) {
	if issuing(obj) {
		return resourceactions.Mutation{Pending: true}, nil
	}
	conditions, _, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return resourceactions.Mutation{}, err
	}
	next := map[string]interface{}{"type": "Issuing", "status": "True", "reason": "ManuallyTriggered", "message": "Certificate re-issuance manually triggered", "lastTransitionTime": metav1.NewTime(time.Now().UTC()).Format(time.RFC3339), "observedGeneration": obj.GetGeneration()}
	found := false
	for i, item := range conditions {
		if c, ok := item.(map[string]interface{}); ok && c["type"] == "Issuing" {
			conditions[i] = next
			found = true
			break
		}
	}
	if !found {
		conditions = append(conditions, next)
	}
	return resourceactions.Mutation{Subresource: "status", Operations: resourceactions.SetField(obj, conditions, "status", "conditions")}, nil
}
