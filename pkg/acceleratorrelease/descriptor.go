package acceleratorrelease

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

const (
	sourceRepository = "https://github.com/SkrobyLabs/kubikles"
	imageRepository  = "ghcr.io/skrobylabs/kubikles-accelerator"
	chartRepository  = "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator"
)

type platform struct {
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	ManifestDigest string `json:"manifestDigest"`
}

type source struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	GitTag     string `json:"gitTag"`
}

type compatibility struct {
	Mode                    string `json:"mode"`
	DesktopBuildVersion     string `json:"desktopBuildVersion"`
	AcceleratorBuildVersion string `json:"acceleratorBuildVersion"`
}

type image struct {
	Repository string     `json:"repository"`
	Digest     string     `json:"digest"`
	Reference  string     `json:"reference"`
	Platforms  []platform `json:"platforms"`
}

type chart struct {
	Repository string `json:"repository"`
	Digest     string `json:"digest"`
	Reference  string `json:"reference"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}

type descriptor struct {
	SchemaVersion int           `json:"schemaVersion"`
	Schema        string        `json:"$schema"`
	BuildVersion  string        `json:"buildVersion"`
	Source        source        `json:"source"`
	Compatibility compatibility `json:"compatibility"`
	Image         image         `json:"image"`
	Chart         chart         `json:"chart"`
}

var (
	digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	commitRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hex64RE  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type descriptorClass int

const (
	descriptorValid descriptorClass = iota
	descriptorIntegrity
)

func validateAndProject(buildVersion string, checksumBytes, descriptorBytes []byte) (VerifiedRelease, descriptorClass) {
	versionMatch := releaseVersion.FindStringSubmatch(buildVersion)
	if versionMatch == nil {
		return VerifiedRelease{}, descriptorIntegrity
	}
	descriptorName := releaseAssetPrefix + buildVersion + ".json"
	descriptorSHA256, ok := parseChecksumLine(checksumBytes, descriptorName)
	if !ok {
		return VerifiedRelease{}, descriptorIntegrity
	}
	descriptorHash := sha256.Sum256(descriptorBytes)
	if descriptorSHA256 != hex.EncodeToString(descriptorHash[:]) {
		return VerifiedRelease{}, descriptorIntegrity
	}
	if !hasUniqueJSONKeys(descriptorBytes) {
		return VerifiedRelease{}, descriptorIntegrity
	}

	var wire descriptor
	decoder := json.NewDecoder(bytes.NewReader(descriptorBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return VerifiedRelease{}, descriptorIntegrity
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return VerifiedRelease{}, descriptorIntegrity
	}
	canonical, err := json.MarshalIndent(wire, "", "  ")
	if err != nil || !bytes.Equal(append(canonical, '\n'), descriptorBytes) {
		return VerifiedRelease{}, descriptorIntegrity
	}

	if wire.SchemaVersion != 1 ||
		wire.Schema != "https://raw.githubusercontent.com/SkrobyLabs/kubikles/"+wire.Source.Commit+"/release/accelerator-release.schema.json" ||
		wire.BuildVersion != buildVersion ||
		wire.Source.Repository != sourceRepository ||
		!commitRE.MatchString(wire.Source.Commit) ||
		wire.Source.GitTag != buildVersion ||
		wire.Compatibility.Mode != "exact-build-version" ||
		wire.Compatibility.DesktopBuildVersion != buildVersion ||
		wire.Compatibility.AcceleratorBuildVersion != buildVersion {
		return VerifiedRelease{}, descriptorIntegrity
	}
	if wire.Image.Repository != imageRepository ||
		!digestRE.MatchString(wire.Image.Digest) ||
		wire.Image.Reference != wire.Image.Repository+"@"+wire.Image.Digest ||
		len(wire.Image.Platforms) != 2 ||
		wire.Image.Platforms[0].OS != "linux" ||
		wire.Image.Platforms[0].Architecture != "amd64" ||
		!digestRE.MatchString(wire.Image.Platforms[0].ManifestDigest) ||
		wire.Image.Platforms[1].OS != "linux" ||
		wire.Image.Platforms[1].Architecture != "arm64" ||
		!digestRE.MatchString(wire.Image.Platforms[1].ManifestDigest) ||
		wire.Image.Platforms[0].ManifestDigest == wire.Image.Platforms[1].ManifestDigest {
		return VerifiedRelease{}, descriptorIntegrity
	}
	chartVersion := strings.TrimPrefix(buildVersion, "v")
	if wire.Chart.Repository != chartRepository ||
		!digestRE.MatchString(wire.Chart.Digest) ||
		wire.Chart.Reference != wire.Chart.Repository+"@"+wire.Chart.Digest ||
		wire.Chart.Version != chartVersion ||
		wire.Chart.AppVersion != buildVersion {
		return VerifiedRelease{}, descriptorIntegrity
	}

	return VerifiedRelease{
		BuildVersion:     buildVersion,
		SourceCommit:     wire.Source.Commit,
		DescriptorSHA256: descriptorSHA256,
		ImageReference:   wire.Image.Reference,
		ChartReference:   wire.Chart.Reference,
	}, descriptorValid
}

// parseChecksumLine validates the canonical publication grammar before the
// descriptor is requested, retaining the promised descriptor identity.
func parseChecksumLine(line []byte, descriptorName string) (string, bool) {
	if len(line) != 64+2+len(descriptorName)+1 {
		return "", false
	}
	digest := string(line[:64])
	if !hex64RE.MatchString(digest) || !bytes.Equal(line[64:], []byte("  "+descriptorName+"\n")) {
		return "", false
	}
	return digest, true
}

func validChecksumLine(line []byte, expectedHash, descriptorName string) bool {
	digest, ok := parseChecksumLine(line, descriptorName)
	return ok && digest == expectedHash
}

func hasUniqueJSONKeys(data []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if !consumeUniqueJSONValue(decoder) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func consumeUniqueJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return false
			}
			key, ok := keyToken.(string)
			if !ok {
				return false
			}
			if _, duplicate := seen[key]; duplicate {
				return false
			}
			seen[key] = struct{}{}
			if !consumeUniqueJSONValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			if !consumeUniqueJSONValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}
