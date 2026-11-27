package rows

import (
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(leasesSpec()) }

func leaseObj(r *Row) *coordinationv1.Lease {
	l, _ := r.E.Obj.(*coordinationv1.Lease)
	return l
}

func leaseHolder(r *Row) string {
	if l := leaseObj(r); l != nil && l.Spec.HolderIdentity != nil {
		return *l.Spec.HolderIdentity
	}
	return ""
}

func leaseDuration(r *Row) int {
	if l := leaseObj(r); l != nil && l.Spec.LeaseDurationSeconds != nil {
		return int(*l.Spec.LeaseDurationSeconds)
	}
	return 0
}

// leaseRenewTime is spec.renewTime as the API serialises a MicroTime, "" when unset.
func leaseRenewTime(r *Row) string {
	l := leaseObj(r)
	if l == nil || l.Spec.RenewTime == nil || l.Spec.RenewTime.IsZero() {
		return ""
	}
	return l.Spec.RenewTime.UTC().Format(metav1.RFC3339Micro)
}

func leasesSpec() *Spec {
	return &Spec{
		Type:       "leases",
		Namespaced: true,
		// holder, duration and renewTime are spec fields the Table representation does not
		// carry, so the type cannot use metadata-only storage.
		Needs: store.ModeFull,
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "holder", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} {
					if h := leaseHolder(r); h != "" {
						return h
					}
					return "-"
				}},
			// A number: select filters see no value for it, as in the legacy list.
			{Key: "duration", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(leaseDuration(r)) }},
			{Key: "renewTime", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return leaseRenewTime(r) }},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "holder", Aliases: []string{"holderidentity", "leader", "owner"}, Extract: leaseHolder},
			SearchField{Name: "duration", Aliases: []string{"leaseduration", "leasedurationseconds"}, Extract: func(r *Row) string {
				if d := leaseDuration(r); d != 0 {
					return itoa(d)
				}
				return ""
			}},
			SearchField{Name: "transitions", Aliases: []string{"leasetransitions", "changes"}, Extract: func(r *Row) string {
				if l := leaseObj(r); l != nil && l.Spec.LeaseTransitions != nil {
					return itoa(int(*l.Spec.LeaseTransitions))
				}
				return "0"
			}},
		),
	}
}
