// Package acceleratoracceptance owns the closed, test-only Accelerator 60A
// acceptance contract, runner, and value-free report.
package acceleratoracceptance

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"sort"
)

const (
	SchemaVersion = 1
	SuiteIdentity = "Kubikles Accelerator 60A"
	BuildIdentity = "v0.0.0"
	MasterTarget  = "test-accelerator-e2e"
)

var errInvalidContract = errors.New("acceptance contract invalid")

type OwnerKind string

const (
	OwnerMake OwnerKind = "make"
	OwnerNPM  OwnerKind = "npm"
	OwnerGo   OwnerKind = "go"
)

type Contract struct {
	SchemaVersion int    `json:"schemaVersion"`
	Suite         string `json:"suite"`
	BuildVersion  string `json:"buildVersion"`
	Cases         []Case `json:"cases"`
}

type Case struct {
	ID            string   `json:"id"`
	Prerequisites []string `json:"prerequisites"`
	Owner         Owner    `json:"owner"`
	Evidence      []string `json:"evidence"`
}

type Owner struct {
	Kind   OwnerKind `json:"kind"`
	Target string    `json:"target,omitempty"`
	Script string    `json:"script,omitempty"`
	Test   string    `json:"test,omitempty"`
}

func (o Owner) Name() string {
	switch o.Kind {
	case OwnerMake:
		return o.Target
	case OwnerNPM:
		return o.Script
	case OwnerGo:
		return o.Test
	default:
		return ""
	}
}

type expectedOwner struct {
	id    string
	kind  OwnerKind
	owner string
}

var expectedOwners = []expectedOwner{
	{"A60A-001-BASELINE", OwnerMake, "test-accelerator-00-kind"},
	{"A60A-002-BINARY", OwnerMake, "build-accelerator"},
	{"A60A-003-IMAGE", OwnerMake, "test-accelerator-image"},
	{"A60A-004-CHART", OwnerMake, "test-accelerator-chart"},
	{"A60A-005-CHART-KIND", OwnerMake, "test-accelerator-chart-kind"},
	{"A60A-006-RELEASE-CONTRACT", OwnerMake, "test-accelerator-release-contract"},
	{"A60A-007-LOCAL-PUBLICATION", OwnerMake, "test-accelerator-publication-local"},
	{"A60A-008-PROVISION", OwnerMake, "test-accelerator-desktop-provision-kind"},
	{"A60A-009-CONNECT", OwnerMake, "test-accelerator-desktop-connector-kind"},
	{"A60A-010-RESUME", OwnerMake, "test-accelerator-desktop-resume-kind"},
	{"A60A-011-DISPOSAL", OwnerMake, "test-accelerator-desktop-disposal-kind"},
	{"A60A-012-LIFECYCLE", OwnerMake, "test-accelerator-desktop-lifecycle-kind"},
	{"A60A-013-BROWSER-ARTIFACT", OwnerNPM, "test:accelerator-browser-artifact"},
	{"A60A-014-BROWSER-LIFECYCLE", OwnerMake, "test-accelerator-browser-lifecycle-kind"},
	{"A60A-015-INTEGRATED-ROUTING", OwnerMake, "test-accelerator-integrated-routing-kind"},
	{"A60A-016-INTEGRATED-HAPPY", OwnerGo, "TestAcceleratorAcceptanceKind/IntegratedHappyPath"},
	{"A60A-017-MISMATCH", OwnerGo, "TestAcceleratorAcceptanceKind/VersionAndArtifactMismatch"},
	{"A60A-018-TRANSPORT-RESUME", OwnerGo, "TestAcceleratorAcceptanceKind/TransportResumeAndRecreate"},
	{"A60A-019-BROWSER-HANDOFF", OwnerGo, "TestAcceleratorAcceptanceKind/BrowserHandoff"},
	{"A60A-020-REAL-GRACE", OwnerGo, "TestAcceleratorAcceptanceKind/RealTwoMinuteAuthenticatedClientGrace"},
	{"A60A-021-SECURITY-PRIVACY", OwnerGo, "TestAcceleratorAcceptanceKind/SecurityPrivacyBackpressure"},
	{"A60A-022-CLEANUP-ISOLATION", OwnerGo, "TestAcceleratorAcceptanceKind/CleanupSweepIsolation"},
}

var requiredPrerequisites = map[string]struct{}{
	"020e8648": {}, "033fde0a": {}, "1cec4c3a": {}, "1ff2446c": {}, "2289380c": {},
	"2d45b95b": {}, "2e710408": {}, "44ea1f82": {}, "53dfebec": {}, "63140918": {},
	"66f24342": {}, "70e5466d": {}, "71facaab": {}, "8ebf3759": {}, "949c4709": {},
	"a02bce49": {}, "a5d2c80a": {}, "ac978c3f": {}, "b1854aa3": {}, "b3ffc033": {},
	"d0eb9876": {}, "d4f712fd": {}, "dd19f7b6": {}, "e34515b0": {}, "e590da1e": {},
	"e6c23dbd": {}, "f256001c": {}, "f2769f29": {}, "f604c5fb": {},
}

var (
	idPattern       = regexp.MustCompile(`^A60A-[0-9]{3}-[A-Z][A-Z0-9-]*$`)
	targetPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	scriptPattern   = regexp.MustCompile(`^[a-z][a-z0-9:-]*$`)
	testPattern     = regexp.MustCompile(`^TestAcceleratorAcceptanceKind/[A-Za-z][A-Za-z0-9]*$`)
	evidencePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z0-9-]+)+$`)
	hashPattern     = regexp.MustCompile(`^[0-9a-f]{8}$`)
)

func LoadContract(path string) (Contract, error) {
	file, err := os.Open(path)
	if err != nil {
		return Contract{}, errInvalidContract
	}
	defer file.Close()
	return ParseContract(file)
}

func ParseContract(reader io.Reader) (Contract, error) {
	data, strictErr := readStrictJSON(reader)
	if strictErr != nil {
		return Contract{}, errInvalidContract
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var contract Contract
	if err := decoder.Decode(&contract); err != nil {
		return Contract{}, errInvalidContract
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Contract{}, errInvalidContract
	}
	if err := ValidateContract(contract); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func ValidateContract(contract Contract) error {
	if contract.SchemaVersion != SchemaVersion || contract.Suite != SuiteIdentity || contract.BuildVersion != BuildIdentity || len(contract.Cases) != len(expectedOwners) {
		return errInvalidContract
	}
	seenIDs := make(map[string]struct{}, len(contract.Cases))
	seenPrerequisites := make(map[string]struct{}, len(requiredPrerequisites))
	composed := false
	for index, testCase := range contract.Cases {
		expected := expectedOwners[index]
		if !idPattern.MatchString(testCase.ID) || testCase.ID != expected.id || testCase.Owner.Kind != expected.kind || testCase.Owner.Name() != expected.owner {
			return errInvalidContract
		}
		if _, exists := seenIDs[testCase.ID]; exists {
			return errInvalidContract
		}
		seenIDs[testCase.ID] = struct{}{}
		if err := validateOwner(testCase.Owner); err != nil {
			return err
		}
		if testCase.Owner.Kind == OwnerGo {
			composed = true
		} else if composed {
			return errInvalidContract
		}
		if len(testCase.Prerequisites) == 0 || !sort.StringsAreSorted(testCase.Prerequisites) {
			return errInvalidContract
		}
		casePrerequisites := make(map[string]struct{}, len(testCase.Prerequisites))
		for _, hash := range testCase.Prerequisites {
			if !hashPattern.MatchString(hash) {
				return errInvalidContract
			}
			if _, allowed := requiredPrerequisites[hash]; !allowed {
				return errInvalidContract
			}
			if _, duplicate := casePrerequisites[hash]; duplicate {
				return errInvalidContract
			}
			casePrerequisites[hash] = struct{}{}
			seenPrerequisites[hash] = struct{}{}
		}
		if len(testCase.Evidence) == 0 || !sort.StringsAreSorted(testCase.Evidence) {
			return errInvalidContract
		}
		seenEvidence := make(map[string]struct{}, len(testCase.Evidence))
		for _, key := range testCase.Evidence {
			if !evidencePattern.MatchString(key) {
				return errInvalidContract
			}
			if _, duplicate := seenEvidence[key]; duplicate {
				return errInvalidContract
			}
			seenEvidence[key] = struct{}{}
		}
	}
	if len(seenPrerequisites) != len(requiredPrerequisites) {
		return errInvalidContract
	}
	return nil
}

func validateOwner(owner Owner) error {
	switch owner.Kind {
	case OwnerMake:
		if owner.Target == "" || owner.Target == MasterTarget || !targetPattern.MatchString(owner.Target) || owner.Script != "" || owner.Test != "" {
			return errInvalidContract
		}
	case OwnerNPM:
		if owner.Script == "" || !scriptPattern.MatchString(owner.Script) || owner.Target != "" || owner.Test != "" {
			return errInvalidContract
		}
	case OwnerGo:
		if owner.Test == "" || !testPattern.MatchString(owner.Test) || owner.Target != "" || owner.Script != "" {
			return errInvalidContract
		}
	default:
		return errInvalidContract
	}
	return nil
}
