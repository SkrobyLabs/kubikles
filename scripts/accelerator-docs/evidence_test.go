package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestEvidenceManifestAndRender(t *testing.T) {
	commit := strings.Repeat("a", 40)
	digest := strings.Repeat("b", 64)
	at := "2026-08-03T12:00:00Z"
	records := map[string]evidenceRecord{}
	manifest := evidenceManifest{1, commit, digest, at, nil}
	for _, owner := range evidenceOwners {
		r := evidenceRecord{1, owner, ownerCommands[owner], commit, digest, at, true, "pass", 1, "fixture", map[string]string{"go": "go1"}, map[string]int64{}}
		for k := range allowedMeasurements[owner] {
			r.Measurements[k] = 1
		}
		records[owner] = r
		b, _ := canonicalJSON(r)
		sum := sha256.Sum256(b)
		manifest.Records = append(manifest.Records, evidenceFile{owner, "run-" + owner + ".json", hex.EncodeToString(sum[:])})
	}
	if err := validateEvidence(manifest, records); err != nil {
		t.Fatal(err)
	}
	page := renderEvidence(manifest, records)
	if !strings.Contains(string(page), commit) || !strings.Contains(string(page), "not performance claims") {
		t.Fatal("render omitted provenance or boundary")
	}
	bad := manifest
	bad.TestedCommit = "HEAD"
	if validateEvidence(bad, records) == nil {
		t.Fatal("invalid identity accepted")
	}
}
