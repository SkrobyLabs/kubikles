package main

import (
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
