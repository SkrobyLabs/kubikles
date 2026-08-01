package server

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"kubikles/pkg/agent"
)

func TestAuthenticatedAcceleratorInfoExact(t *testing.T) {
	token, err := ParseCreatorToken(creatorTestToken)
	if err != nil {
		t.Fatal(err)
	}
	verifier := DeriveCreatorVerifier(token)
	auth, err := NewCreatorAuthenticator(verifier.Encoded())
	if err != nil {
		t.Fatal(err)
	}
	build := agent.BuildIdentity{BuildVersion: "v1.2.3-sensitive-looking", Commit: "0123456789abcdef", Dirty: true}
	resolution := agent.CapabilityResolution{
		Capabilities: []agent.Capability{
			"unknown", agent.CapabilitySecretsWatch, agent.CapabilitySecretsList,
			agent.CapabilitySecretsDetail, agent.CapabilitySecretsList,
		},
		Diagnostics: []agent.CapabilityDiagnostic{
			{Capability: "unknown", Action: "unknown", Outcome: agent.CapabilityCheckOutcomeAllowed, Reason: "drop-unknown"},
			{Capability: agent.CapabilitySecretsWatch, Action: agent.ResourceActionCoreV1SecretsWatch, Outcome: agent.CapabilityCheckOutcomeDenied, Reason: "watch-denied"},
			{Capability: agent.CapabilitySecretsList, Action: agent.ResourceActionCoreV1SecretsList, Outcome: agent.CapabilityCheckOutcomeAllowed},
			{Capability: agent.CapabilitySecretsList, Action: agent.ResourceActionCoreV1SecretsList, Outcome: agent.CapabilityCheckOutcomeDenied, Reason: "drop-duplicate"},
			{Capability: agent.CapabilitySecretsDetail, Action: agent.ResourceActionCoreV1SecretsList, Outcome: agent.CapabilityCheckOutcomeAllowed, Reason: "drop-wrong-action"},
			{Capability: agent.CapabilitySecretsDetail, Action: agent.ResourceActionCoreV1SecretsGet, Outcome: agent.CapabilityCheckOutcomeAPIError, ErrorCode: agent.CapabilityCheckErrorCodeForbidden},
		},
	}
	provider := NewAuthenticatedAcceleratorInfo(build, "accel-public-instance", resolution)
	wantCaps := []agent.Capability{agent.CapabilitySecretsList, agent.CapabilitySecretsDetail, agent.CapabilitySecretsWatch}
	wantDiagnostics := []agent.CapabilityDiagnostic{
		{Capability: agent.CapabilitySecretsList, Action: agent.ResourceActionCoreV1SecretsList, Outcome: agent.CapabilityCheckOutcomeAllowed},
		{Capability: agent.CapabilitySecretsDetail, Action: agent.ResourceActionCoreV1SecretsGet, Outcome: agent.CapabilityCheckOutcomeAPIError, ErrorCode: agent.CapabilityCheckErrorCodeForbidden},
		{Capability: agent.CapabilitySecretsWatch, Action: agent.ResourceActionCoreV1SecretsWatch, Outcome: agent.CapabilityCheckOutcomeDenied, Reason: "watch-denied"},
	}

	for i := range resolution.Capabilities {
		resolution.Capabilities[i] = "changed"
	}
	for i := range resolution.Diagnostics {
		resolution.Diagnostics[i] = agent.CapabilityDiagnostic{
			Capability: "changed",
			Action:     "changed",
			Outcome:    agent.CapabilityCheckOutcomeIncomplete,
			ErrorCode:  agent.CapabilityCheckErrorCodeTimeout,
			Reason:     "changed",
		}
	}

	info := provider.AcceleratorInfo()
	if info.Runtime != "accelerator" || info.Build != build || info.InstanceID != "accel-public-instance" {
		t.Fatalf("identity fields = %#v", info)
	}
	if !reflect.DeepEqual(info.Capabilities, wantCaps) {
		t.Fatalf("capabilities = %#v, want %#v", info.Capabilities, wantCaps)
	}
	if !reflect.DeepEqual(info.CapabilityDiagnostics, wantDiagnostics) {
		t.Fatalf("diagnostics = %#v, want catalog-ordered %#v", info.CapabilityDiagnostics, wantDiagnostics)
	}
	info.Capabilities[0] = "changed"
	info.CapabilityDiagnostics[0] = agent.CapabilityDiagnostic{Capability: "changed", Action: "changed", Outcome: agent.CapabilityCheckOutcomeIncomplete, ErrorCode: agent.CapabilityCheckErrorCodeTimeout, Reason: "changed"}
	fresh := provider.AcceleratorInfo()
	if !reflect.DeepEqual(fresh.Capabilities, wantCaps) || !reflect.DeepEqual(fresh.CapabilityDiagnostics, wantDiagnostics) {
		t.Fatalf("provider returned mutable slices: %#v", fresh)
	}

	guardCalls := 0
	providerCalls := 0
	options := AcceleratorOptions(0, nil, countingProtectedRouteGuard(&guardCalls, auth.Guard))
	options.AcceleratorInfoProvider = AcceleratorInfoProviderFunc(func() AuthenticatedAcceleratorInfo {
		providerCalls++
		return provider.AcceleratorInfo()
	})
	options.MethodAuthorizer = NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: wantCaps})
	srv, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/accelerator-info", nil)
	request.Host = "localhost"
	request.Header.Set("Authorization", "Bearer "+creatorTestToken)
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	wantJSON := "{\"runtime\":\"accelerator\",\"build\":{\"buildVersion\":\"v1.2.3-sensitive-looking\",\"commit\":\"0123456789abcdef\",\"dirty\":true},\"instanceId\":\"accel-public-instance\",\"capabilities\":[\"secrets.list\",\"secrets.detail\",\"secrets.watch\"],\"capabilityDiagnostics\":[{\"capability\":\"secrets.list\",\"action\":\"core/v1/secrets:list\",\"outcome\":\"allowed\"},{\"capability\":\"secrets.detail\",\"action\":\"core/v1/secrets:get\",\"outcome\":\"api_error\",\"errorCode\":\"forbidden\"},{\"capability\":\"secrets.watch\",\"action\":\"core/v1/secrets:watch\",\"outcome\":\"denied\",\"reason\":\"watch-denied\"}]}\n"
	if response.Code != http.StatusOK || response.Body.String() != wantJSON || response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("info response = %d %q %#v", response.Code, response.Body.String(), response.Header())
	}
	if guardCalls != 1 || providerCalls != 1 {
		t.Fatalf("canonical GET calls: guard=%d provider=%d, want 1 each", guardCalls, providerCalls)
	}
	for _, forbidden := range []string{creatorTestToken, verifier.Encoded(), string(auth.context.PrincipalID), string(auth.context.SessionID), "Authorization", "kubeconfig", "drop-unknown", "drop-duplicate", "drop-wrong-action"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("info response exposed %q: %s", forbidden, response.Body.String())
		}
	}

	unauthorizedBodies := make([]string, 0, 2)
	for _, header := range []string{"", "Bearer " + strings.Repeat("A", 42) + "B"} {
		request := httptest.NewRequest(http.MethodGet, "/api/accelerator-info", nil)
		request.Host = "localhost"
		if header != "" {
			request.Header.Set("Authorization", header)
		}
		response := httptest.NewRecorder()
		srv.Handler().ServeHTTP(response, request)
		assertUnauthorizedResponse(t, response)
		unauthorizedBodies = append(unauthorizedBodies, response.Body.String())
	}
	if unauthorizedBodies[0] != unauthorizedBodies[1] {
		t.Fatalf("info auth failures differed: %#v", unauthorizedBodies)
	}

	for _, test := range []struct {
		name, method, path, allow string
		status                    int
	}{
		{name: "method", method: http.MethodPost, path: "/api/accelerator-info", status: http.StatusMethodNotAllowed, allow: http.MethodGet},
		{name: "trailing slash", method: http.MethodGet, path: "/api/accelerator-info/", status: http.StatusNotFound},
		{name: "escaped letter", method: http.MethodGet, path: "/api/%61ccelerator-info", status: http.StatusNotFound},
		{name: "escaped hyphen", method: http.MethodGet, path: "/api/accelerator%2Dinfo", status: http.StatusNotFound},
		{name: "escaped path separator", method: http.MethodGet, path: "/api%2Faccelerator-info", status: http.StatusNotFound},
		{name: "double separator", method: http.MethodGet, path: "/api//accelerator-info", status: http.StatusNotFound},
		{name: "dot segment", method: http.MethodGet, path: "/api/./accelerator-info", status: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			guardBefore, providerBefore := guardCalls, providerCalls
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Host = "localhost"
			request.Header.Set("Authorization", "Bearer "+creatorTestToken)
			response := httptest.NewRecorder()
			srv.Handler().ServeHTTP(response, request)
			if response.Code != test.status || response.Header().Get("Allow") != test.allow {
				t.Fatalf("%s %s response = %d Allow %q, want %d Allow %q", test.method, test.path, response.Code, response.Header().Get("Allow"), test.status, test.allow)
			}
			if guardCalls != guardBefore || providerCalls != providerBefore {
				t.Fatalf("%s %s calls: guard=%d provider=%d, want zero", test.method, test.path, guardCalls-guardBefore, providerCalls-providerBefore)
			}
		})
	}
}

func TestAuthenticatedAcceleratorInfoEndpointEmptyResolution(t *testing.T) {
	response := authenticatedAcceleratorInfoResponse(t, agent.CapabilityResolution{})
	wantJSON := "{\"runtime\":\"accelerator\",\"build\":{\"buildVersion\":\"v1.2.3\",\"commit\":\"0123456789abcdef\",\"dirty\":false},\"instanceId\":\"accel-public-instance\",\"capabilities\":[],\"capabilityDiagnostics\":[]}\n"
	if response.Code != http.StatusOK || response.Body.String() != wantJSON {
		t.Fatalf("info response = %d %q", response.Code, response.Body.String())
	}
}

func TestAuthenticatedAcceleratorInfoEndpointFullyDeniedResolution(t *testing.T) {
	resolution := agent.CapabilityResolution{Diagnostics: []agent.CapabilityDiagnostic{
		{Capability: agent.CapabilitySecretsList, Action: agent.ResourceActionCoreV1SecretsList, Outcome: agent.CapabilityCheckOutcomeDenied, Reason: "list-denied"},
		{Capability: agent.CapabilitySecretsDetail, Action: agent.ResourceActionCoreV1SecretsGet, Outcome: agent.CapabilityCheckOutcomeDenied, Reason: "detail-denied"},
		{Capability: agent.CapabilitySecretsWatch, Action: agent.ResourceActionCoreV1SecretsWatch, Outcome: agent.CapabilityCheckOutcomeDenied, Reason: "watch-denied"},
	}}
	response := authenticatedAcceleratorInfoResponse(t, resolution)
	wantJSON := "{\"runtime\":\"accelerator\",\"build\":{\"buildVersion\":\"v1.2.3\",\"commit\":\"0123456789abcdef\",\"dirty\":false},\"instanceId\":\"accel-public-instance\",\"capabilities\":[],\"capabilityDiagnostics\":[{\"capability\":\"secrets.list\",\"action\":\"core/v1/secrets:list\",\"outcome\":\"denied\",\"reason\":\"list-denied\"},{\"capability\":\"secrets.detail\",\"action\":\"core/v1/secrets:get\",\"outcome\":\"denied\",\"reason\":\"detail-denied\"},{\"capability\":\"secrets.watch\",\"action\":\"core/v1/secrets:watch\",\"outcome\":\"denied\",\"reason\":\"watch-denied\"}]}\n"
	if response.Code != http.StatusOK || response.Body.String() != wantJSON {
		t.Fatalf("info response = %d %q", response.Code, response.Body.String())
	}
}

func authenticatedAcceleratorInfoResponse(t *testing.T, resolution agent.CapabilityResolution) *httptest.ResponseRecorder {
	t.Helper()
	token, err := ParseCreatorToken(creatorTestToken)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewCreatorAuthenticator(DeriveCreatorVerifier(token).Encoded())
	if err != nil {
		t.Fatal(err)
	}
	options := AcceleratorOptions(0, nil, auth.Guard)
	options.AcceleratorInfoProvider = NewAuthenticatedAcceleratorInfo(agent.BuildIdentity{BuildVersion: "v1.2.3", Commit: "0123456789abcdef"}, "accel-public-instance", resolution)
	options.MethodAuthorizer = NewAcceleratorMethodAuthorizer(resolution)
	srv, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/accelerator-info", nil)
	request.Host = "localhost"
	request.Header.Set("Authorization", "Bearer "+creatorTestToken)
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	return response
}

func countingProtectedRouteGuard(calls *int, guard ProtectedRouteGuard) ProtectedRouteGuard {
	return func(next http.Handler) http.Handler {
		guarded := guard(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			(*calls)++
			guarded.ServeHTTP(w, r)
		})
	}
}

func TestAcceleratorInfoWithoutProviderBypassesGuard(t *testing.T) {
	called := false
	s := newAcceleratorTestServer(t, &recordingMethodCaller{}, nil, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; next.ServeHTTP(w, r) })
	})
	w := requestBoundary(s.Handler(), http.MethodGet, "/api/accelerator-info", "localhost", "")
	if w.Code != http.StatusNotFound || called {
		t.Fatalf("status=%d guard=%v", w.Code, called)
	}
}
