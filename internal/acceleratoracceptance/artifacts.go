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
	"strings"
)

var (
	artifactCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	artifactDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type ArtifactFixture struct {
	Descriptor             []byte
	Checksum               []byte
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
	descriptor, err := read("kubikles-accelerator-release-v0.0.0.json")
	if err != nil {
		return ArtifactFixture{}, err
	}
	checksum, err := read("kubikles-accelerator-release-v0.0.0.json.sha256")
	if err != nil {
		return ArtifactFixture{}, err
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
		Descriptor: descriptor, Checksum: checksum, ImageEvidence: evidence, AcceptanceImageIndex: acceptanceIndex,
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

type artifactDescriptor struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Schema        string                `json:"$schema"`
	BuildVersion  string                `json:"buildVersion"`
	Source        artifactSource        `json:"source"`
	Compatibility artifactCompatibility `json:"compatibility"`
	Image         artifactImage         `json:"image"`
	Chart         artifactChart         `json:"chart"`
}

type artifactSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	GitTag     string `json:"gitTag"`
}

type artifactCompatibility struct {
	Mode                    string `json:"mode"`
	DesktopBuildVersion     string `json:"desktopBuildVersion"`
	AcceleratorBuildVersion string `json:"acceleratorBuildVersion"`
}

type artifactImage struct {
	Repository string             `json:"repository"`
	Digest     string             `json:"digest"`
	Reference  string             `json:"reference"`
	Platforms  []artifactPlatform `json:"platforms"`
}

type artifactPlatform struct {
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	ManifestDigest string `json:"manifestDigest"`
}

type artifactChart struct {
	Repository string `json:"repository"`
	Digest     string `json:"digest"`
	Reference  string `json:"reference"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
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
	if len(fixture.Descriptor) == 0 || len(fixture.Checksum) == 0 || len(fixture.ImageEvidence) == 0 || len(fixture.AcceptanceImageIndex) == 0 {
		return errors.New("artifact fixture invalid")
	}
	var descriptor artifactDescriptor
	if decodeArtifactJSON(fixture.Descriptor, &descriptor) != nil {
		return errors.New("artifact fixture invalid")
	}
	var evidence artifactImageEvidence
	if decodeArtifactJSON(fixture.ImageEvidence, &evidence) != nil {
		return errors.New("artifact fixture invalid")
	}
	if !validArtifactDescriptor(descriptor) || !validArtifactEvidence(evidence) {
		return errors.New("artifact fixture invalid")
	}
	var acceptanceIndex artifactAcceptanceImageIndex
	if decodeArtifactJSON(fixture.AcceptanceImageIndex, &acceptanceIndex) != nil || !validAcceptanceImageIndex(acceptanceIndex, fixture.ExecutionArchitecture) {
		return errors.New("artifact fixture invalid")
	}
	sum := sha256.Sum256(fixture.Descriptor)
	wantChecksum := hex.EncodeToString(sum[:]) + "  kubikles-accelerator-release-v0.0.0.json\n"
	if string(fixture.Checksum) != wantChecksum {
		return errors.New("artifact fixture invalid")
	}
	acceptanceSum := sha256.Sum256(fixture.AcceptanceImageIndex)
	if fixture.RegistryImageDigest != descriptor.Image.Digest || fixture.RegistryChartDigest != descriptor.Chart.Digest || fixture.RegistryImageDigest == fixture.RegistryChartDigest || fixture.AcceptanceImageDigest == fixture.RegistryImageDigest || fixture.AcceptanceImageDigest != "sha256:"+hex.EncodeToString(acceptanceSum[:]) || fixture.ReconnectGraceSeconds < 1 || fixture.ReconnectGraceSeconds > 30 || evidence.ImageDigest != descriptor.Image.Digest {
		return errors.New("artifact fixture invalid")
	}
	if fixture.ChartVersion != descriptor.Chart.Version || fixture.ChartAppVersion != descriptor.Chart.AppVersion || fixture.RuntimeBuildVersion != BuildIdentity || fixture.ReleaseArchitecture != "amd64" || !validExecutionArchitecture(fixture.ExecutionArchitecture) {
		return errors.New("artifact fixture invalid")
	}
	selected := ""
	for index, platform := range descriptor.Image.Platforms {
		if platform != evidence.Platforms[index] {
			return errors.New("artifact fixture invalid")
		}
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

func validArtifactDescriptor(descriptor artifactDescriptor) bool {
	if descriptor.SchemaVersion != 1 || descriptor.BuildVersion != BuildIdentity || !artifactCommitPattern.MatchString(descriptor.Source.Commit) {
		return false
	}
	if descriptor.Schema != "https://raw.githubusercontent.com/SkrobyLabs/kubikles/"+descriptor.Source.Commit+"/release/accelerator-release.schema.json" || descriptor.Source.Repository != "https://github.com/SkrobyLabs/kubikles" || descriptor.Source.GitTag != BuildIdentity {
		return false
	}
	if descriptor.Compatibility != (artifactCompatibility{Mode: "exact-build-version", DesktopBuildVersion: BuildIdentity, AcceleratorBuildVersion: BuildIdentity}) {
		return false
	}
	if descriptor.Image.Repository != "ghcr.io/skrobylabs/kubikles-accelerator" || !artifactDigestPattern.MatchString(descriptor.Image.Digest) || descriptor.Image.Reference != descriptor.Image.Repository+"@"+descriptor.Image.Digest || strings.Contains(descriptor.Image.Reference, ":v") {
		return false
	}
	if descriptor.Chart.Repository != "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator" || !artifactDigestPattern.MatchString(descriptor.Chart.Digest) || descriptor.Chart.Reference != descriptor.Chart.Repository+"@"+descriptor.Chart.Digest || descriptor.Chart.Version != "0.0.0" || descriptor.Chart.AppVersion != BuildIdentity {
		return false
	}
	return validArtifactPlatforms(descriptor.Image.Platforms)
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
