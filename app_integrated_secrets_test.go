//go:build !accelerator

package main

import (
	"context"
	"strings"
	"testing"

	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

type recordingSecretReadRouter struct {
	unavailableSecretReadRouter
	retainedContext string
	releases        int
	tokens          []SecretReadSourceToken
	calls           []string
}

func (r *recordingSecretReadRouter) Retain(_ context.Context, contextName string) {
	r.retainedContext = contextName
}
func (r *recordingSecretReadRouter) Release() { r.releases++ }
func (r *recordingSecretReadRouter) record(name string, token SecretReadSourceToken) {
	r.calls = append(r.calls, name)
	r.tokens = append(r.tokens, token)
}
func (r *recordingSecretReadRouter) ListSecretsMetadata(_ context.Context, token SecretReadSourceToken, _, _ string, _ bool) ([]k8s.SecretListItem, error) {
	r.record("list", token)
	return []k8s.SecretListItem{{DataKeys: 1}}, nil
}
func (r *recordingSecretReadRouter) ListHelmReleaseMetadata(_ context.Context, token SecretReadSourceToken, _, _ string) ([]helm.Release, error) {
	r.record("helm-list", token)
	return []helm.Release{{Name: "release"}}, nil
}
func (r *recordingSecretReadRouter) GetSecretData(_ context.Context, token SecretReadSourceToken, _, _ string) ([]k8s.DataEntry, error) {
	r.record("data", token)
	return []k8s.DataEntry{{Key: "key"}}, nil
}
func (r *recordingSecretReadRouter) GetSecretYaml(_ context.Context, token SecretReadSourceToken, _, _ string) (string, error) {
	r.record("yaml", token)
	return "yaml", nil
}
func (r *recordingSecretReadRouter) CancelListRequest(_ context.Context, token SecretReadSourceToken, _ string) (bool, error) {
	r.record("cancel", token)
	return true, nil
}
func (r *recordingSecretReadRouter) SubscribeSecretWatcher(_ context.Context, token SecretReadSourceToken, _ string, _ bool) (acceleratorsecret.SecretWatchSubscription, error) {
	r.record("subscribe", token)
	return acceleratorsecret.SecretWatchSubscription{WatcherSpecID: "spec"}, nil
}
func (r *recordingSecretReadRouter) UnsubscribeSecretWatcher(_ context.Context, token SecretReadSourceToken, _ acceleratorsecret.SecretWatchSpecID) error {
	r.record("unsubscribe", token)
	return nil
}

func TestIntegratedSecretAppBridgesAreExactAndDesktopOnly(t *testing.T) {
	router := &recordingSecretReadRouter{}
	app := &App{runtimeMode: RuntimeModeDesktop, agentRouter: secretReadAgentRouter{base: NoopAgentRouter{}, secrets: router}, ctx: context.Background()}
	app.RetainIntegratedSecretReads()
	app.ReleaseIntegratedSecretReads()
	_, _ = app.ListIntegratedSecretsMetadata("opaque", "request", "ns", true)
	_, _ = app.ListIntegratedHelmReleaseMetadata("opaque", "request", "ns")
	_, _ = app.GetIntegratedSecretData("opaque", "ns", "name")
	_, _ = app.GetIntegratedSecretYaml("opaque", "ns", "name")
	_, _ = app.CancelIntegratedSecretListRequest("opaque", "request")
	_, _ = app.SubscribeIntegratedSecretWatcher("opaque", "ns", true)
	_ = app.UnsubscribeIntegratedSecretWatcher("opaque", "spec")
	if router.releases != 1 || strings.Join(router.calls, ",") != "list,helm-list,data,yaml,cancel,subscribe,unsubscribe" {
		t.Fatalf("release/calls=%d/%v", router.releases, router.calls)
	}
	for _, token := range router.tokens {
		if token != "opaque" {
			t.Fatalf("token=%q", token)
		}
	}
	serverApp := &App{runtimeMode: RuntimeModeServer, agentRouter: secretReadAgentRouter{base: NoopAgentRouter{}, secrets: router}}
	if _, err := serverApp.GetIntegratedSecretData("opaque", "ns", "name"); err == nil {
		t.Fatal("server mode reached integrated bridge")
	}
}
