package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
