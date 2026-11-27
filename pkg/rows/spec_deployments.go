package rows

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(deploymentsSpec()) }

func deploymentObj(r *Row) *appsv1.Deployment {
	o, _ := r.E.Obj.(*appsv1.Deployment)
	return o
}

func deploymentInt(f func(*appsv1.Deployment) int32) func(r *Row) int {
	return func(r *Row) int {
		if o := deploymentObj(r); o != nil {
			return int(f(o))
		}
		return 0
	}
}

func deploymentPodSpec(r *Row) *corev1.PodSpec {
	if o := deploymentObj(r); o != nil {
		return &o.Spec.Template.Spec
	}
	return nil
}

func deploymentSelector(r *Row) *metav1.LabelSelector {
	if o := deploymentObj(r); o != nil {
		return o.Spec.Selector
	}
	return nil
}

func deploymentContainers(r *Row) []corev1.Container {
	if ps := deploymentPodSpec(r); ps != nil {
		return ps.Containers
	}
	return nil
}

// deploymentStrategy is spec.strategy.type || 'RollingUpdate'.
func deploymentStrategy(r *Row) string {
	if o := deploymentObj(r); o != nil {
		return orDefault(string(o.Spec.Strategy.Type), "RollingUpdate")
	}
	return "RollingUpdate"
}

func deploymentsSpec() *Spec {
	// replicas is spec.replicas ?? 1 (the column), specReplicas spec.replicas || 0 (the search field).
	replicas := deploymentInt(func(o *appsv1.Deployment) int32 {
		if o.Spec.Replicas != nil {
			return *o.Spec.Replicas
		}
		return 1
	})
	specReplicas := deploymentInt(func(o *appsv1.Deployment) int32 {
		if o.Spec.Replicas != nil {
			return *o.Spec.Replicas
		}
		return 0
	})
	ready := deploymentInt(func(o *appsv1.Deployment) int32 { return o.Status.ReadyReplicas })
	updated := deploymentInt(func(o *appsv1.Deployment) int32 { return o.Status.UpdatedReplicas })
	available := deploymentInt(func(o *appsv1.Deployment) int32 { return o.Status.AvailableReplicas })

	str := func(key string, f func(r *Row) string) Column {
		return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return f(r) }}
	}
	cols := []Column{nameColumn(), namespaceColumn()}
	cols = append(cols, podsColumn())
	cols = append(cols,
		workloadIntColumn("ready", ready),
		ageColumn(creationAge),
		str("strategy", deploymentStrategy),
		workloadIntColumn("replicas", replicas),
		workloadIntColumn("updatedReplicas", updated),
		workloadIntColumn("availableReplicas", available),
		str("image", func(r *Row) string { return firstImage(deploymentContainers(r)) }),
		str("selector", func(r *Row) string { return labelPairs(selectorMatchLabels(deploymentSelector(r)), ",") }),
		// Display-only: the Ready cell shows ready/total, the Image cell a count and every image.
		Column{Key: "readyText", Kind: KindString, CellType: "string", FilterType: FilterNone, Cell: func(r *Row) interface{} {
			if o := deploymentObj(r); o != nil {
				return readyText(o.Status.ReadyReplicas, o.Status.Replicas)
			}
			return readyText(0, 0)
		}},
		Column{Key: "imageList", Kind: KindString, CellType: "string", FilterType: FilterNone,
			Cell: func(r *Row) interface{} { return imageLines(deploymentContainers(r)) }},
		podRefsColumn(),
	)

	count := func(name string, aliases []string, f func(r *Row) int) SearchField {
		return SearchField{Name: name, Aliases: aliases, Extract: func(r *Row) string { return strconv.Itoa(f(r)) }}
	}
	search := append(commonSearchFields(), workloadSearchFields(deploymentPodSpec, deploymentSelector)...)
	search = append(search,
		count("replicas", []string{"desired"}, specReplicas),
		count("ready", []string{"readyreplicas"}, ready),
		count("available", []string{"availablereplicas"}, available),
		count("updated", []string{"updatedreplicas"}, updated),
		SearchField{Name: "strategy", Aliases: []string{"updatestrategy"}, Extract: func(r *Row) string {
			if o := deploymentObj(r); o != nil {
				return string(o.Spec.Strategy.Type)
			}
			return ""
		}},
		SearchField{Name: "paused", Extract: func(r *Row) string {
			if o := deploymentObj(r); o != nil && o.Spec.Paused {
				return "true"
			}
			return "false"
		}},
	)
	return &Spec{
		Type:       "deployments",
		Namespaced: true,
		Needs:      store.ModeFull, // status counts, selectors and the pod template are not metadata
		Columns:    cols,
		Search:     search,
		Derived:    []Derived{podsDerived(deploymentSelector)},
		Notes:      "The Pods column is derived from the pods the selector matches (Spec.Derived). The podRefs, readyText and imageList columns exist only in view mode (hidden by default): the Pods cell draws one square per pod, the Ready cell shows ready/total and the Image cell counts every image, while the sort values are the pod count, the ready count and the first image. A workload view leases the pods of its namespaces in full mode.",
	}
}
