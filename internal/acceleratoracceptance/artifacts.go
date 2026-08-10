package acceleratoracceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var (
	artifactDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type ArtifactFixture struct {
	ImageEvidence          []byte
	AcceptanceImageIndex   []byte
	RegistryImageDigest    string
	AcceptanceImageDigest  string
	ReconnectGraceSeconds  int
	RegistryChartDigest    string
	ChartVersion           string
	ChartAppVersion        string
	RuntimeBuildVersion    string
	ReleaseArchitecture    string
	ExecutionArchitecture  string
	SelectedManifestDigest string
}

func LoadArtifactFixture(root string) (ArtifactFixture, error) {
	if root == "" || !filepath.IsAbs(root) {
		return ArtifactFixture{}, errors.New("artifact fixture invalid")
	}
	read := func(name string) ([]byte, error) {
		value, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || len(value) == 0 || len(value) > maximumAcceptanceJSONBytes {
			return nil, errors.New("artifact fixture invalid")
		}
		return value, nil
	}
	evidence, err := read("acceptance-image-evidence.json")
	if err != nil {
		return ArtifactFixture{}, err
	}
	acceptanceIndex, err := read("acceptance-image-index.json")
	if err != nil {
		return ArtifactFixture{}, err
	}
	metadataBytes, err := read("acceptance-artifact-metadata.json")
	if err != nil {
		return ArtifactFixture{}, err
	}
	var metadata struct {
		RegistryImageDigest    string `json:"registryImageDigest"`
		AcceptanceImageDigest  string `json:"acceptanceImageDigest"`
		ReconnectGraceSeconds  int    `json:"reconnectGraceSeconds"`
		RegistryChartDigest    string `json:"registryChartDigest"`
		ChartVersion           string `json:"chartVersion"`
		ChartAppVersion        string `json:"chartAppVersion"`
		RuntimeBuildVersion    string `json:"runtimeBuildVersion"`
		ReleaseArchitecture    string `json:"releaseArchitecture"`
		ExecutionArchitecture  string `json:"executionArchitecture"`
		SelectedManifestDigest string `json:"selectedManifestDigest"`
	}
	if decodeArtifactJSON(metadataBytes, &metadata) != nil {
		return ArtifactFixture{}, errors.New("artifact fixture invalid")
	}
	fixture := ArtifactFixture{
		ImageEvidence: evidence, AcceptanceImageIndex: acceptanceIndex,
		RegistryImageDigest: metadata.RegistryImageDigest, RegistryChartDigest: metadata.RegistryChartDigest,
		AcceptanceImageDigest: metadata.AcceptanceImageDigest, ReconnectGraceSeconds: metadata.ReconnectGraceSeconds,
		ChartVersion: metadata.ChartVersion, ChartAppVersion: metadata.ChartAppVersion,
		RuntimeBuildVersion: metadata.RuntimeBuildVersion, ReleaseArchitecture: metadata.ReleaseArchitecture,
		ExecutionArchitecture:  metadata.ExecutionArchitecture,
		SelectedManifestDigest: metadata.SelectedManifestDigest,
	}
	if ValidateArtifactFixture(fixture) != nil {
		return ArtifactFixture{}, errors.New("artifact fixture invalid")
	}
	return fixture, nil
}

type artifactPlatform struct {
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	ManifestDigest string `json:"manifestDigest"`
}

type artifactImageEvidence struct {
	ImageDigest string               `json:"imageDigest"`
	Platforms   []artifactPlatform   `json:"platforms"`
	Inspections []artifactInspection `json:"inspections"`
}

type artifactInspection struct {
	Architecture       string `json:"architecture"`
	BinaryBuildVersion string `json:"binaryBuildVersion"`
	ImageBuildVersion  string `json:"imageBuildVersion"`
}

type artifactAcceptanceImageIndex struct {
	SchemaVersion int                               `json:"schemaVersion"`
	MediaType     string                            `json:"mediaType"`
	Manifests     []artifactAcceptanceImageManifest `json:"manifests"`
}

type artifactAcceptanceImageManifest struct {
	MediaType string           `json:"mediaType"`
	Digest    string           `json:"digest"`
	Size      int64            `json:"size"`
	Platform  artifactPlatform `json:"platform"`
}

func ValidateArtifactFixture(fixture ArtifactFixture) error {
	if len(fixture.ImageEvidence) == 0 || len(fixture.AcceptanceImageIndex) == 0 {
		return errors.New("artifact fixture invalid")
	}
	var evidence artifactImageEvidence
	if decodeArtifactJSON(fixture.ImageEvidence, &evidence) != nil {
		return errors.New("artifact fixture invalid")
	}
	if !validArtifactEvidence(evidence) {
		return errors.New("artifact fixture invalid")
	}
	var acceptanceIndex artifactAcceptanceImageIndex
	if decodeArtifactJSON(fixture.AcceptanceImageIndex, &acceptanceIndex) != nil || !validAcceptanceImageIndex(acceptanceIndex, fixture.ExecutionArchitecture) {
		return errors.New("artifact fixture invalid")
	}
	acceptanceSum := sha256.Sum256(fixture.AcceptanceImageIndex)
	if !artifactDigestPattern.MatchString(fixture.RegistryImageDigest) || !artifactDigestPattern.MatchString(fixture.RegistryChartDigest) || fixture.RegistryImageDigest != evidence.ImageDigest || fixture.RegistryImageDigest == fixture.RegistryChartDigest || fixture.AcceptanceImageDigest == fixture.RegistryImageDigest || fixture.AcceptanceImageDigest != "sha256:"+hex.EncodeToString(acceptanceSum[:]) || fixture.ReconnectGraceSeconds < 1 || fixture.ReconnectGraceSeconds > 30 {
		return errors.New("artifact fixture invalid")
	}
	if fixture.ChartVersion != "0.0.0" || fixture.ChartAppVersion != BuildIdentity || fixture.RuntimeBuildVersion != BuildIdentity || fixture.ReleaseArchitecture != "amd64" || !validExecutionArchitecture(fixture.ExecutionArchitecture) {
		return errors.New("artifact fixture invalid")
	}
	selected := ""
	for _, platform := range evidence.Platforms {
		if platform.Architecture == fixture.ReleaseArchitecture {
			selected = platform.ManifestDigest
		}
	}
	if selected == "" || selected != fixture.SelectedManifestDigest {
		return errors.New("artifact fixture invalid")
	}
	return nil
}

func validExecutionArchitecture(value string) bool {
	return value == "amd64" || value == "arm64"
}

func validAcceptanceImageIndex(index artifactAcceptanceImageIndex, executionArchitecture string) bool {
	if index.SchemaVersion != 2 || index.MediaType != "application/vnd.oci.image.index.v1+json" || !validExecutionArchitecture(executionArchitecture) || len(index.Manifests) != 1 {
		return false
	}
	manifest := index.Manifests[0]
	return manifest.MediaType == "application/vnd.docker.distribution.manifest.v2+json" && artifactDigestPattern.MatchString(manifest.Digest) && manifest.Size > 0 && manifest.Platform.OS == "linux" && manifest.Platform.Architecture == executionArchitecture && manifest.Platform.ManifestDigest == ""
}

func decodeArtifactJSON(data []byte, target any) error {
	strict, err := readStrictJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(strict))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

func validArtifactPlatforms(platforms []artifactPlatform) bool {
	if len(platforms) != 1 || platforms[0].OS != "linux" || platforms[0].Architecture != "amd64" {
		return false
	}
	return artifactDigestPattern.MatchString(platforms[0].ManifestDigest)
}

func validArtifactEvidence(evidence artifactImageEvidence) bool {
	if !artifactDigestPattern.MatchString(evidence.ImageDigest) || !validArtifactPlatforms(evidence.Platforms) || len(evidence.Inspections) != 1 {
		return false
	}
	for index, architecture := range []string{"amd64"} {
		inspection := evidence.Inspections[index]
		if inspection.Architecture != architecture || inspection.BinaryBuildVersion != BuildIdentity || inspection.ImageBuildVersion != BuildIdentity {
			return false
		}
	}
	return true
}
