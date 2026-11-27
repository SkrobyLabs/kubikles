package rows

import (
	"strconv"
	"strings"

	discoveryv1 "k8s.io/api/discovery/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(endpointSlicesSpec()) }

const endpointSliceServiceLabel = "kubernetes.io/service-name"

func endpointSliceObj(r *Row) *discoveryv1.EndpointSlice {
	s, _ := r.E.Obj.(*discoveryv1.EndpointSlice)
	return s
}

func endpointSliceCount(r *Row) int {
	if s := endpointSliceObj(r); s != nil {
		return len(s.Endpoints)
	}
	return 0
}

// endpointSliceReady counts endpoints whose ready condition is explicitly true.
func endpointSliceReady(r *Row) int {
	s := endpointSliceObj(r)
	if s == nil {
		return 0
	}
	n := 0
	for _, e := range s.Endpoints {
		if e.Conditions.Ready != nil && *e.Conditions.Ready {
			n++
		}
	}
	return n
}

func endpointSlicePorts(r *Row) string {
	s := endpointSliceObj(r)
	if s == nil || len(s.Ports) == 0 {
		return "-"
	}
	shown := s.Ports
	if len(shown) > 3 {
		shown = shown[:3]
	}
	parts := make([]string, len(shown))
	for i, p := range shown {
		// A missing port prints as the JavaScript "undefined" in the legacy template string.
		port := "undefined"
		if p.Port != nil {
			port = strconv.Itoa(int(*p.Port))
		}
		proto := "TCP"
		if p.Protocol != nil && *p.Protocol != "" {
			proto = string(*p.Protocol)
		}
		parts[i] = port + "/" + proto
	}
	out := strings.Join(parts, ", ")
	if len(s.Ports) > 3 {
		out += " +" + itoa(len(s.Ports)-3)
	}
	return out
}

func endpointSliceService(r *Row) string {
	if r.M != nil {
		if v := r.M.GetLabels()[endpointSliceServiceLabel]; v != "" {
			return v
		}
	}
	return "-"
}

func endpointSlicesSpec() *Spec {
	return &Spec{
		Type:       "endpointslices",
		Namespaced: true,
		Needs:      store.ModeFull, // endpoints and ports are not metadata
		Notes:      "The readyCount column exists only in view mode (hidden by default): the endpoints cell reads ready/total and needs the ready half. Needs the discovery.k8s.io endpointslices entry of the k8s resource table.",
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			{Key: "service", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return endpointSliceService(r) }},
			{Key: "addressType", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} {
					if s := endpointSliceObj(r); s != nil {
						return string(s.AddressType)
					}
					return ""
				}},
			{Key: "endpoints", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(endpointSliceCount(r)) }},
			{Key: "ports", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return endpointSlicePorts(r) }},
			// Not a column of the legacy list: the endpoints cell reads "ready/total" and needs the
			// ready half. Hidden in the column picker by default.
			{Key: "readyCount", Kind: KindNumber, CellType: "number", FilterType: FilterNone,
				Cell: func(r *Row) interface{} { return numberCell(endpointSliceReady(r)) }},
			ageColumn(creationAge),
		},
		// The legacy list has no field mapping for this type, so only the common fields apply.
		Search: commonSearchFields(),
	}
}
