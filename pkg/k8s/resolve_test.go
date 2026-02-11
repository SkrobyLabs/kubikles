package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// helper to build a pod with a controller ownerReference
func podWithOwner(podName, namespace, ownerKind, ownerName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       ownerKind,
					Name:       ownerName,
					Controller: boolPtr(true),
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
		},
	}
}

// ============================================================================
// resolveControllerChain Tests
// ============================================================================

func TestResolveControllerChain_ReplicaSetToDeployment(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app-abc123",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "my-app",
					Controller: boolPtr(true),
				},
			},
		},
	}
	cs := fake.NewSimpleClientset(rs)
	ctx := context.Background()

	kind, name := resolveControllerChain(cs, ctx, "default", "ReplicaSet", "my-app-abc123")
	if kind != "Deployment" {
		t.Errorf("kind = %q, want %q", kind, "Deployment")
	}
	if name != "my-app" {
		t.Errorf("name = %q, want %q", name, "my-app")
	}
}

func TestResolveControllerChain_ReplicaSetStandalone(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone-rs",
			Namespace: "default",
		},
	}
	cs := fake.NewSimpleClientset(rs)
	ctx := context.Background()

	kind, name := resolveControllerChain(cs, ctx, "default", "ReplicaSet", "standalone-rs")
	if kind != "ReplicaSet" {
		t.Errorf("kind = %q, want %q", kind, "ReplicaSet")
	}
	if name != "standalone-rs" {
		t.Errorf("name = %q, want %q", name, "standalone-rs")
	}
}

func TestResolveControllerChain_ReplicaSetNotFound(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	kind, name := resolveControllerChain(cs, ctx, "default", "ReplicaSet", "missing-rs")
	if kind != "ReplicaSet" {
		t.Errorf("kind = %q, want %q", kind, "ReplicaSet")
	}
	if name != "missing-rs" {
		t.Errorf("name = %q, want %q", name, "missing-rs")
	}
}

func TestResolveControllerChain_JobToCronJob(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cron-12345",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "batch/v1",
					Kind:       "CronJob",
					Name:       "my-cron",
					Controller: boolPtr(true),
				},
			},
		},
	}
	cs := fake.NewSimpleClientset(job)
	ctx := context.Background()

	kind, name := resolveControllerChain(cs, ctx, "default", "Job", "my-cron-12345")
	if kind != "CronJob" {
		t.Errorf("kind = %q, want %q", kind, "CronJob")
	}
	if name != "my-cron" {
		t.Errorf("name = %q, want %q", name, "my-cron")
	}
}

func TestResolveControllerChain_JobStandalone(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "one-off-job",
			Namespace: "default",
		},
	}
	cs := fake.NewSimpleClientset(job)
	ctx := context.Background()

	kind, name := resolveControllerChain(cs, ctx, "default", "Job", "one-off-job")
	if kind != "Job" {
		t.Errorf("kind = %q, want %q", kind, "Job")
	}
	if name != "one-off-job" {
		t.Errorf("name = %q, want %q", name, "one-off-job")
	}
}

func TestResolveControllerChain_JobNotFound(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	kind, name := resolveControllerChain(cs, ctx, "default", "Job", "missing-job")
	if kind != "Job" {
		t.Errorf("kind = %q, want %q", kind, "Job")
	}
	if name != "missing-job" {
		t.Errorf("name = %q, want %q", name, "missing-job")
	}
}

func TestResolveControllerChain_Passthrough(t *testing.T) {
	tests := []struct {
		kind string
		name string
	}{
		{"DaemonSet", "my-ds"},
		{"StatefulSet", "my-sts"},
		{"Deployment", "my-deploy"},
		{"CronJob", "my-cron"},
		{"Node", "my-node"},
		{"UnknownKind", "some-resource"},
	}

	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			kind, name := resolveControllerChain(cs, ctx, "default", tt.kind, tt.name)
			if kind != tt.kind {
				t.Errorf("kind = %q, want %q", kind, tt.kind)
			}
			if name != tt.name {
				t.Errorf("name = %q, want %q", name, tt.name)
			}
		})
	}
}

// ============================================================================
// ResolveTopLevelOwner (public API) Tests
// ============================================================================

func TestResolveTopLevelOwner_ReplicaSetToDeployment(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-abc123",
			Namespace: "production",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "nginx", Controller: boolPtr(true)},
			},
		},
	}
	client := newTestClient(rs)

	result, err := client.ResolveTopLevelOwner("test-context", "production", "ReplicaSet", "nginx-abc123")
	if err != nil {
		t.Fatalf("ResolveTopLevelOwner() error = %v", err)
	}
	if result.Kind != "Deployment" {
		t.Errorf("Kind = %q, want %q", result.Kind, "Deployment")
	}
	if result.Name != "nginx" {
		t.Errorf("Name = %q, want %q", result.Name, "nginx")
	}
}

func TestResolveTopLevelOwner_Passthrough(t *testing.T) {
	client := newTestClient()

	result, err := client.ResolveTopLevelOwner("test-context", "default", "StatefulSet", "my-sts")
	if err != nil {
		t.Fatalf("ResolveTopLevelOwner() error = %v", err)
	}
	if result.Kind != "StatefulSet" {
		t.Errorf("Kind = %q, want %q", result.Kind, "StatefulSet")
	}
	if result.Name != "my-sts" {
		t.Errorf("Name = %q, want %q", result.Name, "my-sts")
	}
}

// ============================================================================
// GetPodEvictionInfo chain resolution Tests
// ============================================================================

func TestGetPodEvictionInfo_ReplicaSetResolvesToDeployment(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy-abc123",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "my-deploy", Controller: boolPtr(true)},
			},
		},
	}
	pod := podWithOwner("test-pod", "default", "ReplicaSet", "my-deploy-abc123")
	client := newTestClient(pod, rs)

	info, err := client.GetPodEvictionInfo("test-context", "default", "test-pod")
	if err != nil {
		t.Fatalf("GetPodEvictionInfo() error = %v", err)
	}
	if info.OwnerKind != "Deployment" {
		t.Errorf("OwnerKind = %q, want %q", info.OwnerKind, "Deployment")
	}
	if info.OwnerName != "my-deploy" {
		t.Errorf("OwnerName = %q, want %q", info.OwnerName, "my-deploy")
	}
	if info.Category != "reschedulable" {
		t.Errorf("Category = %q, want %q", info.Category, "reschedulable")
	}
}

func TestGetPodEvictionInfo_JobResolvesToCronJob(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cron-12345",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "CronJob", Name: "my-cron", Controller: boolPtr(true)},
			},
		},
	}
	pod := podWithOwner("cron-pod", "default", "Job", "my-cron-12345")
	client := newTestClient(pod, job)

	info, err := client.GetPodEvictionInfo("test-context", "default", "cron-pod")
	if err != nil {
		t.Fatalf("GetPodEvictionInfo() error = %v", err)
	}
	if info.OwnerKind != "CronJob" {
		t.Errorf("OwnerKind = %q, want %q", info.OwnerKind, "CronJob")
	}
	if info.OwnerName != "my-cron" {
		t.Errorf("OwnerName = %q, want %q", info.OwnerName, "my-cron")
	}
	if info.Category != "killable" {
		t.Errorf("Category = %q, want %q", info.Category, "killable")
	}
}
