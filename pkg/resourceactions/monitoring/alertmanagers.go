package monitoring

import "kubikles/pkg/resourceactions/maintenance"

func Alertmanager() maintenance.Provider {
	return maintenance.Provider{Group: "monitoring.coreos.com", Resource: "alertmanagers", Versions: []string{"v1"}, PauseField: "paused", PausedValue: true, RunningValue: false, PodAnnotations: []string{"spec", "podMetadata", "annotations"}}
}
