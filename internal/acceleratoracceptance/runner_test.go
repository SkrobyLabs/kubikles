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

func (a *recordingAuditor) Audit(_ context.Context, testCase Case, namespace string) FailureCode {
	a.cases = append(a.cases, testCase.ID)
	a.namespaces = append(a.namespaces, namespace)
	if len(a.cases) == a.failAt {
		return FailureResidue
	}
	return FailureNone
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
		if len(executor.commands) != 13 || len(auditor.cases) != 13 || len(report.Cases) != 13 {
			t.Fatal("focused runner did not execute, audit, and record every focused owner exactly once")
		}
		for index, command := range executor.commands {
			testCase := contract.Cases[index]
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
		wantCalls int
		wantRows  int
		wantCode  FailureCode
		forbidden string
	}{
		{name: "missing tool", executor: &recordingExecutor{checkCode: FailurePreflight}, auditor: &recordingAuditor{}, wantCode: FailurePreflight},
		{name: "command stop", executor: &recordingExecutor{failAt: 3, failureCode: FailureCommand}, auditor: &recordingAuditor{}, wantCalls: 3, wantRows: 2, wantCode: FailureCommand},
		{name: "audit stop", executor: &recordingExecutor{}, auditor: &recordingAuditor{failAt: 2}, wantCalls: 2, wantRows: 1, wantCode: FailureResidue},
		{name: "timeout", executor: &recordingExecutor{waitAt: 1}, auditor: &recordingAuditor{}, wantCalls: 1, wantCode: FailureTimeout},
		{name: "unknown optional result", executor: &recordingExecutor{failAt: 1, failureCode: FailureCode("optional-private-marker")}, auditor: &recordingAuditor{}, wantCalls: 1, wantCode: FailureCommand, forbidden: "private-marker"},
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
			if FailureCodeOf(err) != test.wantCode || len(test.executor.commands) != test.wantCalls || len(report.Cases) != test.wantRows {
				t.Fatal("focused runner failure did not stop at the exact boundary")
			}
			if test.forbidden != "" && strings.Contains(err.Error(), test.forbidden) {
				t.Fatal("focused runner propagated unsafe raw failure material")
			}
		})
	}
}
