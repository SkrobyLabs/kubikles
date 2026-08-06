package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func git(t *testing.T, repository string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repository}, args...)...)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func TestVerifyGitSourceAuthority(t *testing.T) {
	repository := t.TempDir()
	git(t, repository, "init", "-q")
	git(t, repository, "config", "user.name", "Release Test")
	git(t, repository, "config", "user.email", "release@example.invalid")
	if err := os.WriteFile(filepath.Join(repository, "tracked"), []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repository, "add", "tracked")
	git(t, repository, "commit", "-qm", "one")
	git(t, repository, "tag", "v1.4.2")
	first := git(t, repository, "rev-parse", "HEAD")
	if got, err := VerifyGitSource(repository, "v1.4.2", ""); err != nil || got != first {
		t.Fatalf("verify clean release: %q %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(repository, "untracked"), []byte("dirty"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyGitSource(repository, "v1.4.2", first); err == nil {
		t.Fatal("accepted dirty checkout")
	}
	if err := os.Remove(filepath.Join(repository, "untracked")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "tracked"), []byte("two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repository, "add", "tracked")
	git(t, repository, "commit", "-qm", "two")
	second := git(t, repository, "rev-parse", "HEAD")
	if _, err := VerifyGitSource(repository, "v1.4.2", first); err == nil {
		t.Fatal("accepted HEAD/tag mismatch")
	}
	git(t, repository, "tag", "-f", "v1.4.2", second)
	if _, err := VerifyGitSource(repository, "v1.4.2", first); err == nil {
		t.Fatal("accepted moved stable tag")
	}
	if got, err := VerifyGitSource(repository, "v1.4.2", second); err != nil || got != second {
		t.Fatalf("verify moved tag against new authority: %q %v", got, err)
	}
	git(t, repository, "tag", "v1.5.0-alpha.1", second)
	if got, err := VerifyGitSource(repository, "v1.5.0-alpha.1", second); err != nil || got != second {
		t.Fatalf("verify prerelease source: %q %v", got, err)
	}
	// A shallow marker is the exact local authority checked by rev-parse.
	gitDir := git(t, repository, "rev-parse", "--git-dir")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repository, gitDir)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "shallow"), []byte(second+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyGitSource(repository, "v1.4.2", second); err == nil {
		t.Fatal("accepted shallow checkout")
	}
}

const (
	testCommit = "0123456789abcdef0123456789abcdef01234567"
	digestA    = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestB    = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	digestC    = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestNormalizeStableReleaseTag(t *testing.T) {
	for _, tag := range []string{"v0.0.0", "v1.4.2", "v123456789.987654321.42"} {
		got, err := NormalizeStableReleaseTag(tag)
		if err != nil || got.GitTag != tag || got.BuildVersion != tag || got.ChartVersion != strings.TrimPrefix(tag, "v") {
			t.Fatalf("normalize %q: %#v %v", tag, got, err)
		}
	}
	for _, tag := range []string{"", "1.2.3", "V1.2.3", "v1.2", "v1.2.3.4", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2.3-rc.1", "v1.2.3+build", " v1.2.3", "v1.2.3\n", "latest", "dev", "main", testCommit, "v1.2.*", "v1.2.3;echo"} {
		if _, err := NormalizeStableReleaseTag(tag); err == nil {
			t.Fatalf("accepted %q", tag)
		}
	}
}

func TestNormalizeReleaseTagClassifiesCanonicalPrereleases(t *testing.T) {
	for _, tag := range []string{"v1.4.0-alpha", "v1.4.0-alpha.1", "v2.0.0-beta.12", "v2.0.0-rc.1"} {
		got, err := NormalizeReleaseTag(tag)
		if err != nil || !got.Prerelease || got.BuildVersion != tag || got.ChartVersion != strings.TrimPrefix(tag, "v") {
			t.Fatalf("normalize prerelease %q: %#v %v", tag, got, err)
		}
	}
	stable, err := NormalizeReleaseTag("v1.4.0")
	if err != nil || stable.Prerelease {
		t.Fatalf("stable classification: %#v %v", stable, err)
	}
	for _, tag := range []string{"v1.4.0-alpha.01", "v1.4.0+build", "v1.4.0-", "v1.4.0-alpha..1", "v1.4.0a"} {
		if _, err := NormalizeReleaseTag(tag); err == nil {
			t.Fatalf("accepted non-canonical release %q", tag)
		}
	}
}

func testEvidence() Evidence {
	return Evidence{BuildVersion: "v1.4.2", Commit: testCommit, GitTag: "v1.4.2", ImageDigest: digestC, Platforms: []Platform{{OS: "linux", Architecture: "amd64", ManifestDigest: digestA}}, ChartDigest: digestB, ChartVersion: "1.4.2", ChartAppVersion: "v1.4.2"}
}

func TestPrereleaseDescriptorPreservesExactIdentity(t *testing.T) {
	evidence := testEvidence()
	evidence.BuildVersion = "v1.5.0-alpha.1"
	evidence.GitTag = evidence.BuildVersion
	evidence.ChartVersion = "1.5.0-alpha.1"
	evidence.ChartAppVersion = evidence.BuildVersion
	descriptor, err := NewDescriptor(evidence)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := CanonicalDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStrictDescriptor(encoded)
	if err != nil || decoded.BuildVersion != evidence.BuildVersion || decoded.Chart.Version != evidence.ChartVersion || decoded.Source.GitTag != evidence.GitTag {
		t.Fatalf("prerelease descriptor identity = %#v, %v", decoded, err)
	}
}

func TestDescriptorV1CanonicalBytes(t *testing.T) {
	d, err := NewDescriptor(testEvidence())
	if err != nil {
		t.Fatal(err)
	}
	got, err := CanonicalDescriptor(d)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "schemaVersion": 1,
  "$schema": "https://raw.githubusercontent.com/SkrobyLabs/kubikles/0123456789abcdef0123456789abcdef01234567/release/accelerator-release.schema.json",
  "buildVersion": "v1.4.2",
  "source": {
    "repository": "https://github.com/SkrobyLabs/kubikles",
    "commit": "0123456789abcdef0123456789abcdef01234567",
    "gitTag": "v1.4.2"
  },
  "compatibility": {
    "mode": "exact-build-version",
    "desktopBuildVersion": "v1.4.2",
    "acceleratorBuildVersion": "v1.4.2"
  },
  "image": {
    "repository": "ghcr.io/skrobylabs/kubikles-accelerator",
    "digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
    "reference": "ghcr.io/skrobylabs/kubikles-accelerator@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
    "platforms": [
      {
        "os": "linux",
        "architecture": "amd64",
        "manifestDigest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
      }
    ]
  },
  "chart": {
    "repository": "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator",
    "digest": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "reference": "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "version": "1.4.2",
    "appVersion": "v1.4.2"
  }
}
`
	if string(got) != want {
		t.Fatalf("canonical descriptor mismatch\n%s", got)
	}
	if _, err := DecodeStrictDescriptor(got); err != nil {
		t.Fatalf("canonical descriptor rejected: %v", err)
	}
	if !bytes.HasSuffix(got, []byte("\n")) || bytes.HasSuffix(got, []byte("\n\n")) {
		t.Fatal("descriptor newline is not canonical")
	}
	checksum := DescriptorChecksum("kubikles-accelerator-release-v1.4.2.json", got)
	if !strings.HasSuffix(string(checksum), "  kubikles-accelerator-release-v1.4.2.json\n") {
		t.Fatal("checksum format mismatch")
	}
	unknown := append([]byte(nil), got[:len(got)-2]...)
	unknown = append(unknown, []byte(",\"channel\":\"stable\"}\n")...)
	if _, err := DecodeStrictDescriptor(unknown); err == nil {
		t.Fatal("unknown descriptor field accepted")
	}
	duplicate := bytes.Replace(got, []byte(`"schemaVersion": 1,`), []byte(`"schemaVersion": 1, "schemaVersion": 1,`), 1)
	if _, err := DecodeStrictDescriptor(duplicate); err == nil {
		t.Fatal("duplicate descriptor key accepted")
	}
	if _, err := DecodeStrictDescriptor(bytes.Replace(got, []byte("  \"buildVersion\""), []byte(" \"buildVersion\""), 1)); err == nil {
		t.Fatal("non-canonical descriptor bytes accepted")
	}

	compiler := jsonschema.NewCompiler()
	schemaPath := filepath.Join("..", "..", "release", "accelerator-release.schema.json")
	var schemaDocument any
	if err := json.NewDecoder(mustOpen(t, schemaPath)).Decode(&schemaDocument); err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource("schema.json", schemaDocument); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if json.Unmarshal(got, &doc) != nil {
		t.Fatal("decode descriptor")
	}
	if err := schema.Validate(doc); err != nil {
		t.Fatal(err)
	}
}

func mustOpen(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestDescriptorRejectsCrossFieldDrift(t *testing.T) {
	mutations := []func(*Evidence){
		func(e *Evidence) { e.BuildVersion = "v1.4.3" }, func(e *Evidence) { e.Commit = "ABC" }, func(e *Evidence) { e.GitTag = "v1.4.1" }, func(e *Evidence) { e.ImageDigest = "sha256:BAD" }, func(e *Evidence) { e.ChartDigest = "" }, func(e *Evidence) { e.ChartVersion = "v1.4.2" }, func(e *Evidence) { e.ChartAppVersion = "1.4.2" }, func(e *Evidence) { e.Platforms = nil }, func(e *Evidence) {
			e.Platforms = append(e.Platforms, Platform{OS: "linux", Architecture: "s390x", ManifestDigest: digestC})
		}, func(e *Evidence) { e.Platforms[0].Architecture = "arm64" }, func(e *Evidence) { e.Platforms[0].ManifestDigest = "sha256:BAD" },
	}
	for i, mutate := range mutations {
		e := testEvidence()
		mutate(&e)
		if _, err := NewDescriptor(e); err == nil {
			t.Fatalf("mutation %d accepted", i)
		}
	}
}

func TestPackageAcceleratorChartDeterministically(t *testing.T) {
	dir := filepath.Join("..", "..", "deploy", "charts", "kubikles-accelerator")
	a := filepath.Join(t.TempDir(), "a.tgz")
	b := filepath.Join(t.TempDir(), "b.tgz")
	for _, out := range []string{a, b} {
		if err := PackageChart(dir, out, "1.4.2", "v1.4.2", 1700000000); err != nil {
			t.Fatal(err)
		}
		if err := InspectChart(out, dir, "1.4.2", "v1.4.2", 1700000000); err != nil {
			t.Fatal(err)
		}
	}
	ab, _ := os.ReadFile(a)
	bb, _ := os.ReadFile(b)
	if !bytes.Equal(ab, bb) {
		t.Fatal("chart package is not deterministic")
	}
	layoutA, layoutB := t.TempDir(), t.TempDir()
	digestOne, err := PackageChartOCI(a, dir, layoutA, "1.4.2", "v1.4.2", 1700000000)
	if err != nil {
		t.Fatal(err)
	}
	digestTwo, err := PackageChartOCI(b, dir, layoutB, "1.4.2", "v1.4.2", 1700000000)
	if err != nil || digestOne != digestTwo {
		t.Fatalf("chart OCI manifest is not deterministic: %q %q %v", digestOne, digestTwo, err)
	}
	for _, relative := range []string{"oci-layout", "index.json", filepath.Join("blobs", "sha256", strings.TrimPrefix(digestOne, "sha256:"))} {
		one, _ := os.ReadFile(filepath.Join(layoutA, relative))
		two, _ := os.ReadFile(filepath.Join(layoutB, relative))
		if !bytes.Equal(one, two) {
			t.Fatalf("chart OCI layout differs at %s", relative)
		}
	}
	gz, err := gzip.NewReader(bytes.NewReader(ab))
	if err != nil {
		t.Fatal(err)
	}
	if !gz.ModTime.Equal(time.Unix(1700000000, 0).UTC()) || gz.Name != "" || gz.OS != 255 {
		t.Fatal("gzip header is not normalized")
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		if strings.Contains(h.Name, "/tests/") {
			t.Fatal("Go tests leaked into chart package")
		}
	}
}

func copyChartSource(t *testing.T, source string) string {
	t.Helper()
	destination := t.TempDir()
	for _, relative := range chartSourceFiles {
		content, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(destination, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return destination
}

func TestInspectChartRejectsSourceAndHeaderCorruption(t *testing.T) {
	source := filepath.Join("..", "..", "deploy", "charts", "kubikles-accelerator")
	for _, relative := range []string{"templates/job.yaml", "values.schema.json"} {
		mutated := copyChartSource(t, source)
		path := filepath.Join(mutated, filepath.FromSlash(relative))
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(content, []byte("\n# release-corruption\n")...), 0600); err != nil {
			t.Fatal(err)
		}
		archive := filepath.Join(t.TempDir(), "chart.tgz")
		if err := PackageChart(mutated, archive, "1.4.2", "v1.4.2", 1700000000); err != nil {
			t.Fatal(err)
		}
		if err := InspectChart(archive, source, "1.4.2", "v1.4.2", 1700000000); err == nil {
			t.Fatalf("accepted packaged %s corruption", relative)
		}
	}
	archive := filepath.Join(t.TempDir(), "chart.tgz")
	if err := PackageChart(source, archive, "1.4.2", "v1.4.2", 1700000000); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	content[9] = 3 // gzip OS header must remain the canonical unknown value 255.
	if err := os.WriteFile(archive, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := InspectChart(archive, source, "1.4.2", "v1.4.2", 1700000000); err == nil {
		t.Fatal("accepted non-canonical chart archive header")
	}
}

func putBlob(t *testing.T, layout string, content []byte, mediaType string) ociDescriptor {
	t.Helper()
	sum := sha256.Sum256(content)
	d := "sha256:" + hex.EncodeToString(sum[:])
	path := filepath.Join(layout, "blobs", "sha256", strings.TrimPrefix(d, "sha256:"))
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	return ociDescriptor{MediaType: mediaType, Digest: d, Size: int64(len(content))}
}

func putJSONBlob(t *testing.T, layout string, value any, mediaType string) ociDescriptor {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return putBlob(t, layout, b, mediaType)
}

func releaseBinary(t *testing.T, arch, version, commit string) []byte {
	t.Helper()
	dir := t.TempDir()
	source := `package main
import "fmt"
var identity = "unset"
var assets = []string{"frontend/dist/index.html"}
func main(){ fmt.Print(identity, assets) }
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.invalid/releasefixture\n\ngo 1.24.2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "accelerator")
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-tags", "headless,accelerator", "-ldflags", "-s -w -buildid= -X main.identity=kubikles-accelerator-build-identity:"+version+"|"+commit+"|false", "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s release fixture: %v (%s)", arch, err, output)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func releaseLayer(t *testing.T, arch, version, commit string) []byte {
	t.Helper()
	binary := releaseBinary(t, arch, version, commit)
	var content bytes.Buffer
	gz := gzip.NewWriter(&content)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "kubikles-accelerator", Typeflag: tar.TypeReg, Mode: 0555, Uid: 65532, Gid: 65532, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return content.Bytes()
}

func layerDiffID(t *testing.T, layer []byte) string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(layer))
	if err != nil {
		t.Fatal(err)
	}
	uncompressed, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(uncompressed)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func makeOCI(t *testing.T, version, commit string, arches []string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "blobs", "sha256"), 0700); err != nil {
		t.Fatal(err)
	}
	var manifests []ociDescriptor
	for _, arch := range arches {
		cfg := ociConfig{Architecture: arch, OS: "linux"}
		cfg.Config.User = "65532:65532"
		cfg.Config.Entrypoint = []string{"/kubikles-accelerator"}
		cfg.Config.Labels = map[string]string{"org.opencontainers.image.title": "kubikles-accelerator", "org.opencontainers.image.version": version, "org.opencontainers.image.revision": commit}
		cfg.RootFS.Type = "layers"
		layerContent := releaseLayer(t, arch, version, commit)
		cfg.RootFS.DiffIDs = []string{layerDiffID(t, layerContent)}
		cd := putJSONBlob(t, root, cfg, ociConfigMediaType)
		layer := putBlob(t, root, layerContent, ociLayerMediaType)
		m := ociManifest{SchemaVersion: 2, MediaType: ociManifestMediaType, Config: cd, Layers: []ociDescriptor{layer}}
		md := putJSONBlob(t, root, m, ociManifestMediaType)
		md.Platform = &ociPlatform{OS: "linux", Architecture: arch}
		manifests = append(manifests, md)
	}
	index := ociIndex{SchemaVersion: 2, Manifests: manifests}
	id := putJSONBlob(t, root, index, ociIndexMediaType)
	rootIndex := ociIndex{SchemaVersion: 2, Manifests: []ociDescriptor{id}}
	b, _ := json.Marshal(rootIndex)
	os.WriteFile(filepath.Join(root, "index.json"), b, 0600)
	os.WriteFile(filepath.Join(root, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0600)
	return root
}

func rewriteOCIConfig(t *testing.T, layout, architecture string, mutate func(*ociConfig)) {
	t.Helper()
	rootBytes, err := os.ReadFile(filepath.Join(layout, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var root ociIndex
	if json.Unmarshal(rootBytes, &root) != nil {
		t.Fatal("decode root index")
	}
	indexBytes, err := readBlob(layout, root.Manifests[0].Digest)
	if err != nil {
		t.Fatal(err)
	}
	var index ociIndex
	if json.Unmarshal(indexBytes, &index) != nil {
		t.Fatal("decode image index")
	}
	found := false
	for i, descriptor := range index.Manifests {
		if descriptor.Platform == nil || descriptor.Platform.Architecture != architecture {
			continue
		}
		manifestBytes, err := readBlob(layout, descriptor.Digest)
		if err != nil {
			t.Fatal(err)
		}
		var manifest ociManifest
		if json.Unmarshal(manifestBytes, &manifest) != nil {
			t.Fatal("decode manifest")
		}
		configBytes, err := readBlob(layout, manifest.Config.Digest)
		if err != nil {
			t.Fatal(err)
		}
		var config ociConfig
		if json.Unmarshal(configBytes, &config) != nil {
			t.Fatal("decode config")
		}
		mutate(&config)
		manifest.Config = putJSONBlob(t, layout, config, ociConfigMediaType)
		replacement := putJSONBlob(t, layout, manifest, ociManifestMediaType)
		replacement.Platform = descriptor.Platform
		index.Manifests[i] = replacement
		found = true
		break
	}
	if !found {
		t.Fatal("platform config not found")
	}
	root.Manifests[0] = putJSONBlob(t, layout, index, ociIndexMediaType)
	rootBytes, err = json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout, "index.json"), rootBytes, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInspectAcceleratorOCIIndex(t *testing.T) {
	layout := makeOCI(t, "v1.4.2", testCommit, []string{"amd64"})
	digest, platforms, err := InspectOCIIndex(layout, "v1.4.2", testCommit)
	if err != nil {
		t.Fatal(err)
	}
	if !digestRE.MatchString(digest) || len(platforms) != 1 || platforms[0].Architecture != "amd64" {
		t.Fatal("unexpected OCI evidence")
	}
	rootBytes, err := os.ReadFile(filepath.Join(layout, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var root ociIndex
	if json.Unmarshal(rootBytes, &root) != nil {
		t.Fatal("decode fixture root")
	}
	indexBytes, err := readBlob(layout, root.Manifests[0].Digest)
	if err != nil {
		t.Fatal(err)
	}
	var copiedIndex ociIndex
	if json.Unmarshal(indexBytes, &copiedIndex) != nil {
		t.Fatal("decode fixture image index")
	}
	root.Manifests = append(root.Manifests, copiedIndex.Manifests...)
	expandedRoot, _ := json.Marshal(root)
	if err := os.WriteFile(filepath.Join(layout, "index.json"), expandedRoot, 0600); err != nil {
		t.Fatal(err)
	}
	pulledDigest, pulledPlatforms, err := InspectOCIIndex(layout, "v1.4.2", testCommit)
	if err != nil || pulledDigest != digest || len(pulledPlatforms) != 1 {
		t.Fatalf("inspect expanded ORAS layout: %q %#v %v", pulledDigest, pulledPlatforms, err)
	}
	root.Manifests = append(root.Manifests, ociDescriptor{MediaType: ociManifestMediaType, Digest: digestA, Size: 1})
	expandedRoot, _ = json.Marshal(root)
	if err := os.WriteFile(filepath.Join(layout, "index.json"), expandedRoot, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InspectOCIIndex(layout, "v1.4.2", testCommit); err == nil {
		t.Fatal("accepted unrelated root manifest beside ORAS digest copy")
	}
	for _, arches := range [][]string{{"arm64"}, {"amd64", "arm64"}, {"amd64", "amd64"}} {
		if _, _, err := InspectOCIIndex(makeOCI(t, "v1.4.2", testCommit, arches), "v1.4.2", testCommit); err == nil {
			t.Fatalf("accepted platforms %v", arches)
		}
	}
	if _, _, err := InspectOCIIndex(makeOCI(t, "v1.4.1", testCommit, []string{"amd64"}), "v1.4.2", testCommit); err == nil {
		t.Fatal("accepted config version drift")
	}
}

func TestInspectAcceleratorOCIRejectsRuntimeContractDrift(t *testing.T) {
	mutations := []func(*ociConfig){
		func(config *ociConfig) { config.Config.Entrypoint = []string{"/bin/sh"} },
		func(config *ociConfig) { config.Config.Cmd = []string{"serve"} },
		func(config *ociConfig) { config.Config.Env = []string{"HOME=/tmp"} },
		func(config *ociConfig) { config.Config.ExposedPorts = map[string]any{"8080/tcp": map[string]any{}} },
		func(config *ociConfig) { config.Config.Labels["org.opencontainers.image.title"] = "other" },
	}
	for i, mutate := range mutations {
		layout := makeOCI(t, "v1.4.2", testCommit, []string{"amd64"})
		rewriteOCIConfig(t, layout, "amd64", mutate)
		if _, _, err := InspectOCIIndex(layout, "v1.4.2", testCommit); err == nil {
			t.Fatalf("accepted runtime config mutation %d", i)
		}
	}
	if err := inspectAcceleratorLayer([]byte(`{"arch":"amd64"}`), "amd64", "v1.4.2", testCommit); err == nil {
		t.Fatal("accepted fake JSON layer evidence")
	}
	if err := inspectAcceleratorBinary(releaseBinary(t, "amd64", "v1.4.1", testCommit), "amd64", "v1.4.2", testCommit); err == nil {
		t.Fatal("accepted binary with mismatched embedded BuildVersion identity")
	}
}

func TestImmutableComparisonDoesNotLeakBytes(t *testing.T) {
	secret := []byte("hostile-token-do-not-print")
	err := ensureEqual("chart", []byte("expected"), secret)
	if err == nil || strings.Contains(err.Error(), string(secret)) {
		t.Fatal("collision error leaked content")
	}
}

func TestVerifyReleaseAssetNamesExact(t *testing.T) {
	names := []string{
		"Kubikles-windows-amd64.zip", "Kubikles-windows-arm64.zip",
		"Kubikles-macos-arm64.zip", "Kubikles-macos-amd64.zip",
		"Kubikles-linux-amd64.zip",
		"kubikles-accelerator-release-v1.4.2.json",
		"kubikles-accelerator-release-v1.4.2.json.sha256",
		"kubikles-accelerator-image-linux-amd64-v1.4.2.spdx.json",
		"kubikles-accelerator-chart-v1.4.2.spdx.json",
		"kubikles-accelerator-attestations-v1.4.2.jsonl",
	}
	if err := VerifyReleaseAssetNames("v1.4.2", names); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func([]string) []string{
		func(values []string) []string { return values[:len(values)-1] },
		func(values []string) []string { return append(values, "extra.zip") },
		func(values []string) []string { values[0] = "Kubikles-linux-arm64.zip"; return values },
	} {
		copyNames := append([]string(nil), names...)
		if err := VerifyReleaseAssetNames("v1.4.2", mutation(copyNames)); err == nil {
			t.Fatal("accepted non-exact finalized asset set")
		}
	}
}

func TestInspectChartManifestAndRegistryEvidence(t *testing.T) {
	source := filepath.Join("..", "..", "deploy", "charts", "kubikles-accelerator")
	packagePath := filepath.Join(t.TempDir(), "kubikles-accelerator-1.4.2.tgz")
	if err := PackageChart(source, packagePath, "1.4.2", "v1.4.2", 1700000000); err != nil {
		t.Fatal(err)
	}
	layout := t.TempDir()
	digest, err := PackageChartOCI(packagePath, source, layout, "1.4.2", "v1.4.2", 1700000000)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(layout, "blobs", "sha256", strings.TrimPrefix(digest, "sha256:"))
	configDigest, err := ChartConfigDigest(manifestPath, digest)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(layout, "blobs", "sha256", strings.TrimPrefix(configDigest, "sha256:"))
	if err := InspectChartManifest(manifestPath, configPath, packagePath, source, "1.4.2", "v1.4.2", 1700000000, digest); err != nil {
		t.Fatal(err)
	}
	canonicalManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest ociManifest
	if json.Unmarshal(canonicalManifest, &manifest) != nil {
		t.Fatal("decode canonical chart manifest")
	}
	manifest.Annotations["example.invalid/drift"] = "changed"
	changed, _ := json.Marshal(manifest)
	changedPath := filepath.Join(t.TempDir(), "manifest.json")
	_ = os.WriteFile(changedPath, changed, 0600)
	changedSum := sha256.Sum256(changed)
	if err := InspectChartManifest(changedPath, configPath, packagePath, source, "1.4.2", "v1.4.2", 1700000000, "sha256:"+hex.EncodeToString(changedSum[:])); err == nil {
		t.Fatal("accepted chart manifest annotation drift")
	}
	configContent, _ := os.ReadFile(configPath)
	_ = os.WriteFile(configPath, append(configContent, '\n'), 0600)
	if err := InspectChartManifest(manifestPath, configPath, packagePath, source, "1.4.2", "v1.4.2", 1700000000, digest); err == nil {
		t.Fatal("accepted chart config drift")
	}

	d, err := NewDescriptor(testEvidence())
	if err != nil {
		t.Fatal(err)
	}
	descriptorBytes, _ := CanonicalDescriptor(d)
	descriptorPath := filepath.Join(t.TempDir(), "descriptor.json")
	imagePath := filepath.Join(t.TempDir(), "image.json")
	if err := os.WriteFile(descriptorPath, descriptorBytes, 0600); err != nil {
		t.Fatal(err)
	}
	imageBytes, _ := json.MarshalIndent(map[string]any{"imageDigest": digestC, "platforms": d.Image.Platforms}, "", "  ")
	imageBytes = append(imageBytes, '\n')
	if err := os.WriteFile(imagePath, imageBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRegistryEvidence(descriptorPath, imagePath, digestB, "v1.4.2", testCommit); err != nil {
		t.Fatal(err)
	}
	d.Image.Platforms[0].ManifestDigest = digestC
	descriptorBytes, _ = json.MarshalIndent(d, "", "  ")
	descriptorBytes = append(descriptorBytes, '\n')
	if err := os.WriteFile(descriptorPath, descriptorBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRegistryEvidence(descriptorPath, imagePath, digestB, "v1.4.2", testCommit); err == nil {
		t.Fatal("accepted descriptor platform evidence drift")
	}
}
