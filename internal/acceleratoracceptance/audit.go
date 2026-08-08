package acceleratoracceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

const acceleratorClusterScopeSelector = "app.kubernetes.io/name=kubikles-accelerator,app.kubernetes.io/component=accelerator,app.kubernetes.io/part-of=kubikles,app.kubernetes.io/managed-by=Helm"

type FixtureAuditor struct {
	KindPath     string
	KubectlPath  string
	CurlPath     string
	KindName     string
	Kubeconfig   string
	Registry     string
	SentinelName string
}

func (a FixtureAuditor) Audit(ctx context.Context, _ Case, namespace string) FailureCode {
	if ctx == nil || a.KindPath == "" || a.KubectlPath == "" || a.CurlPath == "" || a.KindName == "" || a.Kubeconfig == "" || a.Registry == "" || a.SentinelName == "" || namespace == "" {
		return FailureAudit
	}
	clusters, ok := runAuditCommand(ctx, a.KindPath, "get", "clusters")
	if !ok || !linePresent(clusters, a.KindName) {
		return FailureResidue
	}
	if _, exists := runAuditCommand(ctx, a.KubectlPath, "--kubeconfig", a.Kubeconfig, "get", "namespace", namespace, "-o", "name"); exists {
		return FailureResidue
	}
	clusterScopeClear := false
	for attempt := 0; attempt < 100; attempt++ {
		objects, listed := runAuditCommand(ctx, a.KubectlPath, "--kubeconfig", a.Kubeconfig, "get", "clusterroles,clusterrolebindings", "-l", acceleratorClusterScopeSelector, "-o", "json")
		if listed && acceleratorClusterScopeReleased(objects, namespace) {
			clusterScopeClear = true
			break
		}
		select {
		case <-ctx.Done():
			return FailureResidue
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !clusterScopeClear {
		return FailureResidue
	}
	ready, ok := runAuditCommand(ctx, a.KubectlPath, "--kubeconfig", a.Kubeconfig, "--request-timeout=2s", "get", "--raw=/readyz")
	if !ok || strings.TrimSpace(string(ready)) != "ok" {
		return FailureResidue
	}
	if _, ok = runAuditCommand(ctx, a.KubectlPath, "--kubeconfig", a.Kubeconfig, "get", "namespace", a.SentinelName, "-o", "name"); !ok {
		return FailureResidue
	}
	if _, ok = runAuditCommand(ctx, a.CurlPath, "--noproxy", "*", "--fail", "--silent", "--show-error", "http://"+a.Registry+"/v2/"); !ok {
		return FailureResidue
	}
	return FailureNone
}

func acceleratorClusterScopeReleased(encoded []byte, namespace string, releaseName ...string) bool {
	if namespace == "" {
		return false
	}
	if len(releaseName) > 1 || (len(releaseName) == 1 && releaseName[0] == "") {
		return false
	}
	var list struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Items      []struct {
			Metadata struct {
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if json.Unmarshal(encoded, &list) != nil || list.APIVersion != "v1" || list.Kind != "List" || list.Items == nil {
		return false
	}
	for _, item := range list.Items {
		if item.Metadata.Annotations["meta.helm.sh/release-namespace"] != namespace {
			continue
		}
		if len(releaseName) == 0 || item.Metadata.Annotations["meta.helm.sh/release-name"] == releaseName[0] {
			return false
		}
	}
	return true
}

func runAuditCommand(ctx context.Context, path string, arguments ...string) ([]byte, bool) {
	command := exec.CommandContext(ctx, path, arguments...)
	command.Stderr = nil
	output, err := command.Output()
	if err != nil || len(output) > 1<<20 {
		return nil, false
	}
	return output, true
}

func linePresent(output []byte, exact string) bool {
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		if string(line) == exact {
			return true
		}
	}
	return false
}
