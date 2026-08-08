package acceleratoracceptance

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const ReportSchemaVersion = 1

var errInvalidReport = errors.New("acceptance report invalid")

type Status string

const StatusPass Status = "pass"

type FailureCode string

const (
	FailureNone      FailureCode = "none"
	FailureContract  FailureCode = "contract"
	FailurePreflight FailureCode = "preflight"
	FailureCommand   FailureCode = "command"
	FailureAudit     FailureCode = "audit"
	FailureTimeout   FailureCode = "timeout"
	FailureSignal    FailureCode = "signal"
	FailureResidue   FailureCode = "residue"
	FailureReport    FailureCode = "report"
)

type Report struct {
	SchemaVersion int          `json:"schemaVersion"`
	Cases         []CaseResult `json:"cases"`
}

type CaseResult struct {
	ID          string           `json:"id"`
	Status      Status           `json:"status"`
	Evidence    []EvidenceResult `json:"evidence"`
	DurationMS  int64            `json:"durationMs"`
	FailureCode FailureCode      `json:"failureCode"`
}

type EvidenceResult struct {
	Key    string `json:"key"`
	Passed bool   `json:"passed"`
	Count  uint64 `json:"count"`
}

// TimingSummary is intentionally value-free operational evidence. It contains
// only fixed phase names, checked-in IDs, integer durations, and the fixed cap.
type TimingSummary struct {
	FixtureMS    int64        `json:"fixtureMs"`
	ArtifactMS   int64        `json:"artifactMs"`
	FocusedMS    int64        `json:"focusedMs"`
	ComposedMS   int64        `json:"composedMs"`
	ValidationMS int64        `json:"validationMs"`
	TotalMS      int64        `json:"totalMs"`
	Concurrency  int          `json:"concurrency"`
	Cases        []CaseTiming `json:"cases"`
}

type CaseTiming struct {
	ID         string `json:"id"`
	DurationMS int64  `json:"durationMs"`
}

// TimingSnapshot is the failure-safe counterpart to the all-pass report. Its
// closed fields make it safe to persist while a command or cleanup is failing:
// callers cannot attach argv, paths, environment values, Kubernetes values or
// raw child errors.
type TimingSnapshot struct {
	SchemaVersion    int            `json:"schemaVersion"`
	FixtureMS        int64          `json:"fixtureMs"`
	ArtifactMS       int64          `json:"artifactMs"`
	FocusedMS        int64          `json:"focusedMs"`
	ComposedMS       int64          `json:"composedMs"`
	ValidationMS     int64          `json:"validationMs"`
	CleanupMS        int64          `json:"cleanupMs"`
	CompletedTotalMS int64          `json:"completedTotalMs"`
	LastActivePhase  TimingPhase    `json:"lastActivePhase"`
	FocusedCase      string         `json:"focusedCase"`
	FocusedSubphase  TimingSubphase `json:"focusedSubphase"`
	ComposedCase     string         `json:"composedCase"`
	ComposedStage    ComposedStage  `json:"composedStage"`
	FailureCode      FailureCode    `json:"failureCode"`
}

type TimingPhase string

const (
	TimingFixture    TimingPhase = "fixture"
	TimingArtifact   TimingPhase = "artifact"
	TimingFocused    TimingPhase = "focused"
	TimingComposed   TimingPhase = "composed"
	TimingValidation TimingPhase = "validation"
	TimingCleanup    TimingPhase = "cleanup"
	TimingComplete   TimingPhase = "complete"
)

type TimingSubphase string

const (
	TimingSubphaseNone    TimingSubphase = "none"
	TimingSubphaseCommand TimingSubphase = "command"
	TimingSubphaseAudit   TimingSubphase = "audit"
	TimingSubphaseCleanup TimingSubphase = "cleanup"
)

type ComposedStage string

const (
	ComposedStageNone       ComposedStage = "none"
	ComposedStageSubtest    ComposedStage = "subtest"
	ComposedStageDiagnostic ComposedStage = "diagnostic"
)

func MarshalTimingSnapshot(snapshot TimingSnapshot) ([]byte, error) {
	if !validTimingSnapshot(snapshot) {
		return nil, errInvalidReport
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, errInvalidReport
	}
	return append(encoded, '\n'), nil
}

func validTimingSnapshot(snapshot TimingSnapshot) bool {
	if snapshot.SchemaVersion != ReportSchemaVersion || snapshot.FixtureMS < 0 || snapshot.ArtifactMS < 0 || snapshot.FocusedMS < 0 || snapshot.ComposedMS < 0 || snapshot.ValidationMS < 0 || snapshot.CleanupMS < 0 || snapshot.CompletedTotalMS < 0 {
		return false
	}
	switch snapshot.LastActivePhase {
	case TimingFixture, TimingArtifact, TimingFocused, TimingComposed, TimingValidation, TimingCleanup, TimingComplete:
	default:
		return false
	}
	switch snapshot.FocusedSubphase {
	case TimingSubphaseNone, TimingSubphaseCommand, TimingSubphaseAudit, TimingSubphaseCleanup:
	default:
		return false
	}
	switch snapshot.ComposedStage {
	case ComposedStageNone, ComposedStageSubtest, ComposedStageDiagnostic:
	default:
		return false
	}
	if snapshot.FailureCode != FailureNone {
		switch snapshot.FailureCode {
		case FailureContract, FailurePreflight, FailureCommand, FailureAudit, FailureTimeout, FailureSignal, FailureResidue, FailureReport:
		default:
			return false
		}
	}
	if snapshot.FocusedCase != "" && !idPattern.MatchString(snapshot.FocusedCase) {
		return false
	}
	if snapshot.ComposedCase != "" && !idPattern.MatchString(snapshot.ComposedCase) {
		return false
	}
	if snapshot.LastActivePhase != TimingFocused && (snapshot.FocusedCase != "" || snapshot.FocusedSubphase != TimingSubphaseNone) {
		return false
	}
	if snapshot.LastActivePhase != TimingComposed && (snapshot.ComposedCase != "" || snapshot.ComposedStage != ComposedStageNone) {
		return false
	}
	if snapshot.LastActivePhase == TimingComplete && snapshot.FailureCode != FailureNone {
		return false
	}
	return true
}

func MarshalTimingSummary(contract Contract, report Report, fixtureMS, artifactMS, focusedMS, composedMS, validationMS, totalMS int64) ([]byte, error) {
	if ValidateReport(contract, report) != nil || fixtureMS < 0 || artifactMS < 0 || focusedMS < 0 || composedMS < 0 || validationMS < 0 || totalMS < 0 {
		return nil, errInvalidReport
	}
	summary := TimingSummary{FixtureMS: fixtureMS, ArtifactMS: artifactMS, FocusedMS: focusedMS, ComposedMS: composedMS, ValidationMS: validationMS, TotalMS: totalMS, Concurrency: focusedConcurrency, Cases: make([]CaseTiming, 0, len(report.Cases))}
	for _, result := range report.Cases {
		summary.Cases = append(summary.Cases, CaseTiming{ID: result.ID, DurationMS: result.DurationMS})
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		return nil, errInvalidReport
	}
	return append(encoded, '\n'), nil
}

func NewReport() Report {
	return Report{SchemaVersion: ReportSchemaVersion}
}

func (r *Report) RecordPass(testCase Case, durationMS int64) error {
	if r == nil || durationMS <= 0 || testCase.ID == "" || len(testCase.Evidence) == 0 {
		return errInvalidReport
	}
	for _, result := range r.Cases {
		if result.ID == testCase.ID {
			return errInvalidReport
		}
	}
	evidence := make([]EvidenceResult, 0, len(testCase.Evidence))
	for _, key := range testCase.Evidence {
		evidence = append(evidence, EvidenceResult{Key: key, Passed: true, Count: 1})
	}
	r.Cases = append(r.Cases, CaseResult{
		ID: testCase.ID, Status: StatusPass, Evidence: evidence,
		DurationMS: durationMS, FailureCode: FailureNone,
	})
	return nil
}

func ValidateReport(contract Contract, report Report) error {
	if ValidateContract(contract) != nil || report.SchemaVersion != ReportSchemaVersion || len(report.Cases) != len(contract.Cases) {
		return errInvalidReport
	}
	seen := make(map[string]struct{}, len(report.Cases))
	for index, result := range report.Cases {
		testCase := contract.Cases[index]
		if result.ID != testCase.ID || result.Status != StatusPass || result.DurationMS <= 0 || result.FailureCode != FailureNone || len(result.Evidence) != len(testCase.Evidence) {
			return errInvalidReport
		}
		if _, duplicate := seen[result.ID]; duplicate {
			return errInvalidReport
		}
		seen[result.ID] = struct{}{}
		for evidenceIndex, evidence := range result.Evidence {
			if evidence.Key != testCase.Evidence[evidenceIndex] || !evidence.Passed || evidence.Count == 0 {
				return errInvalidReport
			}
		}
	}
	return nil
}

func MarshalReport(report Report) ([]byte, error) {
	if report.SchemaVersion != ReportSchemaVersion {
		return nil, errInvalidReport
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return nil, errInvalidReport
	}
	return append(encoded, '\n'), nil
}

func ParseReport(reader io.Reader) (Report, error) {
	data, strictErr := readStrictJSON(reader)
	if strictErr != nil {
		return Report{}, errInvalidReport
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var report Report
	if err := decoder.Decode(&report); err != nil {
		return Report{}, errInvalidReport
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Report{}, errInvalidReport
	}
	if report.SchemaVersion != ReportSchemaVersion {
		return Report{}, errInvalidReport
	}
	return report, nil
}

func WriteReportAtomic(path string, contract Contract, report Report) error {
	if path == "" || ValidateReport(contract, report) != nil {
		return errInvalidReport
	}
	encoded, err := MarshalReport(report)
	if err != nil {
		return errInvalidReport
	}
	return writePrivateAtomic(path, encoded)
}

func WriteReportProgressAtomic(path string, contract Contract, report Report) error {
	if path == "" || ValidateContract(contract) != nil || report.SchemaVersion != ReportSchemaVersion || len(report.Cases) == 0 || len(report.Cases) >= len(contract.Cases) {
		return errInvalidReport
	}
	for index, result := range report.Cases {
		expected := contract.Cases[index]
		if result.ID != expected.ID || result.Status != StatusPass || result.DurationMS <= 0 || result.FailureCode != FailureNone || len(result.Evidence) != len(expected.Evidence) {
			return errInvalidReport
		}
		for evidenceIndex, evidence := range result.Evidence {
			if evidence.Key != expected.Evidence[evidenceIndex] || !evidence.Passed || evidence.Count == 0 {
				return errInvalidReport
			}
		}
	}
	encoded, err := MarshalReport(report)
	if err != nil {
		return errInvalidReport
	}
	return writePrivateAtomic(path, encoded)
}

func writePrivateAtomic(path string, encoded []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".accelerator-acceptance-report-*")
	if err != nil {
		return errInvalidReport
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(0o600); err != nil {
		return errInvalidReport
	}
	if _, err = temporary.Write(encoded); err != nil {
		return errInvalidReport
	}
	if err = temporary.Sync(); err != nil {
		return errInvalidReport
	}
	if err = temporary.Close(); err != nil {
		return errInvalidReport
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return errInvalidReport
	}
	committed = true
	return nil
}
