package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
)

type SecretWatchSpecID string
type secretWatchSpec struct {
	namespace           string
	excludeHelmReleases bool
}

func (s secretWatchSpec) id() SecretWatchSpecID {
	const domain = "kubikles/accelerator/secret-watch-spec/v1\x00"
	b := make([]byte, len(domain)+4+len(s.namespace)+1)
	copy(b, domain)
	binary.BigEndian.PutUint32(b[len(domain):], uint32(len(s.namespace)))
	copy(b[len(domain)+4:], s.namespace)
	if s.excludeHelmReleases {
		b[len(b)-1] = 1
	}
	digest := sha256.Sum256(b)
	return SecretWatchSpecID(base64.RawURLEncoding.EncodeToString(digest[:]))
}

type SecretWatchSubscription struct {
	WatcherSpecID SecretWatchSpecID `json:"watcherSpecId"`
}
type AcceleratorSecretResourceEvent struct {
	Type          string                    `json:"type"`
	ResourceType  string                    `json:"resourceType"`
	Namespace     string                    `json:"namespace"`
	WatcherSpecID SecretWatchSpecID         `json:"watcherSpecId"`
	Resource      acceleratorSecretListItem `json:"resource"`
}
type AcceleratorSecretWatcherStatus struct {
	WatcherSpecID SecretWatchSpecID `json:"watcherSpecId"`
	Status        string            `json:"status"`
}
type AcceleratorSecretWatcherError struct {
	WatcherSpecID SecretWatchSpecID `json:"watcherSpecId"`
	Code          string            `json:"code"`
	Recoverable   bool              `json:"recoverable"`
}
