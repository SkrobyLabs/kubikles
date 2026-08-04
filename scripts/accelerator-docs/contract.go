package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
)

type contract struct {
	SchemaVersion      int               `json:"schemaVersion"`
	Product            productContract   `json:"product"`
	Operations         []operation       `json:"operations"`
	DurationsSeconds   map[string]int    `json:"durationsSeconds"`
	Resources          map[string]string `json:"resources"`
	RBACVerbs          []string          `json:"rbacVerbs"`
	ChartObjects       []string          `json:"chartObjects"`
	Artifacts          artifactContract  `json:"artifacts"`
	Commands           []string          `json:"commands"`
	TroubleshootingIDs []string          `json:"troubleshootingIDs"`
	Pages              []string          `json:"pages"`
	AuthorityInputs    []string          `json:"authorityInputs"`
	Plans              []planContract    `json:"plans"`
}

type productContract struct {
	FormalName     string   `json:"formalName"`
	ShortName      string   `json:"shortName"`
	Modes          []string `json:"modes"`
	ReadCategories []string `json:"readCategories"`
}

type operation struct {
	Method     string `json:"method"`
	Capability string `json:"capability"`
	Action     string `json:"action"`
}

type artifactContract struct {
	ImageRepository    string   `json:"imageRepository"`
	ChartRepository    string   `json:"chartRepository"`
	Descriptor         string   `json:"descriptor"`
	DescriptorChecksum string   `json:"descriptorChecksum"`
	SBOMs              []string `json:"sboms"`
	Attestations       []string `json:"attestations"`
}

type planContract struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Group  string `json:"group"`
	Target string `json:"target"`
}

var expectedMethods = []string{"CancelListRequest", "GetSecretData", "GetSecretYaml", "ListSecretsMetadata", "SubscribeSecretWatcher", "UnsubscribeSecretWatcher"}
var expectedPlanIDs = []string{"020e8648", "1cec4c3a", "1ff2446c", "2289380c", "2d45b95b", "2e710408", "2fe9439f", "4026deaf", "44ea1f82", "53dfebec", "63140918", "66f24342", "70e5466d", "71facaab", "8ebf3759", "a02bce49", "ac978c3f", "b1854aa3", "b3ffc033", "d0eb9876", "d4f712fd", "dd19f7b6", "e590da1e", "e6c23dbd", "f256001c", "f2769f29", "f604c5fb"}
var expectedAuthorityInputs = []string{
	"Dockerfile.accelerator", "accelerator_lifecycle_contract.go", "accelerator_secret_router.go", "app_integrated_secrets.go", "deploy/charts/kubikles-accelerator/Chart.yaml", "deploy/charts/kubikles-accelerator/templates/clusterrole.yaml", "deploy/charts/kubikles-accelerator/templates/clusterrolebinding.yaml", "deploy/charts/kubikles-accelerator/templates/creator-verifier-secret.yaml", "deploy/charts/kubikles-accelerator/templates/job.yaml", "deploy/charts/kubikles-accelerator/templates/serviceaccount.yaml", "deploy/charts/kubikles-accelerator/values.schema.json", "internal/acceleratoracceptance/contract.go", "internal/acceleratoracceptance/report.go", "pkg/acceleratorprovision/coordinator.go", "pkg/acceleratorprovision/disposer.go", "pkg/acceleratorrelease/descriptor.go", "pkg/acceleratorsecret/contracts.go", "pkg/agent/call_context.go", "pkg/agent/lifecycle.go", "pkg/agent/policy.go", "pkg/agent/protocol.go", "pkg/k8s/accelerator_capabilities.go", "pkg/server/accelerator_idle.go", "pkg/server/accelerator_rpc.go", "release/accelerator-release.schema.json", "scripts/accelerator-supply-chain/attestation.go", "scripts/accelerator-supply-chain/evidence.go", "security/accelerator-toolchain.json", "security/accelerator-vulnerability-exceptions.schema.json", "test/accelerator/acceptance-v1.json",
}

func loadContract(root string) (contract, error) {
	b, err := os.ReadFile(root + "/docs/accelerator/contract-v1.json")
	if err != nil {
		return contract{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var c contract
	if err := dec.Decode(&c); err != nil {
		return contract{}, fmt.Errorf("DOC-CONTRACT-SCHEMA: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return contract{}, errors.New("DOC-CONTRACT-TRAILING")
	}
	if err := validateContract(c); err != nil {
		return contract{}, err
	}
	return c, nil
}

func validateContract(c contract) error {
	if c.SchemaVersion != 1 || c.Product.FormalName != "Kubikles Accelerator" || c.Product.ShortName != "Accelerator" {
		return errors.New("DOC-CONTRACT-IDENTITY")
	}
	if !equal(c.Product.Modes, []string{"Direct", "Integrated"}) || !equal(c.Product.ReadCategories, []string{"Secret detail", "Secret list", "Secret watch"}) {
		return errors.New("DOC-CONTRACT-MODES")
	}
	methods := make([]string, len(c.Operations))
	for i := range c.Operations {
		methods[i] = c.Operations[i].Method
	}
	if !equal(methods, expectedMethods) {
		return errors.New("DOC-CONTRACT-OPERATIONS")
	}
	wantActions := map[string]string{"CancelListRequest": "", "GetSecretData": "core/v1/secrets:get", "GetSecretYaml": "core/v1/secrets:get", "ListSecretsMetadata": "core/v1/secrets:list", "SubscribeSecretWatcher": "core/v1/secrets:watch", "UnsubscribeSecretWatcher": ""}
	for _, op := range c.Operations {
		if wantActions[op.Method] != op.Action {
			return errors.New("DOC-CONTRACT-ACTIONS")
		}
	}
	if !equal(c.RBACVerbs, []string{"get", "list", "watch"}) || !equal(c.ChartObjects, []string{"ClusterRole", "ClusterRoleBinding", "Job", "Secret", "ServiceAccount"}) {
		return errors.New("DOC-CONTRACT-RBAC")
	}
	wantDurations := map[string]int{"jobTerminationGrace": 30, "reconnectGrace": 120, "ttlAfterFinished": 3600}
	if !equalMap(c.DurationsSeconds, wantDurations) {
		return errors.New("DOC-CONTRACT-DURATIONS")
	}
	wantResources := map[string]string{"requestsCPU": "100m", "requestsMemory": "128Mi", "limitsCPU": "1", "limitsMemory": "512Mi"}
	if !equalMap(c.Resources, wantResources) {
		return errors.New("DOC-CONTRACT-RESOURCES")
	}
	ids := make([]string, len(c.Plans))
	for i, p := range c.Plans {
		ids[i] = p.ID
		if p.Title == "" || p.Group == "" || p.Target == "" {
			return errors.New("DOC-CONTRACT-PLAN")
		}
	}
	if !equal(ids, expectedPlanIDs) || !equal(c.AuthorityInputs, expectedAuthorityInputs) {
		return errors.New("DOC-CONTRACT-AUTHORITY")
	}
	for _, values := range [][]string{c.Commands, c.TroubleshootingIDs, c.Pages, c.Artifacts.SBOMs, c.Artifacts.Attestations} {
		if !sortedUnique(values) {
			return errors.New("DOC-CONTRACT-ORDER")
		}
	}
	return nil
}

func sortedUnique(v []string) bool {
	return sort.StringsAreSorted(v) && len(v) > 0 && func() bool {
		for i := 1; i < len(v); i++ {
			if v[i] == v[i-1] {
				return false
			}
		}
		return true
	}()
}
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func equalMap[K comparable, V comparable](a, b map[K]V) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
