// Package agent defines the dependency-light Kubikles Accelerator contracts.
package agent

// DefaultBuildVersion identifies development builds when no build version is injected.
const DefaultBuildVersion = "dev"

// Capability identifies an Accelerator capability.
type Capability string

const (
	CapabilitySecretsList   Capability = "secrets.list"
	CapabilitySecretsDetail Capability = "secrets.detail"
	CapabilitySecretsWatch  Capability = "secrets.watch"
)

// BuildIdentity contains Accelerator build identity and source diagnostics.
type BuildIdentity struct {
	BuildVersion string `json:"buildVersion"`
	Commit       string `json:"commit"`
	Dirty        bool   `json:"dirty"`
}

// AcceleratorInfo is the wire DTO for Accelerator build identity and capabilities.
type AcceleratorInfo struct {
	BuildIdentity BuildIdentity `json:"buildIdentity"`
	Capabilities  []Capability  `json:"capabilities"`
}

// CompatibilityStatus describes the closed result of a build compatibility check.
type CompatibilityStatus string

const (
	CompatibilityStatusExactMatch           CompatibilityStatus = "exact_match"
	CompatibilityStatusMissingBuildVersion  CompatibilityStatus = "missing_build_version"
	CompatibilityStatusBuildVersionMismatch CompatibilityStatus = "build_version_mismatch"
)

// CompatibilityAction specifies the required Accelerator lifecycle action.
type CompatibilityAction string

const (
	CompatibilityActionReuseAccelerator    CompatibilityAction = "reuse_accelerator"
	CompatibilityActionRecreateAccelerator CompatibilityAction = "recreate_accelerator"
)

// CompatibilityResult preserves observed build versions and the closed compatibility decision.
type CompatibilityResult struct {
	Compatible              bool                `json:"compatible"`
	Status                  CompatibilityStatus `json:"status"`
	Action                  CompatibilityAction `json:"action"`
	DesktopBuildVersion     string              `json:"desktopBuildVersion"`
	AcceleratorBuildVersion string              `json:"acceleratorBuildVersion"`
}

// CheckBuildCompatibility checks literal, non-empty desktop and Accelerator build version equality.
// Desktop-to-Accelerator lifecycle calls this function before publishing an Integrated session.
func CheckBuildCompatibility(desktopBuildVersion, acceleratorBuildVersion string) CompatibilityResult {
	result := CompatibilityResult{
		DesktopBuildVersion:     desktopBuildVersion,
		AcceleratorBuildVersion: acceleratorBuildVersion,
		Action:                  CompatibilityActionRecreateAccelerator,
	}
	if desktopBuildVersion == "" || acceleratorBuildVersion == "" {
		result.Status = CompatibilityStatusMissingBuildVersion
		return result
	}
	if desktopBuildVersion != acceleratorBuildVersion {
		result.Status = CompatibilityStatusBuildVersionMismatch
		return result
	}
	result.Compatible = true
	result.Status = CompatibilityStatusExactMatch
	result.Action = CompatibilityActionReuseAccelerator
	return result
}
