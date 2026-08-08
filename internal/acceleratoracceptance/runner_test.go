package acceleratoracceptance

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordedCommand struct {
	path string
	args []string
	env  []string
}

type recordingExecutor struct {
	mu          sync.Mutex
	checks      []string
	commands    []recordedCommand
	checkCode   FailureCode
	failAt      int
	failureCode FailureCode
	waitAt      int
}

func (e *recordingExecutor) Check(_ context.Context, path string, kind OwnerKind) FailureCode {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.checks = append(e.checks, string(kind)+":"+path)
	if e.checkCode == "" {
		return FailureNone
	}
	return e.checkCode
}

func (e *recordingExecutor) Run(ctx context.Context, path string, args, env []string) FailureCode {
	e.mu.Lock()
	e.commands = append(e.commands, recordedCommand{path: path, args: append([]string(nil), args...), env: append([]string(nil), env...)})
	index := len(e.commands)
	e.mu.Unlock()
	if e.waitAt == index {
		<-ctx.Done()
		return FailureTimeout
	}
	if e.failAt == index {
		return e.failureCode
	}
	return FailureNone
}

type recordingAuditor struct {
	cases      []string
	namespaces []string
	failAt     int
}

type overlappingExecutor struct {
	mu     sync.Mutex
	active int
	max    int
	calls  int
	delay  time.Duration
}

type failingPeerExecutor struct {
	mu    sync.Mutex
	calls int
}

func (e *failingPeerExecutor) Check(context.Context, string, OwnerKind) FailureCode {
	return FailureNone
}
func (e *failingPeerExecutor) Run(ctx context.Context, _ string, _ []string, _ []string) FailureCode {
	e.mu.Lock()
	e.calls++
	call := e.calls
	e.mu.Unlock()
	if call == 1 {
		return FailureCommand
	}
	<-ctx.Done()
	return FailureSignal
}

func (e *overlappingExecutor) Check(context.Context, string, OwnerKind) FailureCode {
	return FailureNone
}
func (e *overlappingExecutor) Run(ctx context.Context, _ string, _ []string, _ []string) FailureCode {
	e.mu.Lock()
	e.active++
	e.calls++
	if e.active > e.max {
		e.max = e.active
	}
	e.mu.Unlock()
	defer func() { e.mu.Lock(); e.active--; e.mu.Unlock() }()
	select {
	case <-time.After(e.delay):
		return FailureNone
	case <-ctx.Done():
		return FailureSignal
	}
}

func (a *recordingAuditor) Audit(_ context.Context, testCase Case, namespace string) FailureCode {
	a.cases = append(a.cases, testCase.ID)
	a.namespaces = append(a.namespaces, namespace)
	if len(a.cases) == a.failAt {
		return FailureResidue
	}
	return FailureNone
}

func TestAcceptanceFocusedRunnerUsesClosedTwoWideWaves(t *testing.T) {
	contract, err := LoadContract(acceptanceContractPath())
	if err != nil {
		t.Fatal("load contract")
	}
	executor := &overlappingExecutor{delay: 15 * time.Millisecond}
	runner := newTestRunner(executor, &recordingAuditor{})
	report := NewReport()
	started := time.Now()
	if err := runner.RunFocused(context.Background(), contract, &report); err != nil {
		t.Fatal("focused run")
	}
	elapsed := time.Since(started)
	executor.mu.Lock()
	max, calls := executor.max, executor.calls
	executor.mu.Unlock()
	if max != focusedConcurrency || calls != 12 {
		t.Fatalf("closed waves max=%d calls=%d", max, calls)
	}
	if elapsed >= time.Duration(calls)*executor.delay {
		t.Fatalf("focused waves did not overlap: elapsed=%s", elapsed)
	}
	if len(report.Cases) != 12 {
		t.Fatal("focused report cardinality")
	}
	for index, result := range report.Cases {
		if result.ID != contract.Cases[index].ID || result.DurationMS <= 0 {
			t.Fatal("focused report ordering or duration")
		}
	}
}

func TestAcceptanceFocusedRunnerCancelsPeerAndAuditsStartedCases(t *testing.T) {
	contract, err := LoadContract(acceptanceContractPath())
	if err != nil {
		t.Fatal("load contract")
	}
	executor := &failingPeerExecutor{}
	auditor := &recordingAuditor{}
	runner := newTestRunner(executor, auditor)
	report := NewReport()
	if code := FailureCodeOf(runner.RunFocused(context.Background(), contract, &report)); code != FailureCommand {
		t.Fatalf("failure code=%s", code)
	}
	executor.mu.Lock()
	calls := executor.calls
	executor.mu.Unlock()
	if calls != 2 || len(auditor.cases) != 2 || len(report.Cases) != 0 {
		t.Fatalf("calls=%d audits=%d rows=%d", calls, len(auditor.cases), len(report.Cases))
	}
}

func runnerEnvironment() []string {
	return []string{
		"BUILD_VERSION=v0.0.0",
		"KUBIKLES_ACCELERATOR_E2E_REUSE=1",
		"KUBIKLES_ACCELERATOR_E2E_KIND_NAME=owned-cluster",
		"KUBIKLES_ACCELERATOR_E2E_KUBECONFIG=/private/kubeconfig",
		"KUBIKLES_ACCELERATOR_E2E_NAMESPACE=owned-case",
		"KUBIKLES_ACCELERATOR_E2E_REGISTRY=127.0.0.1:5000",
		"ACCELERATOR_E2E_OFFLINE=1",
		"ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT=/private/artifacts",
		"ACCELERATOR_IMAGE_REPOSITORY=127.0.0.1:5000/skrobylabs/kubikles-accelerator",
		"ACCELERATOR_IMAGE_DIGEST=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"ACCELERATOR_IMAGE_VERSION=v0.0.0",
		"KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS=5",
	}
}

func newTestRunner(executor CommandExecutor, auditor CaseAuditor) Runner {
	return Runner{
		Executor: executor, Auditor: auditor,
		MakePath: "make-bin", NPMPath: "npm-bin",
		Environment: runnerEnvironment(), Timeout: 20 * time.Millisecond,
	}
}

func TestAcceptanceFocusedRunnerClosedOrderAndFailure(t *testing.T) {
	contract, err := LoadContract(acceptanceContractPath())
	if err != nil {
		t.Fatal("checked-in acceptance contract did not load")
	}
	t.Run("exact order", func(t *testing.T) {
		executor := &recordingExecutor{}
		auditor := &recordingAuditor{}
		runner := newTestRunner(executor, auditor)
		report := NewReport()
		if err := runner.RunFocused(context.Background(), contract, &report); err != nil {
			t.Fatal("exact focused run failed")
		}
		if !reflect.DeepEqual(executor.checks, []string{"make:make-bin", "npm:npm-bin"}) {
			t.Fatal("focused runner tool preflight drifted")
		}
		if len(executor.commands) != 12 || len(auditor.cases) != 12 || len(report.Cases) != 12 {
			t.Fatal("focused runner did not execute, audit, and record every focused owner exactly once")
		}
		commands := make(map[string]recordedCommand, len(executor.commands))
		for _, command := range executor.commands {
			for _, testCase := range contract.Cases {
				if testCase.Owner.Kind != OwnerGo && strings.Contains(strings.Join(command.env, "\n"), "KUBIKLES_ACCELERATOR_E2E_NAMESPACE=owned-case-"+strings.ToLower(strings.TrimPrefix(testCase.ID, "A60A-"))) {
					commands[testCase.ID] = command
				}
			}
		}
		for index, testCase := range contract.Cases[:12] {
			command, ok := commands[testCase.ID]
			if !ok {
				t.Fatalf("focused owner %s was not invoked exactly once", testCase.ID)
			}
			if testCase.Owner.Kind == OwnerMake {
				if command.path != "make-bin" || !reflect.DeepEqual(command.args, []string{"--", testCase.Owner.Target}) {
					t.Fatal("make owner argv drifted")
				}
			} else if testCase.Owner.Kind == OwnerNPM {
				if command.path != "npm-bin" || !reflect.DeepEqual(command.args, []string{"--prefix", "frontend", "run", testCase.Owner.Script}) {
					t.Fatal("npm owner argv drifted")
				}
			} else {
				t.Fatal("composed owner entered focused runner")
			}
			joined := strings.Join(command.env, "\n")
			if !strings.Contains(joined, "KUBIKLES_ACCELERATOR_E2E_ACTIVE=1") || strings.Contains(command.path, "sh") {
				t.Fatal("focused runner did not use direct recursion-guarded argv")
			}
			if !strings.Contains(joined, "KUBIKLES_ACCELERATOR_E2E_NAMESPACE="+auditor.namespaces[index]) || auditor.namespaces[index] == "owned-case" {
				t.Fatal("focused runner did not allocate an isolated case namespace")
			}
			if !strings.Contains(joined, "ACCELERATOR_IMAGE=kubikles-accelerator-e2e:"+auditor.namespaces[index]) {
				t.Fatal("focused runner did not allocate an owned image tag")
			}
			if report.Cases[index].ID != testCase.ID || report.Cases[index].Status != StatusPass {
				t.Fatal("focused pass was not recorded after its exact owner")
			}
		}
	})

	for _, test := range []struct {
		name      string
		executor  *recordingExecutor
		auditor   *recordingAuditor
		mutate    func(*Runner)
		wantCalls bool
		wantCode  FailureCode
		forbidden string
	}{
		{name: "missing tool", executor: &recordingExecutor{checkCode: FailurePreflight}, auditor: &recordingAuditor{}, wantCode: FailurePreflight},
		{name: "command stop", executor: &recordingExecutor{failAt: 3, failureCode: FailureCommand}, auditor: &recordingAuditor{}, wantCalls: true, wantCode: FailureCommand},
		{name: "audit stop", executor: &recordingExecutor{}, auditor: &recordingAuditor{failAt: 2}, wantCalls: true, wantCode: FailureResidue},
		{name: "timeout", executor: &recordingExecutor{waitAt: 1}, auditor: &recordingAuditor{}, wantCalls: true, wantCode: FailureTimeout},
		{name: "unknown optional result", executor: &recordingExecutor{failAt: 1, failureCode: FailureCode("optional-private-marker")}, auditor: &recordingAuditor{}, wantCalls: true, wantCode: FailureCommand, forbidden: "private-marker"},
		{name: "recursive", executor: &recordingExecutor{}, auditor: &recordingAuditor{}, mutate: func(r *Runner) { r.Environment = append(r.Environment, "KUBIKLES_ACCELERATOR_E2E_ACTIVE=1") }, wantCode: FailurePreflight},
		{name: "partial reuse", executor: &recordingExecutor{}, auditor: &recordingAuditor{}, mutate: func(r *Runner) { r.Environment = r.Environment[:2] }, wantCode: FailurePreflight},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := newTestRunner(test.executor, test.auditor)
			if test.mutate != nil {
				test.mutate(&runner)
			}
			report := NewReport()
			err := runner.RunFocused(context.Background(), contract, &report)
			if FailureCodeOf(err) != test.wantCode || (test.wantCalls && len(test.executor.commands) == 0) || len(report.Cases) != 0 {
				t.Fatal("focused runner failure did not stop at the exact boundary")
			}
			if test.forbidden != "" && strings.Contains(err.Error(), test.forbidden) {
				t.Fatal("focused runner propagated unsafe raw failure material")
			}
		})
	}
}
