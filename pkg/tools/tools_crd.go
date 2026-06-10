// Code split from tools.go; see that file for the package overview.
package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"kubikles/pkg/k8s"
)

// --- CRD tool implementations ---

func toolListCRDs(client *k8s.Client, args map[string]interface{}) (string, bool) {
	ctx := client.GetCurrentContext()
	crds, err := client.ListCRDs(ctx)
	if err != nil {
		return fmt.Sprintf("Error listing CRDs: %v", err), true
	}
	if len(crds) == 0 {
		return "No CustomResourceDefinitions found", false
	}

	sort.Slice(crds, func(i, j int) bool {
		if crds[i].Spec.Group != crds[j].Spec.Group {
			return crds[i].Spec.Group < crds[j].Spec.Group
		}
		return crds[i].Name < crds[j].Name
	})

	var lines []string
	lines = append(lines, fmt.Sprintf("%-60s %-30s %-10s %-25s %-12s %-25s", "NAME", "GROUP", "VERSION", "KIND", "SCOPE", "PLURAL"))
	for _, crd := range crds {
		version := ""
		for _, v := range crd.Spec.Versions {
			if v.Served {
				version = v.Name
				break
			}
		}
		scope := string(crd.Spec.Scope)
		kind := crd.Spec.Names.Kind
		plural := crd.Spec.Names.Plural
		lines = append(lines, fmt.Sprintf("%-60s %-30s %-10s %-25s %-12s %-25s", crd.Name, crd.Spec.Group, version, kind, scope, plural))
	}
	return truncate(strings.Join(lines, "\n"), MaxCRDListChars), false
}

func toolListCustomResources(client *k8s.Client, args map[string]interface{}) (string, bool) {
	group := strArg(args, "group")
	version := strArg(args, "version")
	resource := strArg(args, "resource")
	ns := strArg(args, "namespace")

	if group == "" || version == "" || resource == "" {
		return "group, version, and resource are required", true
	}

	ctx := client.GetCurrentContext()
	items, err := client.ListCustomResources(ctx, group, version, resource, ns)
	if err != nil {
		return fmt.Sprintf("Error listing custom resources: %v", err), true
	}
	if len(items) == 0 {
		return fmt.Sprintf("No %s resources found", resource), false
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s", "NAME", "NAMESPACE", "AGE"))
	for _, item := range items {
		name := nestedString(item, "metadata", "name")
		namespace := nestedString(item, "metadata", "namespace")
		createdAt := nestedString(item, "metadata", "creationTimestamp")
		ageStr := "<unknown>"
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			ageStr = age(t)
		}
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s", name, namespace, ageStr))
	}
	return truncate(strings.Join(lines, "\n"), MaxCustomResourceListChars), false
}

func toolGetCustomResourceYaml(client *k8s.Client, args map[string]interface{}) (string, bool) {
	group := strArg(args, "group")
	version := strArg(args, "version")
	resource := strArg(args, "resource")
	name := strArg(args, "name")
	ns := strArg(args, "namespace")

	if group == "" || version == "" || resource == "" || name == "" {
		return "group, version, resource, and name are required", true
	}

	ctx := client.GetCurrentContext()
	yamlStr, err := client.GetCustomResourceYaml(ctx, group, version, resource, ns, name)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}
	return truncate(yamlStr, MaxYAMLChars), false
}

// nestedString extracts a string from a nested map path.
func nestedString(obj map[string]interface{}, keys ...string) string {
	current := obj
	for i, key := range keys {
		val, ok := current[key]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			if s, ok := val.(string); ok {
				return s
			}
			return ""
		}
		if m, ok := val.(map[string]interface{}); ok {
			current = m
		} else {
			return ""
		}
	}
	return ""
}

// redactSecretYaml replaces the values under `data:` and `stringData:` blocks
// with [REDACTED] to prevent leaking secrets to the AI provider.
var secretDataLineRe = regexp.MustCompile(`^(\s+\S+:\s)(.+)$`)

func redactSecretYaml(yaml string) string {
	lines := strings.Split(yaml, "\n")
	inDataBlock := false
	dataIndent := 0
	var out []string

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "data:" || trimmed == "stringData:" {
			inDataBlock = true
			dataIndent = len(line) - len(strings.TrimLeft(line, " "))
			out = append(out, line)
			continue
		}

		if inDataBlock {
			if trimmed == "" || strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
				out = append(out, line)
				continue
			}
			lineIndent := len(line) - len(strings.TrimLeft(line, " "))
			if lineIndent > dataIndent {
				if m := secretDataLineRe.FindStringSubmatch(line); m != nil {
					out = append(out, m[1]+"[REDACTED]")
				} else {
					out = append(out, line)
				}
				continue
			}
			inDataBlock = false
		}

		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
