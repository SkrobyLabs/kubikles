// Package opentelemetry registers operator-specific maintenance fields.
package opentelemetry

import "kubikles/pkg/resourceactions/maintenance"

func TargetAllocator() maintenance.Provider {
	return maintenance.Provider{Group: "opentelemetry.io", Resource: "targetallocators", Versions: []string{"v1alpha1"}, PauseField: "managementState", PausedValue: "unmanaged", RunningValue: "managed", PodAnnotations: []string{"spec", "podAnnotations"}, SidecarMode: false}
}
