package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateAll(root string) error {
	c, err := loadContract(root)
	if err != nil {
		return err
	}
	if err = validateMarkdown(root, c); err != nil {
		return err
	}
	if err = validateClaims(root, c); err != nil {
		return err
	}
	m, records, err := loadEvidence(root)
	if err != nil {
		return err
	}
	want := renderEvidence(m, records)
	got, err := os.ReadFile(filepath.Join(root, "docs/accelerator/evidence.md"))
	if err != nil || string(got) != string(want) {
		return errors.New("DOC-EVIDENCE-RENDER")
	}
	return validateAuthorities(root, c)
}

func validateClaims(root string, c contract) error {
	for _, rel := range c.Pages {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if os.IsNotExist(err) && rel == "docs/accelerator/evidence.md" {
			continue
		}
		if err != nil {
			return err
		}
		s := string(b)
		if !strings.Contains(s, "Kubikles Accelerator") {
			return fmt.Errorf("DOC-NAME-FIRST-USE: %s", rel)
		}
		for _, bad := range []string{"TODO", "TBD", "FIXME", "N-1 support", "backward compatible", "user-selectable RBAC"} {
			if strings.Contains(s, bad) {
				return fmt.Errorf("DOC-FORBIDDEN-CLAIM: %s", rel)
			}
		}
		lower := strings.ToLower(s)
		for _, unsafe := range []string{"kubectl get secret ", "kubectl get secrets ", "kubectl exec ", "`helm uninstall ", ":latest"} {
			if strings.Contains(lower, unsafe) {
				return fmt.Errorf("DOC-UNSAFE-EXAMPLE: %s", rel)
			}
		}
	}
	dev, _ := os.ReadFile(filepath.Join(root, "docs/accelerator/development.md"))
	for _, op := range c.Operations {
		if strings.Count(string(dev), "`"+op.Method+"`") != 1 {
			return errors.New("DOC-OPERATION-COVERAGE")
		}
	}
	ops, _ := os.ReadFile(filepath.Join(root, "docs/accelerator/operations.md"))
	for _, id := range c.TroubleshootingIDs {
		if !strings.Contains(string(ops), "`"+id+"`") {
			return errors.New("DOC-TROUBLESHOOTING-COVERAGE")
		}
	}
	return nil
}

func validateAuthorities(root string, c contract) error {
	checks := map[string][]string{
		"pkg/agent/policy.go":                                   {"ListSecretsMetadata", "GetSecretData", "GetSecretYaml", "CancelListRequest", "SubscribeSecretWatcher", "UnsubscribeSecretWatcher", "core/v1/secrets:get", "core/v1/secrets:list", "core/v1/secrets:watch"},
		"pkg/agent/lifecycle.go":                                {"2 * time.Minute"},
		"accelerator_secret_router.go":                          {"integratedSecretRouter"},
		"app_integrated_secrets.go":                             {"RetainIntegratedSecretReads", "ReleaseIntegratedSecretReads"},
		"pkg/server/accelerator_rpc.go":                         {"DecodeCall", "LookupMethodPolicy", "Authorize"},
		"deploy/charts/kubikles-accelerator/templates/job.yaml": {"ttlSecondsAfterFinished: 3600", "terminationGracePeriodSeconds: 30", "cpu: 100m", "memory: 128Mi", "cpu: \"1\"", "memory: 512Mi"},
		"release/accelerator-release.schema.json":               {"ghcr.io/skrobylabs/kubikles-accelerator", "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator", "exact-build-version"},
	}
	for rel, need := range checks {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return fmt.Errorf("DOC-AUTHORITY-MISSING: %s", rel)
		}
		for _, s := range need {
			if !strings.Contains(string(b), s) {
				return fmt.Errorf("DOC-AUTHORITY-DRIFT: %s", rel)
			}
		}
	}
	return nil
}
