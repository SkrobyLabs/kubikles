package store

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubikles/pkg/k8s"
)

// PodNodeSource implements k8s.PodNodeSource on top of the store. It answers
// from a warm cluster-wide shard and never starts one: when no shard is warm
// (nothing has the pods or nodes list open) it delegates to fallback, normally
// the API-backed default, so behaviour is unchanged until a view has the data.
type PodNodeSource struct {
	m        *Manager
	context  func() string
	fallback k8s.PodNodeSource
}

// NewPodNodeSource builds the source. contextName reports the active context at
// call time.
func NewPodNodeSource(m *Manager, contextName func() string, fallback k8s.PodNodeSource) *PodNodeSource {
	return &PodNodeSource{m: m, context: contextName, fallback: fallback}
}

func (s *PodNodeSource) ListPods(ctx context.Context) (*v1.PodList, error) {
	if items, rv, ok := Peek[v1.Pod](s.m, Key{Context: s.context(), Type: "pods"}); ok {
		return &v1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: rv}, Items: items}, nil
	}
	return s.fallback.ListPods(ctx)
}

func (s *PodNodeSource) ListNodes(ctx context.Context) (*v1.NodeList, error) {
	if items, rv, ok := Peek[v1.Node](s.m, Key{Context: s.context(), Type: "nodes"}); ok {
		return &v1.NodeList{ListMeta: metav1.ListMeta{ResourceVersion: rv}, Items: items}, nil
	}
	return s.fallback.ListNodes(ctx)
}
