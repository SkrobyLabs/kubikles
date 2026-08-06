package acceleratorrelease

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateAndProjectDescriptorV1(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	release, class := validateAndProject(testBuildVersion, checksumBytes, descriptorBytes)
	hash := sha256.Sum256(descriptorBytes)
	want := VerifiedRelease{
		BuildVersion: testBuildVersion, SourceCommit: "0123456789abcdef0123456789abcdef01234567",
		DescriptorSHA256: hex.EncodeToString(hash[:]),
		ImageReference:   "ghcr.io/skrobylabs/kubikles-accelerator@sha256:" + strings.Repeat("c", 64),
		ChartReference:   "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator@sha256:" + strings.Repeat("b", 64),
	}
	if class != descriptorValid || release != want {
		t.Fatalf("projection = %#v, %v", release, class)
	}
}

func TestDescriptorChecksumContract(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	mutations := [][]byte{
		bytes.ToUpper(checksumBytes),
		bytes.Replace(checksumBytes, []byte("  "), []byte(" "), 1),
		bytes.Replace(checksumBytes, []byte("kubikles-accelerator-release-v1.4.2.json"), []byte("other.json"), 1),
		bytes.TrimSuffix(checksumBytes, []byte("\n")), append(append([]byte(nil), checksumBytes...), '\n'),
		bytes.Replace(checksumBytes, checksumBytes[:1], []byte("0"), 1),
	}
	for i, checksum := range mutations {
		if release, class := validateAndProject(testBuildVersion, checksum, descriptorBytes); class != descriptorIntegrity || release != (VerifiedRelease{}) {
			t.Fatalf("checksum mutation %d accepted: %#v, %v", i, release, class)
		}
	}
}

func TestDescriptorV1FailsClosed(t *testing.T) {
	descriptorBytes, _ := goldenPair(t)
	var base descriptor
	if err := json.Unmarshal(descriptorBytes, &base); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*descriptor){
		"schema version":    func(d *descriptor) { d.SchemaVersion = 2 },
		"schema URL":        func(d *descriptor) { d.Schema = "https://example.invalid/schema" },
		"root build":        func(d *descriptor) { d.BuildVersion = "v1.4.3" },
		"source repository": func(d *descriptor) { d.Source.Repository += "/fork" },
		"source commit": func(d *descriptor) {
			d.Source.Commit = strings.Repeat("A", 40)
			d.Schema = "https://raw.githubusercontent.com/SkrobyLabs/kubikles/" + d.Source.Commit + "/release/accelerator-release.schema.json"
		},
		"source tag":          func(d *descriptor) { d.Source.GitTag = "v1.4.3" },
		"compatibility mode":  func(d *descriptor) { d.Compatibility.Mode = "range" },
		"desktop version":     func(d *descriptor) { d.Compatibility.DesktopBuildVersion = "v1.4.3" },
		"accelerator version": func(d *descriptor) { d.Compatibility.AcceleratorBuildVersion = "v1.4.3" },
		"image repository":    func(d *descriptor) { d.Image.Repository += "-other" },
		"image digest":        func(d *descriptor) { d.Image.Digest = "sha256:ABC" },
		"image reference":     func(d *descriptor) { d.Image.Reference = d.Image.Repository + ":v1.4.2" },
		"platform missing":    func(d *descriptor) { d.Image.Platforms = nil },
		"platform extra": func(d *descriptor) {
			d.Image.Platforms = append(d.Image.Platforms, platform{OS: "linux", Architecture: "s390x", ManifestDigest: "sha256:" + strings.Repeat("d", 64)})
		},
		"platform OS":           func(d *descriptor) { d.Image.Platforms[0].OS = "windows" },
		"platform architecture": func(d *descriptor) { d.Image.Platforms[0].Architecture = "arm64" },
		"platform digest":       func(d *descriptor) { d.Image.Platforms[0].ManifestDigest = "sha256:ABC" },
		"chart repository":      func(d *descriptor) { d.Chart.Repository += "-other" },
		"chart digest":          func(d *descriptor) { d.Chart.Digest = "sha256:ABC" },
		"chart reference":       func(d *descriptor) { d.Chart.Reference = d.Chart.Repository + ":1.4.2" },
		"chart version":         func(d *descriptor) { d.Chart.Version = "1.4.3" },
		"chart app version":     func(d *descriptor) { d.Chart.AppVersion = "v1.4.3" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			wire := base
			wire.Image.Platforms = append([]platform(nil), base.Image.Platforms...)
			mutate(&wire)
			mutatedDescriptor, mutatedChecksum := encodeTestPair(t, testBuildVersion, wire)
			if release, class := validateAndProject(testBuildVersion, mutatedChecksum, mutatedDescriptor); class != descriptorIntegrity || release != (VerifiedRelease{}) {
				t.Fatalf("accepted: %#v, %v", release, class)
			}
		})
	}
	rawMutations := map[string][]byte{
		"unknown":      bytes.Replace(descriptorBytes, []byte("\n}"), []byte(",\n  \"channel\": \"stable\"\n}"), 1),
		"duplicate":    bytes.Replace(descriptorBytes, []byte(`"schemaVersion": 1,`), []byte(`"schemaVersion": 1, "schemaVersion": 1,`), 1),
		"trailing":     append(append([]byte(nil), descriptorBytes...), []byte("{}\n")...),
		"noncanonical": bytes.Replace(descriptorBytes, []byte("  \"buildVersion\""), []byte(" \"buildVersion\""), 1),
		"malformed":    descriptorBytes[:len(descriptorBytes)-2],
		"missing":      bytes.Replace(descriptorBytes, []byte("  \"schemaVersion\": 1,\n"), nil, 1),
	}
	for name, mutatedDescriptor := range rawMutations {
		t.Run(name, func(t *testing.T) {
			hash := sha256.Sum256(mutatedDescriptor)
			checksum := []byte(hex.EncodeToString(hash[:]) + "  kubikles-accelerator-release-v1.4.2.json\n")
			if release, class := validateAndProject(testBuildVersion, checksum, mutatedDescriptor); class != descriptorIntegrity || release != (VerifiedRelease{}) {
				t.Fatalf("accepted: %#v, %v", release, class)
			}
		})
	}
}

func TestDescriptorBuildVersionIsLiteral(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	for _, local := range []string{"v1.4.1", "v1.4.3", " v1.4.2", "V1.4.2", "v01.4.2"} {
		if release, class := validateAndProject(local, checksumBytes, descriptorBytes); class != descriptorIntegrity || release != (VerifiedRelease{}) {
			t.Fatalf("local %q accepted", local)
		}
	}
	var wire descriptor
	if err := json.Unmarshal(descriptorBytes, &wire); err != nil {
		t.Fatal(err)
	}
	wire.Chart.Version = "v1.4.2"
	mutatedDescriptor, mutatedChecksum := encodeTestPair(t, testBuildVersion, wire)
	if _, class := validateAndProject(testBuildVersion, mutatedChecksum, mutatedDescriptor); class != descriptorIntegrity {
		t.Fatal("chart version with v prefix accepted")
	}
}

func TestDescriptorCanonicalRoundTrip(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	var wire descriptor
	if err := json.Unmarshal(descriptorBytes, &wire); err != nil {
		t.Fatal(err)
	}
	canonical, rebuiltChecksum := encodeTestPair(t, testBuildVersion, wire)
	if !bytes.Equal(canonical, descriptorBytes) || !bytes.Equal(rebuiltChecksum, checksumBytes) {
		t.Fatal("golden bytes are not canonical")
	}
	if !bytes.HasSuffix(descriptorBytes, []byte("\n")) || bytes.HasSuffix(descriptorBytes, []byte("\n\n")) {
		t.Fatal("final LF contract drifted")
	}
}
