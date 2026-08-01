package main

import (
	"errors"
	"kubikles/pkg/agent"
)

func (a *App) SubscribeSecretWatcher(callContext agent.AuthenticatedCallContext, namespace string, excludeHelmReleases bool) (SecretWatchSubscription, error) {
	if a == nil || a.acceleratorSecretWatches == nil {
		return SecretWatchSubscription{}, errors.New("Accelerator Secret watch unavailable")
	}
	return a.acceleratorSecretWatches.Subscribe(callContext, namespace, excludeHelmReleases)
}
func (a *App) UnsubscribeSecretWatcher(callContext agent.AuthenticatedCallContext, watcherSpecID string) error {
	if a == nil || a.acceleratorSecretWatches == nil {
		return nil
	}
	return a.acceleratorSecretWatches.Unsubscribe(callContext, SecretWatchSpecID(watcherSpecID))
}
