package k8s

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func resourceValue(value string) int64 {
	quantity := resource.MustParse(value)
	return quantity.Value()
}

func TestEffectivePodRequestsIncludesPodLevelResources(t *testing.T) {
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:      "main",
					Resources: v1.ResourceRequirements{},
				},
			},
			Resources: &v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("2"),
					v1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			Overhead: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse("50m"),
				v1.ResourceMemory: resource.MustParse("10Mi"),
			},
		},
	}

	cpu, memory := effectivePodRequests(pod)

	if cpu != 2050 {
		t.Fatalf("expected CPU request 2050m, got %dm", cpu)
	}
	if memory != resourceValue("1034Mi") {
		t.Fatalf("expected memory request 1034Mi, got %d bytes", memory)
	}
}

func TestEffectivePodRequestsUsesMaxOfPodContainersAndInitContainers(t *testing.T) {
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "main",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("400m"),
							v1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				},
				{
					Name: "sidecar",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("300m"),
							v1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				},
			},
			InitContainers: []v1.Container{
				{
					Name: "init",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("900m"),
							v1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
			},
			Resources: &v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("500m"),
					v1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
		},
	}

	cpu, memory := effectivePodRequests(pod)

	if cpu != 900 {
		t.Fatalf("expected CPU request 900m, got %dm", cpu)
	}
	if memory != resourceValue("512Mi") {
		t.Fatalf("expected memory request 512Mi, got %d bytes", memory)
	}
}
