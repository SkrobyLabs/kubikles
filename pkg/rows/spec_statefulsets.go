package rows

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(statefulSetsSpec()) }

func statefulSetObj(r *Row) *appsv1.StatefulSet {
	o, _ := r.E.Obj.(*appsv1.StatefulSet)
	return o
}

func statefulSetInt(f func(*appsv1.StatefulSet) int32) func(r *Row) int {
	return func(r *Row) int {
		if o := statefulSetObj(r); o != nil {
			return int(f(o))
		}
		return 0
	}
}

func statefulSetPodSpec(r *Row) *corev1.PodSpec {
	if o := statefulSetObj(r); o != nil {
		return &o.Spec.Template.Spec
	}
	return nil
}

func statefulSetSelector(r *Row) *metav1.LabelSelector {
	if o := statefulSetObj(r); o != nil {
		return o.Spec.Selector
	}
	return nil
}

func statefulSetContainers(r *Row) []corev1.Container {
	if ps := statefulSetPodSpec(r); ps != nil {
		return ps.Containers
	}
	return nil
}

// statefulSetUpdateStrategy is spec.updateStrategy.type || 'RollingUpdate'.
func statefulSetUpdateStrategy(r *Row) string {
	if o := statefulSetObj(r); o != nil {
		return orDefault(string(o.Spec.UpdateStrategy.Type), "RollingUpdate")
	}
	return "RollingUpdate"
}

func statefulSetsSpec() *Spec {
	replicas := statefulSetInt(func(o *appsv1.StatefulSet) int32 {
		if o.Spec.Replicas != nil {
			return *o.Spec.Replicas
		}
		return 1
	})
	specReplicas := statefulSetInt(func(o *appsv1.StatefulSet) int32 {
		if o.Spec.Replicas != nil {
			return *o.Spec.Replicas
		}
		return 0
	})
	ready := statefulSetInt(func(o *appsv1.StatefulSet) int32 { return o.Status.ReadyReplicas })
	current := statefulSetInt(func(o *appsv1.StatefulSet) int32 { return o.Status.CurrentReplicas })
	updated := statefulSetInt(func(o *appsv1.StatefulSet) int32 { return o.Status.UpdatedReplicas })
	claims := statefulSetInt(func(o *appsv1.StatefulSet) int32 { return int32(len(o.Spec.VolumeClaimTemplates)) })
	serviceName := func(r *Row) string {
		if o := statefulSetObj(r); o != nil {
			return o.Spec.ServiceName
		}
		return ""
	}

	str := func(key string, f func(r *Row) string) Column {
		return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return f(r) }}
	}
	cols := []Column{nameColumn(), namespaceColumn()}
	cols = append(cols, podsColumn())
	cols = append(cols,
		workloadIntColumn("ready", ready),
		ageColumn(creationAge),
		workloadIntColumn("replicas", replicas),
		str("serviceName", serviceName),
		str("updateStrategy", statefulSetUpdateStrategy),
		str("image", func(r *Row) string { return firstImage(statefulSetContainers(r)) }),
		workloadIntColumn("volumeClaims", claims),
		Column{Key: "readyText", Kind: KindString, CellType: "string", FilterType: FilterNone, Cell: func(r *Row) interface{} {
			if o := statefulSetObj(r); o != nil {
				return readyText(o.Status.ReadyReplicas, o.Status.Replicas)
			}
			return readyText(0, 0)
		}},
		Column{Key: "imageList", Kind: KindString, CellType: "string", FilterType: FilterNone,
			Cell: func(r *Row) interface{} { return imageLines(statefulSetContainers(r)) }},
		podRefsColumn(),
	)

	count := func(name string, aliases []string, f func(r *Row) int) SearchField {
		return SearchField{Name: name, Aliases: aliases, Extract: func(r *Row) string { return strconv.Itoa(f(r)) }}
	}
	search := append(commonSearchFields(), workloadSearchFields(statefulSetPodSpec, statefulSetSelector)...)
	search = append(search,
		count("replicas", []string{"desired"}, specReplicas),
		count("ready", []string{"readyreplicas"}, ready),
		count("current", []string{"currentreplicas"}, current),
		count("updated", []string{"updatedreplicas"}, updated),
		SearchField{Name: "servicename", Aliases: []string{"service", "headless"}, Extract: serviceName},
		SearchField{Name: "podmanagementpolicy", Aliases: []string{"podpolicy"}, Extract: func(r *Row) string {
			if o := statefulSetObj(r); o != nil {
				return orDefault(string(o.Spec.PodManagementPolicy), "OrderedReady")
			}
			return "OrderedReady"
		}},
		SearchField{Name: "updatestrategy", Aliases: []string{"strategy"}, Extract: statefulSetUpdateStrategy},
	)
	return &Spec{
		Type:       "statefulsets",
		Namespaced: true,
		Needs:      store.ModeFull, // status counts, selectors, claim templates and the pod template are not metadata
		Columns:    cols,
		Search:     search,
		Derived:    []Derived{podsDerived(statefulSetSelector)},
		Notes:      "The Pods column is derived from the pods the selector matches (Spec.Derived). The podRefs, readyText and imageList columns exist only in view mode (hidden by default): the Pods cell draws one square per pod, the Ready cell shows ready/total and the Image cell counts every image, while the sort values are the pod count, the ready count and the first image. A workload view leases the pods of its namespaces in full mode.",
	}
}
