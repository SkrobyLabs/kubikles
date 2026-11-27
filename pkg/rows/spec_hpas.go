package rows

import (
	"strings"

	autoscalingv2 "k8s.io/api/autoscaling/v2"

	"kubikles/pkg/store"
)

func init() { MustRegister(hpasSpec()) }

func hpaObj(r *Row) *autoscalingv2.HorizontalPodAutoscaler {
	h, _ := r.E.Obj.(*autoscalingv2.HorizontalPodAutoscaler)
	return h
}

// hpaTarget is `${ref.kind}/${ref.name}`. scaleTargetRef is a required struct, so the legacy
// "no reference" dash never occurs for objects that came through the API.
func hpaTarget(r *Row) string {
	h := hpaObj(r)
	if h == nil {
		return "-"
	}
	return h.Spec.ScaleTargetRef.Kind + "/" + h.Spec.ScaleTargetRef.Name
}

func hpaMinReplicas(h *autoscalingv2.HorizontalPodAutoscaler) int {
	if h != nil && h.Spec.MinReplicas != nil {
		return int(*h.Spec.MinReplicas)
	}
	return 1
}

// hpaMinMax is `${spec.minReplicas ?? 1}/${spec.maxReplicas ?? '-'}`; maxReplicas is always
// serialised, so the dash only stands for a missing object.
func hpaMinMax(r *Row) string {
	h := hpaObj(r)
	if h == nil {
		return "1/-"
	}
	return itoa(hpaMinReplicas(h)) + "/" + itoa(int(h.Spec.MaxReplicas))
}

// hpaReplicas is status.currentReplicas ?? '-': the field is omitted while it is zero.
func hpaReplicas(r *Row) interface{} {
	if h := hpaObj(r); h != nil && h.Status.CurrentReplicas != 0 {
		return numberCell(int(h.Status.CurrentReplicas))
	}
	return "-"
}

func hpasSpec() *Spec {
	return &Spec{
		Type:       "hpas",
		Namespaced: true,
		Needs:      store.ModeFull, // scale target, replicas and metrics are not metadata
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "reference", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return hpaTarget(r) }},
			{Key: "minmax", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return hpaMinMax(r) }},
			// A number once the controller reports replicas, the dash before. Select filters see no
			// value for the numbers (the legacy filter value of a number is empty).
			{Key: "replicas", Kind: KindString, CellType: "number | string", FilterType: FilterSelect, Cell: hpaReplicas},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "target", Aliases: []string{"reference", "ref", "scaletarget"}, Extract: hpaTarget},
			SearchField{Name: "targetkind", Aliases: []string{"kind", "refkind"}, Extract: func(r *Row) string {
				if h := hpaObj(r); h != nil {
					return h.Spec.ScaleTargetRef.Kind
				}
				return ""
			}},
			SearchField{Name: "targetname", Aliases: []string{"refname"}, Extract: func(r *Row) string {
				if h := hpaObj(r); h != nil {
					return h.Spec.ScaleTargetRef.Name
				}
				return ""
			}},
			SearchField{Name: "minreplicas", Aliases: []string{"min"}, Extract: func(r *Row) string {
				return itoa(hpaMinReplicas(hpaObj(r)))
			}},
			SearchField{Name: "maxreplicas", Aliases: []string{"max"}, Extract: func(r *Row) string {
				if h := hpaObj(r); h != nil && h.Spec.MaxReplicas != 0 {
					return itoa(int(h.Spec.MaxReplicas))
				}
				return ""
			}},
			SearchField{Name: "currentreplicas", Aliases: []string{"current", "replicas"}, Extract: func(r *Row) string {
				if h := hpaObj(r); h != nil && h.Status.CurrentReplicas != 0 {
					return itoa(int(h.Status.CurrentReplicas))
				}
				return ""
			}},
			SearchField{Name: "desiredreplicas", Aliases: []string{"desired"}, Extract: func(r *Row) string {
				if h := hpaObj(r); h != nil {
					return itoa(int(h.Status.DesiredReplicas))
				}
				return ""
			}},
			SearchField{Name: "metrics", Aliases: []string{"metrictype"}, Extract: func(r *Row) string {
				h := hpaObj(r)
				if h == nil {
					return ""
				}
				types := make([]string, len(h.Spec.Metrics))
				for i, m := range h.Spec.Metrics {
					types[i] = string(m.Type)
				}
				return strings.Join(types, " ")
			}},
		),
	}
}
