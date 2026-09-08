// Package opentelemetry registers operator-specific maintenance fields.
package opentelemetry

import "kubikles/pkg/resourceactions/maintenance"

func OpAMPBridge() maintenance.Provider {
	return maintenance.Provider{Group: "opentelemetry.io", Resource: "opampbridges", Versions: []string{"v1alpha1"}, PodAnnotations: []string{"spec", "podAnnotations"}, SidecarMode: false}
}
