package rows

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(rolesSpec()) }

func roleRules(r *Row) []rbacv1.PolicyRule {
	if role, _ := r.E.Obj.(*rbacv1.Role); role != nil {
		return role.Rules
	}
	return nil
}

func rolesSpec() *Spec {
	return &Spec{
		Type:       "roles",
		Namespaced: true,
		Needs:      store.ModeFull, // rules are not metadata
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "rules", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(len(roleRules(r))) }},
			ageColumn(creationAge),
		},
		// Fields of roles.ts.
		Search: append(commonSearchFields(), ruleSearchFields(roleRules)...),
	}
}
