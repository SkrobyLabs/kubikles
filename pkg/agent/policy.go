package agent

// ResourceActionID identifies a fixed Kubernetes resource action.
type ResourceActionID string

const (
	ResourceActionCoreV1SecretsGet   ResourceActionID = "core/v1/secrets:get"
	ResourceActionCoreV1SecretsList  ResourceActionID = "core/v1/secrets:list"
	ResourceActionCoreV1SecretsWatch ResourceActionID = "core/v1/secrets:watch"

	// NoKubernetesAction is used only by cancellation and unsubscribe lifecycle methods.
	NoKubernetesAction ResourceActionID = ""
)

// ResourceAction is a fixed Kubernetes resource and verb authority entry.
type ResourceAction struct {
	Action   ResourceActionID
	Group    string
	Version  string
	Resource string
	Verb     string
}

// MethodPolicy maps one exact method name to a capability and Kubernetes action.
type MethodPolicy struct {
	Method     string
	Capability Capability
	Action     ResourceActionID
}

var v1Capabilities = []Capability{
	CapabilitySecretsList,
	CapabilitySecretsDetail,
	CapabilitySecretsWatch,
}

var v1ResourceActions = []ResourceAction{
	{Action: ResourceActionCoreV1SecretsGet, Group: "", Version: "v1", Resource: "secrets", Verb: "get"},
	{Action: ResourceActionCoreV1SecretsList, Group: "", Version: "v1", Resource: "secrets", Verb: "list"},
	{Action: ResourceActionCoreV1SecretsWatch, Group: "", Version: "v1", Resource: "secrets", Verb: "watch"},
}

var v1MethodPolicies = []MethodPolicy{
	{Method: "ListSecretsMetadata", Capability: CapabilitySecretsList, Action: ResourceActionCoreV1SecretsList},
	{Method: "ListHelmReleaseMetadata", Capability: CapabilitySecretsList, Action: ResourceActionCoreV1SecretsList},
	{Method: "GetSecretData", Capability: CapabilitySecretsDetail, Action: ResourceActionCoreV1SecretsGet},
	{Method: "GetSecretYaml", Capability: CapabilitySecretsDetail, Action: ResourceActionCoreV1SecretsGet},
	{Method: "CancelListRequest", Capability: CapabilitySecretsList, Action: NoKubernetesAction},
	{Method: "SubscribeSecretWatcher", Capability: CapabilitySecretsWatch, Action: ResourceActionCoreV1SecretsWatch},
	{Method: "UnsubscribeSecretWatcher", Capability: CapabilitySecretsWatch, Action: NoKubernetesAction},
}

// V1Capabilities returns the ordered Accelerator v1 capability catalog.
func V1Capabilities() []Capability { return append([]Capability(nil), v1Capabilities...) }

// V1ResourceActions returns the ordered fixed Kubernetes authority catalog.
func V1ResourceActions() []ResourceAction { return append([]ResourceAction(nil), v1ResourceActions...) }

// V1MethodPolicies returns the ordered exact Accelerator method policy catalog.
func V1MethodPolicies() []MethodPolicy { return append([]MethodPolicy(nil), v1MethodPolicies...) }

// LookupMethodPolicy returns a policy only for an exact v1 Accelerator method name.
func LookupMethodPolicy(method string) (MethodPolicy, bool) {
	for _, policy := range v1MethodPolicies {
		if policy.Method == method {
			return policy, true
		}
	}
	return MethodPolicy{}, false
}
