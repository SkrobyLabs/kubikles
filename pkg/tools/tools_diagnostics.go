// Code split from tools.go; see that file for the package overview.
package tools

import (
	"fmt"
	"strings"
	"time"

	"kubikles/pkg/k8s"
)

// --- Diagnostic tool implementations ---

func toolGetFlowTimeline(client *k8s.Client, args map[string]interface{}) (string, bool) {
	resourceType := strArg(args, "resource_type")
	namespace := strArg(args, "namespace")
	name := strArg(args, "name")

	if resourceType == "" || namespace == "" || name == "" {
		return "resource_type, namespace, and name are required", true
	}

	req := k8s.FlowTimelineRequest{
		ResourceType:    resourceType,
		Namespace:       namespace,
		Name:            name,
		DurationMinutes: 30,
		MaxEntries:      100,
		IncludeLogs:     false,
	}

	entries, err := client.GetFlowTimeline(req)
	if err != nil {
		return fmt.Sprintf("Error getting flow timeline: %v", err), true
	}

	if len(entries) == 0 {
		return fmt.Sprintf("No timeline events found for %s/%s in namespace %s", resourceType, name, namespace), false
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Timeline for %s/%s (namespace: %s)\n\n", resourceType, name, namespace))

	for _, entry := range entries {
		ts := entry.Timestamp.Format(time.RFC3339)
		icon := getTimelineIcon(entry.EntryType, entry.Severity)
		sb.WriteString(fmt.Sprintf("%s %s **%s** [%s]: %s\n", ts, icon, entry.Kind, entry.Severity, entry.Message))
		if entry.Details != "" {
			sb.WriteString(fmt.Sprintf("   Details: %s\n", entry.Details))
		}
		if entry.ResourceRef != "" {
			sb.WriteString(fmt.Sprintf("   Resource: %s\n", entry.ResourceRef))
		}
		sb.WriteString("\n")
	}

	return truncate(sb.String(), MaxToolOutputChars), false
}

func getTimelineIcon(entryType, severity string) string {
	if severity == "warning" || severity == "error" {
		return "[!]"
	}
	switch entryType {
	case "event":
		return "[E]"
	case "log":
		return "[L]"
	case "change":
		return "[C]"
	default:
		return "[.]"
	}
}

func toolGetMultiPodLogs(client *k8s.Client, args map[string]interface{}) (string, bool) {
	namespace := strArg(args, "namespace")
	labelSelectorStr := strArg(args, "label_selector")
	container := strArg(args, "container")
	tailLines := int64(intArg(args, "tail_lines", 50))
	sinceSeconds := int64(intArg(args, "since_seconds", 300))

	if namespace == "" || labelSelectorStr == "" {
		return "namespace and label_selector are required", true
	}

	// Parse label selector string to map
	labelSelector := make(map[string]string)
	pairs := strings.Split(labelSelectorStr, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			labelSelector[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	if len(labelSelector) == 0 {
		return "invalid label_selector format, use 'key=value,key2=value2'", true
	}

	req := k8s.MultiLogRequest{
		Namespace:     namespace,
		LabelSelector: labelSelector,
		Container:     container,
		TailLines:     tailLines,
		SinceSeconds:  sinceSeconds,
		Follow:        false,
		Timestamps:    true,
	}

	entries, err := client.GetMultiPodLogsBatch(req)
	if err != nil {
		return fmt.Sprintf("Error getting multi-pod logs: %v", err), true
	}

	if len(entries) == 0 {
		return fmt.Sprintf("No logs found for pods matching '%s' in namespace %s", labelSelectorStr, namespace), false
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Logs from pods matching '%s' (namespace: %s)\n\n", labelSelectorStr, namespace))

	// Group by pod for cleaner output
	podGroups := make(map[string][]k8s.MultiLogEntry)
	for _, entry := range entries {
		key := entry.PodName
		podGroups[key] = append(podGroups[key], entry)
	}

	for podName, podEntries := range podGroups {
		sb.WriteString(fmt.Sprintf("## Pod: %s\n", podName))
		for _, entry := range podEntries {
			ts := entry.Timestamp.Format("15:04:05")
			sb.WriteString(fmt.Sprintf("[%s] [%s] %s\n", ts, entry.Container, entry.Message))
		}
		sb.WriteString("\n")
	}

	return truncate(sb.String(), MaxToolOutputChars), false
}

func toolDiffResources(client *k8s.Client, args map[string]interface{}) (string, bool) {
	req := k8s.DiffRequest{
		SourceContext:   strArg(args, "source_context"),
		SourceNamespace: strArg(args, "source_namespace"),
		SourceKind:      strArg(args, "source_kind"),
		SourceName:      strArg(args, "source_name"),
		TargetContext:   strArg(args, "target_context"),
		TargetNamespace: strArg(args, "target_namespace"),
		TargetKind:      strArg(args, "target_kind"),
		TargetName:      strArg(args, "target_name"),
	}

	if req.SourceNamespace == "" || req.SourceKind == "" || req.SourceName == "" ||
		req.TargetNamespace == "" || req.TargetKind == "" || req.TargetName == "" {
		return "source and target namespace, kind, and name are required", true
	}

	result, err := client.DiffResources(req)
	if err != nil {
		return fmt.Sprintf("Error comparing resources: %v", err), true
	}

	var sb strings.Builder
	sourceLabel := fmt.Sprintf("%s/%s/%s", req.SourceNamespace, req.SourceKind, req.SourceName)
	targetLabel := fmt.Sprintf("%s/%s/%s", req.TargetNamespace, req.TargetKind, req.TargetName)

	if req.SourceContext != "" {
		sourceLabel = req.SourceContext + "/" + sourceLabel
	}
	if req.TargetContext != "" {
		targetLabel = req.TargetContext + "/" + targetLabel
	}

	sb.WriteString("# Resource Diff\n\n")
	sb.WriteString(fmt.Sprintf("**Source**: %s (exists: %v)\n", sourceLabel, result.SourceExists))
	sb.WriteString(fmt.Sprintf("**Target**: %s (exists: %v)\n\n", targetLabel, result.TargetExists))

	if !result.HasChanges {
		sb.WriteString("**No differences found** - resources are identical\n")
		return sb.String(), false
	}

	sb.WriteString(fmt.Sprintf("**Changes found**: %d\n\n", result.ChangeCount))

	// Show structured changes
	if len(result.Changes) > 0 {
		sb.WriteString("## Summary of Changes\n\n")
		for _, change := range result.Changes {
			switch change.Type {
			case "added":
				sb.WriteString(fmt.Sprintf("+ **%s**: %s\n", change.Path, change.New))
			case "removed":
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", change.Path, change.Old))
			case "changed":
				sb.WriteString(fmt.Sprintf("~ **%s**: %s -> %s\n", change.Path, change.Old, change.New))
			}
		}
		sb.WriteString("\n")
	}

	// Show unified diff if not too long
	if result.UnifiedDiff != "" && len(result.UnifiedDiff) < 5000 {
		sb.WriteString("## Unified Diff\n\n```diff\n")
		sb.WriteString(result.UnifiedDiff)
		sb.WriteString("```\n")
	}

	return truncate(sb.String(), MaxToolOutputChars), false
}

func toolCheckRBACAccess(client *k8s.Client, args map[string]interface{}) (string, bool) {
	subjectKind := strArg(args, "subject_kind")
	subjectName := strArg(args, "subject_name")
	subjectNamespace := strArg(args, "subject_namespace")
	namespace := strArg(args, "target_namespace")
	resource := strArg(args, "resource")
	verb := strArg(args, "verb")

	if subjectKind == "" || subjectName == "" {
		return "subject_kind and subject_name are required", true
	}

	// Normalize and validate subject kind
	switch strings.ToLower(subjectKind) {
	case "user":
		subjectKind = "User"
	case "group":
		subjectKind = "Group"
	case "serviceaccount", "sa":
		subjectKind = "ServiceAccount"
	default:
		return "subject_kind must be User, Group, or ServiceAccount", true
	}

	if subjectKind == "ServiceAccount" && subjectNamespace == "" {
		return "subject_namespace is required for ServiceAccount", true
	}

	// If no specific resource/verb provided, check common ones
	if resource == "" || verb == "" {
		return checkMultipleRBACPermissions(client, subjectKind, subjectName, subjectNamespace, namespace)
	}

	req := k8s.RBACCheckRequest{
		SubjectKind:      subjectKind,
		SubjectName:      subjectName,
		SubjectNamespace: subjectNamespace,
		Namespace:        namespace,
		Resource:         resource,
		Verb:             verb,
	}

	result, err := client.CheckRBACAccess(req)
	if err != nil {
		return fmt.Sprintf("Error checking RBAC access: %v", err), true
	}

	var sb strings.Builder

	// Format subject
	subjectDesc := fmt.Sprintf("%s '%s'", subjectKind, subjectName)
	if subjectKind == "ServiceAccount" {
		subjectDesc = fmt.Sprintf("ServiceAccount '%s/%s'", subjectNamespace, subjectName)
	}

	sb.WriteString(fmt.Sprintf("# RBAC Check: %s\n\n", subjectDesc))
	sb.WriteString(fmt.Sprintf("**Action**: %s %s", verb, resource))
	if namespace != "" {
		sb.WriteString(fmt.Sprintf(" in namespace '%s'", namespace))
	} else {
		sb.WriteString(" (cluster-wide)")
	}
	sb.WriteString("\n\n")

	if result.Allowed {
		sb.WriteString("**Result**: ALLOWED\n")
	} else {
		sb.WriteString("**Result**: DENIED\n")
	}

	if result.Reason != "" {
		sb.WriteString(fmt.Sprintf("**Reason**: %s\n", result.Reason))
	}

	// Show permission chain
	if len(result.Chain) > 0 {
		sb.WriteString("\n## Permission Chain\n\n")
		for _, link := range result.Chain {
			status := "denies"
			if link.Grants {
				status = "grants"
			}
			sb.WriteString(fmt.Sprintf("- %s '%s' %s via rule: %s\n",
				link.Kind, link.Name, status, link.Rule))
		}
	}

	return sb.String(), false
}

// checkMultipleRBACPermissions checks common permissions when no specific resource/verb is provided
func checkMultipleRBACPermissions(client *k8s.Client, subjectKind, subjectName, subjectNamespace, namespace string) (string, bool) {
	var sb strings.Builder

	subjectDesc := fmt.Sprintf("%s '%s'", subjectKind, subjectName)
	if subjectKind == "ServiceAccount" {
		subjectDesc = fmt.Sprintf("ServiceAccount '%s/%s'", subjectNamespace, subjectName)
	}

	sb.WriteString(fmt.Sprintf("# RBAC Permissions Summary: %s\n\n", subjectDesc))
	if namespace != "" {
		sb.WriteString(fmt.Sprintf("**Namespace**: %s\n\n", namespace))
	} else {
		sb.WriteString("**Scope**: Cluster-wide\n\n")
	}

	// Check common resource/verb combinations
	checks := []struct {
		resource string
		verb     string
	}{
		{"pods", "get"},
		{"pods", "list"},
		{"pods", "create"},
		{"pods", "delete"},
		{"pods/log", "get"},
		{"pods/exec", "create"},
		{"deployments", "get"},
		{"deployments", "list"},
		{"deployments", "create"},
		{"deployments", "update"},
		{"services", "get"},
		{"services", "list"},
		{"configmaps", "get"},
		{"configmaps", "list"},
		{"secrets", "get"},
		{"secrets", "list"},
	}

	sb.WriteString("| Resource | Verb | Allowed |\n")
	sb.WriteString("|----------|------|---------|")

	for _, check := range checks {
		req := k8s.RBACCheckRequest{
			SubjectKind:      subjectKind,
			SubjectName:      subjectName,
			SubjectNamespace: subjectNamespace,
			Namespace:        namespace,
			Resource:         check.resource,
			Verb:             check.verb,
		}

		result, err := client.CheckRBACAccess(req)
		status := "?"
		if err == nil {
			if result.Allowed {
				status = "Yes"
			} else {
				status = "No"
			}
		}
		sb.WriteString(fmt.Sprintf("\n| %s | %s | %s |", check.resource, check.verb, status))
	}

	sb.WriteString("\n")
	return truncate(sb.String(), MaxToolOutputChars), false
}
