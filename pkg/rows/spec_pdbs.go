package rows

import (
	"strconv"

	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"kubikles/pkg/store"
)

func init() { MustRegister(pdbsSpec()) }

func pdbObj(r *Row) *policyv1.PodDisruptionBudget {
	p, _ := r.E.Obj.(*policyv1.PodDisruptionBudget)
	return p
}

// intOrStringCell is `value ?? '-'` for an IntOrString: a number, a string, or the dash.
func intOrStringCell(v *intstr.IntOrString) interface{} {
	if v == nil {
		return "-"
	}
	if v.Type == intstr.String {
		return v.StrVal
	}
	return float64(v.IntVal)
}

// intOrStringText is String(value ?? empty).
func intOrStringText(v *intstr.IntOrString) string {
	if v == nil {
		return ""
	}
	if v.Type == intstr.String {
		return v.StrVal
	}
	return strconv.Itoa(int(v.IntVal))
}

func pdbMin(r *Row) *intstr.IntOrString {
	if p := pdbObj(r); p != nil {
		return p.Spec.MinAvailable
	}
	return nil
}

func pdbMax(r *Row) *intstr.IntOrString {
	if p := pdbObj(r); p != nil {
		return p.Spec.MaxUnavailable
	}
	return nil
}

func pdbsSpec() *Spec {
	status := func(f func(s *policyv1.PodDisruptionBudgetStatus) int32) func(r *Row) string {
		return func(r *Row) string {
			if p := pdbObj(r); p != nil {
				return strconv.Itoa(int(f(&p.Status)))
			}
			return ""
		}
	}
	return &Spec{
		Type:       "pdbs",
		Namespaced: true,
		Needs:      store.ModeFull, // budget and status are not metadata
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			// Numbers, percentage strings or the dash. Select filters see the strings only (the
			// legacy filter value of a number is empty).
			{Key: "minAvailable", Kind: KindString, CellType: "number | string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return intOrStringCell(pdbMin(r)) }},
			{Key: "maxUnavailable", Kind: KindString, CellType: "number | string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return intOrStringCell(pdbMax(r)) }},
			{Key: "allowed", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} {
					if p := pdbObj(r); p != nil {
						return numberCell(int(p.Status.DisruptionsAllowed))
					}
					return nil
				}},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "minavailable", Aliases: []string{"min"}, Extract: func(r *Row) string { return intOrStringText(pdbMin(r)) }},
			SearchField{Name: "maxunavailable", Aliases: []string{"max"}, Extract: func(r *Row) string { return intOrStringText(pdbMax(r)) }},
			SearchField{Name: "selector", Aliases: []string{"podselector"}, Extract: func(r *Row) string {
				if p := pdbObj(r); p != nil && p.Spec.Selector != nil {
					return kvText(p.Spec.Selector.MatchLabels)
				}
				return ""
			}},
			SearchField{Name: "currenthealthy", Aliases: []string{"healthy", "current"}, Extract: status(func(s *policyv1.PodDisruptionBudgetStatus) int32 { return s.CurrentHealthy })},
			SearchField{Name: "desiredhealthy", Aliases: []string{"desired"}, Extract: status(func(s *policyv1.PodDisruptionBudgetStatus) int32 { return s.DesiredHealthy })},
			SearchField{Name: "disruptionsallowed", Aliases: []string{"allowed", "disruptions"}, Extract: status(func(s *policyv1.PodDisruptionBudgetStatus) int32 { return s.DisruptionsAllowed })},
			SearchField{Name: "expectedpods", Aliases: []string{"expected", "pods"}, Extract: status(func(s *policyv1.PodDisruptionBudgetStatus) int32 { return s.ExpectedPods })},
		),
	}
}
