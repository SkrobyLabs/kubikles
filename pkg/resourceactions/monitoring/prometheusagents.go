package monitoring

import "kubikles/pkg/resourceactions/maintenance"

func PrometheusAgent() maintenance.Provider {
	return maintenance.Provider{Group: "monitoring.coreos.com", Resource: "prometheusagents", Versions: []string{"v1alpha1"}, PauseField: "paused", PausedValue: true, RunningValue: false, PodAnnotations: []string{"spec", "podMetadata", "annotations"}}
}
