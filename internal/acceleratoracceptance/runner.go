package acceleratoracceptance

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CommandExecutor interface {
	Check(context.Context, string, OwnerKind) FailureCode
	Run(context.Context, string, []string, []string) FailureCode
}

type CaseAuditor interface {
	Audit(context.Context, Case, string) FailureCode
}

type Runner struct {
	Executor    CommandExecutor
	Auditor     CaseAuditor
	MakePath    string
	NPMPath     string
	Environment []string
	Timeout     time.Duration
}

type RunFailure struct{ code FailureCode }

func (f RunFailure) Error() string { return "acceptance runner failed: " + string(f.code) }

func FailureCodeOf(err error) FailureCode {
	if err == nil {
		return FailureNone
	}
	var failure RunFailure
	if errors.As(err, &failure) {
		return failure.code
	}
	return FailureCommand
}

func (r Runner) RunFocused(ctx context.Context, contract Contract, report *Report) error {
	if ctx == nil || ValidateContract(contract) != nil || report == nil || report.SchemaVersion != ReportSchemaVersion || len(report.Cases) != 0 || r.Executor == nil || r.Auditor == nil || r.MakePath == "" || r.NPMPath == "" || r.Timeout <= 0 || !validRunnerEnvironment(r.Environment) {
		return RunFailure{code: FailurePreflight}
	}
	if code := normalizeFailure(r.Executor.Check(ctx, r.MakePath, OwnerMake), FailurePreflight); code != FailureNone {
		return RunFailure{code: code}
	}
	if code := normalizeFailure(r.Executor.Check(ctx, r.NPMPath, OwnerNPM), FailurePreflight); code != FailureNone {
		return RunFailure{code: code}
	}
	childEnvironment := append(append([]string(nil), r.Environment...), "KUBIKLES_ACCELERATOR_E2E_ACTIVE=1")
	byID := make(map[string]Case, len(contract.Cases))
	for _, testCase := range contract.Cases {
		byID[testCase.ID] = testCase
	}
	type outcome struct {
		testCase  Case
		namespace string
		started   time.Time
		code      FailureCode
		duration  int64
	}
	results := make(map[string]outcome, len(contract.Cases))
	for _, wave := range focusedWaves {
		waveContext, cancelWave := context.WithCancel(ctx)
		var wg sync.WaitGroup
		var mu sync.Mutex
		failure := FailureNone
		for _, id := range wave {
			testCase := byID[id]
			caseEnvironment, namespace, ok := runnerCaseEnvironment(childEnvironment, testCase)
			if !ok {
				cancelWave()
				wg.Wait()
				return RunFailure{code: FailurePreflight}
			}
			path, arguments := r.MakePath, []string{"--", testCase.Owner.Target}
			if testCase.Owner.Kind == OwnerNPM {
				path, arguments = r.NPMPath, []string{"--prefix", "frontend", "run", testCase.Owner.Script}
			}
			wg.Add(1)
			go func(testCase Case, path string, arguments, environment []string, namespace string) {
				defer wg.Done()
				started := time.Now()
				commandContext, commandCancel := context.WithTimeout(waveContext, r.Timeout)
				code := r.Executor.Run(commandContext, path, arguments, environment)
				if commandContext.Err() != nil {
					if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
						code = FailureTimeout
					} else {
						code = FailureSignal
					}
				}
				commandCancel()
				code = normalizeFailure(code, FailureCommand)
				mu.Lock()
				results[testCase.ID] = outcome{testCase: testCase, namespace: namespace, started: started, code: code}
				if code != FailureNone && failure == FailureNone {
					failure = code
					cancelWave()
				}
				mu.Unlock()
			}(testCase, path, arguments, caseEnvironment, namespace)
		}
		wg.Wait()
		cancelWave()
		// Audits deliberately use a fresh context after cancellation. They prove
		// cleanup rather than inheriting the cancellation that triggered it.
		for _, id := range wave {
			result := results[id]
			auditContext, auditCancel := context.WithTimeout(context.Background(), r.Timeout)
			auditCode := r.Auditor.Audit(auditContext, result.testCase, result.namespace)
			if auditContext.Err() != nil {
				auditCode = FailureTimeout
			}
			auditCancel()
			auditCode = normalizeFailure(auditCode, FailureAudit)
			result.duration = time.Since(result.started).Milliseconds()
			if result.duration < 1 {
				result.duration = 1
			}
			results[id] = result
			if auditCode != FailureNone {
				failure = FailureResidue
			}
		}
		if failure != FailureNone {
			return RunFailure{code: failure}
		}
	}
	// Evidence order is contract order, never completion order. No pass rows are
	// made visible until every focused owner and audit has succeeded.
	for _, testCase := range contract.Cases {
		if testCase.Owner.Kind == OwnerGo {
			continue
		}
		result, ok := results[testCase.ID]
		if !ok || result.code != FailureNone || report.RecordPass(testCase, result.duration) != nil {
			return RunFailure{code: FailureReport}
		}
	}
	return nil
}

func runnerCaseEnvironment(environment []string, testCase Case) ([]string, string, bool) {
	namespaceIndex := -1
	base := ""
	for index, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && key == "KUBIKLES_ACCELERATOR_E2E_NAMESPACE" {
			namespaceIndex, base = index, value
			break
		}
	}
	if namespaceIndex < 0 || !idPattern.MatchString(testCase.ID) {
		return nil, "", false
	}
	parts := strings.SplitN(testCase.ID, "-", 3)
	if len(parts) != 3 {
		return nil, "", false
	}
	namespace := strings.ToLower(base + "-" + parts[1] + "-" + parts[2])
	if len(namespace) > 63 || !targetPattern.MatchString(namespace) {
		return nil, "", false
	}
	result := append([]string(nil), environment...)
	result[namespaceIndex] = "KUBIKLES_ACCELERATOR_E2E_NAMESPACE=" + namespace
	result = append(result, "ACCELERATOR_IMAGE=kubikles-accelerator-e2e:"+namespace)
	return result, namespace, true
}

func normalizeFailure(code, fallback FailureCode) FailureCode {
	switch code {
	case FailureNone, FailureContract, FailurePreflight, FailureCommand, FailureAudit, FailureTimeout, FailureSignal, FailureResidue, FailureReport:
		return code
	default:
		return fallback
	}
}

var runnerRequiredEnvironment = map[string]string{
	"BUILD_VERSION":                                    BuildIdentity,
	"KUBIKLES_ACCELERATOR_E2E_REUSE":                   "1",
	"KUBIKLES_ACCELERATOR_E2E_KIND_NAME":               "",
	"KUBIKLES_ACCELERATOR_E2E_KUBECONFIG":              "",
	"KUBIKLES_ACCELERATOR_E2E_NAMESPACE":               "",
	"KUBIKLES_ACCELERATOR_E2E_REGISTRY":                "",
	"KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE":  "",
	"ACCELERATOR_E2E_OFFLINE":                          "1",
	"ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT":             "",
	"ACCELERATOR_IMAGE_REPOSITORY":                     "",
	"ACCELERATOR_IMAGE_DIGEST":                         "",
	"ACCELERATOR_IMAGE_VERSION":                        BuildIdentity,
	"KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS": "",
}

func validRunnerEnvironment(environment []string) bool {
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			return false
		}
		if key == "KUBIKLES_ACCELERATOR_E2E_ACTIVE" {
			return false
		}
		if _, duplicate := values[key]; duplicate {
			return false
		}
		values[key] = value
	}
	for key, exact := range runnerRequiredEnvironment {
		value, present := values[key]
		if !present || value == "" || (exact != "" && value != exact) {
			return false
		}
	}
	grace, err := strconv.Atoi(values["KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS"])
	if err != nil || grace < 1 || grace > 30 || values["KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS"] != strconv.Itoa(grace) {
		return false
	}
	if !validExecutionArchitecture(values["KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE"]) {
		return false
	}
	return true
}

// ExecCommandExecutor streams already-safe focused target output directly and
// never retains raw command errors in the acceptance report.
type ExecCommandExecutor struct {
	Directory string
	Stdout    io.Writer
	Stderr    io.Writer
}

func (e ExecCommandExecutor) Check(ctx context.Context, path string, _ OwnerKind) FailureCode {
	if ctx == nil || path == "" {
		return FailurePreflight
	}
	command := exec.CommandContext(ctx, path, "--version")
	command.Dir = e.Directory
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if command.Run() != nil {
		return FailurePreflight
	}
	return FailureNone
}

func (e ExecCommandExecutor) Run(ctx context.Context, path string, arguments, environment []string) FailureCode {
	if ctx == nil || path == "" || len(arguments) == 0 {
		return FailureCommand
	}
	command := exec.Command(path, arguments...)
	command.Dir = e.Directory
	command.Env = append(os.Environ(), environment...)
	command.Stdout = e.Stdout
	command.Stderr = e.Stderr
	if command.Stdout == nil {
		command.Stdout = os.Stdout
	}
	if command.Stderr == nil {
		command.Stderr = os.Stderr
	}
	setCommandProcessGroup(command)
	if command.Start() != nil {
		return FailureCommand
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case <-ctx.Done():
		terminateCommandProcessGroup(command)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			killCommandProcessGroup(command)
			<-done
		}
		return FailureTimeout
	case err := <-done:
		if err != nil {
			return FailureCommand
		}
	}
	return FailureNone
}
