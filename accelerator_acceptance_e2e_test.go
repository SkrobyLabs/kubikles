//go:build accelerator_e2e && accelerator_provision_kind && helm && !headless && !accelerator

package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kubikles/internal/acceleratoracceptance"
	"kubikles/pkg/acceleratorprovision"
)

// TestAcceleratorAcceptanceKind is the single additive 60A composed harness.
// The prerequisite routing smoke runs once and returns only closed, value-free
// observations to the independently named acceptance cases below.
func TestAcceleratorAcceptanceKind(t *testing.T) {
	contract, report, finalReport := loadAcceptanceReportPrefix(t)
	allPassed := true
	record := func(index int, name string, run func(*testing.T), duration func(time.Duration) time.Duration) {
		t.Helper()
		started := time.Now()
		executed := false
		passed := t.Run(name, func(t *testing.T) {
			executed = true
			run(t)
		})
		elapsed := time.Since(started)
		if duration != nil {
			elapsed = duration(elapsed)
		}
		if !executed || !passed {
			allPassed = false
			return
		}
		milliseconds := elapsed.Milliseconds()
		if milliseconds < 1 {
			milliseconds = 1
		}
		if index < 12 || index >= len(contract.Cases) || report.RecordPass(contract.Cases[index], milliseconds) != nil {
			allPassed = false
			t.Error("accelerator acceptance composed report failed")
		}
	}
	var routingOnce sync.Once
	var routing integratedRoutingKindAcceptanceProof
	ensureRouting := func(t *testing.T) integratedRoutingKindAcceptanceProof {
		t.Helper()
		routingOnce.Do(func() { routing = runAcceleratorIntegratedRoutingKind(t) })
		return routing
	}
	record(12, "IntegratedHappyPath", func(t *testing.T) {
		routing := ensureRouting(t)
		if !routing.integratedHappy || !routing.sixOperations || !routing.valueFreeBoundaries {
			t.Fatal("accelerator acceptance integrated proof incomplete")
		}
	}, nil)

	record(13, "VersionAndArtifactMismatch", func(t *testing.T) {
		routing := ensureRouting(t)
		fixture, err := acceleratoracceptance.LoadArtifactFixture(os.Getenv("ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT"))
		if err != nil || acceleratoracceptance.ValidateArtifactFixture(fixture) != nil {
			t.Fatal("accelerator acceptance artifact proof incomplete")
		}
		literal, err := acceleratorprovision.ProveAcceptanceLiteralVersionMatrix()
		if err != nil || !literal.ExactAccepted || literal.AdjacentRejected != 2 || !literal.DirectBeforeCleanup || literal.OneReplacement != 2 || literal.RepeatedMismatch != 2 || !literal.NoThirdWorkload {
			t.Fatal("accelerator acceptance literal mismatch proof incomplete")
		}
		if !routing.mismatchDirect || routing.mismatchAttempts != 2 || !routing.mismatchNoThird {
			t.Fatal("accelerator acceptance stable artifact mismatch proof incomplete")
		}
	}, nil)

	record(14, "TransportResumeAndRecreate", func(t *testing.T) {
		routing := ensureRouting(t)
		if !routing.immediateDirect || !routing.resumedHigherSource || !routing.staleSourceRejected || routing.mismatchAttempts != 2 || !routing.mismatchNoThird {
			t.Fatal("accelerator acceptance transport recovery proof incomplete")
		}
	}, nil)

	record(15, "ConfiguredAuthenticatedClientGrace", func(t *testing.T) {
		routing := ensureRouting(t)
		lowerGrace, upperGrace := integratedRoutingKindGraceBounds()
		if routing.elapsed < lowerGrace || routing.elapsed > upperGrace || !routing.reconnectCancelled || !routing.singleExpiry {
			t.Fatal("accelerator acceptance configured authenticated client grace proof incomplete")
		}
	}, nil)

	record(16, "SecurityPrivacyBackpressure", func(t *testing.T) {
		routing := ensureRouting(t)
		if !routing.loopback || !routing.privacy || !routing.rbac {
			t.Fatal("accelerator acceptance security proof incomplete")
		}
	}, nil)

	record(17, "CleanupSweepIsolation", func(t *testing.T) {
		routing := ensureRouting(t)
		if !routing.ownedCleanup || !routing.sentinel || !routing.sweep {
			t.Fatal("accelerator acceptance cleanup proof incomplete")
		}
	}, nil)
	if allPassed && !t.Failed() {
		if acceleratoracceptance.WriteReportAtomic(finalReport, contract, report) != nil {
			t.Fatal("accelerator acceptance final report failed")
		}
	}
}

func loadAcceptanceReportPrefix(t *testing.T) (acceleratoracceptance.Contract, acceleratoracceptance.Report, string) {
	t.Helper()
	contract, err := acceleratoracceptance.LoadContract(os.Getenv("ACCELERATOR_ACCEPTANCE_CONTRACT"))
	if err != nil {
		t.Fatal("accelerator acceptance contract unavailable")
	}
	progress, err := os.Open(os.Getenv("ACCELERATOR_ACCEPTANCE_PROGRESS_REPORT"))
	if err != nil {
		t.Fatal("accelerator acceptance focused report unavailable")
	}
	report, parseErr := acceleratoracceptance.ParseReport(progress)
	_ = progress.Close()
	if parseErr != nil || report.SchemaVersion != acceleratoracceptance.ReportSchemaVersion || len(report.Cases) != 12 || len(contract.Cases) != 18 {
		t.Fatal("accelerator acceptance focused report invalid")
	}
	for index, result := range report.Cases {
		expected := contract.Cases[index]
		if result.ID != expected.ID || result.Status != acceleratoracceptance.StatusPass || result.DurationMS <= 0 || result.FailureCode != acceleratoracceptance.FailureNone || len(result.Evidence) != len(expected.Evidence) {
			t.Fatal("accelerator acceptance focused report invalid")
		}
		for evidenceIndex, evidence := range result.Evidence {
			if evidence.Key != expected.Evidence[evidenceIndex] || !evidence.Passed || evidence.Count == 0 {
				t.Fatal("accelerator acceptance focused report invalid")
			}
		}
	}
	finalReport := os.Getenv("ACCELERATOR_ACCEPTANCE_FINAL_REPORT")
	if !filepath.IsAbs(finalReport) {
		t.Fatal("accelerator acceptance final report path invalid")
	}
	if _, err = os.Lstat(finalReport); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("accelerator acceptance stale final report")
	}
	return contract, report, finalReport
}
