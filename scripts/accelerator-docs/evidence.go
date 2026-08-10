package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type evidenceManifest struct {
	SchemaVersion         int            `json:"schemaVersion"`
	TestedCommit          string         `json:"testedCommit"`
	AuthorityInputsSHA256 string         `json:"authorityInputsSha256"`
	CapturedAt            string         `json:"capturedAt"`
	Records               []evidenceFile `json:"records"`
}
type evidenceFile struct{ Owner, Path, SHA256 string }

func (f evidenceFile) MarshalJSON() ([]byte, error) {
	type alias evidenceFile
	return json.Marshal(struct {
		Owner  string `json:"owner"`
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}{f.Owner, f.Path, f.SHA256})
}
func (f *evidenceFile) UnmarshalJSON(b []byte) error {
	var v struct {
		Owner  string `json:"owner"`
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	if err := strictDecode(b, &v); err != nil {
		return err
	}
	f.Owner, f.Path, f.SHA256 = v.Owner, v.Path, v.SHA256
	return nil
}

type evidenceRecord struct {
	SchemaVersion         int               `json:"schemaVersion"`
	Owner                 string            `json:"owner"`
	Command               string            `json:"command"`
	TestedCommit          string            `json:"testedCommit"`
	AuthorityInputsSHA256 string            `json:"authorityInputsSha256"`
	CapturedAt            string            `json:"capturedAt"`
	CleanCapture          bool              `json:"cleanCapture"`
	Result                string            `json:"result"`
	DurationMS            int64             `json:"durationMs"`
	Fixture               string            `json:"fixture"`
	Environment           map[string]string `json:"environment"`
	Measurements          map[string]int64  `json:"measurements"`
}

var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)
var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var evidenceOwners = []string{"2fe9439f", "4026deaf", "dd19f7b6"}
var ownerCommands = map[string]string{"dd19f7b6": "make test-accelerator-00-kind", "2fe9439f": "make test-accelerator-e2e", "4026deaf": "make test-accelerator-supply-chain"}
var allowedMeasurements = map[string]map[string]bool{"dd19f7b6": {"exactPodLoopback": true}, "2fe9439f": {"acceptanceCases": true}, "4026deaf": {"sbomSubjects": true}}

func strictDecode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var x any
	if err := d.Decode(&x); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}
func canonicalJSON(v any) ([]byte, error) {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return nil, e
	}
	return append(b, '\n'), nil
}

func loadEvidence(root string) (evidenceManifest, map[string]evidenceRecord, error) {
	b, err := os.ReadFile(filepath.Join(root, "docs/accelerator/evidence/manifest-v1.json"))
	if err != nil {
		return evidenceManifest{}, nil, err
	}
	var m evidenceManifest
	if err = strictDecode(b, &m); err != nil {
		return m, nil, errors.New("DOC-EVIDENCE-MANIFEST")
	}
	records := map[string]evidenceRecord{}
	for _, f := range m.Records {
		data, e := os.ReadFile(filepath.Join(root, "docs/accelerator/evidence", filepath.FromSlash(f.Path)))
		if e != nil {
			return m, nil, errors.New("DOC-EVIDENCE-RECORD")
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != f.SHA256 {
			return m, nil, errors.New("DOC-EVIDENCE-CHECKSUM")
		}
		var rec evidenceRecord
		if strictDecode(data, &rec) != nil {
			return m, nil, errors.New("DOC-EVIDENCE-RECORD")
		}
		records[f.Owner] = rec
	}
	if err := validateEvidence(m, records); err != nil {
		return m, nil, err
	}
	return m, records, nil
}
func validateEvidence(m evidenceManifest, records map[string]evidenceRecord) error {
	if m.SchemaVersion != 1 || !hex40.MatchString(m.TestedCommit) || !hex64.MatchString(m.AuthorityInputsSHA256) || len(m.Records) != 3 {
		return errors.New("DOC-EVIDENCE-IDENTITY")
	}
	if _, e := time.Parse(time.RFC3339, m.CapturedAt); e != nil {
		return errors.New("DOC-EVIDENCE-TIME")
	}
	owners := make([]string, 0, 3)
	for _, f := range m.Records {
		owners = append(owners, f.Owner)
		if f.Path != "run-"+f.Owner+".json" || !hex64.MatchString(f.SHA256) {
			return errors.New("DOC-EVIDENCE-PATH")
		}
		r, ok := records[f.Owner]
		if !ok || r.SchemaVersion != 1 || r.Owner != f.Owner || r.Command != ownerCommands[f.Owner] || r.TestedCommit != m.TestedCommit || r.AuthorityInputsSHA256 != m.AuthorityInputsSHA256 || r.CapturedAt != m.CapturedAt || !r.CleanCapture || r.Result != "pass" || r.DurationMS <= 0 || r.Fixture == "" || len(r.Environment) == 0 {
			return errors.New("DOC-EVIDENCE-RECORD")
		}
		for k, v := range r.Measurements {
			if !allowedMeasurements[f.Owner][k] || v <= 0 {
				return errors.New("DOC-EVIDENCE-MEASUREMENT")
			}
		}
	}
	sort.Strings(owners)
	if !equal(owners, evidenceOwners) {
		return errors.New("DOC-EVIDENCE-OWNERS")
	}
	return nil
}

func renderEvidence(m evidenceManifest, records map[string]evidenceRecord) []byte {
	var b strings.Builder
	b.WriteString("# Kubikles Accelerator acceptance evidence\n\nKubikles Accelerator is referred to here as Accelerator. This generated page records three mandatory gates run serially on one clean source commit. It contains safe summaries, not raw command output.\n\n")
	fmt.Fprintf(&b, "- Tested commit: `%s`\n- Authority-input SHA-256: `%s`\n- Captured at: `%s`\n\n", m.TestedCommit, m.AuthorityInputsSHA256, m.CapturedAt)
	b.WriteString("| Owner | Command | Result | Duration | Raw record |\n|---|---|---|---:|---|\n")
	for _, f := range m.Records {
		r := records[f.Owner]
		fmt.Fprintf(&b, "| `%s` | `%s` | pass | %d ms | [%s](evidence/%s) |\n", r.Owner, r.Command, r.DurationMS, f.Path, f.Path)
	}
	b.WriteString("\nThe observed durations describe this run only. They are not performance claims. Runtime clusters do not enforce SBOM or vulnerability verification.\n\n[Back to the Accelerator overview](../features/accelerator.md)\n")
	return []byte(b.String())
}
