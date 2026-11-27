package rows

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(limitRangesSpec()) }

func limitRangeLimits(r *Row) []corev1.LimitRangeItem {
	if lr, _ := r.E.Obj.(*corev1.LimitRange); lr != nil {
		return lr.Spec.Limits
	}
	return nil
}

func limitRangeHasType(t corev1.LimitType) func(r *Row) string {
	return func(r *Row) string {
		for _, l := range limitRangeLimits(r) {
			if l.Type == t {
				return "true"
			}
		}
		return "false"
	}
}

func limitRangesSpec() *Spec {
	return &Spec{
		Type:       "limitranges",
		Namespaced: true,
		Needs:      store.ModeFull, // the limits are spec, not metadata
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "limits", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(len(limitRangeLimits(r))) }},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "limittype", Aliases: []string{"type", "types"}, Extract: func(r *Row) string {
				limits := limitRangeLimits(r)
				types := make([]string, len(limits))
				for i, l := range limits {
					types[i] = string(l.Type)
				}
				return strings.Join(types, " ")
			}},
			SearchField{Name: "limitcount", Aliases: []string{"count", "limits"}, Extract: func(r *Row) string {
				return itoa(len(limitRangeLimits(r)))
			}},
			SearchField{Name: "hascontainer", Aliases: []string{"container"}, Extract: limitRangeHasType(corev1.LimitTypeContainer)},
			SearchField{Name: "haspod", Aliases: []string{"pod"}, Extract: limitRangeHasType(corev1.LimitTypePod)},
			SearchField{Name: "haspvc", Aliases: []string{"pvc"}, Extract: limitRangeHasType(corev1.LimitTypePersistentVolumeClaim)},
		),
	}
}
