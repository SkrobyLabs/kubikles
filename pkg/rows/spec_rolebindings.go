package rows

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(roleBindingsSpec()) }

func roleBindingObj(r *Row) *rbacv1.RoleBinding {
	b, _ := r.E.Obj.(*rbacv1.RoleBinding)
	return b
}

func roleBindingSubjects(r *Row) []rbacv1.Subject {
	if b := roleBindingObj(r); b != nil {
		return b.Subjects
	}
	return nil
}

func roleBindingRef(r *Row) rbacv1.RoleRef {
	if b := roleBindingObj(r); b != nil {
		return b.RoleRef
	}
	return rbacv1.RoleRef{}
}

func roleBindingsSpec() *Spec {
	return &Spec{
		Type:       "rolebindings",
		Namespaced: true,
		Needs:      store.ModeFull, // roleRef and subjects are not metadata
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "roleRef", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { ref := roleBindingRef(r); return ref.Kind + "/" + ref.Name }},
			{Key: "subjects", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(len(roleBindingSubjects(r))) }},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(), bindingSearchFields(roleBindingSubjects, roleBindingRef)...),
	}
}
