package rows

import (
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(servicesSpec()) }

func serviceObj(r *Row) *corev1.Service {
	s, _ := r.E.Obj.(*corev1.Service)
	return s
}

func servicePorts(r *Row) []corev1.ServicePort {
	if s := serviceObj(r); s != nil {
		return s.Spec.Ports
	}
	return nil
}

// serviceLBIngress is status.loadBalancer.ingress.
func serviceLBIngress(r *Row) []corev1.LoadBalancerIngress {
	if s := serviceObj(r); s != nil {
		return s.Status.LoadBalancer.Ingress
	}
	return nil
}

// servicePortsDisplay is "port/protocol" per port joined with ", ". A port without a protocol
// prints the JavaScript "undefined" of the legacy template string.
func servicePortsDisplay(r *Row) string {
	ports := servicePorts(r)
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = strconv.Itoa(int(p.Port)) + "/" + orDefault(string(p.Protocol), "undefined")
	}
	return strings.Join(parts, ", ")
}

func serviceSpecString(r *Row, pick func(*corev1.Service) string) string {
	if s := serviceObj(r); s != nil {
		return pick(s)
	}
	return ""
}

func serviceSelector(r *Row) map[string]string {
	if s := serviceObj(r); s != nil {
		return s.Spec.Selector
	}
	return nil
}

// serviceSelectorText is the selector as "k=v" pairs joined with sep, in JavaScript key order.
func serviceSelectorText(r *Row, sep string) string {
	sel := serviceSelector(r)
	keys := jsKeys(sel)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + sel[k]
	}
	return strings.Join(parts, sep)
}

// serviceLBAddresses is the IP, else the hostname, of every load balancer entry that has one.
func serviceLBAddresses(r *Row) []string {
	var out []string
	for _, in := range serviceLBIngress(r) {
		if a := orDefault(in.IP, in.Hostname); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func serviceString(key string, f func(r *Row) string) Column {
	return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
		Cell: func(r *Row) interface{} { return f(r) }}
}

func serviceNullableString(key string, predefined []string, f func(r *Row) string) Column {
	return Column{Key: key, Kind: KindString, CellType: "string | null", FilterType: FilterSelect, Predefined: predefined,
		Cell: func(r *Row) interface{} { return stringOrNil(f(r)) }}
}

func servicesSpec() *Spec {
	return &Spec{
		Type:       "services",
		Namespaced: true,
		Needs:      store.ModeFull, // spec and load balancer status are not metadata
		Notes:      "The externalAddresses column exists only in view mode (hidden by default): the external IP cell is the first IP (its sort and filter value) while the legacy display lists every IP or hostname. Sorting by a column that is absent on some rows (clusterIP and type of ExternalName services) cannot match the legacy order, whose comparator is inconsistent for undefined keys; the view orders those rows by age and name.",
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			serviceNullableString("type", []string{"ClusterIP", "NodePort", "LoadBalancer", "ExternalName"}, func(r *Row) string {
				return serviceSpecString(r, func(s *corev1.Service) string { return string(s.Spec.Type) })
			}),
			serviceNullableString("clusterIP", nil, func(r *Row) string {
				return serviceSpecString(r, func(s *corev1.Service) string { return s.Spec.ClusterIP })
			}),
			serviceString("ports", servicePortsDisplay),
			ageColumn(creationAge),
			serviceString("externalIP", func(r *Row) string {
				if in := serviceLBIngress(r); len(in) > 0 {
					return in[0].IP
				}
				return ""
			}),
			// Not a column of the legacy list: its external IP cell lists every load balancer
			// address (IP or hostname). Hidden in the column picker by default.
			{Key: "externalAddresses", Kind: KindString, CellType: "string", FilterType: FilterNone,
				Cell: func(r *Row) interface{} { return strings.Join(serviceLBAddresses(r), ", ") }},
			serviceString("selector", func(r *Row) string { return serviceSelectorText(r, ",") }),
			serviceString("sessionAffinity", func(r *Row) string {
				return orDefault(serviceSpecString(r, func(s *corev1.Service) string { return string(s.Spec.SessionAffinity) }), "None")
			}),
			serviceString("externalName", func(r *Row) string {
				return serviceSpecString(r, func(s *corev1.Service) string { return s.Spec.ExternalName })
			}),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "type", Aliases: []string{"servicetype"}, Extract: func(r *Row) string {
				return orDefault(serviceSpecString(r, func(s *corev1.Service) string { return string(s.Spec.Type) }), "ClusterIP")
			}},
			SearchField{Name: "clusterip", Aliases: []string{"ip", "cluster-ip"}, Extract: func(r *Row) string {
				return serviceSpecString(r, func(s *corev1.Service) string { return s.Spec.ClusterIP })
			}},
			SearchField{Name: "externalip", Aliases: []string{"external-ip", "extip"}, Extract: func(r *Row) string {
				if s := serviceObj(r); s != nil {
					return strings.Join(s.Spec.ExternalIPs, " ")
				}
				return ""
			}},
			SearchField{Name: "loadbalancerip", Aliases: []string{"lbip", "loadbalancer"}, Extract: func(r *Row) string {
				// Entries with neither address contribute an empty element, as join does for undefined.
				ingress := serviceLBIngress(r)
				parts := make([]string, len(ingress))
				for i, in := range ingress {
					parts[i] = orDefault(in.IP, in.Hostname)
				}
				return strings.Join(parts, " ")
			}},
			SearchField{Name: "port", Aliases: []string{"ports"}, Extract: func(r *Row) string {
				ports := servicePorts(r)
				parts := make([]string, len(ports))
				for i, p := range ports {
					parts[i] = strconv.Itoa(int(p.Port))
				}
				return strings.Join(parts, " ")
			}},
			SearchField{Name: "targetport", Aliases: []string{"targetports"}, Extract: func(r *Row) string {
				ports := servicePorts(r)
				parts := make([]string, len(ports))
				for i, p := range ports {
					parts[i] = p.TargetPort.String()
				}
				return strings.Join(parts, " ")
			}},
			SearchField{Name: "nodeport", Aliases: []string{"nodeports"}, Extract: func(r *Row) string {
				var parts []string
				for _, p := range servicePorts(r) {
					if p.NodePort != 0 {
						parts = append(parts, strconv.Itoa(int(p.NodePort)))
					}
				}
				return strings.Join(parts, " ")
			}},
			SearchField{Name: "protocol", Aliases: []string{"protocols"}, Extract: func(r *Row) string {
				ports := servicePorts(r)
				parts := make([]string, len(ports))
				for i, p := range ports {
					parts[i] = string(p.Protocol)
				}
				return strings.Join(parts, " ")
			}},
			SearchField{Name: "selector", Aliases: []string{"sel"}, Extract: func(r *Row) string { return serviceSelectorText(r, " ") }},
			SearchField{Name: "sessionaffinity", Aliases: []string{"affinity", "session"}, Extract: func(r *Row) string {
				return orDefault(serviceSpecString(r, func(s *corev1.Service) string { return string(s.Spec.SessionAffinity) }), "None")
			}},
			SearchField{Name: "externalname", Aliases: []string{"extname"}, Extract: func(r *Row) string {
				return serviceSpecString(r, func(s *corev1.Service) string { return s.Spec.ExternalName })
			}},
			SearchField{Name: "headless", Extract: func(r *Row) string {
				if serviceSpecString(r, func(s *corev1.Service) string { return s.Spec.ClusterIP }) == "None" {
					return "true"
				}
				return "false"
			}},
		),
	}
}
