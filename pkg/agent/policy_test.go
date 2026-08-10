package agent

import "testing"

type expectedCapability string

type expectedResourceActionID string

type expectedMethodPolicy struct {
	Method     string
	Capability expectedCapability
	Action     expectedResourceActionID
}

func TestV1CapabilitiesExact(t *testing.T) {
	got := V1Capabilities()
	want := []expectedCapability{"secrets.list", "secrets.detail", "secrets.watch"}
	if len(got) != len(want) {
		t.Fatalf("capabilities = %#v", got)
	}
	seen := map[Capability]bool{}
	for i, capability := range got {
		if string(capability) != string(want[i]) || seen[capability] {
			t.Fatalf("capabilities = %#v", got)
		}
		seen[capability] = true
	}
	got[0] = "changed"
	if V1Capabilities()[0] != Capability("secrets.list") {
		t.Fatal("capabilities catalog was mutable")
	}
}

func TestV1ResourceActionsExact(t *testing.T) {
	got := V1ResourceActions()
	want := []struct {
		Action   expectedResourceActionID
		Group    string
		Version  string
		Resource string
		Verb     string
	}{{Action: "core/v1/secrets:get", Version: "v1", Resource: "secrets", Verb: "get"}, {Action: "core/v1/secrets:list", Version: "v1", Resource: "secrets", Verb: "list"}, {Action: "core/v1/secrets:watch", Version: "v1", Resource: "secrets", Verb: "watch"}}
	if len(got) != len(want) {
		t.Fatalf("actions = %#v", got)
	}
	seen := map[ResourceActionID]bool{}
	for i, action := range got {
		if string(action.Action) != string(want[i].Action) || action.Group != want[i].Group || action.Version != want[i].Version || action.Resource != want[i].Resource || action.Verb != want[i].Verb || seen[action.Action] {
			t.Fatalf("actions = %#v", got)
		}
		seen[action.Action] = true
		if action.Verb == "create" || action.Verb == "update" || action.Verb == "patch" || action.Verb == "delete" || action.Verb == "deletecollection" || action.Verb == "impersonate" || action.Verb == "escalate" || action.Verb == "bind" {
			t.Fatalf("forbidden verb %q", action.Verb)
		}
	}
}

func TestV1MethodPoliciesExact(t *testing.T) {
	want := []expectedMethodPolicy{
		{Method: "ListSecretsMetadata", Capability: "secrets.list", Action: "core/v1/secrets:list"},
		{Method: "ListHelmReleaseMetadata", Capability: "secrets.list", Action: "core/v1/secrets:list"},
		{Method: "GetSecretData", Capability: "secrets.detail", Action: "core/v1/secrets:get"},
		{Method: "GetSecretYaml", Capability: "secrets.detail", Action: "core/v1/secrets:get"},
		{Method: "CancelListRequest", Capability: "secrets.list", Action: ""},
		{Method: "SubscribeSecretWatcher", Capability: "secrets.watch", Action: "core/v1/secrets:watch"},
		{Method: "UnsubscribeSecretWatcher", Capability: "secrets.watch", Action: ""},
	}
	got := V1MethodPolicies()
	if len(got) != len(want) {
		t.Fatalf("policies = %#v", got)
	}
	capabilities := map[Capability]bool{}
	for _, capability := range V1Capabilities() {
		capabilities[capability] = true
	}
	actions := map[ResourceActionID]bool{ResourceActionID(""): true}
	for _, action := range V1ResourceActions() {
		actions[action.Action] = true
	}
	seen := map[string]bool{}
	for i, policy := range got {
		expected := MethodPolicy{Method: want[i].Method, Capability: Capability(want[i].Capability), Action: ResourceActionID(want[i].Action)}
		if policy != expected || seen[policy.Method] || !capabilities[policy.Capability] || !actions[policy.Action] {
			t.Fatalf("policies = %#v", got)
		}
		lookup, ok := LookupMethodPolicy(expected.Method)
		if !ok || lookup != expected {
			t.Fatalf("lookup %q = %#v, %v", expected.Method, lookup, ok)
		}
		if string(policy.Action) == "" && policy.Method != "CancelListRequest" && policy.Method != "UnsubscribeSecretWatcher" {
			t.Fatalf("unexpected no-action policy: %#v", policy)
		}
		seen[policy.Method] = true
	}
	got[0].Method = "changed"
	if V1MethodPolicies()[0].Method != "ListSecretsMetadata" {
		t.Fatal("method policy catalog was mutable")
	}
}

func TestLookupMethodPolicyFailsClosed(t *testing.T) {
	for _, method := range []string{"ListSecrets", "ListConfigMaps", "ListPods", "ListHelmReleases", "SubscribeResourceWatcher", "SubscribeCRDWatcher", "UpdateSecretData", "DeleteSecret", "ApplyResourceYaml", "", "listsecretsmetadata", "FutureReadMethod"} {
		if policy, ok := LookupMethodPolicy(method); ok {
			t.Fatalf("%q unexpectedly allowed: %#v", method, policy)
		}
	}
}
