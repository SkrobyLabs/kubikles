package issuedetector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// yamlRuleDef is the on-disk format for a YAML rule file.
type yamlRuleDef struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Severity    Severity  `json:"severity"`
	Category    Category  `json:"category"`
	Requires    []string  `json:"requires"`
	Check       yamlCheck `json:"check"`
}

type yamlCheck struct {
	Type             string         `json:"type"` // resourceExists, fieldNotEmpty, statusCheck, orphanCheck, fieldMatch, resourceCount
	Resource         string         `json:"resource"`
	Field            string         `json:"field,omitempty"`
	ReferenceField   string         `json:"referenceField,omitempty"`
	TargetResource   string         `json:"targetResource,omitempty"`
	TargetMatchField string         `json:"targetMatchField,omitempty"`
	ReferencedBy     string         `json:"referencedBy,omitempty"`
	MatchField       string         `json:"matchField,omitempty"`
	Condition        *yamlCondition `json:"condition,omitempty"`
	Operator         string         `json:"operator,omitempty"` // regex, equals, notEquals
	Value            string         `json:"value,omitempty"`
	Threshold        int            `json:"threshold,omitempty"` // for resourceCount: emit finding if count > threshold (default 0 = any exist)
	Message          string         `json:"message"`
	SuggestedFix     string         `json:"suggestedFix,omitempty"`
}

type yamlCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

// LoadYAMLRules reads all .yaml/.yml files from dir and returns parsed rules.
func LoadYAMLRules(dir string) ([]Rule, error) {
	if dir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading rules directory: %w", err)
	}

	var rules []Rule
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("[IssueDetector] Error reading %s: %v", path, err)
			continue
		}

		var def yamlRuleDef
		if err := yaml.Unmarshal(data, &def); err != nil {
			log.Printf("[IssueDetector] Error parsing %s: %v", path, err)
			continue
		}

		if def.ID == "" || def.Name == "" || def.Check.Type == "" {
			log.Printf("[IssueDetector] Skipping %s: missing required fields (id, name, check.type)", path)
			continue
		}

		rules = append(rules, &declarativeRule{def: def})
	}

	return rules, nil
}

// declarativeRule implements Rule using a YAML definition.
type declarativeRule struct {
	def yamlRuleDef
}

func (r *declarativeRule) ID() string                  { return r.def.ID }
func (r *declarativeRule) Name() string                { return r.def.Name }
func (r *declarativeRule) Description() string         { return r.def.Description }
func (r *declarativeRule) Severity() Severity          { return r.def.Severity }
func (r *declarativeRule) Category() Category          { return r.def.Category }
func (r *declarativeRule) RequiredResources() []string { return r.def.Requires }

func (r *declarativeRule) Evaluate(ctx context.Context, cache *ResourceCache) ([]Finding, error) {
	switch r.def.Check.Type {
	case "resourceExists":
		return r.evalResourceExists(cache)
	case "fieldNotEmpty":
		return r.evalFieldNotEmpty(cache)
	case "statusCheck":
		return r.evalStatusCheck(cache)
	case "orphanCheck":
		return r.evalOrphanCheck(cache)
	case "fieldMatch":
		return r.evalFieldMatch(cache)
	case "resourceCount":
		return r.evalResourceCount(cache)
	default:
		return nil, fmt.Errorf("unknown check type: %s", r.def.Check.Type)
	}
}

// ---- Check evaluators ----

func (r *declarativeRule) evalResourceExists(cache *ResourceCache) ([]Finding, error) {
	resources := getResourcesAsJSON(cache, r.def.Check.Resource)
	targets := getResourcesAsJSON(cache, r.def.Check.TargetResource)

	// Build target lookup
	targetNames := make(map[string]bool)
	for _, t := range targets {
		name := extractField(t, r.def.Check.TargetMatchField)
		if name != "" {
			targetNames[name] = true
		}
	}

	var findings []Finding
	for _, res := range resources {
		refs := extractFieldAll(res, r.def.Check.ReferenceField)
		name := extractField(res, ".metadata.name")
		ns := extractField(res, ".metadata.namespace")

		for _, refVal := range refs {
			if refVal != "" && !targetNames[refVal] {
				msg := expandTemplate(r.def.Check.Message, map[string]string{
					"Name": name, "Namespace": ns, "RefValue": refVal,
				})
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: kindFromResource(r.def.Check.Resource), Name: name, Namespace: ns},
					msg, r.def.Check.SuggestedFix, nil,
				))
			}
		}
	}
	return findings, nil
}

func (r *declarativeRule) evalFieldNotEmpty(cache *ResourceCache) ([]Finding, error) {
	resources := getResourcesAsJSON(cache, r.def.Check.Resource)
	var findings []Finding

	for _, res := range resources {
		vals := extractFieldAll(res, r.def.Check.Field)
		name := extractField(res, ".metadata.name")
		ns := extractField(res, ".metadata.namespace")

		hasEmpty := false
		for _, v := range vals {
			if v == "" || v == "null" || v == "<nil>" {
				hasEmpty = true
				break
			}
		}
		// If no values at all, that's also "empty"
		if len(vals) == 0 {
			hasEmpty = true
		}

		if hasEmpty {
			msg := expandTemplate(r.def.Check.Message, map[string]string{
				"Name": name, "Namespace": ns,
			})
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: kindFromResource(r.def.Check.Resource), Name: name, Namespace: ns},
				msg, r.def.Check.SuggestedFix, nil,
			))
		}
	}
	return findings, nil
}

func (r *declarativeRule) evalStatusCheck(cache *ResourceCache) ([]Finding, error) {
	resources := getResourcesAsJSON(cache, r.def.Check.Resource)
	var findings []Finding

	if r.def.Check.Condition == nil {
		return nil, fmt.Errorf("statusCheck requires condition")
	}

	for _, res := range resources {
		name := extractField(res, ".metadata.name")
		ns := extractField(res, ".metadata.namespace")

		// Look at .status.conditions[]
		conditionsRaw := extractFieldRaw(res, ".status.conditions")
		if conditionsRaw == nil {
			continue
		}

		conditions, ok := conditionsRaw.([]interface{})
		if !ok {
			continue
		}

		for _, condRaw := range conditions {
			cond, ok := condRaw.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)

			if condType == r.def.Check.Condition.Type && condStatus != r.def.Check.Condition.Status {
				msg := expandTemplate(r.def.Check.Message, map[string]string{
					"Name": name, "Namespace": ns,
					"ConditionType": condType, "ConditionStatus": condStatus,
				})
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: kindFromResource(r.def.Check.Resource), Name: name, Namespace: ns},
					msg, r.def.Check.SuggestedFix, nil,
				))
			}
		}
	}
	return findings, nil
}

func (r *declarativeRule) evalOrphanCheck(cache *ResourceCache) ([]Finding, error) {
	resources := getResourcesAsJSON(cache, r.def.Check.Resource)
	referencers := getResourcesAsJSON(cache, r.def.Check.ReferencedBy)

	// Build set of referenced names (within same namespace for namespaced resources)
	type nsName struct{ ns, name string }
	referenced := make(map[nsName]bool)

	for _, ref := range referencers {
		refNs := extractField(ref, ".metadata.namespace")
		vals := extractFieldAll(ref, r.def.Check.MatchField)
		for _, v := range vals {
			if v != "" {
				referenced[nsName{refNs, v}] = true
			}
		}
	}

	var findings []Finding
	for _, res := range resources {
		name := extractField(res, ".metadata.name")
		ns := extractField(res, ".metadata.namespace")

		if !referenced[nsName{ns, name}] {
			msg := expandTemplate(r.def.Check.Message, map[string]string{
				"Name": name, "Namespace": ns,
			})
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: kindFromResource(r.def.Check.Resource), Name: name, Namespace: ns},
				msg, r.def.Check.SuggestedFix, nil,
			))
		}
	}
	return findings, nil
}

func (r *declarativeRule) evalFieldMatch(cache *ResourceCache) ([]Finding, error) {
	resources := getResourcesAsJSON(cache, r.def.Check.Resource)
	var findings []Finding

	var re *regexp.Regexp
	if r.def.Check.Operator == "regex" {
		var err error
		re, err = regexp.Compile(r.def.Check.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid regex %q: %w", r.def.Check.Value, err)
		}
	}

	for _, res := range resources {
		vals := extractFieldAll(res, r.def.Check.Field)
		name := extractField(res, ".metadata.name")
		ns := extractField(res, ".metadata.namespace")

		matched := false
		for _, v := range vals {
			switch r.def.Check.Operator {
			case "regex":
				if re != nil && re.MatchString(v) {
					matched = true
				}
			case "equals":
				if v == r.def.Check.Value {
					matched = true
				}
			case "notEquals":
				if v != r.def.Check.Value {
					matched = true
				}
			default:
				if v == r.def.Check.Value {
					matched = true
				}
			}
			if matched {
				break
			}
		}

		if matched {
			msg := expandTemplate(r.def.Check.Message, map[string]string{
				"Name": name, "Namespace": ns,
			})
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: kindFromResource(r.def.Check.Resource), Name: name, Namespace: ns},
				msg, r.def.Check.SuggestedFix, nil,
			))
		}
	}
	return findings, nil
}

func (r *declarativeRule) evalResourceCount(cache *ResourceCache) ([]Finding, error) {
	resources := getResourcesAsJSON(cache, r.def.Check.Resource)
	count := len(resources)

	if count <= r.def.Check.Threshold {
		return nil, nil
	}

	msg := expandTemplate(r.def.Check.Message, map[string]string{
		"Count": fmt.Sprintf("%d", count),
	})
	return []Finding{makeFinding(r,
		ResourceRef{Kind: kindFromResource(r.def.Check.Resource), Name: fmt.Sprintf("(%d resources)", count)},
		msg, r.def.Check.SuggestedFix, nil,
	)}, nil
}

// ---- Helpers ----

// getResourcesAsJSON converts typed K8s objects to []map[string]interface{} via JSON marshal.
func getResourcesAsJSON(cache *ResourceCache, kind string) []map[string]interface{} {
	var raw interface{}
	switch kind {
	case "pods":
		raw = cache.Pods()
	case "services":
		raw = cache.Services()
	case "ingresses":
		raw = cache.Ingresses()
	case "ingressclasses":
		raw = cache.IngressClasses()
	case "endpoints":
		raw = cache.Endpoints()
	case "configmaps":
		raw = cache.ConfigMaps()
	case "secrets":
		raw = cache.Secrets()
	case "deployments":
		raw = cache.Deployments()
	case "statefulsets":
		raw = cache.StatefulSets()
	case "daemonsets":
		raw = cache.DaemonSets()
	case "pvcs":
		raw = cache.PVCs()
	case "pvs":
		raw = cache.PVs()
	case "nodes":
		raw = cache.Nodes()
	case "serviceaccounts":
		raw = cache.ServiceAccounts()
	case "hpas":
		raw = cache.HPAs()
	default:
		return nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	return items
}

// extractField walks a dot-path like ".metadata.name" and returns the string value.
func extractField(obj map[string]interface{}, path string) string {
	val := extractFieldRaw(obj, path)
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// extractFieldAll handles paths with [*] wildcards and returns all matching string values.
func extractFieldAll(obj map[string]interface{}, path string) []string {
	return walkPath(obj, parsePath(path))
}

// extractFieldRaw returns the raw value at a simple dot-path (no wildcards).
func extractFieldRaw(obj map[string]interface{}, path string) interface{} {
	parts := parsePath(path)
	var current interface{} = obj

	for _, part := range parts {
		if part == "[*]" {
			return current // Can't resolve wildcard to single value
		}
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = m[part]
		if current == nil {
			return nil
		}
	}
	return current
}

// parsePath splits ".spec.containers[*].name" into ["spec", "containers", "[*]", "name"]
func parsePath(path string) []string {
	path = strings.TrimPrefix(path, ".")
	var parts []string
	for _, p := range strings.Split(path, ".") {
		if strings.Contains(p, "[*]") {
			base := strings.Replace(p, "[*]", "", 1)
			if base != "" {
				parts = append(parts, base)
			}
			parts = append(parts, "[*]")
		} else if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// walkPath walks through the object following the path parts, expanding [*] wildcards.
func walkPath(obj interface{}, parts []string) []string {
	if len(parts) == 0 {
		if obj == nil {
			return nil
		}
		switch v := obj.(type) {
		case string:
			return []string{v}
		case float64:
			return []string{fmt.Sprintf("%v", v)}
		case nil:
			return nil
		default:
			return []string{fmt.Sprintf("%v", v)}
		}
	}

	part := parts[0]
	rest := parts[1:]

	if part == "[*]" {
		arr, ok := obj.([]interface{})
		if !ok {
			return nil
		}
		var results []string
		for _, item := range arr {
			results = append(results, walkPath(item, rest)...)
		}
		return results
	}

	m, ok := obj.(map[string]interface{})
	if !ok {
		return nil
	}
	return walkPath(m[part], rest)
}

// expandTemplate replaces {{.Key}} placeholders in a message.
func expandTemplate(tmpl string, vars map[string]string) string {
	result := tmpl
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{."+k+"}}", v)
	}
	return result
}

// kindFromResource converts a resource plural name to a Kind name.
func kindFromResource(resource string) string {
	switch resource {
	case "pods":
		return "Pod"
	case "services":
		return "Service"
	case "ingresses":
		return "Ingress"
	case "ingressclasses":
		return "IngressClass"
	case "endpoints":
		return "Endpoints"
	case "configmaps":
		return "ConfigMap"
	case "secrets":
		return "Secret"
	case "deployments":
		return "Deployment"
	case "statefulsets":
		return "StatefulSet"
	case "daemonsets":
		return "DaemonSet"
	case "pvcs":
		return "PVC"
	case "pvs":
		return "PV"
	case "nodes":
		return "Node"
	case "serviceaccounts":
		return "ServiceAccount"
	case "hpas":
		return "HPA"
	default:
		return strings.TrimSuffix(resource, "s")
	}
}
