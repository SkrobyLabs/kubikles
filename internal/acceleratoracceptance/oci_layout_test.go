package acceleratoracceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func dockerMediaLayout(t *testing.T, architectures []string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "blobs", "sha256"), 0o700); err != nil {
		t.Fatal("create OCI blobs")
	}
	putJSON := func(value any, mediaType string) acceptanceOCIDescriptor {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal("marshal OCI fixture")
		}
		descriptor, err := writeAcceptanceOCIBlob(root, encoded, mediaType)
		if err != nil {
			t.Fatal("write OCI fixture")
		}
		return descriptor
	}
	config, err := writeAcceptanceOCIBlob(root, []byte(`{"architecture":"fixture"}`), acceptanceDockerConfigMediaType)
	if err != nil {
		t.Fatal("write config fixture")
	}
	base, err := writeAcceptanceOCIBlob(root, []byte("base"), acceptanceDockerLayerMediaType)
	if err != nil {
		t.Fatal("write base fixture")
	}
	binary, err := writeAcceptanceOCIBlob(root, []byte("binary"), acceptanceDockerLayerMediaType)
	if err != nil {
		t.Fatal("write binary fixture")
	}
	children := make([]acceptanceOCIDescriptor, 0, len(architectures))
	for _, architecture := range architectures {
		manifest := acceptanceOCIManifest{SchemaVersion: 2, MediaType: acceptanceDockerManifestMediaType, Config: config, Layers: []acceptanceOCIDescriptor{base, binary}}
		descriptor := putJSON(manifest, acceptanceDockerManifestMediaType)
		descriptor.Platform = &acceptanceOCIPlatform{OS: "linux", Architecture: architecture}
		children = append(children, descriptor)
	}
	imageIndex := acceptanceOCIIndex{SchemaVersion: 2, MediaType: acceptanceOCIIndexMediaType, Manifests: children}
	rootDescriptor := putJSON(imageIndex, acceptanceOCIIndexMediaType)
	rootDescriptor.Annotations = map[string]string{acceptanceOCIReferenceAnnotationName: BuildIdentity}
	layout := acceptanceOCIIndex{SchemaVersion: 2, Manifests: append([]acceptanceOCIDescriptor{rootDescriptor}, children...)}
	encoded, err := json.Marshal(layout)
	if err != nil || os.WriteFile(filepath.Join(root, "index.json"), encoded, 0o600) != nil {
		t.Fatal("write layout index")
	}
	return root
}

func TestCanonicalizeAcceptanceOCIImageLayout(t *testing.T) {
	root := dockerMediaLayout(t, []string{"amd64", "arm64"})
	if err := CanonicalizeAcceptanceOCIImageLayout(root, BuildIdentity); err != nil {
		t.Fatal("canonicalize Docker media layout")
	}
	encoded, err := os.ReadFile(filepath.Join(root, "index.json"))
	var layout acceptanceOCIIndex
	if err != nil || json.Unmarshal(encoded, &layout) != nil || len(layout.Manifests) != 1 || layout.Manifests[0].MediaType != acceptanceOCIIndexMediaType {
		t.Fatal("canonical layout root differs")
	}
	indexBytes, err := readAcceptanceOCIBlob(root, layout.Manifests[0])
	var imageIndex acceptanceOCIIndex
	if err != nil || json.Unmarshal(indexBytes, &imageIndex) != nil || len(imageIndex.Manifests) != 2 {
		t.Fatal("canonical image index differs")
	}
	for _, child := range imageIndex.Manifests {
		if child.MediaType != acceptanceOCIManifestMediaType {
			t.Fatal("child manifest media type differs")
		}
		manifestBytes, err := readAcceptanceOCIBlob(root, child)
		var manifest acceptanceOCIManifest
		if err != nil || json.Unmarshal(manifestBytes, &manifest) != nil || manifest.MediaType != acceptanceOCIManifestMediaType || manifest.Config.MediaType != acceptanceOCIConfigMediaType {
			t.Fatal("canonical manifest differs")
		}
		for _, layer := range manifest.Layers {
			if layer.MediaType != acceptanceOCILayerMediaType {
				t.Fatal("canonical layer media type differs")
			}
		}
	}
}

func TestCanonicalizeAcceptanceOCIImageLayoutRejectsDrift(t *testing.T) {
	if CanonicalizeAcceptanceOCIImageLayout(dockerMediaLayout(t, []string{"amd64"}), BuildIdentity) == nil {
		t.Fatal("accepted missing platform")
	}
	if CanonicalizeAcceptanceOCIImageLayout(dockerMediaLayout(t, []string{"amd64", "amd64"}), BuildIdentity) == nil {
		t.Fatal("accepted duplicate platform")
	}
	root := dockerMediaLayout(t, []string{"amd64", "arm64"})
	encoded, err := os.ReadFile(filepath.Join(root, "index.json"))
	var layout acceptanceOCIIndex
	if err != nil || json.Unmarshal(encoded, &layout) != nil {
		t.Fatal("read mutation fixture")
	}
	layout.Manifests[0].Digest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	encoded, _ = json.Marshal(layout)
	if os.WriteFile(filepath.Join(root, "index.json"), encoded, 0o600) != nil || CanonicalizeAcceptanceOCIImageLayout(root, BuildIdentity) == nil {
		t.Fatal("accepted missing root blob")
	}
}
