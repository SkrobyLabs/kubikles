// Package opentelemetry registers operator-specific maintenance fields.
package opentelemetry

import "kubikles/pkg/resourceactions/maintenance"

func Collector() maintenance.Provider {
	return maintenance.Provider{Group: "opentelemetry.io", Resource: "opentelemetrycollectors", Versions: []string{"v1beta1"}, PauseField: "managementState", PausedValue: "unmanaged", RunningValue: "managed", PodAnnotations: []string{"spec", "podAnnotations"}, SidecarMode: true}
}
