package rows

import (
	"strconv"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(ingressesSpec()) }

const ingressClassAnnotation = "kubernetes.io/ingress.class"

func ingressObj(r *Row) *networkingv1.Ingress {
	i, _ := r.E.Obj.(*networkingv1.Ingress)
	return i
}

func ingressRules(r *Row) []networkingv1.IngressRule {
	if i := ingressObj(r); i != nil {
		return i.Spec.Rules
	}
	return nil
}

func ingressLB(r *Row) []networkingv1.IngressLoadBalancerIngress {
	if i := ingressObj(r); i != nil {
		return i.Status.LoadBalancer.Ingress
	}
	return nil
}

func ingressHosts(r *Row) string {
	var hosts []string
	for _, rule := range ingressRules(r) {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
	}
	if len(hosts) == 0 {
		return "*"
	}
	return strings.Join(hosts, ", ")
}

// ingressPaths is the first three paths (an empty path reads as "/") with "..." for more, or the dash.
func ingressPaths(r *Row) string {
	var paths []string
	for _, rule := range ingressRules(r) {
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			paths = append(paths, orDefault(p.Path, "/"))
		}
	}
	if len(paths) == 0 {
		return "-"
	}
	shown := paths
	if len(shown) > 3 {
		shown = shown[:3]
	}
	out := strings.Join(shown, ", ")
	if len(paths) > 3 {
		out += "..."
	}
	return out
}

func ingressClassOf(r *Row) string {
	if i := ingressObj(r); i != nil && i.Spec.IngressClassName != nil && *i.Spec.IngressClassName != "" {
		return *i.Spec.IngressClassName
	}
	if r.M != nil {
		if v := r.M.GetAnnotations()[ingressClassAnnotation]; v != "" {
			return v
		}
	}
	return "-"
}

func ingressAddress(r *Row) string {
	var addrs []string
	for _, lb := range ingressLB(r) {
		if a := orDefault(lb.IP, lb.Hostname); a != "" {
			addrs = append(addrs, a)
		}
	}
	if len(addrs) == 0 {
		return "-"
	}
	return strings.Join(addrs, ", ")
}

func ingressStatus(r *Row) string {
	for _, lb := range ingressLB(r) {
		if lb.IP != "" || lb.Hostname != "" {
			return "Active"
		}
	}
	if len(ingressRules(r)) == 0 {
		if i := ingressObj(r); i == nil || i.Spec.DefaultBackend == nil {
			return "No Rules"
		}
	}
	return "Pending"
}

func ingressTLSCount(r *Row) int {
	if i := ingressObj(r); i != nil {
		return len(i.Spec.TLS)
	}
	return 0
}

func ingressDefaultBackend(r *Row) *networkingv1.IngressBackend {
	if i := ingressObj(r); i != nil {
		return i.Spec.DefaultBackend
	}
	return nil
}

// ingressDefaultBackendTarget is empty without a default backend, the dash for a backend that is
// not a service, else "service:port" (a port with neither number nor name prints "undefined").
func ingressDefaultBackendTarget(r *Row) string {
	b := ingressDefaultBackend(r)
	if b == nil {
		return ""
	}
	if b.Service == nil {
		return "-"
	}
	port := "undefined"
	if b.Service.Port.Number != 0 {
		port = strconv.Itoa(int(b.Service.Port.Number))
	} else if b.Service.Port.Name != "" {
		port = b.Service.Port.Name
	}
	return b.Service.Name + ":" + port
}

func ingressesSpec() *Spec {
	str := func(key string, f func(r *Row) string) Column {
		return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return f(r) }}
	}
	return &Spec{
		Type:       "ingresses",
		Namespaced: true,
		Needs:      store.ModeFull, // rules, class and load balancer status are not metadata
		Notes:      "The tlsCount and defaultBackendTarget columns exist only in view mode (hidden by default): the TLS cell reads Yes (n) and the default backend cell reads service:port, while the sort and filter values are Yes/No and the service name.",
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			str("status", ingressStatus),
			str("class", ingressClassOf),
			str("hosts", ingressHosts),
			str("address", ingressAddress),
			ageColumn(creationAge),
			str("paths", ingressPaths),
			str("tls", func(r *Row) string {
				if ingressTLSCount(r) > 0 {
					return "Yes"
				}
				return "No"
			}),
			// Not a column of the legacy list: the TLS cell reads "Yes (n)" and needs the count.
			{Key: "tlsCount", Kind: KindNumber, CellType: "number", FilterType: FilterNone,
				Cell: func(r *Row) interface{} { return numberCell(ingressTLSCount(r)) }},
			str("defaultBackend", func(r *Row) string {
				if b := ingressDefaultBackend(r); b != nil && b.Service != nil {
					return b.Service.Name
				}
				return ""
			}),
			// Not a column of the legacy list: the default backend cell reads "service:port".
			{Key: "defaultBackendTarget", Kind: KindString, CellType: "string", FilterType: FilterNone,
				Cell: func(r *Row) interface{} { return ingressDefaultBackendTarget(r) }},
			{Key: "rules", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(len(ingressRules(r))) }},
		},
		// The legacy list has no field mapping for this type, so only the common fields apply.
		Search: commonSearchFields(),
	}
}
