package acceleratoracceptance

// The shared fixture is deliberately narrower than the shell fixture which
// creates it.  It describes only immutable inputs; namespace, kubeconfig,
// HOME, release and cleanup state stay with the individual acceptance case.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type SharedFixture struct {
	BuildVersion            string `json:"buildVersion"`
	ExecutionArchitecture   string `json:"executionArchitecture"`
	ReconnectGraceSeconds   int    `json:"reconnectGraceSeconds"`
	ArtifactRoot            string `json:"artifactRoot"`
	ProductionImageDigest   string `json:"productionImageDigest"`
	AcceptanceImageDigest   string `json:"acceptanceImageDigest"`
	ChartDigest             string `json:"chartDigest"`
	ChartArchive            string `json:"chartArchive"`
	ChartArchiveSHA256      string `json:"chartArchiveSha256"`
	DesktopTestBinary       string `json:"desktopTestBinary"`
	DesktopTestBinarySHA256 string `json:"desktopTestBinarySha256"`
}

// WriteSharedFixtureAtomic persists the private manifest used by focused
// owners.  It intentionally reuses the report writer's 0600 atomic handling.
func WriteSharedFixtureAtomic(path string, fixture SharedFixture) error {
	if err := ValidateSharedFixture(fixture); err != nil || path == "" || !filepath.IsAbs(path) {
		return errors.New("shared fixture invalid")
	}
	encoded, err := json.Marshal(fixture)
	if err != nil {
		return errors.New("shared fixture invalid")
	}
	return writePrivateAtomic(path, append(encoded, '\n'))
}

func LoadSharedFixture(path string) (SharedFixture, error) {
	if path == "" || !filepath.IsAbs(path) {
		return SharedFixture{}, errors.New("shared fixture invalid")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return SharedFixture{}, errors.New("shared fixture invalid")
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 || len(data) > maximumAcceptanceJSONBytes {
		return SharedFixture{}, errors.New("shared fixture invalid")
	}
	strict, err := readStrictJSON(bytes.NewReader(data))
	if err != nil {
		return SharedFixture{}, errors.New("shared fixture invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(strict))
	decoder.DisallowUnknownFields()
	var fixture SharedFixture
	if err = decoder.Decode(&fixture); err != nil {
		return SharedFixture{}, errors.New("shared fixture invalid")
	}
	var trailing struct{}
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) || ValidateSharedFixture(fixture) != nil {
		return SharedFixture{}, errors.New("shared fixture invalid")
	}
	return fixture, nil
}

func ValidateSharedFixture(fixture SharedFixture) error {
	if fixture.BuildVersion != BuildIdentity || !validExecutionArchitecture(fixture.ExecutionArchitecture) || fixture.ReconnectGraceSeconds < 1 || fixture.ReconnectGraceSeconds > 30 ||
		!artifactDigestPattern.MatchString(fixture.ProductionImageDigest) || !artifactDigestPattern.MatchString(fixture.AcceptanceImageDigest) ||
		fixture.ProductionImageDigest == fixture.AcceptanceImageDigest || !artifactDigestPattern.MatchString(fixture.ChartDigest) || !validFixtureFile(fixture.ArtifactRoot, false, "") ||
		!validFixtureFile(fixture.ChartArchive, true, fixture.ChartArchiveSHA256) || !validFixtureFile(fixture.DesktopTestBinary, true, fixture.DesktopTestBinarySHA256) {
		return errors.New("shared fixture invalid")
	}
	artifactRoot, err := filepath.EvalSymlinks(fixture.ArtifactRoot)
	if err != nil || !filepath.IsAbs(artifactRoot) || !pathWithin(artifactRoot, fixture.ChartArchive) || !pathWithin(artifactRoot, fixture.DesktopTestBinary) {
		return errors.New("shared fixture invalid")
	}
	return nil
}

func validFixtureFile(path string, hashed bool, expected string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() && !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return false
	}
	if !hashed {
		return info.IsDir()
	}
	if !info.Mode().IsRegular() || len(expected) != 64 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == expected
}

func pathWithin(root, candidate string) bool {
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, resolved)
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
