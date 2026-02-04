package k8s

import (
	"context"
	"fmt"
	"strings"

	authv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// RBACCheckRequest specifies what permission to check
type RBACCheckRequest struct {
	SubjectKind      string `json:"subjectKind"` // "ServiceAccount", "User", "Group"
	SubjectName      string `json:"subjectName"`
	SubjectNamespace string `json:"subjectNamespace"` // For ServiceAccounts only
	Verb             string `json:"verb"`             // "get", "list", "create", "delete", etc.
	Resource         string `json:"resource"`         // "pods", "deployments", etc.
	ResourceName     string `json:"resourceName"`     // Optional: specific resource name
	Namespace        string `json:"namespace"`        // Target namespace (empty = cluster-wide)
	APIGroup         string `json:"apiGroup"`         // Optional: API group (empty = core)
}

// RBACCheckResult contains the permission check result
type RBACCheckResult struct {
	Allowed bool            `json:"allowed"`
	Reason  string          `json:"reason"`
	Chain   []RBACChainLink `json:"chain"` // How permission was granted/denied
}

// RBACChainLink represents a step in the RBAC chain
type RBACChainLink struct {
	Kind      string `json:"kind"` // "ClusterRole", "Role", "ClusterRoleBinding", "RoleBinding"
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Grants    bool   `json:"grants"` // true if this link grants the permission
	Rule      string `json:"rule"`   // The specific rule that matched
}

// CheckRBACAccess checks if a subject has permission to perform an action
func (c *Client) CheckRBACAccess(req RBACCheckRequest) (*RBACCheckResult, error) {
	result := &RBACCheckResult{
		Chain: []RBACChainLink{},
	}

	// Build impersonation config
	config, err := c.getConfigForRBAC()
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	// Create impersonation config
	impConfig := rest.CopyConfig(config)

	switch req.SubjectKind {
	case "ServiceAccount":
		if req.SubjectNamespace == "" {
			req.SubjectNamespace = "default"
		}
		impConfig.Impersonate = rest.ImpersonationConfig{
			UserName: fmt.Sprintf("system:serviceaccount:%s:%s", req.SubjectNamespace, req.SubjectName),
		}
	case "User":
		impConfig.Impersonate = rest.ImpersonationConfig{
			UserName: req.SubjectName,
		}
	case "Group":
		impConfig.Impersonate = rest.ImpersonationConfig{
			Groups: []string{req.SubjectName},
		}
	default:
		return nil, fmt.Errorf("invalid subject kind: %s", req.SubjectKind)
	}

	// Create impersonated client
	impClient, err := kubernetes.NewForConfig(impConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create impersonated client: %w", err)
	}

	// Create SelfSubjectAccessReview
	sar := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace: req.Namespace,
				Verb:      req.Verb,
				Resource:  req.Resource,
				Name:      req.ResourceName,
				Group:     req.APIGroup,
			},
		},
	}

	ctx := context.Background()
	review, err := impClient.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		// If we can't check via impersonation, try to trace the chain manually
		result.Allowed = false
		result.Reason = fmt.Sprintf("Failed to check access: %v", err)

		// Still try to trace the chain for explanation
		chain := c.traceRBACChain(req)
		result.Chain = chain

		return result, nil
	}

	result.Allowed = review.Status.Allowed
	result.Reason = review.Status.Reason
	if result.Reason == "" {
		if result.Allowed {
			result.Reason = "Access granted"
		} else {
			result.Reason = review.Status.EvaluationError
			if result.Reason == "" {
				result.Reason = "Access denied"
			}
		}
	}

	// Trace the chain to explain why
	result.Chain = c.traceRBACChain(req)

	return result, nil
}

// traceRBACChain finds which roles/bindings grant or deny the permission
func (c *Client) traceRBACChain(req RBACCheckRequest) []RBACChainLink {
	var chain []RBACChainLink
	ctx := context.Background()

	cs, err := c.getClientset()
	if err != nil {
		return chain
	}

	// Build subject matcher
	subjectMatcher := func(subjects []rbacv1.Subject) bool {
		for _, s := range subjects {
			switch req.SubjectKind {
			case "ServiceAccount":
				if s.Kind == "ServiceAccount" && s.Name == req.SubjectName {
					if s.Namespace == "" || s.Namespace == req.SubjectNamespace {
						return true
					}
				}
			case "User":
				if s.Kind == "User" && s.Name == req.SubjectName {
					return true
				}
			case "Group":
				if s.Kind == "Group" && s.Name == req.SubjectName {
					return true
				}
			}
		}
		return false
	}

	// Check ClusterRoleBindings
	crbs, err := cs.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, crb := range crbs.Items {
			if subjectMatcher(crb.Subjects) {
				// Check if the referenced ClusterRole grants the permission
				if crb.RoleRef.Kind == "ClusterRole" {
					cr, err := cs.RbacV1().ClusterRoles().Get(ctx, crb.RoleRef.Name, metav1.GetOptions{})
					if err == nil {
						grants, rule := roleGrantsAccess(cr.Rules, req)
						chain = append(chain, RBACChainLink{
							Kind:   "ClusterRoleBinding",
							Name:   crb.Name,
							Grants: grants,
							Rule:   fmt.Sprintf("binds to ClusterRole/%s", crb.RoleRef.Name),
						})
						if grants {
							chain = append(chain, RBACChainLink{
								Kind:   "ClusterRole",
								Name:   cr.Name,
								Grants: true,
								Rule:   rule,
							})
						}
					}
				}
			}
		}
	}

	// Check RoleBindings in the target namespace
	if req.Namespace != "" {
		rbs, err := cs.RbacV1().RoleBindings(req.Namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, rb := range rbs.Items {
				if subjectMatcher(rb.Subjects) {
					var grants bool
					var rule string

					if rb.RoleRef.Kind == "ClusterRole" {
						cr, err := cs.RbacV1().ClusterRoles().Get(ctx, rb.RoleRef.Name, metav1.GetOptions{})
						if err == nil {
							grants, rule = roleGrantsAccess(cr.Rules, req)
						}
					} else if rb.RoleRef.Kind == "Role" {
						r, err := cs.RbacV1().Roles(req.Namespace).Get(ctx, rb.RoleRef.Name, metav1.GetOptions{})
						if err == nil {
							grants, rule = roleGrantsAccess(r.Rules, req)
						}
					}

					chain = append(chain, RBACChainLink{
						Kind:      "RoleBinding",
						Name:      rb.Name,
						Namespace: rb.Namespace,
						Grants:    grants,
						Rule:      fmt.Sprintf("binds to %s/%s", rb.RoleRef.Kind, rb.RoleRef.Name),
					})

					if grants {
						chain = append(chain, RBACChainLink{
							Kind:      rb.RoleRef.Kind,
							Name:      rb.RoleRef.Name,
							Namespace: req.Namespace,
							Grants:    true,
							Rule:      rule,
						})
					}
				}
			}
		}
	}

	return chain
}

// roleGrantsAccess checks if a set of policy rules grants access for the request
func roleGrantsAccess(rules []rbacv1.PolicyRule, req RBACCheckRequest) (bool, string) {
	for _, rule := range rules {
		// Check API groups
		groupMatch := false
		for _, g := range rule.APIGroups {
			if g == "*" || g == req.APIGroup {
				groupMatch = true
				break
			}
		}
		if !groupMatch && len(rule.APIGroups) > 0 {
			continue
		}

		// Check resources
		resourceMatch := false
		for _, r := range rule.Resources {
			if r == "*" || r == req.Resource {
				resourceMatch = true
				break
			}
		}
		if !resourceMatch {
			continue
		}

		// Check verbs
		verbMatch := false
		for _, v := range rule.Verbs {
			if v == "*" || v == req.Verb {
				verbMatch = true
				break
			}
		}
		if !verbMatch {
			continue
		}

		// Check resource names (if specified)
		if req.ResourceName != "" && len(rule.ResourceNames) > 0 {
			nameMatch := false
			for _, n := range rule.ResourceNames {
				if n == req.ResourceName {
					nameMatch = true
					break
				}
			}
			if !nameMatch {
				continue
			}
		}

		// All checks passed
		ruleStr := formatPolicyRule(rule)
		return true, ruleStr
	}

	return false, ""
}

// formatPolicyRule formats a policy rule for display
func formatPolicyRule(rule rbacv1.PolicyRule) string {
	parts := []string{}

	if len(rule.APIGroups) > 0 {
		parts = append(parts, fmt.Sprintf("apiGroups: %v", rule.APIGroups))
	}
	if len(rule.Resources) > 0 {
		parts = append(parts, fmt.Sprintf("resources: %v", rule.Resources))
	}
	if len(rule.Verbs) > 0 {
		parts = append(parts, fmt.Sprintf("verbs: %v", rule.Verbs))
	}
	if len(rule.ResourceNames) > 0 {
		parts = append(parts, fmt.Sprintf("resourceNames: %v", rule.ResourceNames))
	}

	return strings.Join(parts, ", ")
}

// getConfigForRBAC returns the current kubeconfig for RBAC checks
func (c *Client) getConfigForRBAC() (*rest.Config, error) {
	// Use empty context name to get current context's config
	return c.GetRestConfigForContext("")
}
