package rows

import (
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(endpointsSpec()) }

func endpointsSubsets(r *Row) []corev1.EndpointSubset {
	if ep, _ := r.E.Obj.(*corev1.Endpoints); ep != nil {
		return ep.Subsets
	}
	return nil
}

func endpointsReady(r *Row) int {
	n := 0
	for _, s := range endpointsSubsets(r) {
		n += len(s.Addresses)
	}
	return n
}

func endpointsNotReady(r *Row) int {
	n := 0
	for _, s := range endpointsSubsets(r) {
		n += len(s.NotReadyAddresses)
	}
	return n
}

// endpointsPortList is the unique "port/protocol" strings in first-seen order; a missing
// protocol reads as TCP.
func endpointsPortList(r *Row) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range endpointsSubsets(r) {
		for _, p := range s.Ports {
			key := strconv.Itoa(int(p.Port)) + "/" + orDefault(string(p.Protocol), "TCP")
			if !seen[key] {
				seen[key] = true
				out = append(out, key)
			}
		}
	}
	return out
}

// endpointsPorts is the list cell: the first three ports plus a "+N" overflow, or the dash.
func endpointsPorts(r *Row) string {
	ports := endpointsPortList(r)
	if len(ports) == 0 {
		return "-"
	}
	shown := ports
	if len(shown) > 3 {
		shown = shown[:3]
	}
	out := strings.Join(shown, ", ")
	if len(ports) > 3 {
		out += " +" + itoa(len(ports)-3)
	}
	return out
}

func endpointsSpec() *Spec {
	return &Spec{
		Type:       "endpoints",
		Namespaced: true,
		Needs:      store.ModeFull, // subsets are not metadata
		Notes:      "The readyCount column exists only in view mode (hidden by default): the endpoints cell reads ready/total and needs the ready half.",
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "endpoints", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(endpointsReady(r) + endpointsNotReady(r)) }},
			{Key: "ports", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return endpointsPorts(r) }},
			// Not a column of the legacy list: the endpoints cell reads "ready/total" and needs the
			// ready half. Hidden in the column picker by default.
			{Key: "readyCount", Kind: KindNumber, CellType: "number", FilterType: FilterNone,
				Cell: func(r *Row) interface{} { return numberCell(endpointsReady(r)) }},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "subsets", Aliases: []string{"subsetcount"}, Extract: func(r *Row) string { return itoa(len(endpointsSubsets(r))) }},
			SearchField{Name: "addresses", Aliases: []string{"ips", "ip"}, Extract: func(r *Row) string {
				var ips []string
				for _, s := range endpointsSubsets(r) {
					for _, a := range s.Addresses {
						ips = append(ips, a.IP)
					}
				}
				return strings.Join(ips, " ")
			}},
			SearchField{Name: "ready", Aliases: []string{"readycount"}, Extract: func(r *Row) string { return itoa(endpointsReady(r)) }},
			SearchField{Name: "notready", Aliases: []string{"notreadycount"}, Extract: func(r *Row) string { return itoa(endpointsNotReady(r)) }},
			SearchField{Name: "ports", Aliases: []string{"port"}, Extract: func(r *Row) string { return strings.Join(endpointsPortList(r), " ") }},
			SearchField{Name: "targetref", Aliases: []string{"target", "targets"}, Extract: func(r *Row) string {
				var refs []string
				for _, s := range endpointsSubsets(r) {
					for _, a := range s.Addresses {
						if a.TargetRef != nil {
							refs = append(refs, a.TargetRef.Kind+"/"+a.TargetRef.Name)
						}
					}
				}
				return uniqueJoin(refs, " ")
			}},
		),
	}
}
