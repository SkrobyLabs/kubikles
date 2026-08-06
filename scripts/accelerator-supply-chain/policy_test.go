package supplychain

import (
	"strings"
	"testing"
	"time"
)

func completeArtifacts() []Artifact {
	return []Artifact{ArtifactImageAMD64, ArtifactChart, ArtifactSourceGo, ArtifactSourceNPM}
}

func TestVulnerabilityPolicyClosedMatrix(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	report := ScanReport{SchemaVersion: 1, DBUpdatedAt: now.Add(-time.Hour).Format(time.RFC3339), Artifacts: completeArtifacts(), Findings: []Finding{
		{ID: "CVE-2026-12345", PURL: "pkg:generic/a@1.0", Artifact: ArtifactImageAMD64, Severity: "HIGH"},
		{ID: "GHSA-2345-6789-cfgh", PURL: "pkg:npm/b@2.0", Artifact: ArtifactSourceNPM, Severity: "LOW"},
	}}
	exception := VulnerabilityException{ID: "CVE-2026-12345", PURL: "pkg:generic/a@1.0", Artifact: ArtifactImageAMD64, Owner: "@security-owner", Justification: "Upstream mitigation is under review", ExpiresOn: "2026-08-04"}
	result, err := EvaluatePolicy(report, []VulnerabilityException{exception}, now, true)
	if err != nil || result.High != 1 || result.Low != 1 || len(result.Excepted) != 1 {
		t.Fatal("exact vulnerability exception")
	}
	if _, err := EvaluatePolicy(report, nil, now, true); err == nil {
		t.Fatal("accepted unexcepted high finding")
	}
	if _, err := EvaluatePolicy(report, []VulnerabilityException{exception}, now.Add(24*time.Hour), true); err == nil {
		t.Fatal("accepted exception on expiry date")
	}
	stale := report
	stale.DBUpdatedAt = now.Add(-25 * time.Hour).Format(time.RFC3339)
	if _, err := EvaluatePolicy(stale, []VulnerabilityException{exception}, now, true); err == nil {
		t.Fatal("accepted stale database")
	}
	incomplete := report
	incomplete.Artifacts = incomplete.Artifacts[:3]
	if _, err := EvaluatePolicy(incomplete, []VulnerabilityException{exception}, now, true); err == nil {
		t.Fatal("accepted incomplete scan")
	}
	invalid := exception
	invalid.PURL = "pkg:generic/*"
	if ValidateExceptions([]VulnerabilityException{invalid}) == nil {
		t.Fatal("accepted wildcard exception")
	}
	if _, err := ParseExceptions([]byte(`[{"id":"CVE-2026-12345","purl":"pkg:generic/a@1.0","artifact":"image-linux-amd64","owner":"@o","justification":"x","expiresOn":"2026-08-04","extra":true}]`)); err == nil {
		t.Fatal("accepted exception unknown field")
	}
	if _, err := ValidateReleaseIdentity("v1.0.0", strings.Repeat("a", 40)); err != nil {
		t.Fatal("test identity")
	}
}
