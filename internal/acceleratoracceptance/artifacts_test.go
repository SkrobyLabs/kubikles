package acceleratoracceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func exactArtifactFixture(t *testing.T) ArtifactFixture {
	t.Helper()
	digest := func(value byte) string { return "sha256:" + strings.Repeat(string(value), 64) }
	platforms := []artifactPlatform{{OS: "linux", Architecture: "amd64", ManifestDigest: digest('b')}}
	evidence, err := json.Marshal(artifactImageEvidence{ImageDigest: digest('a'), Platforms: platforms, Inspections: []artifactInspection{{Architecture: "amd64", BinaryBuildVersion: BuildIdentity, ImageBuildVersion: BuildIdentity}}})
	if err != nil {
		t.Fatal("marshal evidence fixture")
	}
	acceptanceIndex, err := json.Marshal(artifactAcceptanceImageIndex{
		SchemaVersion: 2, MediaType: "application/vnd.oci.image.index.v1+json",
		Manifests: []artifactAcceptanceImageManifest{{
			MediaType: "application/vnd.docker.distribution.manifest.v2+json", Digest: digest('e'), Size: 123,
			Platform: artifactPlatform{OS: "linux", Architecture: "amd64"},
		}},
	})
	if err != nil {
		t.Fatal("marshal acceptance index fixture")
	}
	acceptanceIndex = append(acceptanceIndex, '\n')
	acceptanceSum := sha256.Sum256(acceptanceIndex)
	return ArtifactFixture{
		ImageEvidence: append(evidence, '\n'), AcceptanceImageIndex: acceptanceIndex,
		RegistryImageDigest: digest('a'), AcceptanceImageDigest: "sha256:" + hex.EncodeToString(acceptanceSum[:]), RegistryChartDigest: digest('d'), ReconnectGraceSeconds: 5,
		ChartVersion: "0.0.0", ChartAppVersion: BuildIdentity,
		RuntimeBuildVersion: BuildIdentity, ReleaseArchitecture: "amd64", ExecutionArchitecture: "amd64", SelectedManifestDigest: digest('b'),
	}
}

func TestAcceptanceExactLocalReleaseFixture(t *testing.T) {
	fixture := exactArtifactFixture(t)
	if err := ValidateArtifactFixture(fixture); err != nil {
		t.Fatal("canonical artifact fixture failed")
	}
	t.Run("native arm64 acceptance runtime with amd64 release", func(t *testing.T) {
		candidate := exactArtifactFixture(t)
		var index artifactAcceptanceImageIndex
		if err := json.Unmarshal(candidate.AcceptanceImageIndex, &index); err != nil {
			t.Fatal("decode acceptance index")
		}
		index.Manifests[0].Platform.Architecture = "arm64"
		encoded, err := json.Marshal(index)
		if err != nil {
			t.Fatal("encode acceptance index")
		}
		candidate.AcceptanceImageIndex = append(encoded, '\n')
		sum := sha256.Sum256(candidate.AcceptanceImageIndex)
		candidate.AcceptanceImageDigest = "sha256:" + hex.EncodeToString(sum[:])
		candidate.ExecutionArchitecture = "arm64"
		if err := ValidateArtifactFixture(candidate); err != nil {
			t.Fatal("native acceptance runtime rejected")
		}
	})
	mutations := []struct {
		name   string
		mutate func(*ArtifactFixture)
	}{
		{"missing platform", func(f *ArtifactFixture) {
			f.ImageEvidence = []byte(strings.Replace(string(f.ImageEvidence), `[{"os":"linux","architecture":"amd64","manifestDigest":"sha256:`+strings.Repeat("b", 64)+`"}]`, `[]`, 1))
		}},
		{"extra platform", func(f *ArtifactFixture) {
			f.ImageEvidence = []byte(strings.Replace(string(f.ImageEvidence), `]`, `,{"os":"linux","architecture":"s390x","manifestDigest":"sha256:`+strings.Repeat("e", 64)+`"}]`, 1))
		}},
		{"platform digest", func(f *ArtifactFixture) { f.SelectedManifestDigest = "sha256:" + strings.Repeat("f", 64) }},
		{"chart digest", func(f *ArtifactFixture) { f.RegistryChartDigest = "invalid" }},
		{"chart version", func(f *ArtifactFixture) { f.ChartVersion = "0.0.1" }},
		{"chart app version", func(f *ArtifactFixture) { f.ChartAppVersion = "v0.0.1" }},
		{"registry image", func(f *ArtifactFixture) { f.RegistryImageDigest = "sha256:" + strings.Repeat("e", 64) }},
		{"runtime version", func(f *ArtifactFixture) { f.RuntimeBuildVersion = "v0.0.1" }},
		{"release architecture", func(f *ArtifactFixture) { f.ReleaseArchitecture = "arm64" }},
		{"execution architecture", func(f *ArtifactFixture) { f.ExecutionArchitecture = "s390x" }},
		{"acceptance platform mismatch", func(f *ArtifactFixture) { f.ExecutionArchitecture = "arm64" }},
		{"acceptance index digest", func(f *ArtifactFixture) { f.AcceptanceImageIndex[0] = ' ' }},
		{"binary version", func(f *ArtifactFixture) {
			f.ImageEvidence = []byte(strings.Replace(string(f.ImageEvidence), `"binaryBuildVersion":"v0.0.0"`, `"binaryBuildVersion":"v0.0.1"`, 1))
		}},
		{"image label", func(f *ArtifactFixture) {
			f.ImageEvidence = []byte(strings.Replace(string(f.ImageEvidence), `"imageBuildVersion":"v0.0.0"`, `"imageBuildVersion":"v0.0.1"`, 1))
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			candidate := exactArtifactFixture(t)
			test.mutate(&candidate)
			if ValidateArtifactFixture(candidate) == nil {
				t.Fatal("artifact mutation was accepted")
			}
		})
	}
}
