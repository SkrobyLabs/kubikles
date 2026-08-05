package supplychain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	ToolchainSchemaVersion = 1
	ImageRepository        = "ghcr.io/skrobylabs/kubikles-accelerator"
	ChartRepository        = "ghcr.io/skrobylabs/helm/kubikles-accelerator"
	Repository             = "SkrobyLabs/kubikles"
	ReleaseWorkflow        = "SkrobyLabs/kubikles/.github/workflows/release.yml"
	OIDCIssuer             = "https://token.actions.githubusercontent.com"
	SLSAPredicate          = "https://slsa.dev/provenance/v1"
	SPDXPredicate          = "https://spdx.dev/Document/v2.3"
)

var (
	releaseVersionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?$`)
	hex40Pattern          = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hex64Pattern          = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type ActionPin struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
	Version  string `json:"version"`
}

type ToolPin struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Asset   string `json:"asset"`
	SHA256  string `json:"sha256"`
}

type Toolchain struct {
	SchemaVersion int         `json:"schemaVersion"`
	Actions       []ActionPin `json:"actions"`
	Tools         []ToolPin   `json:"tools"`
}

var expectedActions = []ActionPin{
	{"actions/attest", "508db95dd578ae2727ebd6217d5ba78e4fbda05d", "v4.2.1"},
	{"actions/checkout", "11bd71901bbe5b1630ceea73d27597364c9af683", "v4.2.2"},
	{"actions/download-artifact", "d3f86a106a0bac45b974a628896c90dbdf5c8093", "v4.3.0"},
	{"actions/setup-go", "d35c59abb061a4a6fb18e82ac0862c26744d6ab5", "v5.5.0"},
	{"actions/setup-node", "49933ea5288caeca8642d1e84afbd3f7d6820020", "v4.4.0"},
	{"actions/upload-artifact", "ea165f8d65b6e75b540449e92b4886f43607fa02", "v4.6.2"},
	{"docker/login-action", "184bdaa0721073962dff0199f1fb9940f07167d1", "v3.5.0"},
	{"docker/setup-buildx-action", "e468171a9de216ec08956ac3ada2f0791b6bd435", "v3.11.1"},
}

var expectedTools = []ToolPin{
	{"buildkit", "0.31.2", "moby/buildkit OCI image", "2f5adac4ecd194d9f8c10b7b5d7bceb5186853db1b26e5abd3a657af0b7e26ec"},
	{"buildx", "0.36.0", "docker plugin", "df28b0a0b6a44453a87bd53c438432f4120962c9"},
	{"gh", "2.97.0", "gh_2.97.0_linux_amd64.tar.gz", "a2c9b8497e1f85b1ad0dfcb78b5a622e098801b8e461e459e88e1ee12f018112"},
	{"helm", "3.21.3", "helm-v3.21.3-linux-amd64.tar.gz", "15e041a93a590dce8100f39385cd98c84a765c9e36aeeb9e2dc6ff9e4769e2e0"},
	{"oras", "1.3.3", "oras_1.3.3_linux_amd64.tar.gz", "9ce999f8d2de03fc03968b29d743077a58783e545e5eaa53917ca177352d0e59"},
	{"syft", "1.44.0", "syft_1.44.0_linux_amd64.tar.gz", "0e91737aee2b5baf1d255b959630194a302335d848ff97bb07921eb6205b5f5a"},
	{"trivy", "0.70.0", "trivy_0.70.0_Linux-64bit.tar.gz", "8b4376d5d6befe5c24d503f10ff136d9e0c49f9127a4279fd110b727929a5aa9"},
}

func LoadToolchain(path string) (Toolchain, error) {
	file, err := os.Open(path)
	if err != nil {
		return Toolchain{}, errors.New("supply-chain toolchain unavailable")
	}
	defer file.Close()
	var result Toolchain
	if err := decodeStrict(file, &result); err != nil || ValidateToolchain(result) != nil {
		return Toolchain{}, errors.New("supply-chain toolchain invalid")
	}
	return result, nil
}

func ValidateToolchain(value Toolchain) error {
	if value.SchemaVersion != ToolchainSchemaVersion || !equalJSON(value.Actions, expectedActions) || !equalJSON(value.Tools, expectedTools) {
		return errors.New("invalid toolchain")
	}
	for index, action := range value.Actions {
		if !hex40Pattern.MatchString(action.Revision) || action.Name == "" || action.Version == "" || index > 0 && value.Actions[index-1].Name >= action.Name {
			return errors.New("invalid action pin")
		}
	}
	for index, tool := range value.Tools {
		if !hex64Pattern.MatchString(tool.SHA256) && tool.Name != "buildx" || tool.Name == "" || tool.Version == "" || tool.Asset == "" || index > 0 && value.Tools[index-1].Name >= tool.Name {
			return errors.New("invalid tool pin")
		}
	}
	return nil
}

func ValidateReleaseIdentity(version, commit string) (string, error) {
	if !releaseVersionPattern.MatchString(version) || !hex40Pattern.MatchString(commit) {
		return "", errors.New("invalid release identity")
	}
	return strings.TrimPrefix(version, "v"), nil
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func StableAssetNames(version string) ([]string, error) {
	if _, err := ValidateReleaseIdentity(version, strings.Repeat("a", 40)); err != nil {
		return nil, err
	}
	return []string{
		"kubikles-accelerator-image-linux-amd64-" + version + ".spdx.json",
		"kubikles-accelerator-image-linux-arm64-" + version + ".spdx.json",
		"kubikles-accelerator-chart-" + version + ".spdx.json",
		"kubikles-accelerator-attestations-" + version + ".jsonl",
	}, nil
}

func decodeStrict(reader io.Reader, target any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, 8<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

func equalJSON(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return bytes.Equal(a, b)
}

func SortedUnique(values []string) bool {
	return sort.StringsAreSorted(values) && len(values) == len(uniqueStrings(values))
}

func uniqueStrings(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
