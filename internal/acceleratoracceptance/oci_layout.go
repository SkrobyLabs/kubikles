package acceleratoracceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	acceptanceOCIIndexMediaType          = "application/vnd.oci.image.index.v1+json"
	acceptanceOCIManifestMediaType       = "application/vnd.oci.image.manifest.v1+json"
	acceptanceOCIConfigMediaType         = "application/vnd.oci.image.config.v1+json"
	acceptanceOCILayerMediaType          = "application/vnd.oci.image.layer.v1.tar+gzip"
	acceptanceDockerManifestMediaType    = "application/vnd.docker.distribution.manifest.v2+json"
	acceptanceDockerConfigMediaType      = "application/vnd.docker.container.image.v1+json"
	acceptanceDockerLayerMediaType       = "application/vnd.docker.image.rootfs.diff.tar.gzip"
	acceptanceOCIReferenceAnnotationName = "org.opencontainers.image.ref.name"
)

type acceptanceOCIPlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

type acceptanceOCIDescriptor struct {
	MediaType   string                 `json:"mediaType"`
	Digest      string                 `json:"digest"`
	Size        int64                  `json:"size"`
	URLs        []string               `json:"urls,omitempty"`
	Annotations map[string]string      `json:"annotations,omitempty"`
	Data        []byte                 `json:"data,omitempty"`
	Platform    *acceptanceOCIPlatform `json:"platform,omitempty"`
}

type acceptanceOCIIndex struct {
	SchemaVersion int                       `json:"schemaVersion"`
	MediaType     string                    `json:"mediaType,omitempty"`
	Manifests     []acceptanceOCIDescriptor `json:"manifests"`
	Annotations   map[string]string         `json:"annotations,omitempty"`
}

type acceptanceOCIManifest struct {
	SchemaVersion int                       `json:"schemaVersion"`
	MediaType     string                    `json:"mediaType"`
	Config        acceptanceOCIDescriptor   `json:"config"`
	Layers        []acceptanceOCIDescriptor `json:"layers"`
	Annotations   map[string]string         `json:"annotations,omitempty"`
}

func CanonicalizeAcceptanceOCIImageLayout(root, tag string) error {
	if !filepath.IsAbs(root) || tag != BuildIdentity {
		return errors.New("OCI layout invalid")
	}
	indexBytes, err := os.ReadFile(filepath.Join(root, "index.json"))
	if err != nil || len(indexBytes) == 0 || len(indexBytes) > maximumAcceptanceJSONBytes {
		return errors.New("OCI layout invalid")
	}
	var layout acceptanceOCIIndex
	if json.Unmarshal(indexBytes, &layout) != nil || layout.SchemaVersion != 2 {
		return errors.New("OCI layout invalid")
	}
	rootIndex := -1
	for index, descriptor := range layout.Manifests {
		if descriptor.MediaType == acceptanceOCIIndexMediaType && descriptor.Annotations[acceptanceOCIReferenceAnnotationName] == tag {
			if rootIndex != -1 || descriptor.Platform != nil {
				return errors.New("OCI layout invalid")
			}
			rootIndex = index
		}
	}
	if rootIndex == -1 {
		return errors.New("OCI layout invalid")
	}
	rootDescriptor := layout.Manifests[rootIndex]
	rootBytes, err := readAcceptanceOCIBlob(root, rootDescriptor)
	if err != nil {
		return err
	}
	var imageIndex acceptanceOCIIndex
	if json.Unmarshal(rootBytes, &imageIndex) != nil || imageIndex.SchemaVersion != 2 || imageIndex.MediaType != acceptanceOCIIndexMediaType || len(imageIndex.Manifests) != 2 {
		return errors.New("OCI layout invalid")
	}
	seen := map[string]bool{}
	for index, child := range imageIndex.Manifests {
		if child.Platform == nil || child.Platform.OS != "linux" || child.MediaType != acceptanceDockerManifestMediaType || (child.Platform.Architecture != "amd64" && child.Platform.Architecture != "arm64") || seen[child.Platform.Architecture] {
			return errors.New("OCI layout invalid")
		}
		seen[child.Platform.Architecture] = true
		manifestBytes, readErr := readAcceptanceOCIBlob(root, child)
		if readErr != nil {
			return readErr
		}
		var manifest acceptanceOCIManifest
		if json.Unmarshal(manifestBytes, &manifest) != nil || manifest.SchemaVersion != 2 || manifest.MediaType != acceptanceDockerManifestMediaType || manifest.Config.MediaType != acceptanceDockerConfigMediaType || len(manifest.Layers) < 2 {
			return errors.New("OCI layout invalid")
		}
		if _, readErr = readAcceptanceOCIBlob(root, manifest.Config); readErr != nil {
			return readErr
		}
		manifest.MediaType = acceptanceOCIManifestMediaType
		manifest.Config.MediaType = acceptanceOCIConfigMediaType
		for layerIndex := range manifest.Layers {
			if manifest.Layers[layerIndex].MediaType != acceptanceDockerLayerMediaType {
				return errors.New("OCI layout invalid")
			}
			if _, readErr = readAcceptanceOCIBlob(root, manifest.Layers[layerIndex]); readErr != nil {
				return readErr
			}
			manifest.Layers[layerIndex].MediaType = acceptanceOCILayerMediaType
		}
		canonical, marshalErr := json.Marshal(manifest)
		if marshalErr != nil {
			return errors.New("OCI layout invalid")
		}
		canonicalDescriptor, writeErr := writeAcceptanceOCIBlob(root, canonical, acceptanceOCIManifestMediaType)
		if writeErr != nil {
			return writeErr
		}
		canonicalDescriptor.Platform = child.Platform
		imageIndex.Manifests[index] = canonicalDescriptor
	}
	if !seen["amd64"] || !seen["arm64"] {
		return errors.New("OCI layout invalid")
	}
	canonicalIndex, err := json.Marshal(imageIndex)
	if err != nil {
		return errors.New("OCI layout invalid")
	}
	canonicalRoot, err := writeAcceptanceOCIBlob(root, canonicalIndex, acceptanceOCIIndexMediaType)
	if err != nil {
		return err
	}
	canonicalRoot.Annotations = rootDescriptor.Annotations
	layout.Manifests = []acceptanceOCIDescriptor{canonicalRoot}
	canonicalLayout, err := json.Marshal(layout)
	if err != nil {
		return errors.New("OCI layout invalid")
	}
	canonicalLayout = append(canonicalLayout, '\n')
	return writePrivateAtomic(filepath.Join(root, "index.json"), canonicalLayout)
}

func readAcceptanceOCIBlob(root string, descriptor acceptanceOCIDescriptor) ([]byte, error) {
	if !artifactDigestPattern.MatchString(descriptor.Digest) || descriptor.Size <= 0 {
		return nil, errors.New("OCI layout invalid")
	}
	data, err := os.ReadFile(filepath.Join(root, "blobs", "sha256", descriptor.Digest[len("sha256:"):]))
	if err != nil || int64(len(data)) != descriptor.Size {
		return nil, errors.New("OCI layout invalid")
	}
	digest := sha256.Sum256(data)
	if "sha256:"+hex.EncodeToString(digest[:]) != descriptor.Digest {
		return nil, errors.New("OCI layout invalid")
	}
	return data, nil
}

func writeAcceptanceOCIBlob(root string, data []byte, mediaType string) (acceptanceOCIDescriptor, error) {
	digest := sha256.Sum256(data)
	digestHex := hex.EncodeToString(digest[:])
	path := filepath.Join(root, "blobs", "sha256", digestHex)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return acceptanceOCIDescriptor{}, errors.New("OCI layout invalid")
	}
	return acceptanceOCIDescriptor{MediaType: mediaType, Digest: "sha256:" + digestHex, Size: int64(len(data))}, nil
}
