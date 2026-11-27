package k8s

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodContainerRunning(t *testing.T) {
	running := v1.ContainerState{Running: &v1.ContainerStateRunning{}}
	terminated := v1.ContainerState{Terminated: &v1.ContainerStateTerminated{}}
	now := metav1.NewTime(time.Now())

	pod := &v1.Pod{Status: v1.PodStatus{
		ContainerStatuses:     []v1.ContainerStatus{{Name: "app", State: running}, {Name: "done", State: terminated}},
		InitContainerStatuses: []v1.ContainerStatus{{Name: "init", State: terminated}},
	}}
	deleting := pod.DeepCopy()
	deleting.DeletionTimestamp = &now
	allDone := &v1.Pod{Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{{Name: "app", State: terminated}}}}
	sidecar := &v1.Pod{Status: v1.PodStatus{InitContainerStatuses: []v1.ContainerStatus{{Name: "side", State: running}}}}

	cases := []struct {
		name      string
		pod       *v1.Pod
		container string
		want      bool
	}{
		{"running container", pod, "app", true},
		{"terminated container", pod, "done", false},
		{"terminated init container", pod, "init", false},
		{"unknown container", pod, "missing", false},
		{"any container, one running", pod, "", true},
		{"any container, none running", allDone, "", false},
		{"running sidecar init container", sidecar, "side", true},
		{"pod being deleted", deleting, "app", false},
		{"nil pod", nil, "app", false},
	}
	for _, tc := range cases {
		if got := podContainerRunning(tc.pod, tc.container); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func logTestPod(state v1.ContainerState) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Status:     v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{{Name: "app", State: state}}},
	}
}

// The fake clientset ends every log stream cleanly after one line, like a
// proxy that closes long-lived responses.
func TestStreamPodLogsEndsOnCleanCloseWhenContainerTerminated(t *testing.T) {
	c := newTestClient(logTestPod(v1.ContainerState{Terminated: &v1.ContainerStateTerminated{}}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lines := 0
	if err := c.StreamPodLogs(ctx, "ns", "p", "app", true, 10, func(string) { lines++ }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lines != 1 {
		t.Fatalf("got %d lines, want 1", lines)
	}
}

func TestStreamPodLogsReopensCleanCloseWhileContainerRuns(t *testing.T) {
	c := newTestClient(logTestPod(v1.ContainerState{Running: &v1.ContainerStateRunning{}}))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	lines := 0
	err := c.StreamPodLogs(ctx, "ns", "p", "app", true, 10, func(string) { lines++ })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stream should run until cancelled, got %v", err)
	}
	if lines < 2 {
		t.Fatalf("got %d lines, want the stream reopened at least once", lines)
	}
}
