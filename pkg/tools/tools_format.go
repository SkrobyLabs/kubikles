// Code split from tools.go; see that file for the package overview.
package tools

import (
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
)

// --- Helpers ---

func age(t time.Time) string {
	if t.IsZero() {
		return "<unknown>"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func eventTime(e v1.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	if !e.EventTime.IsZero() {
		return e.EventTime.Time
	}
	return e.CreationTimestamp.Time
}

func formatBytesCompact(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)
	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.1fTB", float64(bytes)/float64(tb))
	case bytes >= gb:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func formatCPUCompact(millicores int64) string {
	if millicores >= 1000 {
		return fmt.Sprintf("%.1fcores", float64(millicores)/1000.0)
	}
	return fmt.Sprintf("%dm", millicores)
}

func pct(usage, capacity int64) int {
	if capacity == 0 {
		return 0
	}
	return int(usage * 100 / capacity)
}

// NormalizeKind maps plural/variant resource kind strings to their canonical singular form.
// sync: frontend/src/components/layout/AIPanel.jsx:kindToViewName
func NormalizeKind(kind string) string {
	switch kind {
	case "pods":
		return "pod"
	case "deployments":
		return "deployment"
	case "statefulsets":
		return "statefulset"
	case "daemonsets":
		return "daemonset"
	case "replicasets":
		return "replicaset"
	case "jobs":
		return "job"
	case "cronjobs":
		return "cronjob"
	case "services":
		return "service"
	case "ingresses":
		return "ingress"
	case "configmaps":
		return "configmap"
	case "secrets":
		return "secret"
	case "nodes":
		return "node"
	case "namespaces":
		return "namespace"
	case "pvcs", "persistentvolumeclaims":
		return "pvc"
	case "pvs", "persistentvolumes":
		return "pv"
	case "storageclasses":
		return "storageclass"
	case "hpas", "horizontalpodautoscalers":
		return "hpa"
	case "pdbs", "poddisruptionbudgets":
		return "pdb"
	case "serviceaccounts":
		return "serviceaccount"
	case "networkpolicies":
		return "networkpolicy"
	case "ingressclasses":
		return "ingressclass"
	default:
		return kind
	}
}
