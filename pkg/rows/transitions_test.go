package rows

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"kubikles/pkg/store"
)

func restartPod(ns, name, owner string, restarts ...int32) *store.Entry {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID("u-" + ns + "-" + name), ResourceVersion: "1"}}
	if owner != "" {
		p.OwnerReferences = []metav1.OwnerReference{{Kind: "ReplicaSet", Name: owner, UID: "o"}}
	}
	for i, r := range restarts {
		p.Status.ContainerStatuses = append(p.Status.ContainerStatuses, corev1.ContainerStatus{Name: string(rune('a' + i)), RestartCount: r})
	}
	p.Status.Phase = corev1.PodRunning
	return entryOf(p, p)
}

func transitions(env *testEnv) []Transition {
	var out []Transition
	for _, n := range env.m.TakeNotices() {
		out = append(out, n.Transitions...)
	}
	return out
}

// A pod view with notifyOn reports what the pod notification bell needs: restarts and
// disappearances of pods visible in the filtered list, and every new pod.
func TestPodViewReportsTransitions(t *testing.T) {
	env := newEnv(t)
	pods := env.p.shard("pods")
	pods.upsert(restartPod("a", "keep-1", "rs-1", 0, 1))
	pods.upsert(restartPod("a", "keep-2", "rs-1", 2))
	pods.upsert(restartPod("a", "other", "rs-2", 0))
	// The search hides "other": it is in scope but not in the filtered list.
	if _, err := env.m.Open(ViewSpec{ViewID: "pv", ClientToken: "t", Type: "pods", Namespaces: []string{"*"}, Search: "name:keep", NotifyOn: []string{NotifyPods}}); err != nil {
		t.Fatal(err)
	}
	if got := transitions(env); got != nil {
		t.Fatalf("opening the view reported %v", got)
	}

	// Restart of a visible pod: the container that restarted and the pod's total.
	pods.upsert(restartPod("a", "keep-1", "rs-1", 0, 2))
	got := transitions(env)
	want := []Transition{{Kind: TransitionRestart, UID: "u-a-keep-1", Namespace: "a", Name: "keep-1", Owner: "ReplicaSet/rs-1", Total: 2, Containers: []ContainerRestart{{Name: "b", Count: 2}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("restart transitions = %+v, want %+v", got, want)
	}

	// A restart of a pod the filter hides is not reported; no change at all is silent.
	pods.upsert(restartPod("a", "other", "rs-2", 3))
	pods.upsert(restartPod("a", "keep-2", "rs-1", 2))
	if got := transitions(env); got != nil {
		t.Fatalf("hidden restart or no-op reported %+v", got)
	}

	// Several restarts between two notices are one event measured from the first state.
	pods.upsert(restartPod("a", "keep-2", "rs-1", 3))
	pods.upsert(restartPod("a", "keep-2", "rs-1", 5))
	got = transitions(env)
	want = []Transition{{Kind: TransitionRestart, UID: "u-a-keep-2", Namespace: "a", Name: "keep-2", Owner: "ReplicaSet/rs-1", Total: 5, Containers: []ContainerRestart{{Name: "a", Count: 5}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged restart = %+v, want %+v", got, want)
	}

	// A visible pod is deleted and its replacement appears: a deletion and an addition, in that
	// order of arrival; a hidden pod's deletion is silent.
	pods.remove(restartPod("a", "keep-1", "", 0).ID())
	pods.remove(restartPod("a", "other", "", 0).ID())
	pods.upsert(restartPod("a", "keep-3", "rs-1", 0))
	got = transitions(env)
	want = []Transition{
		{Kind: TransitionDeleted, UID: "u-a-keep-1", Namespace: "a", Name: "keep-1", Owner: "ReplicaSet/rs-1"},
		{Kind: TransitionAdded, UID: "u-a-keep-3", Namespace: "a", Name: "keep-3", Owner: "ReplicaSet/rs-1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("delete and add = %+v, want %+v", got, want)
	}

	// A pod that appears and vanishes between two notices was never seen.
	pods.upsert(restartPod("a", "fleeting", "", 0))
	pods.remove(restartPod("a", "fleeting", "", 0).ID())
	if got := transitions(env); got != nil {
		t.Fatalf("fleeting pod reported %+v", got)
	}

	// A pod that restarts and is then deleted is reported as deleted only.
	pods.upsert(restartPod("a", "keep-3", "rs-1", 4))
	pods.remove(restartPod("a", "keep-3", "", 0).ID())
	got = transitions(env)
	if len(got) != 1 || got[0].Kind != TransitionDeleted || got[0].Name != "keep-3" {
		t.Fatalf("restart then delete = %+v", got)
	}

	// Re-planning the view (a namespace change) re-seeds it: nothing is reported for the difference.
	ns := []string{"a", "b"}
	pods.upsert(restartPod("b", "keep-b", "", 0))
	if got := transitions(env); len(got) != 1 || got[0].Kind != TransitionAdded {
		t.Fatalf("pod outside the namespaces = %+v", got)
	}
	pods.upsert(restartPod("a", "keep-4", "", 0))
	transitions(env)
	if _, err := env.m.Update("pv", ViewPatch{Namespaces: &ns}); err != nil {
		t.Fatal(err)
	}
	if got := transitions(env); got != nil {
		t.Fatalf("rebuild reported %+v", got)
	}
}

func TestNotifyOnIsValidated(t *testing.T) {
	env := newEnv(t)
	if _, err := env.m.Open(ViewSpec{ViewID: "x", ClientToken: "t", Type: "configmaps", Namespaces: []string{"*"}, NotifyOn: []string{NotifyPods}}); err == nil {
		t.Error("notifyOn pods on configmaps must fail")
	}
	if _, err := env.m.Open(ViewSpec{ViewID: "y", ClientToken: "t", Type: "pods", Namespaces: []string{"*"}, NotifyOn: []string{"nodes"}}); err == nil {
		t.Error("unknown notifyOn kind must fail")
	}
}

// Transitions of an inactive view stay pending and are bounded.
func TestTransitionLogIsBounded(t *testing.T) {
	var l transitionLog
	for i := 0; i < maxPendingTransitions+50; i++ {
		e := restartPod("a", "p"+string(rune('A'+i%26))+string(rune('a'+i/26)), "", 0)
		l.added(e.Obj.(*corev1.Pod))
	}
	if got := len(l.drain()); got != maxPendingTransitions {
		t.Errorf("drained %d transitions, want %d", got, maxPendingTransitions)
	}
	if l.drain() != nil {
		t.Error("drain must clear the log")
	}
}
