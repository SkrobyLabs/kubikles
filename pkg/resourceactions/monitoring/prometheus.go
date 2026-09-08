// Package monitoring registers documented Prometheus Operator maintenance controls.
// https://prometheus-operator.dev/docs/api-reference/api/
package monitoring

import "kubikles/pkg/resourceactions/maintenance"

func Prometheus() maintenance.Provider {
	return maintenance.Provider{Group: "monitoring.coreos.com", Resource: "prometheuses", Versions: []string{"v1"}, PauseField: "paused", PausedValue: true, RunningValue: false, PodAnnotations: []string{"spec", "podMetadata", "annotations"}}
}
