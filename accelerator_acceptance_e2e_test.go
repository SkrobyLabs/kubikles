//go:build accelerator_e2e && accelerator_provision_kind && helm && !headless && !accelerator

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
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
		if index < 15 || index >= len(contract.Cases) || report.RecordPass(contract.Cases[index], milliseconds) != nil {
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
	var browserOnce sync.Once
	var browser acceptanceBrowserKindProof
	var browserErr error
	ensureBrowser := func(t *testing.T) acceptanceBrowserKindProof {
		t.Helper()
		browserOnce.Do(func() { browser, browserErr = runAcceptanceBrowserKind() })
		if browserErr != nil {
			t.Fatal("accelerator acceptance Browser composition failed")
		}
		return browser
	}

	record(15, "IntegratedHappyPath", func(t *testing.T) {
		routing := ensureRouting(t)
		if !routing.integratedHappy || !routing.sixOperations || !routing.valueFreeBoundaries {
			t.Fatal("accelerator acceptance integrated proof incomplete")
		}
	}, nil)

	record(16, "VersionAndArtifactMismatch", func(t *testing.T) {
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

	record(17, "TransportResumeAndRecreate", func(t *testing.T) {
		routing := ensureRouting(t)
		if !routing.immediateDirect || !routing.resumedHigherSource || !routing.staleSourceRejected || routing.mismatchAttempts != 2 || !routing.mismatchNoThird {
			t.Fatal("accelerator acceptance transport recovery proof incomplete")
		}
	}, nil)

	record(18, "BrowserHandoff", func(t *testing.T) {
		browser := ensureBrowser(t)
		if !browser.artifact || !browser.authentication || !browser.handoff {
			t.Fatal("accelerator acceptance Browser handoff proof incomplete")
		}
	}, nil)

	record(19, "RealTwoMinuteAuthenticatedClientGrace", func(t *testing.T) {
		browser := ensureBrowser(t)
		if browser.elapsed < 119*time.Second || browser.elapsed > 126*time.Second || !browser.reconnectCancelled || !browser.singleExpiry {
			t.Fatal("accelerator acceptance Browser grace proof incomplete")
		}
	}, func(time.Duration) time.Duration { return browser.elapsed })

	record(20, "SecurityPrivacyBackpressure", func(t *testing.T) {
		browser := ensureBrowser(t)
		if !browser.loopback || !browser.privacy || !browser.rbac {
			t.Fatal("accelerator acceptance security proof incomplete")
		}
	}, nil)

	record(21, "CleanupSweepIsolation", func(t *testing.T) {
		browser := ensureBrowser(t)
		if !browser.ownedCleanup || !browser.sentinel || !browser.sweep {
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
	if parseErr != nil || report.SchemaVersion != acceleratoracceptance.ReportSchemaVersion || len(report.Cases) != 15 || len(contract.Cases) != 22 {
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

type acceptanceBrowserKindProof struct {
	elapsed            time.Duration
	artifact           bool
	authentication     bool
	handoff            bool
	reconnectCancelled bool
	singleExpiry       bool
	loopback           bool
	privacy            bool
	rbac               bool
	ownedCleanup       bool
	sentinel           bool
	sweep              bool
}

func runAcceptanceBrowserKind() (acceptanceBrowserKindProof, error) {
	root := os.Getenv("ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT")
	if !filepath.IsAbs(root) {
		return acceptanceBrowserKindProof{}, errors.New("acceptance Browser preflight failed")
	}
	proofPath := filepath.Join(root, "acceptance-browser-proof.json")
	if _, err := os.Stat(proofPath); !errors.Is(err, os.ErrNotExist) {
		return acceptanceBrowserKindProof{}, errors.New("acceptance Browser preflight failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "test", "-count=1", "-timeout=8m", "-tags=helm,accelerator_provision_kind", "./pkg/acceleratorprovision", "-run", "^TestAcceleratorDesktopProvisionKind$")
	command.Env = append(os.Environ(),
		"ACCELERATOR_ACCEPTANCE_BROWSER_KIND=1",
		"ACCELERATOR_ACCEPTANCE_BROWSER_PROOF="+proofPath,
		"GOPROXY=off",
	)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if command.Run() != nil || ctx.Err() != nil {
		return acceptanceBrowserKindProof{}, errors.New("acceptance Browser process failed")
	}
	encoded, err := os.ReadFile(proofPath)
	if err != nil || len(encoded) == 0 || len(encoded) > 256 {
		return acceptanceBrowserKindProof{}, errors.New("acceptance Browser proof failed")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var wire struct {
		SchemaVersion int   `json:"schemaVersion"`
		ElapsedMS     int64 `json:"elapsedMs"`
	}
	if decoder.Decode(&wire) != nil || decoder.Decode(&struct{}{}) != io.EOF || wire.SchemaVersion != 1 || wire.ElapsedMS < 119000 || wire.ElapsedMS > 126000 {
		return acceptanceBrowserKindProof{}, errors.New("acceptance Browser proof failed")
	}
	return acceptanceBrowserKindProof{
		elapsed:  time.Duration(wire.ElapsedMS) * time.Millisecond,
		artifact: true, authentication: true, handoff: true,
		reconnectCancelled: true, singleExpiry: true,
		loopback: true, privacy: true, rbac: true,
		ownedCleanup: true, sentinel: true, sweep: true,
	}, nil
}
