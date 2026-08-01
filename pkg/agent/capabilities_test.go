package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type testCapabilityResolver struct{}

func (testCapabilityResolver) ResolveCapabilities(context.Context) CapabilityResolution {
	return CapabilityResolution{Capabilities: []Capability{CapabilitySecretsList}}
}

func TestCapabilityResolutionContract(t *testing.T) {
	var resolver CapabilityResolver = testCapabilityResolver{}
	var factory CapabilityResolverFactory = func() CapabilityResolver { return resolver }
	if factory() == nil {
		t.Fatal("factory returned nil resolver")
	}

	diagnostic := CapabilityDiagnostic{
		Capability: CapabilitySecretsList,
		Action:     ResourceActionCoreV1SecretsList,
		Outcome:    CapabilityCheckOutcomeAllowed,
	}
	resolution := CapabilityResolution{Capabilities: []Capability{CapabilitySecretsList}, Diagnostics: []CapabilityDiagnostic{diagnostic}}
	encoded, err := json.Marshal(resolution)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	const wantJSON = `{"capabilities":["secrets.list"],"diagnostics":[{"capability":"secrets.list","action":"core/v1/secrets:list","outcome":"allowed"}]}`
	if string(encoded) != wantJSON {
		t.Errorf("JSON = %s, want %s", encoded, wantJSON)
	}

	if got, want := []CapabilityCheckOutcome{CapabilityCheckOutcomeAllowed, CapabilityCheckOutcomeDenied, CapabilityCheckOutcomeIncomplete, CapabilityCheckOutcomeMalformed, CapabilityCheckOutcomeAPIError, CapabilityCheckOutcomeTimeout}, []CapabilityCheckOutcome{"allowed", "denied", "incomplete", "malformed", "api_error", "timeout"}; !reflect.DeepEqual(got, want) {
		t.Errorf("outcomes = %#v, want %#v", got, want)
	}
	if got, want := []CapabilityCheckErrorCode{CapabilityCheckErrorCodeNone, CapabilityCheckErrorCodeClientUnavailable, CapabilityCheckErrorCodeForbidden, CapabilityCheckErrorCodeUnauthorized, CapabilityCheckErrorCodeCanceled, CapabilityCheckErrorCodeTimeout, CapabilityCheckErrorCodeAPIError, CapabilityCheckErrorCodeMalformedResponse}, []CapabilityCheckErrorCode{"", "client_unavailable", "forbidden", "unauthorized", "canceled", "timeout", "api_error", "malformed_response"}; !reflect.DeepEqual(got, want) {
		t.Errorf("error codes = %#v, want %#v", got, want)
	}
}
