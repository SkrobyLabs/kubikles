package rows

import (
	"strconv"

	schedulingv1 "k8s.io/api/scheduling/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(priorityClassesSpec()) }

func priorityClassObj(r *Row) *schedulingv1.PriorityClass {
	pc, _ := r.E.Obj.(*schedulingv1.PriorityClass)
	return pc
}

func priorityClassValue(r *Row) int {
	if pc := priorityClassObj(r); pc != nil {
		return int(pc.Value)
	}
	return 0
}

func priorityClassGlobalDefault(r *Row) string {
	if pc := priorityClassObj(r); pc != nil && pc.GlobalDefault {
		return "Yes"
	}
	return "No"
}

// priorityClassPreemption is item.preemptionPolicy || 'PreemptLowerPriority'.
func priorityClassPreemption(r *Row) string {
	if pc := priorityClassObj(r); pc != nil && pc.PreemptionPolicy != nil && *pc.PreemptionPolicy != "" {
		return string(*pc.PreemptionPolicy)
	}
	return "PreemptLowerPriority"
}

func priorityClassesSpec() *Spec {
	return &Spec{
		Type:       "priorityclasses",
		Namespaced: false,
		Needs:      store.ModeFull, // value and description are not metadata
		Columns: []Column{
			nameColumn(),
			{Key: "value", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(priorityClassValue(r)) }},
			{Key: "globalDefault", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return priorityClassGlobalDefault(r) }},
			{Key: "preemption", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return priorityClassPreemption(r) }},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "value", Aliases: []string{"priority", "priorityvalue"}, Extract: func(r *Row) string {
				return strconv.Itoa(priorityClassValue(r))
			}},
			SearchField{Name: "globaldefault", Aliases: []string{"default", "isdefault"}, Extract: func(r *Row) string {
				if priorityClassGlobalDefault(r) == "Yes" {
					return "true"
				}
				return "false"
			}},
			SearchField{Name: "preemption", Aliases: []string{"preemptionpolicy", "preemptlowerpriority"}, Extract: priorityClassPreemption},
			SearchField{Name: "description", Aliases: []string{"desc"}, Extract: func(r *Row) string {
				if pc := priorityClassObj(r); pc != nil {
					return pc.Description
				}
				return ""
			}},
		),
	}
}
