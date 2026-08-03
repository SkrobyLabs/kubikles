package acceleratoracceptance

import (
	"bytes"
	"strings"
	"testing"
)

func exactPassReport(t *testing.T, contract Contract) Report {
	t.Helper()
	report := NewReport()
	for _, testCase := range contract.Cases {
		if err := report.RecordPass(testCase, 1); err != nil {
			t.Fatal("exact pass result was rejected")
		}
	}
	return report
}

func cloneReport(report Report) Report {
	clone := report
	clone.Cases = append([]CaseResult(nil), report.Cases...)
	for index := range clone.Cases {
		clone.Cases[index].Evidence = append([]EvidenceResult(nil), report.Cases[index].Evidence...)
	}
	return clone
}

func TestAcceptanceReportRequiresAllPassEvidence(t *testing.T) {
	contract, err := LoadContract(acceptanceContractPath())
	if err != nil {
		t.Fatal("checked-in acceptance contract did not load")
	}
	report := exactPassReport(t, contract)
	if err := ValidateReport(contract, report); err != nil {
		t.Fatal("exact pass report was rejected")
	}
	encoded, err := MarshalReport(report)
	if err != nil {
		t.Fatal("exact pass report did not marshal")
	}
	parsed, err := ParseReport(bytes.NewReader(encoded))
	if err != nil || ValidateReport(contract, parsed) != nil {
		t.Fatal("canonical report did not round trip")
	}
	for _, forbidden := range []string{
		`"timestamp":`, `"hostname":`, `"path":`, `"namespace":`, `"objectIdentity":`, `"sessionIdentity":`, `"digest":`,
		"http://private-marker", "https://private-marker", "endpoint-private-marker", "payload-private-marker", "credential-private-marker", "raw-error-private-marker",
	} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatal("canonical report contains forbidden unsafe vocabulary")
		}
	}

	mutations := []struct {
		name   string
		mutate func(*Report)
	}{
		{name: "missing row", mutate: func(r *Report) { r.Cases = r.Cases[1:] }},
		{name: "duplicate row", mutate: func(r *Report) { r.Cases[1] = r.Cases[0] }},
		{name: "unknown row", mutate: func(r *Report) { r.Cases[0].ID = "A60A-999-FUTURE" }},
		{name: "failed", mutate: func(r *Report) { r.Cases[0].Status = Status("failed") }},
		{name: "skipped", mutate: func(r *Report) { r.Cases[0].Status = Status("skipped") }},
		{name: "optional", mutate: func(r *Report) { r.Cases[0].Status = Status("optional") }},
		{name: "evidence false", mutate: func(r *Report) { r.Cases[0].Evidence[0].Passed = false }},
		{name: "evidence missing", mutate: func(r *Report) { r.Cases[0].Evidence = r.Cases[0].Evidence[1:] }},
		{name: "evidence extra", mutate: func(r *Report) {
			r.Cases[0].Evidence = append(r.Cases[0].Evidence, EvidenceResult{Key: "future.evidence", Passed: true, Count: 1})
		}},
		{name: "evidence zero", mutate: func(r *Report) { r.Cases[0].Evidence[0].Count = 0 }},
		{name: "duration zero", mutate: func(r *Report) { r.Cases[0].DurationMS = 0 }},
		{name: "failure code", mutate: func(r *Report) { r.Cases[0].FailureCode = FailureCommand }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			candidate := cloneReport(report)
			mutation.mutate(&candidate)
			if ValidateReport(contract, candidate) == nil {
				t.Fatal("invalid report mutation was accepted")
			}
		})
	}
}

func TestAcceptanceReportRejectsUnknownAndUnsafeJSON(t *testing.T) {
	contract, err := LoadContract(acceptanceContractPath())
	if err != nil {
		t.Fatal("checked-in acceptance contract did not load")
	}
	encoded, err := MarshalReport(exactPassReport(t, contract))
	if err != nil {
		t.Fatal("exact pass report did not marshal")
	}
	for _, mutation := range []struct {
		name string
		raw  []byte
	}{
		{name: "unknown field", raw: bytes.Replace(encoded, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"path":"private-marker"`), 1)},
		{name: "truncated", raw: encoded[:len(encoded)-2]},
		{name: "trailing", raw: append(append([]byte(nil), encoded...), []byte(`{}`)...)},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			if parsed, err := ParseReport(bytes.NewReader(mutation.raw)); err == nil || ValidateReport(contract, parsed) == nil {
				t.Fatal("unsafe or noncanonical report JSON was accepted")
			}
		})
	}
}
