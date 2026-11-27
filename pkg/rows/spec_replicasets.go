package rows

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(replicaSetsSpec()) }

func replicaSetObj(r *Row) *appsv1.ReplicaSet {
	o, _ := r.E.Obj.(*appsv1.ReplicaSet)
	return o
}

func replicaSetInt(f func(*appsv1.ReplicaSet) int32) func(r *Row) int {
	return func(r *Row) int {
		if o := replicaSetObj(r); o != nil {
			return int(f(o))
		}
		return 0
	}
}

func replicaSetPodSpec(r *Row) *corev1.PodSpec {
	if o := replicaSetObj(r); o != nil {
		return &o.Spec.Template.Spec
	}
	return nil
}

func replicaSetSelector(r *Row) *metav1.LabelSelector {
	if o := replicaSetObj(r); o != nil {
		return o.Spec.Selector
	}
	return nil
}

func replicaSetsSpec() *Spec {
	desired := replicaSetInt(func(o *appsv1.ReplicaSet) int32 {
		if o.Spec.Replicas != nil {
			return *o.Spec.Replicas
		}
		return 0
	})
	current := replicaSetInt(func(o *appsv1.ReplicaSet) int32 { return o.Status.Replicas })
	ready := replicaSetInt(func(o *appsv1.ReplicaSet) int32 { return o.Status.ReadyReplicas })
	available := replicaSetInt(func(o *appsv1.ReplicaSet) int32 { return o.Status.AvailableReplicas })
	labeled := replicaSetInt(func(o *appsv1.ReplicaSet) int32 { return o.Status.FullyLabeledReplicas })

	count := func(name string, aliases []string, f func(r *Row) int) SearchField {
		return SearchField{Name: name, Aliases: aliases, Extract: func(r *Row) string { return strconv.Itoa(f(r)) }}
	}

	cols := []Column{
		nameColumn(),
		namespaceColumn(),
		workloadIntColumn("desired", desired),
		workloadIntColumn("current", current),
		workloadIntColumn("ready", ready),
		ageColumn(creationAge),
	}
	cols = append(cols, controlledByColumns()...)

	search := append(commonSearchFields(), workloadSearchFields(replicaSetPodSpec, replicaSetSelector)...)
	search = append(search,
		count("replicas", []string{"desired"}, desired),
		count("current", []string{"currentreplicas"}, current),
		count("ready", []string{"readyreplicas"}, ready),
		count("available", []string{"availablereplicas"}, available),
		count("fulllabeled", []string{"labeled"}, labeled),
	)
	return &Spec{
		Type:       "replicasets",
		Namespaced: true,
		Needs:      store.ModeFull, // replicas, owner references and the pod template are not metadata
		Columns:    cols,
		Search:     search,
		Notes:      "The controllerRef column exists only in view mode (hidden by default): the Controlled By cell is the controller kind (its sort and filter value) while the link needs the whole reference.",
	}
}
