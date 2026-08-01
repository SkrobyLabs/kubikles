package agent

import "context"

// CapabilityCheckOutcome is the closed result of one required capability action check.
type CapabilityCheckOutcome string

const (
	CapabilityCheckOutcomeAllowed    CapabilityCheckOutcome = "allowed"
	CapabilityCheckOutcomeDenied     CapabilityCheckOutcome = "denied"
	CapabilityCheckOutcomeIncomplete CapabilityCheckOutcome = "incomplete"
	CapabilityCheckOutcomeMalformed  CapabilityCheckOutcome = "malformed"
	CapabilityCheckOutcomeAPIError   CapabilityCheckOutcome = "api_error"
	CapabilityCheckOutcomeTimeout    CapabilityCheckOutcome = "timeout"
)

// CapabilityCheckErrorCode is a safe, closed classification of a review failure.
type CapabilityCheckErrorCode string

const (
	CapabilityCheckErrorCodeNone              CapabilityCheckErrorCode = ""
	CapabilityCheckErrorCodeClientUnavailable CapabilityCheckErrorCode = "client_unavailable"
	CapabilityCheckErrorCodeForbidden         CapabilityCheckErrorCode = "forbidden"
	CapabilityCheckErrorCodeUnauthorized      CapabilityCheckErrorCode = "unauthorized"
	CapabilityCheckErrorCodeCanceled          CapabilityCheckErrorCode = "canceled"
	CapabilityCheckErrorCodeTimeout           CapabilityCheckErrorCode = "timeout"
	CapabilityCheckErrorCodeAPIError          CapabilityCheckErrorCode = "api_error"
	CapabilityCheckErrorCodeMalformedResponse CapabilityCheckErrorCode = "malformed_response"
)

// CapabilityDiagnostic records the closed result of checking one resource action.
type CapabilityDiagnostic struct {
	Capability Capability               `json:"capability"`
	Action     ResourceActionID         `json:"action"`
	Outcome    CapabilityCheckOutcome   `json:"outcome"`
	Reason     string                   `json:"reason,omitempty"`
	ErrorCode  CapabilityCheckErrorCode `json:"errorCode,omitempty"`
}

// CapabilityResolution is a deterministic, value-level capability resolution result.
type CapabilityResolution struct {
	Capabilities []Capability           `json:"capabilities"`
	Diagnostics  []CapabilityDiagnostic `json:"diagnostics"`
}

// CapabilityResolver resolves Accelerator capabilities without returning an operational error.
type CapabilityResolver interface {
	ResolveCapabilities(context.Context) CapabilityResolution
}

// CapabilityResolverFactory constructs a narrow capability resolver for downstream composition.
type CapabilityResolverFactory func() CapabilityResolver
