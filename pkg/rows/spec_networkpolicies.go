package rows

import (
	"strings"

	networkingv1 "k8s.io/api/networking/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(networkPoliciesSpec()) }

func netpolObj(r *Row) *networkingv1.NetworkPolicy {
	n, _ := r.E.Obj.(*networkingv1.NetworkPolicy)
	return n
}

func netpolMatchLabels(r *Row) map[string]string {
	if n := netpolObj(r); n != nil {
		return n.Spec.PodSelector.MatchLabels
	}
	return nil
}

// netpolPodSelector is the list cell: "All Pods", or the first two labels plus a "+N" overflow.
func netpolPodSelector(r *Row) string {
	labels := netpolMatchLabels(r)
	if len(labels) == 0 {
		return "All Pods"
	}
	keys := jsKeys(labels)
	shown := keys
	if len(shown) > 2 {
		shown = shown[:2]
	}
	parts := make([]string, len(shown))
	for i, k := range shown {
		parts[i] = k + "=" + labels[k]
	}
	out := strings.Join(parts, ", ")
	if extra := len(keys) - 2; extra > 0 {
		out += ", +" + itoa(extra)
	}
	return out
}

// netpolTypes is the policy types joined by sep; the Ingress default applies when none are set.
func netpolTypes(r *Row, sep string) string {
	n := netpolObj(r)
	if n == nil || len(n.Spec.PolicyTypes) == 0 {
		return "Ingress"
	}
	types := make([]string, len(n.Spec.PolicyTypes))
	for i, t := range n.Spec.PolicyTypes {
		types[i] = string(t)
	}
	return strings.Join(types, sep)
}

func netpolRuleCount(ingress bool) func(r *Row) int {
	return func(r *Row) int {
		n := netpolObj(r)
		if n == nil {
			return 0
		}
		if ingress {
			return len(n.Spec.Ingress)
		}
		return len(n.Spec.Egress)
	}
}

func networkPoliciesSpec() *Spec {
	ingress, egress := netpolRuleCount(true), netpolRuleCount(false)
	has := func(count func(r *Row) int) func(r *Row) string {
		return func(r *Row) string {
			if count(r) > 0 {
				return "true"
			}
			return "false"
		}
	}
	return &Spec{
		Type:       "networkpolicies",
		Namespaced: true,
		Needs:      store.ModeFull, // selector, types and rules are not metadata
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "podSelector", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return netpolPodSelector(r) }},
			{Key: "policyTypes", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return netpolTypes(r, ", ") }},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "podselector", Aliases: []string{"selector", "pods"}, Extract: func(r *Row) string {
				labels := netpolMatchLabels(r)
				if len(labels) == 0 {
					return "all"
				}
				return kvText(labels)
			}},
			SearchField{Name: "policytype", Aliases: []string{"policytypes", "type", "types"}, Extract: func(r *Row) string { return netpolTypes(r, " ") }},
			SearchField{Name: "ingress", Aliases: []string{"hasingress"}, Extract: has(ingress)},
			SearchField{Name: "egress", Aliases: []string{"hasegress"}, Extract: has(egress)},
			SearchField{Name: "ingresscount", Aliases: []string{"ingressrules"}, Extract: func(r *Row) string { return itoa(ingress(r)) }},
			SearchField{Name: "egresscount", Aliases: []string{"egressrules"}, Extract: func(r *Row) string { return itoa(egress(r)) }},
		),
	}
}
