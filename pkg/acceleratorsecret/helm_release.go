package acceleratorsecret

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
)

const (
	helmReleaseSecretType       = "helm.sh/release.v1"
	helmReleaseSecretNamePrefix = "sh.helm.release.v1."
	maxHelmReleasePayload       = 2 << 20
	maxHelmReleaseJSON          = 64 << 20
)

var errInvalidHelmReleasePayload = errors.New("invalid Helm release Secret payload")

// HelmReleaseMetadata is the closed list-only projection returned by the
// Accelerator. It contains no values, manifests, hooks, notes, or resources.
type HelmReleaseMetadata struct {
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	Revision     int       `json:"revision"`
	Status       string    `json:"status"`
	Chart        string    `json:"chart"`
	ChartVersion string    `json:"chartVersion"`
	AppVersion   string    `json:"appVersion"`
	Updated      time.Time `json:"updated"`
	Description  string    `json:"description"`
}

type storedReleaseProjection struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int    `json:"version"`
	Info      *struct {
		Status       string    `json:"status"`
		LastDeployed time.Time `json:"last_deployed"`
		Description  string    `json:"description"`
	} `json:"info"`
	Chart *struct {
		Metadata *struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
	} `json:"chart"`
}

type releaseSecretIdentity struct {
	name      string
	revision  int
	namespace string
}

// ProjectLatestHelmReleaseSecrets converts Helm 3 storage Secrets into latest
// release summaries. Invalid latest revisions are counted and skipped so one
// damaged release cannot hide every healthy release in the cluster.
func ProjectLatestHelmReleaseSecrets(secrets []v1.Secret) ([]HelmReleaseMetadata, int) {
	latest := make(map[string]releaseSecretIdentity)
	indices := make(map[string]int)
	for index := range secrets {
		secret := &secrets[index]
		if string(secret.Type) != helmReleaseSecretType || secret.Namespace == "" {
			continue
		}
		releaseName, revision, ok := parseHelmReleaseSecretName(secret.Name)
		if !ok {
			continue
		}
		key := secret.Namespace + "\x00" + releaseName
		if current, exists := latest[key]; exists && current.revision >= revision {
			continue
		}
		latest[key] = releaseSecretIdentity{name: releaseName, revision: revision, namespace: secret.Namespace}
		indices[key] = index
	}

	releases := make([]HelmReleaseMetadata, 0, len(latest))
	failures := 0
	for key, identity := range latest {
		decoded, err := decodeHelmReleasePayload(secrets[indices[key]].Data["release"])
		if err != nil || decoded.Name != identity.name || decoded.Namespace != identity.namespace || decoded.Revision != identity.revision {
			failures++
			continue
		}
		releases = append(releases, *decoded)
	}
	sort.Slice(releases, func(i, j int) bool {
		if !releases[i].Updated.Equal(releases[j].Updated) {
			return releases[i].Updated.After(releases[j].Updated)
		}
		if releases[i].Namespace != releases[j].Namespace {
			return releases[i].Namespace < releases[j].Namespace
		}
		return releases[i].Name < releases[j].Name
	})
	return releases, failures
}

func decodeHelmReleasePayload(payload []byte) (*HelmReleaseMetadata, error) {
	if len(payload) == 0 || len(payload) > maxHelmReleasePayload {
		return nil, errInvalidHelmReleasePayload
	}
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(payload)))
	n, err := base64.StdEncoding.Decode(decoded, payload)
	if err != nil || n == 0 {
		clear(decoded)
		return nil, errInvalidHelmReleasePayload
	}
	decoded = decoded[:n]
	defer clear(decoded)

	jsonPayload := decoded
	if len(decoded) >= 3 && bytes.Equal(decoded[:3], []byte{0x1f, 0x8b, 0x08}) {
		reader, gzipErr := gzip.NewReader(bytes.NewReader(decoded))
		if gzipErr != nil {
			return nil, errInvalidHelmReleasePayload
		}
		reader.Multistream(false)
		jsonPayload, err = io.ReadAll(io.LimitReader(reader, maxHelmReleaseJSON+1))
		closeErr := reader.Close()
		if err != nil || closeErr != nil || len(jsonPayload) == 0 || len(jsonPayload) > maxHelmReleaseJSON {
			clear(jsonPayload)
			return nil, errInvalidHelmReleasePayload
		}
		defer clear(jsonPayload)
	} else if len(jsonPayload) > maxHelmReleaseJSON {
		return nil, errInvalidHelmReleasePayload
	}

	var stored storedReleaseProjection
	if json.Unmarshal(jsonPayload, &stored) != nil || stored.Name == "" || stored.Namespace == "" || stored.Version < 1 || stored.Info == nil || stored.Chart == nil || stored.Chart.Metadata == nil || stored.Chart.Metadata.Name == "" {
		return nil, errInvalidHelmReleasePayload
	}
	return &HelmReleaseMetadata{
		Name: stored.Name, Namespace: stored.Namespace, Revision: stored.Version,
		Status: stored.Info.Status, Chart: stored.Chart.Metadata.Name,
		ChartVersion: stored.Chart.Metadata.Version, AppVersion: stored.Chart.Metadata.AppVersion,
		Updated: stored.Info.LastDeployed, Description: stored.Info.Description,
	}, nil
}

func parseHelmReleaseSecretName(name string) (string, int, bool) {
	if !strings.HasPrefix(name, helmReleaseSecretNamePrefix) {
		return "", 0, false
	}
	storageName := strings.TrimPrefix(name, helmReleaseSecretNamePrefix)
	versionAt := strings.LastIndex(storageName, ".v")
	if versionAt <= 0 || versionAt+2 >= len(storageName) {
		return "", 0, false
	}
	revision, err := strconv.Atoi(storageName[versionAt+2:])
	if err != nil || revision < 1 {
		return "", 0, false
	}
	return storageName[:versionAt], revision, true
}
