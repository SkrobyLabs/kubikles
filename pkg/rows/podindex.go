package rows

import (
	"sort"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"

	"kubikles/pkg/store"
)

// The derived-column engine. Workload lists show a value computed from the pods they select
// (the Pods column of deployments and stateful sets). The legacy lists pair a workload with a pod
// by namespace and equality of every spec.selector.matchLabels entry with the pod's labels
// (getDeploymentPods; matchExpressions are ignored and a selector without matchLabels selects
// nothing), not by owner references, so that is the relation the index implements: an inverted
// index from (namespace, label key, label value) to pods. A pod change touches only the
// workloads whose selector matches the pod before or after the change.

// PodRef is one pod a workload selects, as the Pods column shows it.
type PodRef struct {
	Name   string
	UID    string
	Status string
}

type labelKV struct{ k, v string }

// podRec is what the index keeps of a pod: only the fields a derived value can read.
type podRec struct {
	id, ns, name, uid, status string
	labels                    map[string]string
}

func (a *podRec) same(b *podRec) bool {
	if a.name != b.name || a.uid != b.uid || a.status != b.status || len(a.labels) != len(b.labels) {
		return false
	}
	for k, v := range a.labels {
		if bv, ok := b.labels[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

type podNS struct {
	pods    map[string]*podRec
	byLabel map[labelKV]map[string]*podRec
}

// podDelta is one pod whose derived-relevant state changed; old or new is nil for an addition or
// a removal.
type podDelta struct {
	ns       string
	old, new *podRec
}

// podIndex is the pod side of the derived engine, owned by one view. It is safe for concurrent
// use: a batch of changes is applied under the write lock and bumps gen, so a reader that saw the
// same gen before and after its reads saw one consistent state.
type podIndex struct {
	mu   sync.RWMutex
	gen  uint64
	byID map[string]*podRec
	ns   map[string]*podNS
}

func newPodIndex() *podIndex {
	return &podIndex{byID: map[string]*podRec{}, ns: map[string]*podNS{}}
}

// Gen returns the number of effective changes applied so far.
func (x *podIndex) Gen() uint64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.gen
}

// Len returns the number of pods held.
func (x *podIndex) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.byID)
}

func recOf(e *store.Entry) *podRec {
	rec := &podRec{id: e.ID(), ns: e.Namespace, name: e.Name, uid: e.UID}
	if m, err := meta.Accessor(e.Obj); err == nil && m != nil {
		rec.ns, rec.name, rec.uid = m.GetNamespace(), m.GetName(), string(m.GetUID())
		rec.labels = m.GetLabels()
	}
	p, _ := e.Obj.(*corev1.Pod)
	rec.status = podEffectiveStatus(p)
	return rec
}

func (x *podIndex) putLocked(rec *podRec) {
	n := x.ns[rec.ns]
	if n == nil {
		n = &podNS{pods: map[string]*podRec{}, byLabel: map[labelKV]map[string]*podRec{}}
		x.ns[rec.ns] = n
	}
	n.pods[rec.id] = rec
	for k, v := range rec.labels {
		kv := labelKV{k, v}
		b := n.byLabel[kv]
		if b == nil {
			b = map[string]*podRec{}
			n.byLabel[kv] = b
		}
		b[rec.id] = rec
	}
	x.byID[rec.id] = rec
}

func (x *podIndex) dropLocked(rec *podRec) {
	n := x.ns[rec.ns]
	if n != nil {
		delete(n.pods, rec.id)
		for k, v := range rec.labels {
			kv := labelKV{k, v}
			if b := n.byLabel[kv]; b != nil {
				delete(b, rec.id)
				if len(b) == 0 {
					delete(n.byLabel, kv)
				}
			}
		}
		if len(n.pods) == 0 {
			delete(x.ns, rec.ns)
		}
	}
	delete(x.byID, rec.id)
}

// reset replaces the content with the given entries.
func (x *podIndex) reset(entries []*store.Entry) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.byID = make(map[string]*podRec, len(entries))
	x.ns = map[string]*podNS{}
	for _, e := range entries {
		x.putLocked(recOf(e))
	}
	x.gen++
}

// apply folds a batch of store changes in and returns the pods whose relevant state changed (a
// resource-version-only update is not one).
func (x *podIndex) apply(batch []store.Change) []podDelta {
	x.mu.Lock()
	defer x.mu.Unlock()
	var out []podDelta
	for _, c := range batch {
		switch c.Type {
		case store.ChangeAdded, store.ChangeModified:
			if c.New == nil {
				continue
			}
			rec := recOf(c.New)
			old := x.byID[rec.id]
			if old != nil && old.same(rec) {
				continue
			}
			if old != nil {
				x.dropLocked(old)
			}
			x.putLocked(rec)
			if old != nil && old.ns != rec.ns {
				// Cannot happen for one object (uids are unique), but a replaced id must not
				// leave the old namespace's workloads stale.
				out = append(out, podDelta{ns: old.ns, old: old}, podDelta{ns: rec.ns, new: rec})
			} else {
				out = append(out, podDelta{ns: rec.ns, old: old, new: rec})
			}
		case store.ChangeDeleted:
			if c.Old == nil {
				continue
			}
			if old := x.byID[c.Old.ID()]; old != nil {
				x.dropLocked(old)
				out = append(out, podDelta{ns: old.ns, old: old})
			}
		}
	}
	if len(out) > 0 {
		x.gen++
	}
	return out
}

// selects reports whether every selector entry is present in labels with an equal value.
func selects(sel, labels map[string]string) bool {
	for k, v := range sel {
		if lv, ok := labels[k]; !ok || lv != v {
			return false
		}
	}
	return true
}

// match returns the pods of a namespace selected by matchLabels, ordered by name then uid. An
// empty selector selects nothing (the legacy list treats a missing matchLabels that way).
func (x *podIndex) match(ns string, sel map[string]string) []PodRef {
	if len(sel) == 0 {
		return nil
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	n := x.ns[ns]
	if n == nil {
		return nil
	}
	var smallest map[string]*podRec
	for k, v := range sel {
		b := n.byLabel[labelKV{k, v}]
		if len(b) == 0 {
			return nil
		}
		if smallest == nil || len(b) < len(smallest) {
			smallest = b
		}
	}
	var out []PodRef
	for _, rec := range smallest {
		if selects(sel, rec.labels) {
			out = append(out, PodRef{Name: rec.name, UID: rec.uid, Status: rec.status})
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

// podEffectiveStatus is getEffectivePodStatus (utils/k8s-helpers.ts): the worst status among the
// container states, Terminating for a deleted pod, else the phase.
func podEffectiveStatus(p *corev1.Pod) string {
	if p == nil {
		return "Unknown"
	}
	if p.DeletionTimestamp != nil {
		return "Terminating"
	}
	check := p.Status.ContainerStatuses
	if len(check) > 1 {
		var nonSucceeded []corev1.ContainerStatus
		for _, s := range check {
			if !(s.State.Terminated != nil && s.State.Terminated.ExitCode == 0) {
				nonSucceeded = append(nonSucceeded, s)
			}
		}
		if len(nonSucceeded) > 0 {
			check = nonSucceeded
		}
	}
	severity := func(s string) int {
		switch s {
		case "Failed":
			return 100
		case "Terminating":
			return 90
		case "ErrImagePull":
			return 80
		case "CrashLoopBackOff":
			return 70
		case "ImagePullBackOff":
			return 60
		case "ContainerCreating":
			return 50
		case "Pending":
			return 40
		case "Running":
			return 30
		case "Succeeded":
			return 20
		}
		return 0
	}
	worst, worstPriority := "", -1
	for i := range check {
		st := &check[i].State
		cur := ""
		switch {
		case st.Waiting != nil:
			cur = st.Waiting.Reason
		case st.Terminated != nil:
			if st.Terminated.ExitCode == 0 {
				cur = "Succeeded"
			} else {
				cur = "Failed"
			}
		case st.Running != nil:
			cur = "Running"
		}
		if cur == "" {
			continue
		}
		if pr := severity(cur); pr > worstPriority {
			worstPriority, worst = pr, cur
		}
	}
	if worst != "" {
		return worst
	}
	return orDefault(string(p.Status.Phase), "Unknown")
}
