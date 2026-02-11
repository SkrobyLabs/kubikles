package issuedetector

import (
	"context"
	"fmt"
	"strings"
)

func rbacRules() []Rule {
	return []Rule{
		&ruleRBAC001{baseRule: baseRule{id: "RBAC001", name: "Unused Role", description: "Role is not referenced by any RoleBinding in the same namespace", severity: SeverityInfo, category: CategorySecurity, requires: []string{"roles", "rolebindings"}}},
		&ruleRBAC002{baseRule: baseRule{id: "RBAC002", name: "Unused ClusterRole", description: "ClusterRole is not referenced by any ClusterRoleBinding or RoleBinding", severity: SeverityInfo, category: CategorySecurity, requires: []string{"clusterroles", "clusterrolebindings", "rolebindings"}}},
	}
}

// systemNamespaces is the set of namespaces skipped for RBAC unused checks.
var systemNamespaces = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
}

// RBAC001: Unused Role
type ruleRBAC001 struct{ baseRule }

func (r *ruleRBAC001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	roles := cache.Roles()
	bindings := cache.RoleBindings()

	// Build set of bound role names per namespace: "ns/name" -> true
	boundRoles := make(map[string]bool)
	for _, rb := range bindings {
		if rb.RoleRef.Kind == "Role" {
			boundRoles[rb.Namespace+"/"+rb.RoleRef.Name] = true
		}
	}

	var findings []Finding
	for _, role := range roles {
		if systemNamespaces[role.Namespace] {
			continue
		}
		key := role.Namespace + "/" + role.Name
		if !boundRoles[key] {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Role", Name: role.Name, Namespace: role.Namespace},
				fmt.Sprintf("Role '%s' in namespace '%s' is not referenced by any RoleBinding", role.Name, role.Namespace),
				"Remove the unused Role or create a RoleBinding that references it",
				map[string]string{
					"roleName":  role.Name,
					"namespace": role.Namespace,
				},
			))
		}
	}
	return findings, nil
}

// RBAC002: Unused ClusterRole
type ruleRBAC002 struct{ baseRule }

func (r *ruleRBAC002) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	clusterRoles := cache.ClusterRoles()
	clusterBindings := cache.ClusterRoleBindings()
	roleBindings := cache.RoleBindings()

	// Build set of bound ClusterRole names
	boundClusterRoles := make(map[string]bool)
	for _, crb := range clusterBindings {
		if crb.RoleRef.Kind == "ClusterRole" {
			boundClusterRoles[crb.RoleRef.Name] = true
		}
	}
	// RoleBindings can also reference ClusterRoles
	for _, rb := range roleBindings {
		if rb.RoleRef.Kind == "ClusterRole" {
			boundClusterRoles[rb.RoleRef.Name] = true
		}
	}

	var findings []Finding
	for _, cr := range clusterRoles {
		if strings.HasPrefix(cr.Name, "system:") {
			continue
		}
		if !boundClusterRoles[cr.Name] {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "ClusterRole", Name: cr.Name},
				fmt.Sprintf("ClusterRole '%s' is not referenced by any ClusterRoleBinding or RoleBinding", cr.Name),
				"Remove the unused ClusterRole or create a binding that references it",
				map[string]string{
					"clusterRoleName": cr.Name,
				},
			))
		}
	}
	return findings, nil
}
