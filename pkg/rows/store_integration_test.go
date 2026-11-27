package rows

import (
	"encoding/json"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"kubikles/pkg/store"
)

// The cluster-scoped types served end to end: the golden fixtures are loaded into a fake
// clientset, the real store lists them, and a real view answers every recorded search and the
// recorded name order. The debug cluster cannot hold these types, so this is the live-data check.
func TestClusterScopedViewsOverRealStore(t *testing.T) {
	checkViewsOverRealStore(t, []string{
		"validatingwebhookconfigurations", "mutatingwebhookconfigurations", "csidrivers", "csinodes",
		"persistentvolumes", "clusterroles", "clusterrolebindings",
	})
}

// The namespaced configuration and policy types, served the same way.
func TestConfigPolicyViewsOverRealStore(t *testing.T) {
	checkViewsOverRealStore(t, []string{
		"hpas", "pdbs", "resourcequotas", "limitranges", "leases", "networkpolicies", "serviceaccounts",
	})
}

// The namespaced network and RBAC types, served the same way.
func TestNetworkRbacViewsOverRealStore(t *testing.T) {
	checkViewsOverRealStore(t, []string{
		"roles", "rolebindings", "endpoints", "endpointslices", "services", "ingresses",
	})
}

// The workload types without pod-derived columns and the persistent volume claims.
func TestWorkloadStorageViewsOverRealStore(t *testing.T) {
	checkViewsOverRealStore(t, []string{"replicasets", "jobs", "daemonsets", "persistentvolumeclaims"})
}

// Pods and nodes, served the same way (their metrics columns have no data here).
func TestPodNodeViewsOverRealStore(t *testing.T) {
	checkViewsOverRealStore(t, []string{"pods", "nodes"})
}

// The metrics overlays end to end: the real store feeds the rows, the hub feeds the metrics, and
// every recorded state of the golden file gives the recorded cpu order, availability and cells.
func TestPodNodeMetricsOverRealStore(t *testing.T) {
	// Namespaces are left out: their fixtures repeat names, which a fake clientset cannot hold
	// (the golden tests cover them).
	for _, typ := range []string{"pods", "nodes"} {
		t.Run(typ, func(t *testing.T) {
			g := loadGolden(t, typ)
			skip := map[string]bool{}
			for _, uid := range g.NoSort {
				skip[uid] = true
			}
			// Rows without a creation timestamp have no consistent place in a sort that falls back
			// to age (see the sort goldens), so they are not loaded.
			var objs []runtime.Object
			for _, e := range variantsFor(t, g)[0].entries {
				if !skip[e.UID] {
					objs = append(objs, e.Obj)
				}
			}
			cs := fake.NewSimpleClientset(objs...)
			sm := store.New(fakeClients{cs}, store.Options{})
			t.Cleanup(sm.Close)
			hub := NewOverlayHub(nil)
			m := NewManager(Config{Provider: StoreProvider{M: sm}, Context: func() string { return "c" }, NoJanitor: true, Overlays: hub})
			t.Cleanup(m.Close)
			res, err := m.Open(ViewSpec{ViewID: "v", ClientToken: "t", Type: typ, Namespaces: []string{"*"}, Sort: SortSpec{Key: "cpu", Dir: "desc"}})
			if err != nil {
				t.Fatal(err)
			}
			waitFor(t, typ+" loaded", func() bool { st, _ := m.State("v"); return st.Phase == PhaseComplete && st.Total == len(objs) })
			cpuIdx, metricsIdx := -1, -1
			for i, k := range res.Columns {
				switch k {
				case "cpu":
					cpuIdx = i
				case "metrics":
					metricsIdx = i
				}
			}
			name := overlayName(typ)
			for _, st := range g.Overlay.States {
				hub.Publish("c", name, st.Available, goldenRecords(t, typ, st.Raw))
				var got []string
				waitFor(t, typ+"/"+st.Name+" order", func() bool {
					refs, _, err := m.Refs("v", 0, 0, 0)
					if err != nil {
						return false
					}
					got = got[:0]
					for _, r := range refs {
						if !skip[r.UID] {
							got = append(got, r.UID)
						}
					}
					return reflect.DeepEqual(got, st.SortOrders["cpu"]["desc"])
				})
				w, err := m.Window("v", 0, 1000, 0)
				if err != nil {
					t.Fatal(err)
				}
				for uid, row := range w.Rows {
					if want := st.Cells[uid]["cpu"]; jsonOf(row.C[cpuIdx]) != jsonOf(want) {
						t.Errorf("%s/%s %s cpu cell %v, legacy %v", typ, st.Name, uid, row.C[cpuIdx], want)
					}
					if want := st.Cells[uid]["metrics"]; jsonOf(row.C[metricsIdx]) != jsonOf(want) {
						t.Errorf("%s/%s %s metrics cell %v, legacy %v", typ, st.Name, uid, row.C[metricsIdx], want)
					}
				}
				info, ok := w.State.Overlays[name]
				switch {
				case !ok:
					t.Fatalf("%s/%s: no overlay state", typ, st.Name)
				case st.Available == nil && info.Available != nil, st.Available != nil && (info.Available == nil || *info.Available != *st.Available):
					t.Errorf("%s/%s availability %v, want %v", typ, st.Name, info.Available, st.Available)
				}
			}
		})
	}
}

// orderKeys names the column whose recorded order a type is checked by when it is not the name.
// The legacy job list has no sort value for its name column, so a name sort is a tie everywhere and
// leaves the rows without a creation timestamp (which no cluster produces) in an order that depends
// on arrival; the age order is a real order.
var orderKeys = map[string]string{"jobs": "age"}

func checkViewsOverRealStore(t *testing.T, types []string) {
	for _, typ := range types {
		orderKey := "name"
		if k, ok := orderKeys[typ]; ok {
			orderKey = k
		}
		t.Run(typ, func(t *testing.T) {
			g := loadGolden(t, typ)
			var objs []runtime.Object
			for _, e := range variantsFor(t, g)[0].entries {
				objs = append(objs, e.Obj)
			}
			cs := fake.NewSimpleClientset(objs...)
			sm := store.New(fakeClients{cs}, store.Options{})
			t.Cleanup(sm.Close)
			m := NewManager(Config{Provider: StoreProvider{M: sm}, Context: func() string { return "c" }, NoJanitor: true})
			t.Cleanup(m.Close)

			res, err := m.Open(ViewSpec{ViewID: "v", ClientToken: "t", Type: typ, Namespaces: []string{"*"}, Sort: SortSpec{Key: orderKey, Dir: "asc"}})
			if err != nil {
				t.Fatal(err)
			}
			want := len(g.Fixtures)
			waitFor(t, typ+" loaded", func() bool { st, _ := m.State("v"); return st.Phase == PhaseComplete && st.Total == want })
			if !reflect.DeepEqual(res.Columns, g.Columns) {
				t.Fatalf("projected columns %v, legacy columns %v", res.Columns, g.Columns)
			}

			skip := map[string]bool{}
			for _, uid := range g.NoSort {
				skip[uid] = true
			}
			refs, _, err := m.Refs("v", 0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			var gotOrder []string
			for _, r := range refs {
				if !skip[r.UID] {
					gotOrder = append(gotOrder, r.UID)
				}
			}
			if !reflect.DeepEqual(gotOrder, g.SortOrders[orderKey]["asc"]) {
				t.Errorf("%s order\n got  %v\n want %v", orderKey, gotOrder, g.SortOrders[orderKey]["asc"])
			}

			for _, c := range g.SearchCases {
				q := c.Query
				st, err := m.Update("v", ViewPatch{Search: &q})
				if err != nil {
					t.Fatalf("search %q: %v", q, err)
				}
				if st.Filtered != len(c.Matches) {
					t.Errorf("search %q: %d rows, legacy %d", q, st.Filtered, len(c.Matches))
				}
			}

			// Namespaced types: a namespace selection scopes the view like the legacy request did.
			if spec, _ := Lookup(typ); spec != nil && spec.Namespaced {
				empty := ""
				if _, err := m.Update("v", ViewPatch{Search: &empty}); err != nil {
					t.Fatal(err)
				}
				perNS := map[string]int{}
				for _, raw := range g.Fixtures {
					var meta struct {
						Metadata struct {
							Namespace string `json:"namespace"`
						} `json:"metadata"`
					}
					must(t, json.Unmarshal(raw, &meta))
					perNS[meta.Metadata.Namespace]++
				}
				for _, ns := range []string{"prod", "team-a"} {
					sel := []string{ns}
					if _, err := m.Update("v", ViewPatch{Namespaces: &sel}); err != nil {
						t.Fatal(err)
					}
					waitFor(t, typ+" scoped to "+ns, func() bool {
						st, _ := m.State("v")
						return st.Total == perNS[ns] && st.Filtered == perNS[ns]
					})
				}
			}
		})
	}
}

// Deployments and stateful sets served end to end: the golden workloads and the pods of
// a recorded pod list go into a fake clientset, the real store lists both, and a real view gives
// the recorded derived cells and Pods order; a second recorded list is then applied pod by pod
// through the store watch and must give that list's cells.
func TestDeploymentStatefulSetViewsOverRealStore(t *testing.T) {
	checkViewsOverRealStore(t, []string{"deployments", "statefulsets"})
	for _, typ := range []string{"deployments", "statefulsets"} {
		t.Run(typ+"/derived", func(t *testing.T) {
			g := loadGolden(t, typ)
			skip := map[string]bool{}
			for _, uid := range g.NoSort {
				skip[uid] = true
			}
			var objs []runtime.Object
			for _, e := range variantsFor(t, g)[0].entries {
				if !skip[e.UID] {
					objs = append(objs, e.Obj)
				}
			}
			first, second := g.Derived.States[1], g.Derived.States[2]
			for _, e := range podEntries(t, first.Pods) {
				objs = append(objs, e.Obj)
			}
			cs := fake.NewSimpleClientset(objs...)
			sm := store.New(fakeClients{cs}, store.Options{})
			t.Cleanup(sm.Close)
			m := NewManager(Config{Provider: StoreProvider{M: sm}, Context: func() string { return "c" }, NoJanitor: true})
			t.Cleanup(m.Close)
			res, err := m.Open(ViewSpec{ViewID: "v", ClientToken: "t", Type: typ, Namespaces: []string{"*"}, Sort: SortSpec{Key: "pods", Dir: "desc"}})
			if err != nil {
				t.Fatal(err)
			}
			check := func(label string, st interface{}, cells map[string]map[string]interface{}, order []string) {
				t.Helper()
				waitFor(t, typ+" "+label, func() bool {
					w, err := m.Window("v", 0, 1000, 0)
					if err != nil || w.State.Phase != PhaseComplete {
						return false
					}
					for uid, row := range w.Rows {
						for i, k := range res.Columns {
							if k == "pods" || k == "podRefs" {
								if jsonOf(row.C[i]) != jsonOf(cells[uid][k]) {
									return false
								}
							}
						}
					}
					var got []string
					refs, _, _ := m.Refs("v", 0, 0, 0)
					for _, r := range refs {
						if !skip[r.UID] {
							got = append(got, r.UID)
						}
					}
					return reflect.DeepEqual(got, order)
				})
			}
			waitFor(t, typ+" loaded", func() bool {
				st, _ := m.State("v")
				return st.Phase == PhaseComplete && st.Total == len(objs)-len(first.Pods)
			})
			check("first list", nil, first.Cells, first.SortOrders["pods"]["desc"])

			// Move the cluster to the second list through the clientset: the store watch feeds the
			// pod index and the workloads re-derive.
			want := map[string]bool{}
			for _, e := range podEntries(t, second.Pods) {
				want[e.ID()] = true
			}
			ctx := t.Context()
			for _, e := range podEntries(t, first.Pods) {
				if !want[e.ID()] {
					p := e.Obj.(*corev1.Pod)
					must(t, cs.CoreV1().Pods(p.Namespace).Delete(ctx, p.Name, metav1.DeleteOptions{}))
				}
			}
			for _, e := range podEntries(t, second.Pods) {
				p := e.Obj.(*corev1.Pod)
				if _, err := cs.CoreV1().Pods(p.Namespace).Create(ctx, p, metav1.CreateOptions{}); err != nil {
					if _, err := cs.CoreV1().Pods(p.Namespace).Update(ctx, p, metav1.UpdateOptions{}); err != nil {
						t.Fatal(err)
					}
				}
			}
			check("second list", nil, second.Cells, second.SortOrders["pods"]["desc"])
		})
	}
}

// A pod view with notifyOn over the real store: restarts, deletions and replacements made in the
// (fake) cluster arrive through the store's watch and surface as transitions in the view's notices.
func TestPodViewTransitionsOverRealStore(t *testing.T) {
	mk := func(name string, restarts int32, owner string) *corev1.Pod {
		p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: name, UID: types.UID("uid-" + name)}}
		p.OwnerReferences = []metav1.OwnerReference{{Kind: "ReplicaSet", Name: owner, UID: "o"}}
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "app", RestartCount: restarts}}
		return p
	}
	cs := fake.NewSimpleClientset(mk("web-1", 0, "web"), mk("db-1", 0, "db"))
	sm := store.New(fakeClients{cs}, store.Options{})
	t.Cleanup(sm.Close)
	m := NewManager(Config{Provider: StoreProvider{M: sm}, Context: func() string { return "c" }, NoJanitor: true})
	t.Cleanup(m.Close)
	if _, err := m.Open(ViewSpec{ViewID: "v", ClientToken: "t", Type: "pods", Namespaces: []string{"*"}, NotifyOn: []string{NotifyPods}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "loaded", func() bool { st, _ := m.State("v"); return st.Phase == PhaseComplete && st.Total == 2 })
	ctx := t.Context()
	var got []Transition
	collect := func(want int) {
		t.Helper()
		waitFor(t, "transitions", func() bool {
			for _, n := range m.TakeNotices() {
				got = append(got, n.Transitions...)
			}
			return len(got) >= want
		})
	}

	_, err := cs.CoreV1().Pods("ns").Update(ctx, mk("web-1", 3, "web"), metav1.UpdateOptions{})
	must(t, err)
	collect(1)
	if got[0].Kind != TransitionRestart || got[0].Name != "web-1" || got[0].Total != 3 || len(got[0].Containers) != 1 || got[0].Containers[0].Count != 3 {
		t.Fatalf("restart transition = %+v", got[0])
	}

	must(t, cs.CoreV1().Pods("ns").Delete(ctx, "web-1", metav1.DeleteOptions{}))
	_, err = cs.CoreV1().Pods("ns").Create(ctx, mk("web-2", 0, "web"), metav1.CreateOptions{})
	must(t, err)
	collect(3)
	kinds := map[string]Transition{}
	for _, tr := range got[1:] {
		kinds[tr.Kind] = tr
	}
	if kinds[TransitionDeleted].Name != "web-1" || kinds[TransitionDeleted].Owner != "ReplicaSet/web" || kinds[TransitionAdded].Name != "web-2" || kinds[TransitionAdded].Owner != "ReplicaSet/web" {
		t.Fatalf("delete/add transitions = %+v", got[1:])
	}
}
