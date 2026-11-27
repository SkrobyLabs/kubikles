package rows

import (
	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(namespacesSpec()) }

func namespaceObj(r *Row) *corev1.Namespace {
	ns, _ := r.E.Obj.(*corev1.Namespace)
	return ns
}

// namespaceStatus is getNamespaceStatus: Active and Terminating as is, any other phase verbatim
// and "Unknown" when there is none.
func namespaceStatus(r *Row) string {
	ns := namespaceObj(r)
	if ns == nil || ns.Status.Phase == "" {
		return "Unknown"
	}
	return string(ns.Status.Phase)
}

func namespaceMetricColumn(key string, field int) Column {
	return Column{Key: key, Kind: KindNumber, CellType: "number", FilterType: FilterSelect, Volatile: true,
		Cell: func(r *Row) interface{} { return overlayCell(r, OverlayNamespaceMetrics, r.Name(), field) }}
}

func namespacesSpec() *Spec {
	return &Spec{
		Type:       "namespaces",
		Namespaced: false,
		Needs:      store.ModeFull, // status.phase and spec.finalizers are searchable
		Columns: []Column{
			nameColumn(),
			// The legacy cpu and memory columns are sortable (committed percentage of the cluster
			// capacity, -1 without a sample) and have the default select filter, which never has
			// values because their value is a number.
			namespaceMetricColumn("cpu", nsCPUCommittedPercent),
			namespaceMetricColumn("memory", nsMemCommittedPercent),
			{Key: "status", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return namespaceStatus(r) }},
			ageColumn(creationAge),
			{Key: "metrics", Kind: KindString, CellType: "string", FilterType: FilterNone,
				Cell: func(r *Row) interface{} { return overlayRecordCell(r, OverlayNamespaceMetrics, r.Name()) }},
		},
		Overlays: []Overlay{{
			Name:    OverlayNamespaceMetrics,
			Columns: []string{"cpu", "memory", "metrics"},
			Key:     NodeKey,
		}},
		Search: []SearchField{
			nameField(),
			labelsField(),
			annotationsField(),
			uidField(),
			{Name: "status", Aliases: []string{"phase", "state"}, Extract: namespaceStatus},
			{Name: "finalizers", Aliases: []string{"finalizer"}, Extract: func(r *Row) string {
				ns := namespaceObj(r)
				if ns == nil {
					return ""
				}
				out := ""
				for i, f := range ns.Spec.Finalizers {
					if i > 0 {
						out += " "
					}
					out += string(f)
				}
				return out
			}},
		},
		Notes: "cpu and memory are overlay columns (pod metrics summed per namespace over the cluster capacity): they sort like the legacy columns, and the view refreshes them on every metrics poll while the legacy list fetches them once per visit. The metrics column exists only in view mode (hidden by default) and carries the aggregate the bars draw.",
	}
}
