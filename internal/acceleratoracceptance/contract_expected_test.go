package acceleratoracceptance

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type expectedCase struct {
	id            string
	prerequisites string
	kind          OwnerKind
	owner         string
	evidence      string
}

func acceptanceContractPath() string {
	return filepath.Join("..", "..", "test", "accelerator", "acceptance-v1.json")
}

func TestAcceptanceContractExactCaseSet(t *testing.T) {
	contract, err := LoadContract(acceptanceContractPath())
	if err != nil {
		t.Fatal("checked-in acceptance contract did not load")
	}
	expected := []expectedCase{
		{"A60A-001-BASELINE", "dd19f7b6", OwnerMake, "test-accelerator-00-kind", "baseline.direct,baseline.loopback,baseline.pod"},
		{"A60A-002-BINARY", "e6c23dbd", OwnerMake, "build-accelerator", "artifact.binary,artifact.version"},
		{"A60A-003-IMAGE", "e6c23dbd", OwnerMake, "test-accelerator-image", "artifact.image,artifact.platforms"},
		{"A60A-004-CHART", "a02bce49", OwnerMake, "test-accelerator-chart", "chart.objects,chart.rbac,chart.version"},
		{"A60A-005-CHART-KIND", "a02bce49", OwnerMake, "test-accelerator-chart-kind", "chart.job,chart.ttl"},
		{"A60A-006-RELEASE-CONTRACT", "1ff2446c", OwnerMake, "test-accelerator-release-contract", "release.checksum,release.descriptor,release.immutable"},
		{"A60A-007-LOCAL-PUBLICATION", "1ff2446c", OwnerMake, "test-accelerator-publication-local", "release.loopback,release.readback"},
		{"A60A-008-PROVISION", "1cec4c3a,71facaab", OwnerMake, "test-accelerator-desktop-provision-kind", "desktop.provision,desktop.resolve"},
		{"A60A-009-CONNECT", "020e8648,2d45b95b,44ea1f82,8ebf3759,f604c5fb", OwnerMake, "test-accelerator-desktop-connector-kind", "desktop.auth,desktop.connect,desktop.tunnel"},
		{"A60A-010-RESUME", "ac978c3f", OwnerMake, "test-accelerator-desktop-resume-kind", "desktop.generation,desktop.resume"},
		{"A60A-011-DISPOSAL", "70e5466d", OwnerMake, "test-accelerator-desktop-disposal-kind", "desktop.dispose,desktop.sweep"},
		{"A60A-012-LIFECYCLE", "2289380c,2e710408,63140918,66f24342,b1854aa3,b3ffc033,e590da1e", OwnerMake, "test-accelerator-desktop-lifecycle-kind", "desktop.coordinator,desktop.direct,desktop.explicit-enable,desktop.replacement"},
		{"A60A-016-INTEGRATED-HAPPY", "1cec4c3a,53dfebec,71facaab,b1854aa3,d0eb9876,d4f712fd", OwnerGo, "TestAcceleratorAcceptanceKind/IntegratedHappyPath", "composed.integrated,composed.operations,composed.value-free"},
		{"A60A-017-MISMATCH", "020e8648,1cec4c3a,66f24342,70e5466d,71facaab,b1854aa3", OwnerGo, "TestAcceleratorAcceptanceKind/VersionAndArtifactMismatch", "mismatch.direct,mismatch.literal,mismatch.one-replacement"},
		{"A60A-018-TRANSPORT-RESUME", "020e8648,ac978c3f,b1854aa3,d0eb9876", OwnerGo, "TestAcceleratorAcceptanceKind/TransportResumeAndRecreate", "recovery.direct,recovery.resume,recovery.stale-fence"},
		{"A60A-020-REAL-GRACE", "8ebf3759,b1854aa3", OwnerGo, "TestAcceleratorAcceptanceKind/RealTwoMinuteAuthenticatedClientGrace", "grace.cancelled,grace.elapsed,grace.single-expiry"},
		{"A60A-021-SECURITY-PRIVACY", "2d45b95b,44ea1f82,a02bce49,d0eb9876", OwnerGo, "TestAcceleratorAcceptanceKind/SecurityPrivacyBackpressure", "security.loopback,security.privacy,security.rbac"},
		{"A60A-022-CLEANUP-ISOLATION", "70e5466d,b1854aa3", OwnerGo, "TestAcceleratorAcceptanceKind/CleanupSweepIsolation", "cleanup.owned,cleanup.sentinel,cleanup.sweep"},
	}
	if contract.SchemaVersion != SchemaVersion || contract.Suite != SuiteIdentity || contract.BuildVersion != BuildIdentity || len(contract.Cases) != len(expected) {
		t.Fatal("acceptance contract identity or case cardinality drifted")
	}
	actual := make([]expectedCase, 0, len(contract.Cases))
	for _, testCase := range contract.Cases {
		actual = append(actual, expectedCase{
			id:            testCase.ID,
			prerequisites: strings.Join(testCase.Prerequisites, ","),
			kind:          testCase.Owner.Kind,
			owner:         testCase.Owner.Name(),
			evidence:      strings.Join(testCase.Evidence, ","),
		})
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("acceptance contract exact case/hash/owner/evidence set drifted")
	}
}

func TestAcceptanceFocusedWaveDefinitionClosed(t *testing.T) {
	contract, err := LoadContract(acceptanceContractPath())
	if err != nil {
		t.Fatal("load contract")
	}
	if err := validateFocusedWaves(contract); err != nil {
		t.Fatal("checked-in focused waves invalid")
	}
	seen := map[string]bool{}
	for _, wave := range focusedWaves {
		if len(wave) < 1 || len(wave) > focusedConcurrency {
			t.Fatal("focused wave width escaped fixed cap")
		}
		for _, id := range wave {
			if seen[id] {
				t.Fatal("focused owner repeated")
			}
			seen[id] = true
		}
	}
	if len(seen) != 12 {
		t.Fatal("focused waves did not cover every focused owner")
	}
	for _, testCase := range contract.Cases {
		if testCase.Owner.Kind == OwnerGo && seen[testCase.ID] {
			t.Fatal("composed owner entered focused waves")
		}
	}
}

func TestAcceptanceContractRejectsClosedContractMutations(t *testing.T) {
	original, err := LoadContract(acceptanceContractPath())
	if err != nil {
		t.Fatal("checked-in acceptance contract did not load")
	}
	clone := func() Contract {
		copyContract := original
		copyContract.Cases = append([]Case(nil), original.Cases...)
		for index := range copyContract.Cases {
			copyContract.Cases[index].Prerequisites = append([]string(nil), original.Cases[index].Prerequisites...)
			copyContract.Cases[index].Evidence = append([]string(nil), original.Cases[index].Evidence...)
		}
		return copyContract
	}
	mutations := []struct {
		name   string
		mutate func(*Contract)
	}{
		{name: "missing", mutate: func(c *Contract) { c.Cases = c.Cases[1:] }},
		{name: "duplicate", mutate: func(c *Contract) { c.Cases[1] = c.Cases[0] }},
		{name: "renamed", mutate: func(c *Contract) { c.Cases[0].ID = "A60A-001-RENAMED" }},
		{name: "unknown prerequisite", mutate: func(c *Contract) { c.Cases[0].Prerequisites = []string{"ffffffff"} }},
		{name: "duplicate evidence", mutate: func(c *Contract) { c.Cases[0].Evidence = []string{"baseline.direct", "baseline.direct"} }},
		{name: "unknown target", mutate: func(c *Contract) { c.Cases[0].Owner.Target = "test-accelerator-future" }},
		{name: "master recursion", mutate: func(c *Contract) { c.Cases[0].Owner.Target = MasterTarget }},
		{name: "owner empty", mutate: func(c *Contract) { c.Cases[0].Owner = Owner{} }},
		{name: "focused after composed", mutate: func(c *Contract) {
			c.Cases[len(c.Cases)-1].Owner = Owner{Kind: OwnerMake, Target: "test-accelerator-chart"}
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			candidate := clone()
			mutation.mutate(&candidate)
			if ValidateContract(candidate) == nil {
				t.Fatal("closed contract mutation was accepted")
			}
		})
	}

	raw, err := os.ReadFile(acceptanceContractPath())
	if err != nil {
		t.Fatal("checked-in acceptance contract unavailable")
	}
	for _, mutation := range []struct {
		name string
		raw  []byte
	}{
		{name: "unknown field", raw: bytes.Replace(raw, []byte(`"schemaVersion": 1`), []byte(`"schemaVersion": 1, "future": true`), 1)},
		{name: "disabled", raw: bytes.Replace(raw, []byte(`"id": "A60A-001-BASELINE"`), []byte(`"id": "A60A-001-BASELINE", "enabled": false`), 1)},
		{name: "skip", raw: bytes.Replace(raw, []byte(`"id": "A60A-001-BASELINE"`), []byte(`"id": "A60A-001-BASELINE", "skip": true`), 1)},
		{name: "trailing JSON", raw: append(append([]byte(nil), raw...), []byte(` {}`)...)},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			if _, err := ParseContract(bytes.NewReader(mutation.raw)); err == nil {
				t.Fatal("noncanonical contract JSON was accepted")
			}
		})
	}
}
