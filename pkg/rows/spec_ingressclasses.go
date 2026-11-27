package rows

import (
	networkingv1 "k8s.io/api/networking/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(ingressClassesSpec()) }

const ingressDefaultAnnotation = "ingressclass.kubernetes.io/is-default-class"

func ingressClassController(r *Row) string {
	if ic, _ := r.E.Obj.(*networkingv1.IngressClass); ic != nil {
		return ic.Spec.Controller
	}
	return ""
}

func ingressClassDefault(r *Row) string {
	if r.M != nil && r.M.GetAnnotations()[ingressDefaultAnnotation] == "true" {
		return "Yes"
	}
	return "No"
}

func ingressClassesSpec() *Spec {
	return &Spec{
		Type:       "ingressclasses",
		Namespaced: false,
		Needs:      store.ModeFull, // spec.controller
		Columns: []Column{
			nameColumn(),
			{Key: "controller", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return ingressClassController(r) }},
			{Key: "default", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return ingressClassDefault(r) }},
			ageColumn(creationAge),
		},
		// The legacy list has no field mapping for this type, so only the common fields apply.
		Search: commonSearchFields(),
	}
}
