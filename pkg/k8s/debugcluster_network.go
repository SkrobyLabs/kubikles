//go:build debugcluster

package k8s

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// networkObjectsPerNamespace is how many of each network and RBAC kind (ingresses, endpoints,
// endpoint slices, roles, role bindings) a debug namespace holds. Like the policy objects, these
// lists exist to be looked at, not to be scaled.
const networkObjectsPerNamespace = 6

// generateNetworkObjects builds the namespaced network and RBAC objects of one debug namespace.
// Values vary with the index so sorting, filtering and search have something to chew on.
func generateNetworkObjects(nsName string) []runtime.Object {
	var out []runtime.Object
	meta := func(kind string, i int) metav1.ObjectMeta {
		name := fmt.Sprintf("debug-%s-%d", kind, i)
		return metav1.ObjectMeta{
			Name:              name,
			Namespace:         nsName,
			UID:               uid(kind, nsName, name),
			ResourceVersion:   nextRV(),
			CreationTimestamp: metav1.Now(),
			Labels:            map[string]string{"app": fmt.Sprintf("app-%d", i%3)},
		}
	}
	pathType := networkingv1.PathTypePrefix
	for i := range networkObjectsPerNamespace {
		class := "nginx"
		ing := &networkingv1.Ingress{ObjectMeta: meta("ing", i)}
		if i%3 != 0 {
			ing.Spec.IngressClassName = &class
		}
		if i%5 != 4 {
			ing.Spec.Rules = []networkingv1.IngressRule{{
				Host: fmt.Sprintf("app-%d.%s.example.com", i, nsName),
				IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
					Path: "/", PathType: &pathType,
					Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: fmt.Sprintf("svc-%d", i), Port: networkingv1.ServiceBackendPort{Number: 80}}},
				}}}},
			}}
		}
		if i%2 == 0 {
			ing.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{IP: fmt.Sprintf("34.1.%d.%d", i, i+1)}}
		}
		if i%3 == 1 {
			ing.Spec.TLS = []networkingv1.IngressTLS{{SecretName: "tls"}}
		}
		out = append(out, ing)

		ep := &corev1.Endpoints{ObjectMeta: meta("ep", i)}
		if i%4 != 3 {
			sub := corev1.EndpointSubset{
				Addresses: []corev1.EndpointAddress{{IP: fmt.Sprintf("10.1.%d.1", i)}},
				Ports:     []corev1.EndpointPort{{Port: int32(80 + i), Protocol: corev1.ProtocolTCP}},
			}
			for k := 0; k < i%3; k++ {
				sub.NotReadyAddresses = append(sub.NotReadyAddresses, corev1.EndpointAddress{IP: fmt.Sprintf("10.2.%d.%d", i, k)})
			}
			ep.Subsets = []corev1.EndpointSubset{sub}
		}
		out = append(out, ep)

		ready := i%2 == 0
		slice := &discoveryv1.EndpointSlice{
			ObjectMeta:  meta("eps", i),
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints:   []discoveryv1.Endpoint{{Addresses: []string{fmt.Sprintf("10.1.%d.1", i)}, Conditions: discoveryv1.EndpointConditions{Ready: &ready}}},
		}
		slice.Labels["kubernetes.io/service-name"] = fmt.Sprintf("svc-%d", i)
		port := int32(80 + i)
		proto := corev1.ProtocolTCP
		slice.Ports = []discoveryv1.EndpointPort{{Port: &port, Protocol: &proto}}
		out = append(out, slice)

		role := &rbacv1.Role{ObjectMeta: meta("role", i)}
		for k := 0; k < i%4; k++ {
			role.Rules = append(role.Rules, rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}})
		}
		out = append(out, role)

		rb := &rbacv1.RoleBinding{
			ObjectMeta: meta("rb", i),
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: fmt.Sprintf("debug-role-%d", i)},
		}
		for k := 0; k < i%3; k++ {
			rb.Subjects = append(rb.Subjects, rbacv1.Subject{Kind: "ServiceAccount", Name: fmt.Sprintf("debug-sa-%d", k), Namespace: nsName})
		}
		out = append(out, rb)
	}
	return out
}
