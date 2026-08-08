package acceleratoracceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedFixtureRejectsTamperingAndMutablePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	chart := filepath.Join(root, "chart.tgz")
	binary := filepath.Join(root, "desktop.test")
	if err := os.WriteFile(chart, []byte("chart"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	hash := func(path string) string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	fixture := SharedFixture{
		BuildVersion: BuildIdentity, ExecutionArchitecture: "arm64", ReconnectGraceSeconds: 5, ArtifactRoot: root,
		ProductionImageDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AcceptanceImageDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ChartDigest:           "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		ChartArchive:          chart, ChartArchiveSHA256: hash(chart),
		DesktopTestBinary: binary, DesktopTestBinarySHA256: hash(binary),
	}
	path := filepath.Join(root, "fixture.json")
	if err := WriteSharedFixtureAtomic(path, fixture); err != nil {
		t.Fatal("write canonical fixture")
	}
	if _, err := LoadSharedFixture(path); err != nil {
		t.Fatal("load canonical fixture")
	}
	invalidArchitecture := fixture
	invalidArchitecture.ExecutionArchitecture = "s390x"
	if err := ValidateSharedFixture(invalidArchitecture); err == nil {
		t.Fatal("accepted unsupported execution architecture")
	}
	if err := os.WriteFile(chart, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSharedFixture(path); err == nil {
		t.Fatal("accepted tampered chart")
	}
	fixture.ChartArchiveSHA256 = hash(chart)
	fixture.DesktopTestBinary = filepath.Join(filepath.Dir(root), "outside")
	if err := os.WriteFile(fixture.DesktopTestBinary, []byte("outside"), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.DesktopTestBinarySHA256 = hash(fixture.DesktopTestBinary)
	if err := ValidateSharedFixture(fixture); err == nil {
		t.Fatal("accepted mutable path outside artifact root")
	}
}
