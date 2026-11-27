package rows

import (
	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

// NotifyPods is the ViewSpec.NotifyOn value that reports pod transitions.
const NotifyPods = "pods"

// Transition kinds.
const (
	TransitionRestart = "restart"
	TransitionDeleted = "deleted"
	TransitionAdded   = "added"
)

// maxPendingTransitions bounds what a view holds between two notices (an inactive view keeps
// its notices pending); the oldest are dropped first.
const maxPendingTransitions = 256

// ContainerRestart is one container whose restart count rose.
type ContainerRestart struct {
	Name  string `json:"name"`
	Count int32  `json:"count"`
}

// Transition is one pod event the pod notification bell reports. It is what the legacy hook
// (usePodNotifications) finds by diffing two snapshots of the pod list, computed from the changes
// the view applies: a restart of a pod that is visible in the filtered list, the disappearance of
// a visible pod, and the appearance of any pod. A view that is rebuilt (namespace switch, store
// reset) reports nothing for the difference, like the hook, which re-seeds on an empty list.
type Transition struct {
	Kind      string `json:"kind"`
	UID       string `json:"uid"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// Owner is "{kind}/{name}" of the first owner reference, "" without one (getOwnerKey).
	Owner string `json:"owner,omitempty"`
	// Total and Containers describe a restart: the pod's restart count over its containers and
	// the containers whose count rose since the last report.
	Total      int                `json:"total,omitempty"`
	Containers []ContainerRestart `json:"containers,omitempty"`
}

// pendingTransition is a transition under construction: a restart keeps the counts of the
// first pod state since the last notice and the latest pod, so several restarts between two
// notices become one event.
type pendingTransition struct {
	kind      string
	uid       string
	ns, name  string
	owner     string
	oldTotal  int
	oldCounts map[string]int32
	cur       *corev1.Pod
}

// transitionLog collects transitions between two notices, merged per pod.
type transitionLog struct {
	order []string
	m     map[string]*pendingTransition
}

func podRestartTotal(p *corev1.Pod) int {
	n := 0
	for _, cs := range p.Status.ContainerStatuses {
		n += int(cs.RestartCount)
	}
	return n
}

func podOwnerKey(p *corev1.Pod) string {
	if len(p.OwnerReferences) == 0 {
		return ""
	}
	return p.OwnerReferences[0].Kind + "/" + p.OwnerReferences[0].Name
}

func podOf(e *store.Entry) *corev1.Pod {
	if e == nil {
		return nil
	}
	p, _ := e.Obj.(*corev1.Pod)
	return p
}

func (l *transitionLog) put(t *pendingTransition) {
	if l.m == nil {
		l.m = map[string]*pendingTransition{}
	}
	if _, ok := l.m[t.uid]; !ok {
		l.order = append(l.order, t.uid)
	}
	l.m[t.uid] = t
	for len(l.order) > maxPendingTransitions {
		delete(l.m, l.order[0])
		l.order = l.order[1:]
	}
}

func (l *transitionLog) drop(uid string) {
	if _, ok := l.m[uid]; !ok {
		return
	}
	delete(l.m, uid)
	for i, id := range l.order {
		if id == uid {
			l.order = append(l.order[:i], l.order[i+1:]...)
			break
		}
	}
}

// restart records that a pod visible in the filtered list gained restarts.
func (l *transitionLog) restart(old, cur *corev1.Pod) {
	uid := string(cur.UID)
	if t := l.m[uid]; t != nil {
		if t.kind == TransitionRestart {
			t.cur = cur
		}
		return // a pod that appeared since the last notice is just new
	}
	counts := make(map[string]int32, len(old.Status.ContainerStatuses))
	for _, cs := range old.Status.ContainerStatuses {
		counts[cs.Name] = cs.RestartCount
	}
	l.put(&pendingTransition{kind: TransitionRestart, uid: uid, ns: cur.Namespace, name: cur.Name, owner: podOwnerKey(cur),
		oldTotal: podRestartTotal(old), oldCounts: counts, cur: cur})
}

// added records a pod that entered the scope.
func (l *transitionLog) added(p *corev1.Pod) {
	uid := string(p.UID)
	if l.m[uid] != nil {
		return
	}
	l.put(&pendingTransition{kind: TransitionAdded, uid: uid, ns: p.Namespace, name: p.Name, owner: podOwnerKey(p), cur: p})
}

// deleted records that a pod left the scope. visible says whether the filtered list held it.
func (l *transitionLog) deleted(p *corev1.Pod, visible bool) {
	uid := string(p.UID)
	prev := l.m[uid]
	l.drop(uid)
	if prev != nil && prev.kind == TransitionAdded {
		return // appeared and vanished between two notices: never seen
	}
	if visible {
		l.put(&pendingTransition{kind: TransitionDeleted, uid: uid, ns: p.Namespace, name: p.Name, owner: podOwnerKey(p)})
	}
}

// drain returns the pending transitions in order and clears the log.
func (l *transitionLog) drain() []Transition {
	if len(l.order) == 0 {
		return nil
	}
	out := make([]Transition, 0, len(l.order))
	for _, uid := range l.order {
		t := l.m[uid]
		tr := Transition{Kind: t.kind, UID: t.uid, Namespace: t.ns, Name: t.name, Owner: t.owner}
		if t.kind == TransitionRestart {
			tr.Total = podRestartTotal(t.cur)
			if tr.Total <= t.oldTotal {
				continue
			}
			for _, cs := range t.cur.Status.ContainerStatuses {
				if cs.RestartCount > t.oldCounts[cs.Name] {
					tr.Containers = append(tr.Containers, ContainerRestart{Name: cs.Name, Count: cs.RestartCount})
				}
			}
		}
		out = append(out, tr)
	}
	l.order, l.m = nil, nil
	return out
}

// noteTransitionLocked records the transition a store change causes, before the change is
// applied to the core (it compares with the row the core holds).
func (v *View) noteTransitionLocked(c store.Change) {
	if !v.cfg.notifyPods {
		return
	}
	switch c.Type {
	case store.ChangeAdded, store.ChangeModified:
		cur := podOf(c.New)
		if cur == nil {
			return
		}
		old := v.core.rows[c.New.ID()]
		switch {
		case old == nil:
			v.trans.added(cur)
		case old.inIndex:
			if prev := podOf(old.e); prev != nil && podRestartTotal(cur) > podRestartTotal(prev) {
				v.trans.restart(prev, cur)
			}
		}
	case store.ChangeDeleted:
		gone := podOf(c.Old)
		if gone == nil {
			return
		}
		old := v.core.rows[c.Old.ID()]
		if old == nil {
			return
		}
		v.trans.deleted(gone, old.inIndex)
	}
	v.markDirtyLocked(false)
}
