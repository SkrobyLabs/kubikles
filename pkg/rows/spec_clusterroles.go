package rows

import (
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(clusterRolesSpec()) }

func clusterRoleObj(r *Row) *rbacv1.ClusterRole {
	cr, _ := r.E.Obj.(*rbacv1.ClusterRole)
	return cr
}

func clusterRoleRules(r *Row) []rbacv1.PolicyRule {
	if cr := clusterRoleObj(r); cr != nil {
		return cr.Rules
	}
	return nil
}

func clusterRoleSelectors(r *Row) int {
	if cr := clusterRoleObj(r); cr != nil && cr.AggregationRule != nil {
		return len(cr.AggregationRule.ClusterRoleSelectors)
	}
	return 0
}

// ruleValues is the deduplicated union of one string list over all rules, joined with spaces.
func ruleValues(rules []rbacv1.PolicyRule, pick func(rbacv1.PolicyRule) []string) string {
	var vals []string
	for _, rule := range rules {
		vals = append(vals, pick(rule)...)
	}
	return uniqueJoin(vals, " ")
}

// ruleSearchFields is the rule fields of roles.ts, shared by roles and cluster roles.
func ruleSearchFields(rulesOf func(r *Row) []rbacv1.PolicyRule) []SearchField {
	return []SearchField{
		{Name: "rules", Aliases: []string{"rule"}, Extract: func(r *Row) string {
			rules := rulesOf(r)
			parts := make([]string, len(rules))
			for i, rule := range rules {
				parts[i] = strings.Join(rule.APIGroups, ",") + "/" + strings.Join(rule.Resources, ",") + ":" + strings.Join(rule.Verbs, ",")
			}
			return strings.Join(parts, " ")
		}},
		{Name: "rulecount", Aliases: []string{"rulescount", "numrules"}, Extract: func(r *Row) string { return itoa(len(rulesOf(r))) }},
		{Name: "verbs", Aliases: []string{"verb"}, Extract: func(r *Row) string {
			return ruleValues(rulesOf(r), func(rule rbacv1.PolicyRule) []string { return rule.Verbs })
		}},
		{Name: "resources", Aliases: []string{"resource"}, Extract: func(r *Row) string {
			return ruleValues(rulesOf(r), func(rule rbacv1.PolicyRule) []string { return rule.Resources })
		}},
		{Name: "apigroups", Aliases: []string{"apigroup", "groups", "group"}, Extract: func(r *Row) string {
			return ruleValues(rulesOf(r), func(rule rbacv1.PolicyRule) []string {
				out := make([]string, len(rule.APIGroups))
				for i, g := range rule.APIGroups {
					out[i] = orDefault(g, "core")
				}
				return out
			})
		}},
	}
}

func clusterRolesSpec() *Spec {
	return &Spec{
		Type:       "clusterroles",
		Namespaced: false,
		Needs:      store.ModeFull, // rules and aggregation are not metadata
		Columns: []Column{
			nameColumn(),
			{Key: "rules", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(len(clusterRoleRules(r))) }},
			{Key: "aggregation", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return yesNo(clusterRoleSelectors(r) > 0) }},
			ageColumn(creationAge),
		},
		// Fields of roles.ts followed by the aggregation field of clusterroles.ts.
		Search: append(append(commonSearchFields(), ruleSearchFields(clusterRoleRules)...),
			SearchField{Name: "aggregation", Aliases: []string{"aggregate", "aggregationrule"}, Extract: func(r *Row) string {
				cr := clusterRoleObj(r)
				if cr == nil || cr.AggregationRule == nil {
					return ""
				}
				parts := make([]string, len(cr.AggregationRule.ClusterRoleSelectors))
				for i, sel := range cr.AggregationRule.ClusterRoleSelectors {
					keys := jsKeys(sel.MatchLabels)
					kv := make([]string, len(keys))
					for j, k := range keys {
						kv[j] = k + "=" + sel.MatchLabels[k]
					}
					parts[i] = strings.Join(kv, ",")
				}
				return strings.Join(parts, " ")
			}},
		),
	}
}
