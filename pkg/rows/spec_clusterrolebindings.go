package rows

import (
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(clusterRoleBindingsSpec()) }

func clusterRoleBindingObj(r *Row) *rbacv1.ClusterRoleBinding {
	b, _ := r.E.Obj.(*rbacv1.ClusterRoleBinding)
	return b
}

func bindingSubjects(r *Row) []rbacv1.Subject {
	if b := clusterRoleBindingObj(r); b != nil {
		return b.Subjects
	}
	return nil
}

func bindingRoleRef(r *Row) rbacv1.RoleRef {
	if b := clusterRoleBindingObj(r); b != nil {
		return b.RoleRef
	}
	return rbacv1.RoleRef{}
}

func subjectsJoin(subjects []rbacv1.Subject, keep func(rbacv1.Subject) bool, text func(rbacv1.Subject) string) string {
	var out []string
	for _, s := range subjects {
		if keep == nil || keep(s) {
			out = append(out, text(s))
		}
	}
	return strings.Join(out, " ")
}

func ofKind(kind string) func(rbacv1.Subject) bool {
	return func(s rbacv1.Subject) bool { return s.Kind == kind }
}

func subjectName(s rbacv1.Subject) string { return s.Name }

// bindingSearchFields is the field table of rolebindings.ts, shared by role bindings and cluster
// role bindings (the objects have the same shape).
func bindingSearchFields(subjectsOf func(r *Row) []rbacv1.Subject, refOf func(r *Row) rbacv1.RoleRef) []SearchField {
	return []SearchField{
		{Name: "roleref", Aliases: []string{"role", "ref"}, Extract: func(r *Row) string {
			ref := refOf(r)
			return ref.Kind + "/" + ref.Name
		}},
		{Name: "rolerefkind", Aliases: []string{"refkind"}, Extract: func(r *Row) string { return refOf(r).Kind }},
		{Name: "rolerefname", Aliases: []string{"refname", "rolename"}, Extract: func(r *Row) string { return refOf(r).Name }},
		{Name: "subjects", Aliases: []string{"subject"}, Extract: func(r *Row) string {
			return subjectsJoin(subjectsOf(r), nil, func(s rbacv1.Subject) string { return s.Kind + "/" + s.Name })
		}},
		{Name: "subjectcount", Aliases: []string{"subjectscount", "numsubjects"}, Extract: func(r *Row) string { return itoa(len(subjectsOf(r))) }},
		{Name: "subjectkind", Aliases: []string{"subjectkinds"}, Extract: func(r *Row) string {
			var kinds []string
			for _, s := range subjectsOf(r) {
				kinds = append(kinds, s.Kind)
			}
			return uniqueJoin(kinds, " ")
		}},
		{Name: "subjectnames", Aliases: []string{"subjectname"}, Extract: func(r *Row) string { return subjectsJoin(subjectsOf(r), nil, subjectName) }},
		{Name: "users", Aliases: []string{"user"}, Extract: func(r *Row) string { return subjectsJoin(subjectsOf(r), ofKind("User"), subjectName) }},
		{Name: "groups", Aliases: []string{"group"}, Extract: func(r *Row) string { return subjectsJoin(subjectsOf(r), ofKind("Group"), subjectName) }},
		{Name: "serviceaccounts", Aliases: []string{"serviceaccount", "sa"}, Extract: func(r *Row) string {
			return subjectsJoin(subjectsOf(r), ofKind("ServiceAccount"), func(s rbacv1.Subject) string {
				if s.Namespace != "" {
					return s.Namespace + "/" + s.Name
				}
				return s.Name
			})
		}},
	}
}

func clusterRoleBindingsSpec() *Spec {
	return &Spec{
		Type:       "clusterrolebindings",
		Namespaced: false,
		Needs:      store.ModeFull, // roleRef and subjects are not metadata
		Columns: []Column{
			nameColumn(),
			{Key: "roleRef", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { ref := bindingRoleRef(r); return ref.Kind + "/" + ref.Name }},
			{Key: "subjects", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(len(bindingSubjects(r))) }},
			ageColumn(creationAge),
		},
		// Fields of rolebindings.ts (a cluster role binding has the same shape).
		Search: append(commonSearchFields(), bindingSearchFields(bindingSubjects, bindingRoleRef)...),
	}
}
