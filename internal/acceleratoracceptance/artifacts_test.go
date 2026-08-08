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
	descriptor := artifactDescriptor{
		SchemaVersion: 1,
		Schema:        "https://raw.githubusercontent.com/SkrobyLabs/kubikles/" + strings.Repeat("c", 40) + "/release/accelerator-release.schema.json",
		BuildVersion:  BuildIdentity,
		Source:        artifactSource{Repository: "https://github.com/SkrobyLabs/kubikles", Commit: strings.Repeat("c", 40), GitTag: BuildIdentity},
		Compatibility: artifactCompatibility{Mode: "exact-build-version", DesktopBuildVersion: BuildIdentity, AcceleratorBuildVersion: BuildIdentity},
		Image:         artifactImage{Repository: "ghcr.io/skrobylabs/kubikles-accelerator", Digest: digest('a'), Reference: "ghcr.io/skrobylabs/kubikles-accelerator@" + digest('a'), Platforms: []artifactPlatform{{OS: "linux", Architecture: "amd64", ManifestDigest: digest('b')}}},
		Chart:         artifactChart{Repository: "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator", Digest: digest('d'), Reference: "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator@" + digest('d'), Version: "0.0.0", AppVersion: BuildIdentity},
	}
	descriptorBytes, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal("marshal descriptor fixture")
	}
	descriptorBytes = append(descriptorBytes, '\n')
	sum := sha256.Sum256(descriptorBytes)
	checksum := []byte(hex.EncodeToString(sum[:]) + "  kubikles-accelerator-release-v0.0.0.json\n")
	evidence, err := json.Marshal(artifactImageEvidence{ImageDigest: digest('a'), Platforms: descriptor.Image.Platforms, Inspections: []artifactInspection{{Architecture: "amd64", BinaryBuildVersion: BuildIdentity, ImageBuildVersion: BuildIdentity}}})
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
		Descriptor: descriptorBytes, Checksum: checksum, ImageEvidence: append(evidence, '\n'), AcceptanceImageIndex: acceptanceIndex,
		RegistryImageDigest: digest('a'), AcceptanceImageDigest: "sha256:" + hex.EncodeToString(acceptanceSum[:]), RegistryChartDigest: digest('d'), ReconnectGraceSeconds: 5,
		ChartVersion: "0.0.0", ChartAppVersion: BuildIdentity,
		RuntimeBuildVersion: BuildIdentity, ReleaseArchitecture: "amd64", ExecutionArchitecture: "amd64", SelectedManifestDigest: digest('b'),
	}
}

func refreshArtifactChecksum(fixture *ArtifactFixture) {
	sum := sha256.Sum256(fixture.Descriptor)
	fixture.Checksum = []byte(hex.EncodeToString(sum[:]) + "  kubikles-accelerator-release-v0.0.0.json\n")
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
		{"descriptor build version", func(f *ArtifactFixture) {
			f.Descriptor = []byte(strings.Replace(string(f.Descriptor), `"buildVersion":"v0.0.0"`, `"buildVersion":"v0.0.1"`, 1))
		}},
		{"descriptor source", func(f *ArtifactFixture) {
			f.Descriptor = []byte(strings.Replace(string(f.Descriptor), strings.Repeat("c", 40), strings.Repeat("e", 40), 1))
		}},
		{"checksum", func(f *ArtifactFixture) { f.Checksum[0] = '0' }},
		{"missing platform", func(f *ArtifactFixture) {
			f.ImageEvidence = []byte(strings.Replace(string(f.ImageEvidence), `[{"os":"linux","architecture":"amd64","manifestDigest":"sha256:`+strings.Repeat("b", 64)+`"}]`, `[]`, 1))
		}},
		{"extra platform", func(f *ArtifactFixture) {
			f.ImageEvidence = []byte(strings.Replace(string(f.ImageEvidence), `]`, `,{"os":"linux","architecture":"s390x","manifestDigest":"sha256:`+strings.Repeat("e", 64)+`"}]`, 1))
		}},
		{"platform digest", func(f *ArtifactFixture) { f.SelectedManifestDigest = "sha256:" + strings.Repeat("f", 64) }},
		{"chart digest", func(f *ArtifactFixture) { f.RegistryChartDigest = "sha256:" + strings.Repeat("e", 64) }},
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
		{"tag only image", func(f *ArtifactFixture) {
			f.Descriptor = []byte(strings.Replace(string(f.Descriptor), `ghcr.io/skrobylabs/kubikles-accelerator@sha256:`, `ghcr.io/skrobylabs/kubikles-accelerator:v0.0.0#sha256:`, 1))
		}},
		{"duplicate descriptor key", func(f *ArtifactFixture) {
			f.Descriptor = []byte(strings.Replace(string(f.Descriptor), `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1))
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			candidate := exactArtifactFixture(t)
			test.mutate(&candidate)
			if test.name != "checksum" {
				refreshArtifactChecksum(&candidate)
			}
			if ValidateArtifactFixture(candidate) == nil {
				t.Fatal("artifact mutation was accepted")
			}
		})
	}
}
