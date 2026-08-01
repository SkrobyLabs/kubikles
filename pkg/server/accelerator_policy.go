package server

import "kubikles/pkg/agent"

type AcceleratorMethodAuthorizer struct{ capabilities map[agent.Capability]struct{} }

func NewAcceleratorMethodAuthorizer(resolution agent.CapabilityResolution) *AcceleratorMethodAuthorizer {
	a := &AcceleratorMethodAuthorizer{capabilities: make(map[agent.Capability]struct{})}
	for _, wanted := range agent.V1Capabilities() {
		for _, got := range resolution.Capabilities {
			if got == wanted {
				a.capabilities[wanted] = struct{}{}
			}
		}
	}
	return a
}
func (a *AcceleratorMethodAuthorizer) Authorize(call agent.AuthenticatedCallContext, method string) bool {
	if a == nil || !call.IsAuthenticated() || method == "SubscribeSecretWatcher" || method == "UnsubscribeSecretWatcher" {
		return false
	}
	policy, ok := agent.LookupMethodPolicy(method)
	if !ok {
		return false
	}
	_, ok = a.capabilities[policy.Capability]
	return ok
}
