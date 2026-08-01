package server

import (
	"context"
	"embed"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAcceleratorSecretWatchMethodsUseSettledPolicy(t *testing.T) {
	call := agent.AuthenticatedCallContext{PrincipalID: "creator-a", SessionID: "http-a"}
	authorizer := NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: agent.V1Capabilities()})
	for _, method := range []string{"SubscribeSecretWatcher", "UnsubscribeSecretWatcher"} {
		if !authorizer.Authorize(call, method) {
			t.Fatalf("watch method %q was not authorized with secrets.watch", method)
		}
	}
	if NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{}).Authorize(call, "SubscribeSecretWatcher") {
		t.Fatal("watch method was authorized without secrets.watch")
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
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s response = %d %q %#v", method, response.Code, response.Body.String(), response.Header())
		}
	}
	if caller.callCount() != 2 {
		t.Fatalf("watch methods reached generated dispatch %d times", caller.callCount())
	}
}

func TestAcceleratorPolicyHTTPMatrixRejectsBeforeCaller(t *testing.T) {
	trusted := agent.AuthenticatedCallContext{PrincipalID: "creator-policy", SessionID: "http-policy"}
	methods := []struct {
		name       string
		capability agent.Capability
	}{
		{"ListSecretsMetadata", agent.CapabilitySecretsList},
		{"CancelListRequest", agent.CapabilitySecretsList},
		{"GetSecretData", agent.CapabilitySecretsDetail},
		{"GetSecretYaml", agent.CapabilitySecretsDetail},
		{"SubscribeSecretWatcher", agent.CapabilitySecretsWatch},
		{"UnsubscribeSecretWatcher", agent.CapabilitySecretsWatch},
	}
	for _, granted := range []agent.Capability{agent.CapabilitySecretsList, agent.CapabilitySecretsDetail, agent.CapabilitySecretsWatch} {
		t.Run(string(granted), func(t *testing.T) {
			caller := &recordingMethodCaller{}
			options := AcceleratorOptions(0, nil, func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), creatorContextKey{}, trusted)))
				})
			})
			options.MethodAuthorizer = NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: []agent.Capability{granted}})
			srv, err := NewWithOptions(caller, embed.FS{}, options)
			if err != nil {
				t.Fatal(err)
			}
			allowedCalls := 0
			for _, test := range methods {
				response := requestBoundary(srv.Handler(), http.MethodPost, "/api/call", "localhost", `{"method":"`+test.name+`","args":[{"PrincipalID":"forged","SessionID":"forged"}]}`)
				if test.capability == granted {
					allowedCalls++
					if response.Code != http.StatusOK {
						t.Fatalf("%s status = %d %q", test.name, response.Code, response.Body.String())
					}
				} else if response.Code != http.StatusForbidden || response.Body.String() != "{\"error\":\"forbidden\"}\n" {
					t.Fatalf("%s denied response = %d %q", test.name, response.Code, response.Body.String())
				}
			}
			if caller.callCount() != allowedCalls {
				t.Fatalf("caller calls = %d, want %d; forbidden calls must stop before dispatch", caller.callCount(), allowedCalls)
			}
			if caller.context != trusted {
				t.Fatalf("caller context = %#v, want server-owned %#v", caller.context, trusted)
			}
		})
	}
}

func TestBrowserBearerAuthorizesSecretWatchHTTPDispatch(t *testing.T) {
	sessions := NewBrowserSessionManager(NoopBrowserSessionRevoker{})
	ticket, _, err := sessions.Mint()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := sessions.Exchange(ticket)
	if err != nil {
		t.Fatal(err)
	}
	browserCall, ok := sessions.Authenticate(bearer)
	if !ok || !browserCall.IsAuthenticated() {
		t.Fatalf("browser bearer did not authenticate: %#v", browserCall)
	}
	caller := &recordingMethodCaller{}
	options := AcceleratorOptions(0, nil, CreatorOrBrowserGuard(nil, sessions))
	options.BrowserSessions = sessions
	options.MethodAuthorizer = NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: []agent.Capability{agent.CapabilitySecretsWatch}})
	srv, err := NewWithOptions(caller, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/call", strings.NewReader(`{"method":"SubscribeSecretWatcher","args":[{"PrincipalID":"forged","SessionID":"forged"},"evidence",false]}`))
	request.Host = "localhost"
	request.Header.Set("Authorization", "Bearer "+bearer.encoded())
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || caller.callCount() != 1 {
		t.Fatalf("browser watch dispatch = %d %q calls=%d", response.Code, response.Body.String(), caller.callCount())
	}
	if caller.context != browserCall || caller.method != "SubscribeSecretWatcher" {
		t.Fatalf("browser watch caller = context %#v method %q, want %#v", caller.context, caller.method, browserCall)
	}
	if len(caller.args) != 3 || strings.Contains(string(caller.args[0]), string(browserCall.PrincipalID)) || strings.Contains(string(caller.args[0]), string(browserCall.SessionID)) {
		t.Fatalf("browser server-owned identity/args = %#v / %#v", caller.context, caller.args)
	}
}
