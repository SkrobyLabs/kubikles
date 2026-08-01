package server

import (
	"net/http"

	"kubikles/pkg/agent"
)

type AuthenticatedAcceleratorInfo struct {
	Runtime               string                       `json:"runtime"`
	Build                 agent.BuildIdentity          `json:"build"`
	InstanceID            string                       `json:"instanceId"`
	Capabilities          []agent.Capability           `json:"capabilities"`
	CapabilityDiagnostics []agent.CapabilityDiagnostic `json:"capabilityDiagnostics"`
}
type immutableAcceleratorInfo struct{ info AuthenticatedAcceleratorInfo }

func NewAuthenticatedAcceleratorInfo(build agent.BuildIdentity, instance string, resolution agent.CapabilityResolution) AcceleratorInfoProvider {
	info := AuthenticatedAcceleratorInfo{
		Runtime:               "accelerator",
		Build:                 build,
		InstanceID:            instance,
		Capabilities:          make([]agent.Capability, 0),
		CapabilityDiagnostics: make([]agent.CapabilityDiagnostic, 0),
	}
	for _, wanted := range agent.V1Capabilities() {
		for _, cap := range resolution.Capabilities {
			if cap == wanted {
				info.Capabilities = append(info.Capabilities, cap)
				break
			}
		}
		for _, diagnostic := range resolution.Diagnostics {
			if diagnostic.Capability == wanted && diagnostic.Action == actionForCapability(wanted) {
				info.CapabilityDiagnostics = append(info.CapabilityDiagnostics, diagnostic)
				break
			}
		}
	}
	return &immutableAcceleratorInfo{info: info}
}

func actionForCapability(capability agent.Capability) agent.ResourceActionID {
	switch capability {
	case agent.CapabilitySecretsList:
		return agent.ResourceActionCoreV1SecretsList
	case agent.CapabilitySecretsDetail:
		return agent.ResourceActionCoreV1SecretsGet
	case agent.CapabilitySecretsWatch:
		return agent.ResourceActionCoreV1SecretsWatch
	default:
		return ""
	}
}
func (p *immutableAcceleratorInfo) AcceleratorInfo() AuthenticatedAcceleratorInfo {
	result := p.info
	result.Capabilities = make([]agent.Capability, len(p.info.Capabilities))
	copy(result.Capabilities, p.info.Capabilities)
	result.CapabilityDiagnostics = make([]agent.CapabilityDiagnostic, len(p.info.CapabilityDiagnostics))
	copy(result.CapabilityDiagnostics, p.info.CapabilityDiagnostics)
	return result
}
func (s *Server) handleAcceleratorInfo(w http.ResponseWriter, r *http.Request) {
	if s.options.AcceleratorInfoProvider == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, s.options.AcceleratorInfoProvider.AcceleratorInfo())
}
