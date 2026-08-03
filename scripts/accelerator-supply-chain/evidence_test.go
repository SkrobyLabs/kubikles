package supplychain

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparedReleaseIsTheOnlyPublishedInput(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if os.Mkdir(source, 0o700) != nil || os.Mkdir(filepath.Join(source, "image-layout"), 0o700) != nil {
		t.Fatal("fixture directory")
	}
	if os.WriteFile(filepath.Join(source, "image-layout", "index.json"), []byte("exact-index\n"), 0o600) != nil || os.WriteFile(filepath.Join(source, "chart.tgz"), []byte("exact-chart\n"), 0o600) != nil {
		t.Fatal("fixture input")
	}
	archive := filepath.Join(root, "prepared.tar")
	checksum := archive + ".sha256"
	commit := strings.Repeat("a", 40)
	if err := CreateBundle(source, archive, checksum, "v1.4.0", commit, 1700000000); err != nil {
		t.Fatal("bundle creation")
	}
	secondArchive := filepath.Join(root, "prepared-second.tar")
	if err := CreateBundle(source, secondArchive, secondArchive+".sha256", "v1.4.0", commit, 1700000000); err != nil {
		t.Fatal("second bundle creation")
	}
	first, _ := os.ReadFile(archive)
	second, _ := os.ReadFile(secondArchive)
	if string(first) != string(second) {
		t.Fatal("bundle is not deterministic")
	}
	destination := filepath.Join(root, "verified")
	manifest, err := VerifyBundle(archive, checksum, destination, "v1.4.0", commit)
	if err != nil || len(manifest.Files) != 2 {
		t.Fatal("bundle verification")
	}
	changed := append([]byte(nil), first...)
	offset := bytes.Index(changed, []byte("exact-index"))
	if offset < 0 {
		t.Fatal("bundle content unavailable")
	}
	changed[offset] ^= 1
	tampered := filepath.Join(root, "tampered.tar")
	if os.WriteFile(tampered, changed, 0o600) != nil || os.WriteFile(tampered+".sha256", []byte(strings.TrimPrefix(Digest(changed), "sha256:")+"  tampered.tar\n"), 0o600) != nil {
		t.Fatal("tamper fixture")
	}
	if _, err := VerifyBundle(tampered, tampered+".sha256", filepath.Join(root, "tampered-output"), "v1.4.0", commit); err == nil {
		t.Fatal("accepted mutated archive")
	}
	if _, err := VerifyBundle(archive, checksum, filepath.Join(root, "wrong-source"), "v1.4.0", strings.Repeat("b", 40)); err == nil {
		t.Fatal("accepted changed source identity")
	}
}
