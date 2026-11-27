package rows

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(daemonSetsSpec()) }

func daemonSetObj(r *Row) *appsv1.DaemonSet {
	o, _ := r.E.Obj.(*appsv1.DaemonSet)
	return o
}

func daemonSetStatusInt(pick func(*appsv1.DaemonSetStatus) int32) func(r *Row) int {
	return func(r *Row) int {
		if o := daemonSetObj(r); o != nil {
			return int(pick(&o.Status))
		}
		return 0
	}
}

func daemonSetPodSpec(r *Row) *corev1.PodSpec {
	if o := daemonSetObj(r); o != nil {
		return &o.Spec.Template.Spec
	}
	return nil
}

func daemonSetContainers(r *Row) []corev1.Container {
	if ps := daemonSetPodSpec(r); ps != nil {
		return ps.Containers
	}
	return nil
}

func daemonSetSelector(r *Row) *metav1.LabelSelector {
	if o := daemonSetObj(r); o != nil {
		return o.Spec.Selector
	}
	return nil
}

// daemonSetUpdateStrategy is spec.updateStrategy.type || 'RollingUpdate'.
func daemonSetUpdateStrategy(r *Row) string {
	if o := daemonSetObj(r); o != nil {
		return orDefault(string(o.Spec.UpdateStrategy.Type), "RollingUpdate")
	}
	return "RollingUpdate"
}

func daemonSetsSpec() *Spec {
	desired := daemonSetStatusInt(func(s *appsv1.DaemonSetStatus) int32 { return s.DesiredNumberScheduled })
	current := daemonSetStatusInt(func(s *appsv1.DaemonSetStatus) int32 { return s.CurrentNumberScheduled })
	ready := daemonSetStatusInt(func(s *appsv1.DaemonSetStatus) int32 { return s.NumberReady })
	available := daemonSetStatusInt(func(s *appsv1.DaemonSetStatus) int32 { return s.NumberAvailable })
	updated := daemonSetStatusInt(func(s *appsv1.DaemonSetStatus) int32 { return s.UpdatedNumberScheduled })
	misscheduled := daemonSetStatusInt(func(s *appsv1.DaemonSetStatus) int32 { return s.NumberMisscheduled })

	str := func(key string, f func(r *Row) string) Column {
		return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return f(r) }}
	}
	cols := []Column{
		nameColumn(),
		namespaceColumn(),
		workloadIntColumn("desired", desired),
		workloadIntColumn("current", current),
		workloadIntColumn("ready", ready),
		workloadIntColumn("available", available),
		ageColumn(creationAge),
		workloadIntColumn("upToDate", updated),
		str("updateStrategy", daemonSetUpdateStrategy),
		str("image", func(r *Row) string { return firstImage(daemonSetContainers(r)) }),
		str("selector", func(r *Row) string { return labelPairs(selectorMatchLabels(daemonSetSelector(r)), ",") }),
		str("nodeSelector", func(r *Row) string {
			if ps := daemonSetPodSpec(r); ps != nil {
				return labelPairs(ps.NodeSelector, ",")
			}
			return ""
		}),
		// Display-only: the Image cell shows a count and every image in its tooltip.
		{Key: "imageList", Kind: KindString, CellType: "string", FilterType: FilterNone,
			Cell: func(r *Row) interface{} { return imageLines(daemonSetContainers(r)) }},
	}

	count := func(name string, aliases []string, f func(r *Row) int) SearchField {
		return SearchField{Name: name, Aliases: aliases, Extract: func(r *Row) string { return strconv.Itoa(f(r)) }}
	}
	search := append(commonSearchFields(), workloadSearchFields(daemonSetPodSpec, daemonSetSelector)...)
	search = append(search,
		count("desired", []string{"desirednumber"}, desired),
		count("current", []string{"currentnumber"}, current),
		count("ready", []string{"readynumber"}, ready),
		count("available", []string{"availablenumber"}, available),
		count("updated", []string{"updatednumber"}, updated),
		count("misscheduled", nil, misscheduled),
		SearchField{Name: "updatestrategy", Aliases: []string{"strategy"}, Extract: daemonSetUpdateStrategy},
	)
	return &Spec{
		Type:       "daemonsets",
		Namespaced: true,
		Needs:      store.ModeFull, // status counts, selectors and the pod template are not metadata
		Columns:    cols,
		Search:     search,
		Notes:      "The imageList column exists only in view mode (hidden by default): the Image cell is the first image (its sort and filter value) while the legacy display counts every image and lists them in its tooltip.",
	}
}
