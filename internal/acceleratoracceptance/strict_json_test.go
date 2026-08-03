package acceleratoracceptance

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceJSONRejectsDuplicateKeysAndWritesReportAtomically(t *testing.T) {
	contractBytes, err := os.ReadFile(acceptanceContractPath())
	if err != nil {
		t.Fatal("read contract")
	}
	duplicateContract := bytes.Replace(contractBytes, []byte(`"schemaVersion": 1`), []byte(`"schemaVersion": 1, "schemaVersion": 1`), 1)
	if _, err = ParseContract(bytes.NewReader(duplicateContract)); err == nil {
		t.Fatal("contract accepted a duplicate JSON key")
	}
	contract, err := ParseContract(bytes.NewReader(contractBytes))
	if err != nil {
		t.Fatal("parse contract")
	}
	report := NewReport()
	for _, testCase := range contract.Cases {
		if err = report.RecordPass(testCase, 1); err != nil {
			t.Fatal("record report")
		}
	}
	encoded, err := MarshalReport(report)
	if err != nil {
		t.Fatal("marshal report")
	}
	duplicateReport := bytes.Replace(encoded, []byte(`"status":"pass"`), []byte(`"status":"pass","status":"pass"`), 1)
	if _, err = ParseReport(bytes.NewReader(duplicateReport)); err == nil {
		t.Fatal("report accepted a duplicate JSON key")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "report.json")
	if err = WriteReportAtomic(path, contract, report); err != nil {
		t.Fatal("atomic report write")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("atomic report permissions")
	}
	written, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(written, encoded) {
		t.Fatal("atomic report bytes")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || strings.HasPrefix(entries[0].Name(), ".accelerator-") {
		t.Fatal("atomic report retained a temporary file")
	}
	invalid := report
	invalid.Cases = invalid.Cases[:len(invalid.Cases)-1]
	if err = WriteReportAtomic(path, contract, invalid); err == nil {
		t.Fatal("atomic writer accepted an incomplete report")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, encoded) {
		t.Fatal("invalid report replaced the complete report")
	}
}
