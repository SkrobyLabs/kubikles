package supplychain

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const EvidenceSchemaVersion = 1

type EvidenceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   int64  `json:"mode"`
}

type EvidenceManifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	BuildVersion  string         `json:"buildVersion"`
	SourceCommit  string         `json:"sourceCommit"`
	SourceEpoch   int64          `json:"sourceEpoch"`
	Files         []EvidenceFile `json:"files"`
}

func CreateBundle(source, archive, checksum, version, commit string, epoch int64) error {
	if _, err := ValidateReleaseIdentity(version, commit); err != nil || epoch <= 0 || !filepath.IsAbs(source) || !filepath.IsAbs(archive) || !filepath.IsAbs(checksum) {
		return errors.New("bundle identity invalid")
	}
	if filepath.Clean(source) == filepath.Clean(filepath.Dir(archive)) || strings.HasPrefix(filepath.Clean(archive)+string(filepath.Separator), filepath.Clean(source)+string(filepath.Separator)) {
		return errors.New("bundle output overlaps input")
	}
	manifest, err := collectEvidence(source, version, commit, epoch)
	if err != nil {
		return err
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return errors.New("bundle manifest invalid")
	}
	manifestBytes = append(manifestBytes, '\n')
	temporary := archive + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.New("bundle output unavailable")
	}
	removeTemporary := true
	defer func() {
		file.Close()
		if removeTemporary {
			os.Remove(temporary)
		}
	}()
	writer := tar.NewWriter(file)
	modTime := time.Unix(epoch, 0).UTC()
	if err := writeTarEntry(writer, "accelerator-supply-chain-evidence.json", manifestBytes, 0o600, modTime); err != nil {
		return errors.New("bundle write failed")
	}
	for _, entry := range manifest.Files {
		data, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(entry.Path)))
		if err != nil || int64(len(data)) != entry.Size || Digest(data) != "sha256:"+entry.SHA256 {
			return errors.New("bundle input changed")
		}
		if err := writeTarEntry(writer, entry.Path, data, entry.Mode, modTime); err != nil {
			return errors.New("bundle write failed")
		}
	}
	if writer.Close() != nil || file.Sync() != nil || file.Close() != nil || os.Rename(temporary, archive) != nil {
		return errors.New("bundle finalization failed")
	}
	removeTemporary = false
	data, err := os.ReadFile(archive)
	if err != nil {
		return errors.New("bundle read-back failed")
	}
	line := strings.TrimPrefix(Digest(data), "sha256:") + "  " + filepath.Base(archive) + "\n"
	if err := os.WriteFile(checksum, []byte(line), 0o600); err != nil {
		return errors.New("bundle checksum failed")
	}
	return nil
}

func VerifyBundle(archive, checksum, destination, version, commit string) (EvidenceManifest, error) {
	if _, err := ValidateReleaseIdentity(version, commit); err != nil || !filepath.IsAbs(archive) || !filepath.IsAbs(checksum) || !filepath.IsAbs(destination) {
		return EvidenceManifest{}, errors.New("bundle identity invalid")
	}
	archiveData, err := os.ReadFile(archive)
	if err != nil {
		return EvidenceManifest{}, errors.New("bundle unavailable")
	}
	checksumData, err := os.ReadFile(checksum)
	wantLine := strings.TrimPrefix(Digest(archiveData), "sha256:") + "  " + filepath.Base(archive) + "\n"
	if err != nil || string(checksumData) != wantLine {
		return EvidenceManifest{}, errors.New("bundle checksum invalid")
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return EvidenceManifest{}, errors.New("bundle destination invalid")
	}
	reader := tar.NewReader(bytes.NewReader(archiveData))
	var manifest EvidenceManifest
	seen := map[string]struct{}{}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || header.Typeflag != tar.TypeReg || !safeRelativePath(header.Name) || header.Size < 0 || header.Size > 1<<30 || header.Mode != 0o600 {
			return EvidenceManifest{}, errors.New("bundle entry invalid")
		}
		if _, duplicate := seen[header.Name]; duplicate {
			return EvidenceManifest{}, errors.New("bundle entry duplicate")
		}
		seen[header.Name] = struct{}{}
		data, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
		if err != nil || int64(len(data)) != header.Size {
			return EvidenceManifest{}, errors.New("bundle entry invalid")
		}
		if header.Name == "accelerator-supply-chain-evidence.json" {
			if decodeStrict(bytes.NewReader(data), &manifest) != nil {
				return EvidenceManifest{}, errors.New("bundle evidence invalid")
			}
			continue
		}
		path := filepath.Join(destination, filepath.FromSlash(header.Name))
		if !strings.HasPrefix(filepath.Clean(path)+string(filepath.Separator), filepath.Clean(destination)+string(filepath.Separator)) {
			return EvidenceManifest{}, errors.New("bundle path invalid")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil || os.WriteFile(path, data, 0o600) != nil {
			return EvidenceManifest{}, errors.New("bundle extraction failed")
		}
	}
	if validateEvidence(manifest, version, commit) != nil || len(seen) != len(manifest.Files)+1 {
		return EvidenceManifest{}, errors.New("bundle evidence mismatch")
	}
	for _, entry := range manifest.Files {
		data, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(entry.Path)))
		if err != nil || int64(len(data)) != entry.Size || Digest(data) != "sha256:"+entry.SHA256 {
			return EvidenceManifest{}, errors.New("bundle file mismatch")
		}
	}
	return manifest, nil
}

func collectEvidence(root, version, commit string, epoch int64) (EvidenceManifest, error) {
	result := EvidenceManifest{SchemaVersion: EvidenceSchemaVersion, BuildVersion: version, SourceCommit: commit, SourceEpoch: epoch}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return errors.New("bundle input type invalid")
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || !safeRelativePath(filepath.ToSlash(relative)) || relative == "accelerator-supply-chain-evidence.json" {
			return errors.New("bundle input path invalid")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		result.Files = append(result.Files, EvidenceFile{Path: filepath.ToSlash(relative), SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data)), Mode: 0o600})
		return nil
	})
	if err != nil || len(result.Files) == 0 {
		return EvidenceManifest{}, errors.New("bundle input invalid")
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	return result, validateEvidence(result, version, commit)
}

func validateEvidence(manifest EvidenceManifest, version, commit string) error {
	if manifest.SchemaVersion != EvidenceSchemaVersion || manifest.BuildVersion != version || manifest.SourceCommit != commit || manifest.SourceEpoch <= 0 || len(manifest.Files) == 0 {
		return errors.New("invalid evidence identity")
	}
	previous := ""
	for _, entry := range manifest.Files {
		if !safeRelativePath(entry.Path) || entry.Path <= previous || !hex64Pattern.MatchString(entry.SHA256) || entry.Size < 0 || entry.Mode != 0o600 {
			return errors.New("invalid evidence file")
		}
		previous = entry.Path
	}
	return nil
}

func safeRelativePath(path string) bool {
	return path != "" && path == filepath.ToSlash(filepath.Clean(path)) && path != "." && !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "../") && !strings.Contains(path, "\\")
}

func writeTarEntry(writer *tar.Writer, name string, data []byte, mode int64, modTime time.Time) error {
	header := &tar.Header{Name: name, Mode: mode, Size: int64(len(data)), ModTime: modTime, AccessTime: modTime, ChangeTime: modTime, Typeflag: tar.TypeReg, Format: tar.FormatPAX}
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	_, err := writer.Write(data)
	return err
}

func VerifyChecksumFile(path, expectedBase string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || scanner.Text() == "" || scanner.Scan() || scanner.Err() != nil {
		return errors.New("checksum invalid")
	}
	parts := strings.Split(scanner.Text(), "  ")
	if len(parts) != 2 || !hex64Pattern.MatchString(parts[0]) || parts[1] != expectedBase {
		return fmt.Errorf("checksum invalid")
	}
	return nil
}
