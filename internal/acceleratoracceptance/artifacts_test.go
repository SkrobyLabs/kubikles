package acceleratoracceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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
		Image:         artifactImage{Repository: "ghcr.io/skrobylabs/kubikles-accelerator", Digest: digest('a'), Reference: "ghcr.io/skrobylabs/kubikles-accelerator@" + digest('a'), Platforms: []artifactPlatform{{OS: "linux", Architecture: "amd64", ManifestDigest: digest('b')}, {OS: "linux", Architecture: "arm64", ManifestDigest: digest('c')}}},
		Chart:         artifactChart{Repository: "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator", Digest: digest('d'), Reference: "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator@" + digest('d'), Version: "0.0.0", AppVersion: BuildIdentity},
	}
	descriptorBytes, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal("marshal descriptor fixture")
	}
	descriptorBytes = append(descriptorBytes, '\n')
	sum := sha256.Sum256(descriptorBytes)
	checksum := []byte(hex.EncodeToString(sum[:]) + "  kubikles-accelerator-release-v0.0.0.json\n")
	evidence, err := json.Marshal(artifactImageEvidence{ImageDigest: digest('a'), Platforms: descriptor.Image.Platforms, Inspections: []artifactInspection{{Architecture: "amd64", BinaryBuildVersion: BuildIdentity, ImageBuildVersion: BuildIdentity}, {Architecture: "arm64", BinaryBuildVersion: BuildIdentity, ImageBuildVersion: BuildIdentity}}})
	if err != nil {
		t.Fatal("marshal evidence fixture")
	}
	return ArtifactFixture{
		Descriptor: descriptorBytes, Checksum: checksum, ImageEvidence: append(evidence, '\n'),
		RegistryImageDigest: digest('a'), RegistryChartDigest: digest('d'),
		ChartVersion: "0.0.0", ChartAppVersion: BuildIdentity,
		RuntimeBuildVersion: BuildIdentity, HostArchitecture: "arm64", SelectedManifestDigest: digest('c'),
	}
}

func TestAcceptanceOfflineBuildPlumbing(t *testing.T) {
	for _, file := range []string{"Dockerfile.accelerator", "scripts/test-accelerator-image.sh", "scripts/build-accelerator-e2e-artifacts.sh", "scripts/publish-accelerator-release.sh"} {
		source, err := os.ReadFile(filepath.Join("..", "..", file))
		if err != nil {
			t.Fatalf("read offline source %s", file)
		}
		text := string(source)
		if strings.Contains(file, "test-accelerator-image") {
			for _, exact := range []string{"ACCELERATOR_E2E_OFFLINE", "docker image inspect \"$base\"", "--network=none", "--pull=false"} {
				if !strings.Contains(text, exact) {
					t.Fatalf("offline image test is missing %s", exact)
				}
			}
		}
		if strings.Contains(file, "build-accelerator-e2e-artifacts") {
			for _, exact := range []string{"docker image inspect", "--network=none", "--pull=false", "publish_registry", "ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT"} {
				if !strings.Contains(text, exact) {
					t.Fatalf("offline artifact build is missing %s", exact)
				}
			}
		}
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
			f.ImageEvidence = []byte(strings.Replace(string(f.ImageEvidence), `,{"os":"linux","architecture":"arm64","manifestDigest":"sha256:`+strings.Repeat("c", 64)+`"}`, "", 1))
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
