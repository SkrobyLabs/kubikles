package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"kubikles/internal/acceleratoracceptance"
)

func TestValidateReportModeRechecksExactFinalReport(t *testing.T) {
	contractPath := filepath.Join("..", "..", "..", "..", "test", "accelerator", "acceptance-v1.json")
	contract, err := acceleratoracceptance.LoadContract(contractPath)
	if err != nil {
		t.Fatal("load acceptance contract")
	}
	report := acceleratoracceptance.NewReport()
	for _, testCase := range contract.Cases {
		if err = report.RecordPass(testCase, 1); err != nil {
			t.Fatal("record exact acceptance case")
		}
	}
	reportPath := filepath.Join(t.TempDir(), "final-report.json")
	if err = acceleratoracceptance.WriteReportAtomic(reportPath, contract, report); err != nil {
		t.Fatal("write exact final report")
	}
	t.Setenv("ACCELERATOR_ACCEPTANCE_CONTRACT", contractPath)
	t.Setenv("ACCELERATOR_ACCEPTANCE_FINAL_REPORT", reportPath)
	for _, key := range []string{"ACCELERATOR_ACCEPTANCE_FIXTURE_MS", "ACCELERATOR_ACCEPTANCE_ARTIFACT_MS", "ACCELERATOR_ACCEPTANCE_FOCUSED_MS", "ACCELERATOR_ACCEPTANCE_COMPOSED_MS", "ACCELERATOR_ACCEPTANCE_VALIDATION_MS", "ACCELERATOR_ACCEPTANCE_TOTAL_MS"} {
		t.Setenv(key, "1")
	}
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()
	os.Args = []string{"accelerator-e2e-runner", "validate-report"}
	if err = run(); err != nil {
		t.Fatal("exact final report rejected")
	}

	report.Cases = report.Cases[:len(report.Cases)-1]
	encoded, err := acceleratoracceptance.MarshalReport(report)
	if err != nil || os.WriteFile(reportPath, encoded, 0o600) != nil {
		t.Fatal("write incomplete report fixture")
	}
	if err = run(); err == nil {
		t.Fatal("incomplete final report accepted")
	}
}

func TestTimingSummaryIsValueFreeAndOrdered(t *testing.T) {
	contract, err := acceleratoracceptance.LoadContract(filepath.Join("..", "..", "..", "..", "test", "accelerator", "acceptance-v1.json"))
	if err != nil {
		t.Fatal("load contract")
	}
	report := acceleratoracceptance.NewReport()
	for _, testCase := range contract.Cases {
		if report.RecordPass(testCase, 7) != nil {
			t.Fatal("report")
		}
	}
	encoded, err := acceleratoracceptance.MarshalTimingSummary(contract, report, 1, 2, 3, 4, 5, 6)
	if err != nil {
		t.Fatal("summary")
	}
	for _, forbidden := range []string{"/private/", "sha256:", "KUBIKLES_", "argv", "stdout"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("timing summary leaked %s", forbidden)
		}
	}
	if !bytes.Contains(encoded, []byte(`"concurrency":2`)) || !bytes.Contains(encoded, []byte(`"id":"A60A-001-BASELINE"`)) {
		t.Fatal("timing summary shape")
	}
}
