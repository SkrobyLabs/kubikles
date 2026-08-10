package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type gate struct {
	owner, command, fixture string
	measurements            map[string]int64
}

func captureEvidence(root string) error {
	c, err := loadContract(root)
	if err != nil {
		return err
	}
	tools := []string{"git", "make", "go", "node", "npm", "docker", "kind", "kubectl", "helm", "jq", "syft", "trivy", "oras"}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("DOC-CAPTURE-PREREQUISITE: %s is required", tool)
		}
	}
	if err := runQuiet(root, "docker", "info"); err != nil {
		return errors.New("DOC-CAPTURE-PREREQUISITE: Docker daemon is required")
	}
	if err := runQuiet(root, "docker", "buildx", "version"); err != nil {
		return errors.New("DOC-CAPTURE-PREREQUISITE: Docker Buildx is required")
	}
	commit, err := output(root, "git", "rev-parse", "HEAD")
	if err != nil || !hex40.MatchString(commit) {
		return errors.New("DOC-CAPTURE-COMMIT")
	}
	if err = assertClean(root, commit); err != nil {
		return err
	}
	digest, err := authorityDigest(root, c.AuthorityInputs)
	if err != nil {
		return err
	}
	environment, err := captureEnvironment(root)
	if err != nil {
		return err
	}
	captured := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	gates := []gate{
		{"dd19f7b6", "make test-accelerator-00-kind", "disposable exact-Pod loopback and Direct baseline", map[string]int64{"exactPodLoopback": 1}},
		{"2fe9439f", "make test-accelerator-e2e", "closed offline Integrated acceptance", map[string]int64{"acceptanceCases": 18}},
		{"4026deaf", "make test-accelerator-supply-chain", "reproducible linux/amd64 image and chart release set", map[string]int64{"sbomSubjects": 2}},
	}
	records := map[string]evidenceRecord{}
	for _, g := range gates {
		if err = assertUnchanged(root, commit, digest, c); err != nil {
			return err
		}
		start := time.Now()
		cmd := exec.Command("make", strings.TrimPrefix(g.command, "make "))
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "BUILD_VERSION=v0.0.0")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("DOC-CAPTURE-GATE: %s failed", g.owner)
		}
		duration := time.Since(start).Milliseconds()
		if duration < 1 {
			duration = 1
		}
		if err = assertUnchanged(root, commit, digest, c); err != nil {
			return err
		}
		records[g.owner] = evidenceRecord{1, g.owner, g.command, commit, digest, captured, true, "pass", duration, g.fixture, environment, g.measurements}
	}
	manifest := evidenceManifest{SchemaVersion: 1, TestedCommit: commit, AuthorityInputsSHA256: digest, CapturedAt: captured}
	tmp, err := os.MkdirTemp(filepath.Join(root, "docs/accelerator"), ".evidence-")
	if err != nil {
		return err
	}
	if err = os.Chmod(tmp, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for _, owner := range evidenceOwners {
		b, e := canonicalJSON(records[owner])
		if e != nil {
			return e
		}
		name := "run-" + owner + ".json"
		if e = os.WriteFile(filepath.Join(tmp, name), b, 0o600); e != nil {
			return e
		}
		sum := sha256.Sum256(b)
		manifest.Records = append(manifest.Records, evidenceFile{owner, name, hex.EncodeToString(sum[:])})
	}
	mb, err := canonicalJSON(manifest)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(tmp, "manifest-v1.json"), mb, 0o600); err != nil {
		return err
	}
	if err = validateEvidence(manifest, records); err != nil {
		return err
	}
	page := renderEvidence(manifest, records)
	return installEvidence(root, tmp, page)
}

func captureEnvironment(root string) (map[string]string, error) {
	type query struct {
		name string
		args []string
	}
	qs := []query{{"go", []string{"go", "version"}}, {"node", []string{"node", "--version"}}, {"npm", []string{"npm", "--version"}}, {"docker", []string{"docker", "--version"}}, {"buildx", []string{"docker", "buildx", "version"}}, {"kind", []string{"kind", "version"}}, {"kubectl", []string{"kubectl", "version", "--client=true"}}, {"helm", []string{"helm", "version", "--short"}}, {"syft", []string{"syft", "version", "-o", "json"}}, {"trivy", []string{"trivy", "--version"}}, {"oras", []string{"oras", "version"}}}
	m := map[string]string{"os": runtime.GOOS, "architecture": runtime.GOARCH, "buildVersion": "v0.0.0"}
	for _, q := range qs {
		v, e := output(root, q.args[0], q.args[1:]...)
		if e != nil {
			return nil, fmt.Errorf("DOC-CAPTURE-VERSION: %s", q.name)
		}
		if q.name == "syft" {
			var version struct {
				Version string `json:"version"`
			}
			if json.Unmarshal([]byte(v), &version) != nil || version.Version == "" {
				return nil, fmt.Errorf("DOC-CAPTURE-VERSION: %s", q.name)
			}
			v = version.Version
		} else {
			v = strings.Split(v, "\n")[0]
		}
		if len(v) > 240 || unsafeEvidence(v) {
			return nil, fmt.Errorf("DOC-CAPTURE-VERSION: %s", q.name)
		}
		m[q.name] = v
	}
	return m, nil
}
func unsafeEvidence(s string) bool {
	v := strings.ToLower(s)
	for _, bad := range []string{"token", "bearer", "cookie", "authorization:", "signedurl", "signed-url"} {
		if strings.Contains(v, bad) {
			return true
		}
	}
	return false
}
func output(root, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	b, e := cmd.Output()
	return strings.TrimSpace(string(b)), e
}
func runQuiet(root, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	return cmd.Run()
}
func assertClean(root, commit string) error {
	head, e := output(root, "git", "rev-parse", "HEAD")
	if e != nil || head != commit {
		return errors.New("DOC-CAPTURE-HEAD-CHANGED")
	}
	s, e := output(root, "git", "status", "--porcelain", "--untracked-files=all")
	if e != nil || s != "" {
		return errors.New("DOC-CAPTURE-DIRTY: commit documentation before capture")
	}
	return nil
}
func assertUnchanged(root, commit, digest string, c contract) error {
	if err := assertClean(root, commit); err != nil {
		return err
	}
	d, err := authorityDigest(root, c.AuthorityInputs)
	if err != nil || d != digest {
		return errors.New("DOC-CAPTURE-AUTHORITY-CHANGED")
	}
	return nil
}
func installEvidence(root, tmp string, page []byte) error {
	dest := filepath.Join(root, "docs/accelerator/evidence")
	backup := dest + ".previous"
	if _, err := os.Stat(backup); err == nil {
		return errors.New("DOC-CAPTURE-ATOMIC-STATE")
	}
	if err := os.Rename(dest, backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = os.RemoveAll(dest)
			_ = os.Rename(backup, dest)
		}
	}()
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	pagePath := filepath.Join(root, "docs/accelerator/evidence.md")
	pageTmp := pagePath + ".tmp"
	if err := os.WriteFile(pageTmp, page, 0o644); err != nil {
		return err
	}
	if err := os.Rename(pageTmp, pagePath); err != nil {
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	rollback = false
	return nil
}
