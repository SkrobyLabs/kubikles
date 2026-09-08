// Package resourceactions provides operator actions independently of their UI.
package resourceactions

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type ResourceRef struct {
	Context   string `json:"context"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
}

func (r ResourceRef) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: r.Group, Version: r.Version, Resource: r.Resource}
}

type Mode struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	SelectTargets bool   `json:"selectTargets"`
}
type Action struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Modes       []Mode `json:"modes"`
}
type Target struct {
	ResourceRef
	Kind    string `json:"kind"`
	Pending bool   `json:"pending"`
}
type Plan struct {
	RequestID   string      `json:"requestId"`
	Fingerprint string      `json:"fingerprint,omitempty"`
	Source      ResourceRef `json:"source"`
	ActionID    string      `json:"actionId"`
	Mode        string      `json:"mode"`
	Summary     string      `json:"summary"`
	Targets     []Target    `json:"targets"`
	// Only trusted providers supply mutations. Never accept patches from the UI.
	Annotations map[string]string `json:"-"`
}
type Result struct {
	Message string `json:"message,omitempty"`
	Target  Target `json:"target"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}
type Provider interface {
	Actions(group, resource string) []Action
	Prepare(context.Context, dynamic.Interface, ResourceRef, string, string) (Plan, error)
}

// Mutation is produced from a freshly fetched object by trusted provider code.
// Status changes and normal patches share the executor's UID/version guards.
type Mutation struct {
	Create      *Creation
	Subresource string
	Operations  []map[string]interface{}
	Pending     bool
}
type Mutator interface {
	Mutation(Plan, Target, *unstructured.Unstructured) (Mutation, error)
}

// Creation is a provider-owned request object, never supplied by the frontend.
type Creation struct {
	Resource schema.GroupVersionResource
	Object   *unstructured.Unstructured
}
