package server

import (
	"context"
	"embed"
	"net/http"
	"testing"

	"kubikles/pkg/agent"
)

func TestAcceleratorMethodAuthorizerExactPositiveMatrix(t *testing.T) {
	authenticated := agent.AuthenticatedCallContext{PrincipalID: "creator-a", SessionID: "http-a"}
	methods := []struct {
		name       string
		capability agent.Capability
	}{
		{name: "ListSecretsMetadata", capability: agent.CapabilitySecretsList},
		{name: "GetSecretData", capability: agent.CapabilitySecretsDetail},
		{name: "GetSecretYaml", capability: agent.CapabilitySecretsDetail},
		{name: "CancelListRequest", capability: agent.CapabilitySecretsList},
	}
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			all := NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: agent.V1Capabilities()})
			if !all.Authorize(authenticated, method.name) {
				t.Fatal("exact method with required capability was denied")
			}
			withoutRequired := make([]agent.Capability, 0, 2)
			for _, capability := range agent.V1Capabilities() {
				if capability != method.capability {
					withoutRequired = append(withoutRequired, capability)
				}
			}
			if NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: withoutRequired}).Authorize(authenticated, method.name) {
				t.Fatal("method allowed without its required capability")
			}
			if NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: []agent.Capability{"unknown", method.capability, method.capability}}).Authorize(authenticated, method.name) != true {
				t.Fatal("normalized required capability was not honored")
			}
			for _, call := range []agent.AuthenticatedCallContext{
				{},
				{PrincipalID: authenticated.PrincipalID},
				{SessionID: authenticated.SessionID},
			} {
				if all.Authorize(call, method.name) {
					t.Fatalf("partial context %#v was authorized", call)
				}
			}
		})
	}
	if (*AcceleratorMethodAuthorizer)(nil).Authorize(authenticated, "ListSecretsMetadata") {
		t.Fatal("nil authorizer allowed a method")
	}
}

func TestAcceleratorMethodAuthorizerFailsClosed(t *testing.T) {
	authorizer := NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: append(agent.V1Capabilities(), "future.superuser")})
	call := agent.AuthenticatedCallContext{PrincipalID: "creator-overprivileged", SessionID: "http-overprivileged"}
	negativeCorpus := []string{
		"", " ", "ListSecrets", "listsecretsmetadata", "LISTSECRETMetadata", "ListSecretsMetadata ",
		"GetSecret", "CreateSecret", "UpdateSecret", "PatchSecret", "DeleteSecret", "ApplySecret", "ReplaceSecret",
		"ListPods", "GetPodYaml", "DeletePod", "GetPodLogs", "SubscribePodWatcher", "UnsubscribePodWatcher",
		"ListConfigMaps", "GetConfigMapData", "GetConfigMapYaml", "UpdateConfigMap", "DeleteConfigMap", "SubscribeConfigMapWatcher",
		"ListHelmReleases", "GetHelmRelease", "HelmTemplateRelease", "HelmDryRunUpgrade", "UninstallHelmRelease",
		"SubscribeWatcher", "UnsubscribeWatcher", "StartResourceWatcher", "StopResourceWatcher",
		"CreateTerminalSession", "CloseTerminalSession", "ExecuteCommand", "ExecPod", "DownloadPodFile", "UploadPodFile", "DeletePodFile",
		"AddPortForwardConfig", "DeletePortForwardConfig", "GetPortForwardConfigs", "GetKubeconfig", "SetCurrentContext", "DeleteContext",
		"GetVersionInfo", "GetEmbeddedBrowserStatus", "OpenBrowser", "Shutdown", "FutureGeneratedExport", "ListSecretsMetadataV2",
	}
	for _, method := range negativeCorpus {
		if authorizer.Authorize(call, method) {
			t.Errorf("negative method %q was authorized", method)
		}
	}
}

func TestAcceleratorStatefulMethodsExplicitlyUnavailable(t *testing.T) {
	call := agent.AuthenticatedCallContext{PrincipalID: "creator-a", SessionID: "http-a"}
	authorizer := NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: agent.V1Capabilities()})
	// These remain unavailable until the downstream session and Secret ownership child exists.
	for _, method := range []string{"SubscribeSecretWatcher", "UnsubscribeSecretWatcher"} {
		if authorizer.Authorize(call, method) {
			t.Fatalf("reserved stateful method %q was authorized directly", method)
		}
	}
	if !authorizer.Authorize(call, "CancelListRequest") {
		t.Fatal("CancelListRequest must remain available with secrets.list")
	}
	if NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: []agent.Capability{agent.CapabilitySecretsDetail, agent.CapabilitySecretsWatch}}).Authorize(call, "CancelListRequest") {
		t.Fatal("CancelListRequest was allowed without secrets.list")
	}

	caller := &recordingMethodCaller{}
	options := AcceleratorOptions(0, nil, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trusted := context.WithValue(r.Context(), creatorContextKey{}, call)
			next.ServeHTTP(w, r.WithContext(trusted))
		})
	})
	options.MethodAuthorizer = authorizer
	server, err := NewWithOptions(caller, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"SubscribeSecretWatcher", "UnsubscribeSecretWatcher"} {
		response := requestBoundary(server.Handler(), http.MethodPost, "/api/call", "localhost", `{"method":"`+method+`"}`)
		if response.Code != http.StatusForbidden || response.Body.String() != "{\"error\":\"forbidden\"}\n" || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s response = %d %q %#v", method, response.Code, response.Body.String(), response.Header())
		}
	}
	if caller.callCount() != 0 {
		t.Fatalf("reserved methods reached generated dispatch %d times", caller.callCount())
	}
}
