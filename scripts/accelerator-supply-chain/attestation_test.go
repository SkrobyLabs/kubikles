package supplychain

import (
	"strings"
	"testing"
)

func TestExactAttestationSubjectPredicateMatrix(t *testing.T) {
	digests := ReleaseDigests{
		ImageIndex: "sha256:" + strings.Repeat("1", 64), ImageAMD64: "sha256:" + strings.Repeat("2", 64), ImageARM64: "sha256:" + strings.Repeat("3", 64),
		Chart: "sha256:" + strings.Repeat("4", 64), Descriptor: "sha256:" + strings.Repeat("5", 64), BuildVersion: "v1.4.0",
	}
	plan, err := ExactAttestationPlan(digests)
	if err != nil || len(plan) != 6 || plan[0].SubjectName != ImageRepository || plan[0].PredicateType != SLSAPredicate || plan[3].PredicateType != SPDXPredicate || plan[2].PushToRegistry {
		t.Fatal("exact attestation plan")
	}
	identity := VerifiedIdentity{
		Issuer: OIDCIssuer, Repository: Repository, SignerRepository: Repository, Workflow: ReleaseWorkflow,
		Certificate: "https://github.com/" + ReleaseWorkflow + "@refs/tags/v1.4.0", SourceRef: "refs/tags/v1.4.0",
		SourceCommit: strings.Repeat("a", 40), SignerDigest: strings.Repeat("a", 40), Runner: "github-hosted",
	}
	if ValidateVerifiedIdentity(identity, "v1.4.0", strings.Repeat("a", 40)) != nil {
		t.Fatal("exact verified identity")
	}
	identity.Runner = "self-hosted"
	if ValidateVerifiedIdentity(identity, "v1.4.0", strings.Repeat("a", 40)) == nil {
		t.Fatal("accepted untrusted runner")
	}
}
