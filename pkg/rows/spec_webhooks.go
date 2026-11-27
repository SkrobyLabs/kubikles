package rows

import (
	"strconv"
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"

	"kubikles/pkg/store"
)

func init() {
	MustRegister(webhookConfigsSpec("validatingwebhookconfigurations", false))
	MustRegister(webhookConfigsSpec("mutatingwebhookconfigurations", true))
}

// webhookView is what the columns and search fields read of one webhook, whichever of the two
// configuration kinds it comes from.
type webhookView struct {
	failurePolicy      string
	sideEffects        string
	reinvocationPolicy string
	service            string
	hasService         bool
	rules              []admissionv1.RuleWithOperations
}

func policyString[T ~string](p *T) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

func webhookViews(r *Row) []webhookView {
	switch o := r.E.Obj.(type) {
	case *admissionv1.ValidatingWebhookConfiguration:
		out := make([]webhookView, len(o.Webhooks))
		for i, w := range o.Webhooks {
			out[i] = webhookView{failurePolicy: policyString(w.FailurePolicy), sideEffects: policyString(w.SideEffects), rules: w.Rules}
			setWebhookService(&out[i], w.ClientConfig)
		}
		return out
	case *admissionv1.MutatingWebhookConfiguration:
		out := make([]webhookView, len(o.Webhooks))
		for i, w := range o.Webhooks {
			out[i] = webhookView{failurePolicy: policyString(w.FailurePolicy), sideEffects: policyString(w.SideEffects),
				reinvocationPolicy: policyString(w.ReinvocationPolicy), rules: w.Rules}
			setWebhookService(&out[i], w.ClientConfig)
		}
		return out
	}
	return nil
}

func setWebhookService(v *webhookView, cc admissionv1.WebhookClientConfig) {
	if cc.Service != nil {
		v.hasService = true
		v.service = cc.Service.Namespace + "/" + cc.Service.Name
	}
}

// orDefault is `value || fallback` for strings.
func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// uniqueJoin is Array.from(new Set(values)).join(sep): first occurrences, in order.
func uniqueJoin(values []string, sep string) string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return strings.Join(out, sep)
}

func webhookPolicies(r *Row, sep string, pick func(webhookView) string) string {
	views := webhookViews(r)
	vals := make([]string, len(views))
	for i, v := range views {
		vals[i] = pick(v)
	}
	return uniqueJoin(vals, sep)
}

func webhookFailurePolicies(r *Row, sep string) string {
	return webhookPolicies(r, sep, func(v webhookView) string { return orDefault(v.failurePolicy, "Fail") })
}

func webhookReinvocationPolicies(r *Row, sep string) string {
	return webhookPolicies(r, sep, func(v webhookView) string { return orDefault(v.reinvocationPolicy, "Never") })
}

func webhookRuleValues(r *Row, pick func(admissionv1.RuleWithOperations) []string) string {
	var vals []string
	for _, v := range webhookViews(r) {
		for _, rule := range v.rules {
			vals = append(vals, pick(rule)...)
		}
	}
	return uniqueJoin(vals, " ")
}

func webhookConfigsSpec(typ string, mutating bool) *Spec {
	// The column joins with ", " and shows "-" for no webhooks; the search field joins with " ".
	dashed := func(s string) string { return orDefault(s, "-") }
	cols := []Column{
		nameColumn(),
		{Key: "webhooks", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return numberCell(len(webhookViews(r))) }},
		{Key: "failurePolicy", Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return dashed(webhookFailurePolicies(r, ", ")) }},
	}
	if mutating {
		cols = append(cols, Column{Key: "reinvocation", Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return dashed(webhookReinvocationPolicies(r, ", ")) }})
	}
	cols = append(cols, ageColumn(creationAge))

	search := append(commonSearchFields(),
		SearchField{Name: "webhooks", Aliases: []string{"webhookcount", "count"}, Extract: func(r *Row) string {
			return strconv.Itoa(len(webhookViews(r)))
		}},
		SearchField{Name: "failurepolicy", Aliases: []string{"failure", "onfailure"}, Extract: func(r *Row) string {
			return webhookFailurePolicies(r, " ")
		}},
		SearchField{Name: "sideeffects", Aliases: []string{"sideeffect"}, Extract: func(r *Row) string {
			return webhookPolicies(r, " ", func(v webhookView) string { return orDefault(v.sideEffects, "Unknown") })
		}},
		SearchField{Name: "service", Aliases: []string{"services", "target"}, Extract: func(r *Row) string {
			var vals []string
			for _, v := range webhookViews(r) {
				if v.hasService {
					vals = append(vals, v.service)
				}
			}
			return uniqueJoin(vals, " ")
		}},
		SearchField{Name: "resources", Aliases: []string{"resource"}, Extract: func(r *Row) string {
			return webhookRuleValues(r, func(rule admissionv1.RuleWithOperations) []string { return rule.Resources })
		}},
		SearchField{Name: "operations", Aliases: []string{"operation", "ops"}, Extract: func(r *Row) string {
			return webhookRuleValues(r, func(rule admissionv1.RuleWithOperations) []string {
				ops := make([]string, len(rule.Operations))
				for i, op := range rule.Operations {
					ops[i] = string(op)
				}
				return ops
			})
		}},
	)
	if mutating {
		search = append(search, SearchField{Name: "reinvocation", Aliases: []string{"reinvocationpolicy"}, Extract: func(r *Row) string {
			return webhookReinvocationPolicies(r, " ")
		}})
	}
	return &Spec{
		Type:       typ,
		Namespaced: false,
		Needs:      store.ModeFull, // webhook policies and rules are not metadata
		Columns:    cols,
		Search:     search,
		Notes: "A service reference without a namespace reads \"undefined/name\" in the legacy search and \"/name\" here; " +
			"the API server requires the namespace, so real objects never differ.",
	}
}
