package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"kubikles/internal/acceleratoracceptance"
)

func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "accelerator-e2e-runner: failed")
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "validate-report" {
		validationStarted := time.Now()
		contract, err := acceleratoracceptance.LoadContract(os.Getenv("ACCELERATOR_ACCEPTANCE_CONTRACT"))
		if err != nil {
			return fmt.Errorf("contract")
		}
		reportFile, err := os.Open(os.Getenv("ACCELERATOR_ACCEPTANCE_FINAL_REPORT"))
		if err != nil {
			return fmt.Errorf("report")
		}
		report, parseErr := acceleratoracceptance.ParseReport(reportFile)
		closeErr := reportFile.Close()
		if parseErr != nil || closeErr != nil || acceleratoracceptance.ValidateReport(contract, report) != nil {
			return fmt.Errorf("report")
		}
		values := make([]int64, 0, 6)
		for _, key := range []string{"ACCELERATOR_ACCEPTANCE_FIXTURE_MS", "ACCELERATOR_ACCEPTANCE_ARTIFACT_MS", "ACCELERATOR_ACCEPTANCE_FOCUSED_MS", "ACCELERATOR_ACCEPTANCE_COMPOSED_MS", "ACCELERATOR_ACCEPTANCE_VALIDATION_MS", "ACCELERATOR_ACCEPTANCE_TOTAL_MS"} {
			value, conversionErr := strconv.ParseInt(os.Getenv(key), 10, 64)
			if conversionErr != nil || value < 0 || strings.TrimSpace(os.Getenv(key)) == "" {
				return fmt.Errorf("timing")
			}
			values = append(values, value)
		}
		values[4] = time.Since(validationStarted).Milliseconds()
		values[5] += values[4]
		summary, summaryErr := acceleratoracceptance.MarshalTimingSummary(contract, report, values[0], values[1], values[2], values[3], values[4], values[5])
		if summaryErr != nil {
			return fmt.Errorf("timing")
		}
		fmt.Fprint(os.Stderr, "accelerator-e2e: timing "+string(summary))
		return nil
	}
	if len(os.Args) != 1 {
		return fmt.Errorf("arguments")
	}
	contractPath := os.Getenv("ACCELERATOR_ACCEPTANCE_CONTRACT")
	reportPath := os.Getenv("ACCELERATOR_ACCEPTANCE_PROGRESS_REPORT")
	root := os.Getenv("ACCELERATOR_ACCEPTANCE_ROOT")
	timeoutSeconds, err := strconv.Atoi(os.Getenv("ACCELERATOR_ACCEPTANCE_TARGET_TIMEOUT_SECONDS"))
	if contractPath == "" || reportPath == "" || root == "" || timeoutSeconds < 60 || timeoutSeconds > 3600 || err != nil {
		return fmt.Errorf("preflight")
	}
	contract, err := acceleratoracceptance.LoadContract(contractPath)
	if err != nil {
		return fmt.Errorf("contract")
	}
	makePath, err := exec.LookPath("make")
	if err != nil {
		return fmt.Errorf("make")
	}
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm")
	}
	kindPath, err := exec.LookPath("kind")
	if err != nil {
		return fmt.Errorf("kind")
	}
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("kubectl")
	}
	curlPath, err := exec.LookPath("curl")
	if err != nil {
		return fmt.Errorf("curl")
	}
	environment := []string{
		"BUILD_VERSION=" + os.Getenv("BUILD_VERSION"),
		"KUBIKLES_ACCELERATOR_E2E_REUSE=" + os.Getenv("KUBIKLES_ACCELERATOR_E2E_REUSE"),
		"KUBIKLES_ACCELERATOR_E2E_KIND_NAME=" + os.Getenv("KUBIKLES_ACCELERATOR_E2E_KIND_NAME"),
		"KUBIKLES_ACCELERATOR_E2E_KUBECONFIG=" + os.Getenv("KUBIKLES_ACCELERATOR_E2E_KUBECONFIG"),
		"KUBIKLES_ACCELERATOR_E2E_NAMESPACE=" + os.Getenv("KUBIKLES_ACCELERATOR_E2E_NAMESPACE"),
		"KUBIKLES_ACCELERATOR_E2E_REGISTRY=" + os.Getenv("KUBIKLES_ACCELERATOR_E2E_REGISTRY"),
		"ACCELERATOR_E2E_OFFLINE=" + os.Getenv("ACCELERATOR_E2E_OFFLINE"),
		"ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT=" + os.Getenv("ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT"),
		"ACCELERATOR_IMAGE_REPOSITORY=" + os.Getenv("ACCELERATOR_IMAGE_REPOSITORY"),
		"ACCELERATOR_IMAGE_DIGEST=" + os.Getenv("ACCELERATOR_IMAGE_DIGEST"),
		"ACCELERATOR_IMAGE_VERSION=" + os.Getenv("ACCELERATOR_IMAGE_VERSION"),
	}
	auditor := acceleratoracceptance.FixtureAuditor{
		KindPath: kindPath, KubectlPath: kubectlPath, CurlPath: curlPath,
		KindName:     os.Getenv("KUBIKLES_ACCELERATOR_E2E_KIND_NAME"),
		Kubeconfig:   os.Getenv("KUBIKLES_ACCELERATOR_E2E_KUBECONFIG"),
		Registry:     os.Getenv("KUBIKLES_ACCELERATOR_E2E_REGISTRY"),
		SentinelName: "kubikles-a60a-sentinel",
	}
	runner := acceleratoracceptance.Runner{
		Executor: acceleratoracceptance.ExecCommandExecutor{Directory: root},
		Auditor:  auditor, MakePath: makePath, NPMPath: npmPath,
		Environment: environment, Timeout: time.Duration(timeoutSeconds) * time.Second,
	}
	report := acceleratoracceptance.NewReport()
	if err = runner.RunFocused(context.Background(), contract, &report); err != nil {
		return fmt.Errorf("focused")
	}
	if err = acceleratoracceptance.WriteReportProgressAtomic(reportPath, contract, report); err != nil {
		return fmt.Errorf("report")
	}
	return nil
}
