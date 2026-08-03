package acceleratoracceptance

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
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
	for _, testCase := range contract.Cases {
		if testCase.Owner.Kind == OwnerGo {
			continue
		}
		path, arguments := r.MakePath, []string{"--", testCase.Owner.Target}
		if testCase.Owner.Kind == OwnerNPM {
			path = r.NPMPath
			arguments = []string{"--prefix", "frontend", "run", testCase.Owner.Script}
		}
		caseEnvironment, namespace, ok := runnerCaseEnvironment(childEnvironment, testCase)
		if !ok {
			return RunFailure{code: FailurePreflight}
		}
		commandContext, cancel := context.WithTimeout(ctx, r.Timeout)
		started := time.Now()
		code := r.Executor.Run(commandContext, path, arguments, caseEnvironment)
		if commandContext.Err() != nil {
			code = FailureTimeout
		}
		cancel()
		if code = normalizeFailure(code, FailureCommand); code != FailureNone {
			return RunFailure{code: code}
		}
		auditContext, auditCancel := context.WithTimeout(ctx, r.Timeout)
		code = r.Auditor.Audit(auditContext, testCase, namespace)
		if auditContext.Err() != nil {
			code = FailureTimeout
		}
		auditCancel()
		if code = normalizeFailure(code, FailureAudit); code != FailureNone {
			return RunFailure{code: code}
		}
		durationMS := time.Since(started).Milliseconds()
		if durationMS < 1 {
			durationMS = 1
		}
		if err := report.RecordPass(testCase, durationMS); err != nil {
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
	"BUILD_VERSION":                        BuildIdentity,
	"KUBIKLES_ACCELERATOR_E2E_REUSE":       "1",
	"KUBIKLES_ACCELERATOR_E2E_KIND_NAME":   "",
	"KUBIKLES_ACCELERATOR_E2E_KUBECONFIG":  "",
	"KUBIKLES_ACCELERATOR_E2E_NAMESPACE":   "",
	"KUBIKLES_ACCELERATOR_E2E_REGISTRY":    "",
	"ACCELERATOR_E2E_OFFLINE":              "1",
	"ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT": "",
	"ACCELERATOR_IMAGE_REPOSITORY":         "",
	"ACCELERATOR_IMAGE_DIGEST":             "",
	"ACCELERATOR_IMAGE_VERSION":            BuildIdentity,
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
