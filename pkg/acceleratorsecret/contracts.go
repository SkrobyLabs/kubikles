// Package acceleratorsecret defines the closed Secret-only Accelerator wire contract.
package acceleratorsecret

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubikles/pkg/k8s"
)

const secretWatchSpecDomain = "kubikles/accelerator/secret-watch-spec/v1\x00"

type SecretWatchSpecID string

func SecretWatchSpecIDFor(namespace string, excludeHelmReleases bool) SecretWatchSpecID {
	b := make([]byte, len(secretWatchSpecDomain)+4+len(namespace)+1)
	copy(b, secretWatchSpecDomain)
	binary.BigEndian.PutUint32(b[len(secretWatchSpecDomain):], uint32(len(namespace)))
	copy(b[len(secretWatchSpecDomain)+4:], namespace)
	if excludeHelmReleases {
		b[len(b)-1] = 1
	}
	digest := sha256.Sum256(b)
	return SecretWatchSpecID(base64.RawURLEncoding.EncodeToString(digest[:]))
}

func ValidSecretWatchSpecID(id SecretWatchSpecID) bool {
	if len(id) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(string(id))
	return err == nil && len(decoded) == sha256.Size && base64.RawURLEncoding.EncodeToString(decoded) == string(id)
}

const (
	EventResource                    = "resource-event"
	EventWatcherStatus               = "watcher-status"
	EventWatcherError                = "watcher-error"
	SecretResourceType               = "secrets"
	WatchStatusConnected             = "connected"
	WatchStatusReconnecting          = "reconnecting"
	WatchErrorUnavailable            = "watch_unavailable"
	WatchErrorResourceVersionExpired = "resource_version_expired"
	WatchErrorMalformed              = "malformed_watch_event"
)

type SecretWatchSubscription struct {
	WatcherSpecID SecretWatchSpecID `json:"watcherSpecId"`
}

// SecretListItem is the closed value-free Accelerator summary. It deliberately
// excludes the richer labels and annotations present on the desktop DTO.
type SecretListItem struct {
	Metadata struct {
		Name              string      `json:"name"`
		Namespace         string      `json:"namespace"`
		UID               string      `json:"uid"`
		CreationTimestamp metav1.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Type     string `json:"type"`
	DataKeys int    `json:"dataKeys"`
}

func ProjectSecretListItem(item k8s.SecretListItem) SecretListItem {
	var projected SecretListItem
	projected.Metadata.Name = item.Metadata.Name
	projected.Metadata.Namespace = item.Metadata.Namespace
	projected.Metadata.UID = item.Metadata.UID
	projected.Metadata.CreationTimestamp = item.Metadata.CreationTimestamp
	projected.Type = item.Type
	projected.DataKeys = item.DataKeys
	return projected
}

func KubernetesSecretListItems(items []SecretListItem) []k8s.SecretListItem {
	converted := make([]k8s.SecretListItem, len(items))
	for index, item := range items {
		converted[index].Metadata.Name = item.Metadata.Name
		converted[index].Metadata.Namespace = item.Metadata.Namespace
		converted[index].Metadata.UID = item.Metadata.UID
		converted[index].Metadata.CreationTimestamp = item.Metadata.CreationTimestamp
		converted[index].Metadata.Labels = nil
		converted[index].Metadata.Annotations = nil
		converted[index].Type = item.Type
		converted[index].DataKeys = item.DataKeys
	}
	return converted
}

type SecretResourceEvent struct {
	Type          string            `json:"type"`
	ResourceType  string            `json:"resourceType"`
	Namespace     string            `json:"namespace"`
	WatcherSpecID SecretWatchSpecID `json:"watcherSpecId"`
	Resource      SecretListItem    `json:"resource"`
}

type SecretWatcherStatus struct {
	WatcherSpecID SecretWatchSpecID `json:"watcherSpecId"`
	Status        string            `json:"status"`
}

type SecretWatcherError struct {
	WatcherSpecID SecretWatchSpecID `json:"watcherSpecId"`
	Code          string            `json:"code"`
	Recoverable   bool              `json:"recoverable"`
}
