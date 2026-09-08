// Package grafana registers operator-specific maintenance fields.
package grafana

import "kubikles/pkg/resourceactions/maintenance"

func Agent() maintenance.Provider {
	return maintenance.Provider{Group: "monitoring.grafana.com", Resource: "grafanaagents", Versions: []string{"v1alpha1"}, PauseField: "paused", PausedValue: true, RunningValue: false, PodAnnotations: []string{"spec", "podMetadata", "annotations"}, SidecarMode: false}
}
