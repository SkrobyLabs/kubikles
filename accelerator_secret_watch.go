package main

import (
	"kubikles/pkg/acceleratorsecret"
)

type SecretWatchSpecID = acceleratorsecret.SecretWatchSpecID
type secretWatchSpec struct {
	namespace           string
	excludeHelmReleases bool
}

func (s secretWatchSpec) id() SecretWatchSpecID {
	return acceleratorsecret.SecretWatchSpecIDFor(s.namespace, s.excludeHelmReleases)
}

type SecretWatchSubscription = acceleratorsecret.SecretWatchSubscription
type AcceleratorSecretResourceEvent = acceleratorsecret.SecretResourceEvent
type AcceleratorSecretWatcherStatus = acceleratorsecret.SecretWatcherStatus
type AcceleratorSecretWatcherError = acceleratorsecret.SecretWatcherError
