package rows

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"kubikles/pkg/store"
)

func init() { MustRegister(resourceQuotasSpec()) }

func quotaObj(r *Row) *corev1.ResourceQuota {
	q, _ := r.E.Obj.(*corev1.ResourceQuota)
	return q
}

func quotaHard(r *Row) corev1.ResourceList {
	if q := quotaObj(r); q != nil {
		return q.Spec.Hard
	}
	return nil
}

// quantityText is Object.entries(list).map(([k, v]) => `${k}=${v}`).join(' ') for a resource
// list, with the values as the API serialises them.
func quantityText(list corev1.ResourceList) string {
	if len(list) == 0 {
		return ""
	}
	m := make(map[string]string, len(list))
	for k, v := range list {
		q := v
		m[string(k)] = quantityString(&q)
	}
	return kvText(m)
}

func quantityString(q *resource.Quantity) string { return q.String() }

func resourceQuotasSpec() *Spec {
	return &Spec{
		Type:       "resourcequotas",
		Namespaced: true,
		Needs:      store.ModeFull, // hard limits, usage and scopes are not metadata
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "resources", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(len(quotaHard(r))) }},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "hard", Aliases: []string{"limits", "resources"}, Extract: func(r *Row) string {
				return quantityText(quotaHard(r))
			}},
			SearchField{Name: "used", Aliases: []string{"usage"}, Extract: func(r *Row) string {
				if q := quotaObj(r); q != nil {
					return quantityText(q.Status.Used)
				}
				return ""
			}},
			SearchField{Name: "scopes", Aliases: []string{"scope"}, Extract: func(r *Row) string {
				q := quotaObj(r)
				if q == nil {
					return ""
				}
				scopes := make([]string, len(q.Spec.Scopes))
				for i, s := range q.Spec.Scopes {
					scopes[i] = string(s)
				}
				return strings.Join(scopes, " ")
			}},
			SearchField{Name: "resourcecount", Aliases: []string{"count"}, Extract: func(r *Row) string {
				return itoa(len(quotaHard(r)))
			}},
		),
	}
}
