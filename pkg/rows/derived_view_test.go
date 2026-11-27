package rows

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"kubikles/pkg/store"
)

func depEntry(ns, name string, sel map[string]string) *store.Entry {
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID("d-" + ns + "-" + name), ResourceVersion: "1",
		CreationTimestamp: metav1.NewTime(time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC))}}
	d.Spec.Selector = &metav1.LabelSelector{MatchLabels: sel}
	return entryOf(d, d)
}

func labelledPod(ns, name string, labels map[string]string, running bool) *store.Entry {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID("p-" + ns + "-" + name), ResourceVersion: "1", Labels: labels}}
	p.Status.Phase = corev1.PodPending
	if running {
		p.Status.Phase = corev1.PodRunning
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}
	}
	return entryOf(p, p)
}

func windowCell(t *testing.T, w ViewWindow, cols []string, uid, col string) interface{} {
	t.Helper()
	row, ok := w.Rows[uid]
	if !ok {
		t.Fatalf("row %s not in window", uid)
	}
	for i, k := range cols {
		if k == col {
			return row.C[i]
		}
	}
	t.Fatalf("no column %s", col)
	return nil
}

// A deployments view leases the pods of its namespaces, shows the pods each workload selects and
// follows pod changes without the workload changing.
func TestWorkloadViewFollowsPods(t *testing.T) {
	env := newEnv(t)
	deps := env.p.shard("deployments")
	pods := env.p.shard("pods")
	deps.upsert(depEntry("a", "web", map[string]string{"app": "web"}))
	deps.upsert(depEntry("a", "db", map[string]string{"app": "db"}))
	deps.upsert(depEntry("b", "web", map[string]string{"app": "web"}))
	pods.upsert(labelledPod("a", "web-1", map[string]string{"app": "web"}, true))
	pods.upsert(labelledPod("a", "web-2", map[string]string{"app": "web"}, false))
	pods.upsert(labelledPod("b", "web-9", map[string]string{"app": "web"}, true))

	res, err := env.m.Open(ViewSpec{ViewID: "dv", ClientToken: "t", Type: "deployments", Namespaces: []string{"*"}, Sort: SortSpec{Key: "pods", Dir: "desc"}})
	if err != nil {
		t.Fatal(err)
	}
	if av := res.State.Overlays["pods"].Available; av == nil || !*av {
		t.Fatalf("pods availability = %v, want true once the shard is synced", av)
	}
	w, err := env.m.Window("dv", 0, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if w.Order[0] != "d-a-web" {
		t.Fatalf("order = %v, want the deployment with most pods first", w.Order)
	}
	if got := windowCell(t, w, res.Columns, "d-a-web", "pods"); got != float64(2) {
		t.Errorf("a/web pods = %v", got)
	}
	if got := windowCell(t, w, res.Columns, "d-a-web", "podRefs"); got != `[["web-1","p-a-web-1","Running"],["web-2","p-a-web-2","Pending"]]` {
		t.Errorf("a/web podRefs = %v", got)
	}
	if got := windowCell(t, w, res.Columns, "d-a-db", "pods"); got != float64(0) {
		t.Errorf("a/db pods = %v", got)
	}

	// A pod appears for db and one of web's pods starts running: no workload object changes.
	pods.upsert(labelledPod("a", "db-1", map[string]string{"app": "db"}, true))
	pods.upsert(labelledPod("a", "web-2", map[string]string{"app": "web"}, true))
	w2, err := env.m.Window("dv", 0, 10, w.Version)
	if err != nil {
		t.Fatal(err)
	}
	if got := windowCell(t, w2, res.Columns, "d-a-db", "pods"); got != float64(1) {
		t.Errorf("a/db pods after add = %v", got)
	}
	if got := windowCell(t, w2, res.Columns, "d-a-web", "podRefs"); got != `[["web-1","p-a-web-1","Running"],["web-2","p-a-web-2","Running"]]` {
		t.Errorf("a/web podRefs after status change = %v", got)
	}
	if _, touched := w2.Rows["d-b-web"]; touched {
		t.Error("the workload of another namespace was re-sent")
	}

	// A pod leaves, and a relabelled pod moves from one workload to the other.
	pods.remove(labelledPod("a", "web-1", nil, true).ID())
	pods.upsert(labelledPod("a", "db-1", map[string]string{"app": "web"}, true))
	w3, _ := env.m.Window("dv", 0, 10, w2.Version)
	if got := windowCell(t, w3, res.Columns, "d-a-web", "pods"); got != float64(2) {
		t.Errorf("a/web pods after remove and relabel = %v", got)
	}
	if got := windowCell(t, w3, res.Columns, "d-a-db", "pods"); got != float64(0) {
		t.Errorf("a/db pods after relabel = %v", got)
	}

	// Narrowing the namespaces re-plans the pod leases; the cells keep matching.
	only := []string{"b"}
	if _, err := env.m.Update("dv", ViewPatch{Namespaces: &only}); err != nil {
		t.Fatal(err)
	}
	w4, _ := env.m.Window("dv", 0, 10, 0)
	if len(w4.Order) != 1 || windowCell(t, w4, res.Columns, "d-b-web", "pods") != float64(1) {
		t.Errorf("after narrowing: order %v", w4.Order)
	}

	before := env.p.activeLeases()
	env.m.CloseView("dv")
	if got := env.p.activeLeases(); got >= before || got != 0 {
		t.Errorf("leases after close = %d (was %d), want every lease released", got, before)
	}
}

// Until the pod shard has listed, the view says the pods are not available, so the list shows
// placeholders instead of "no pods".
func TestWorkloadViewReportsPodsLoading(t *testing.T) {
	env := newEnv(t)
	env.p.shard("pods").setState(func(s *store.State) { s.Synced = false; s.Phase = store.PhaseLoading })
	env.p.shard("deployments").upsert(depEntry("a", "web", map[string]string{"app": "web"}))
	res, err := env.m.Open(ViewSpec{ViewID: "dv", ClientToken: "t", Type: "deployments", Namespaces: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	if av := res.State.Overlays["pods"].Available; av == nil || *av {
		t.Fatalf("pods availability = %v, want false while loading", av)
	}
	env.p.shard("pods").setState(func(s *store.State) { s.Synced = true; s.Phase = store.PhaseWatching })
	st, _ := env.m.State("dv")
	if av := st.Overlays["pods"].Available; av == nil || !*av {
		t.Fatalf("pods availability after sync = %v", av)
	}
}
