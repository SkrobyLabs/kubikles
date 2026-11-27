package rows

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"kubikles/pkg/store"
)

// bruteRefs recomputes the pods a workload selects from the raw pod objects, without the index:
// every pod of the namespace whose labels hold every matchLabels entry (none without matchLabels).
func bruteRefs(ns string, sel map[string]string, pods map[string]*corev1.Pod) []PodRef {
	if len(sel) == 0 {
		return nil
	}
	var out []PodRef
	for _, p := range pods {
		if p.Namespace != ns {
			continue
		}
		ok := true
		for k, v := range sel {
			if got, has := p.Labels[k]; !has || got != v {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, PodRef{Name: p.Name, UID: string(p.UID), Status: podEffectiveStatus(p)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].UID < out[j].UID
	})
	return out
}

type propWorld struct {
	r     *rand.Rand
	pods  map[string]*corev1.Pod
	deps  map[string]*appsv1.Deployment
	nsAll []string
	seq   int
}

var propLabelVocab = []map[string]string{
	{"app": "a"}, {"app": "b"}, {"app": "a", "tier": "x"}, {"tier": "x"}, {"app": "a", "tier": "y"}, {},
}

func (w *propWorld) labels() map[string]string {
	l := map[string]string{}
	for k, v := range propLabelVocab[w.r.Intn(len(propLabelVocab))] {
		l[k] = v
	}
	if w.r.Intn(3) == 0 {
		l["extra"] = fmt.Sprint(w.r.Intn(3))
	}
	return l
}

func (w *propWorld) newPod(id string) *corev1.Pod {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-" + id, Namespace: w.nsAll[w.r.Intn(len(w.nsAll))], UID: types.UID("uid-" + id), Labels: w.labels()}}
	w.reshape(p)
	return p
}

// reshape randomizes the status fields the effective status reads.
func (w *propWorld) reshape(p *corev1.Pod) {
	p.Status = corev1.PodStatus{Phase: []corev1.PodPhase{"", corev1.PodRunning, corev1.PodPending}[w.r.Intn(3)]}
	states := []corev1.ContainerState{
		{Running: &corev1.ContainerStateRunning{}},
		{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
		{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}},
		{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}},
		{},
	}
	for i, n := 0, w.r.Intn(3); i < n; i++ {
		p.Status.ContainerStatuses = append(p.Status.ContainerStatuses, corev1.ContainerStatus{State: states[w.r.Intn(len(states))]})
	}
	p.DeletionTimestamp = nil
	if w.r.Intn(10) == 0 {
		now := metav1.NewTime(time.Unix(1700000000, 0))
		p.DeletionTimestamp = &now
	}
}

func (w *propWorld) newDeployment(id string) *appsv1.Deployment {
	sel := propLabelVocab[w.r.Intn(len(propLabelVocab))]
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "dep-" + id, Namespace: w.nsAll[w.r.Intn(len(w.nsAll))], UID: types.UID("duid-" + id),
		CreationTimestamp: metav1.NewTime(time.Unix(1700000000+int64(w.r.Intn(50)), 0))}}
	if len(sel) > 0 {
		d.Spec.Selector = &metav1.LabelSelector{MatchLabels: sel}
	}
	return d
}

func (w *propWorld) next() string { w.seq++; return fmt.Sprint(w.seq) }

// A random history of pod and workload changes applied incrementally keeps every workload's
// derived cells equal to a brute-force recomputation from the raw pods, and the incrementally
// updated view equal to one rebuilt from scratch.
func TestDerivedIncrementalEqualsBruteForce(t *testing.T) {
	spec, _ := Lookup("deployments")
	for seed := int64(1); seed <= 40; seed++ {
		w := &propWorld{r: rand.New(rand.NewSource(seed)), pods: map[string]*corev1.Pod{}, deps: map[string]*appsv1.Deployment{}, nsAll: []string{"n1", "n2", "n3"}}
		sortKey := []string{"name", "pods", "age"}[int(seed)%3]
		dir := []string{"asc", "desc"}[int(seed/3)%2]
		cfg := testCfg(t, spec, ViewSpec{Namespaces: []string{"*"}, Sort: SortSpec{Key: sortKey, Dir: dir}})
		now := time.Unix(1700001000, 0)
		for i := 0; i < 25; i++ {
			id := w.next()
			w.pods[id] = w.newPod(id)
		}
		for i := 0; i < 8; i++ {
			id := w.next()
			w.deps[id] = w.newDeployment(id)
		}
		idx := newPodIndex()
		podEntriesOf := func() []*store.Entry {
			out := make([]*store.Entry, 0, len(w.pods))
			for _, p := range w.pods {
				out = append(out, entryOf(p, p))
			}
			return out
		}
		depEntriesOf := func() []*store.Entry {
			out := make([]*store.Entry, 0, len(w.deps))
			for _, d := range w.deps {
				out = append(out, entryOf(d, d))
			}
			return out
		}
		idx.reset(podEntriesOf())
		inc := buildCoreDv(cfg, depEntriesOf(), nil, now, 1, nil, idx)
		ver := uint64(1)
		bump := func() uint64 { ver++; return ver }

		for step := 0; step < 60; step++ {
			var podChanges []store.Change
			eff := newEffect()
			for k, n := 0, 1+w.r.Intn(6); k < n; k++ {
				switch w.r.Intn(8) {
				case 0, 1: // add a pod
					id := w.next()
					p := w.newPod(id)
					w.pods[id] = p
					podChanges = append(podChanges, store.Change{Type: store.ChangeAdded, New: entryOf(p, p)})
				case 2, 3: // relabel a pod
					for id, p := range w.pods {
						np := p.DeepCopy()
						np.Labels = w.labels()
						w.pods[id] = np
						podChanges = append(podChanges, store.Change{Type: store.ChangeModified, Old: entryOf(p, p), New: entryOf(np, np)})
						break
					}
				case 4, 5: // change a pod's status
					for id, p := range w.pods {
						np := p.DeepCopy()
						w.reshape(np)
						w.pods[id] = np
						podChanges = append(podChanges, store.Change{Type: store.ChangeModified, Old: entryOf(p, p), New: entryOf(np, np)})
						break
					}
				case 6: // delete a pod
					for id, p := range w.pods {
						delete(w.pods, id)
						podChanges = append(podChanges, store.Change{Type: store.ChangeDeleted, Old: entryOf(p, p)})
						break
					}
				case 7: // a workload changes: new, modified selector, or deleted
					switch w.r.Intn(3) {
					case 0:
						id := w.next()
						d := w.newDeployment(id)
						w.deps[id] = d
						inc.upsert(entryOf(d, d), nil, now, bump, &eff)
					case 1:
						for id, d := range w.deps {
							nd := d.DeepCopy()
							if sel := propLabelVocab[w.r.Intn(len(propLabelVocab))]; len(sel) > 0 {
								nd.Spec.Selector = &metav1.LabelSelector{MatchLabels: sel}
							} else {
								nd.Spec.Selector = nil
							}
							w.deps[id] = nd
							inc.upsert(entryOf(nd, nd), nil, now, bump, &eff)
							break
						}
					default:
						for id, d := range w.deps {
							delete(w.deps, id)
							inc.remove(entryOf(d, d), &eff)
							break
						}
					}
				}
			}
			if deltas := idx.apply(podChanges); len(deltas) > 0 {
				inc.applyPods(deltas, false, nil, now, bump, &eff)
			}

			// 1. Every row's derived cells equal the brute-force recomputation.
			for _, d := range w.deps {
				vr := inc.rows[string(d.UID)]
				if vr == nil {
					t.Fatalf("seed %d step %d: missing row %s", seed, step, d.UID)
				}
				var sel map[string]string
				if d.Spec.Selector != nil {
					sel = d.Spec.Selector.MatchLabels
				}
				want := bruteRefs(d.Namespace, sel, w.pods)
				pi, ri := colIndex(inc.cfg, "pods"), colIndex(inc.cfg, "podRefs")
				if got := vr.cells[pi]; got != numberCell(len(want)) {
					t.Fatalf("seed %d step %d: %s pods = %v, want %d", seed, step, d.Name, got, len(want))
				}
				if got, wantRefs := vr.cells[ri], podRefsCell(want); got != wantRefs {
					t.Fatalf("seed %d step %d: %s podRefs = %v, want %v", seed, step, d.Name, got, wantRefs)
				}
			}
			// 2. The incrementally updated view equals one rebuilt from scratch.
			rebuilt := newPodIndex()
			rebuilt.reset(podEntriesOf())
			full := buildCoreDv(cfg, depEntriesOf(), nil, now, 1, nil, rebuilt)
			if d := sameCores(inc, full); d != "" {
				t.Fatalf("seed %d step %d (sort %s %s): %s", seed, step, sortKey, dir, d)
			}
		}
	}
}

func colIndex(cfg *compiled, key string) int {
	for i, k := range cfg.colKeys {
		if k == key {
			return i
		}
	}
	return -1
}

// A pod update that changes neither labels nor status (a resource-version bump) is not a delta,
// so a busy cluster does not re-derive workloads for nothing.
func TestPodIndexIgnoresIrrelevantUpdates(t *testing.T) {
	x := newPodIndex()
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "n", UID: "u", Labels: map[string]string{"app": "x"}, ResourceVersion: "1"}}
	if d := x.apply([]store.Change{{Type: store.ChangeAdded, New: entryOf(p, p)}}); len(d) != 1 {
		t.Fatalf("add deltas = %d", len(d))
	}
	gen := x.Gen()
	q := p.DeepCopy()
	q.ResourceVersion = "2"
	if d := x.apply([]store.Change{{Type: store.ChangeModified, Old: entryOf(p, p), New: entryOf(q, q)}}); len(d) != 0 {
		t.Errorf("rv-only update produced %d deltas", len(d))
	}
	if x.Gen() != gen {
		t.Error("generation moved on a no-op update")
	}
	if got := x.match("n", map[string]string{"app": "x"}); len(got) != 1 || got[0].Name != "a" {
		t.Errorf("match = %v", got)
	}
	if got := x.match("n", nil); got != nil {
		t.Errorf("empty selector must select nothing, got %v", got)
	}
}
