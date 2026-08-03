package supplychain

import (
	"errors"
	"fmt"
	"strings"
)

type ReleaseDigests struct {
	ImageIndex   string
	ImageAMD64   string
	ImageARM64   string
	Chart        string
	Descriptor   string
	BuildVersion string
}

type Attestation struct {
	ID             string `json:"id"`
	SubjectName    string `json:"subjectName"`
	SubjectDigest  string `json:"subjectDigest"`
	PredicateType  string `json:"predicateType"`
	SBOMPath       string `json:"sbomPath,omitempty"`
	PushToRegistry bool   `json:"pushToRegistry"`
}

type VerifiedIdentity struct {
	Issuer           string `json:"issuer"`
	Repository       string `json:"repository"`
	SignerRepository string `json:"signerRepository"`
	Workflow         string `json:"workflow"`
	Certificate      string `json:"certificateIdentity"`
	SourceRef        string `json:"sourceRef"`
	SourceCommit     string `json:"sourceCommit"`
	SignerDigest     string `json:"signerDigest"`
	Runner           string `json:"runner"`
}

func ExactAttestationPlan(digests ReleaseDigests) ([]Attestation, error) {
	if _, err := ValidateReleaseIdentity(digests.BuildVersion, strings.Repeat("a", 40)); err != nil {
		return nil, err
	}
	for _, digest := range []string{digests.ImageIndex, digests.ImageAMD64, digests.ImageARM64, digests.Chart, digests.Descriptor} {
		if !strings.HasPrefix(digest, "sha256:") || !hex64Pattern.MatchString(strings.TrimPrefix(digest, "sha256:")) {
			return nil, errors.New("invalid attestation digest")
		}
	}
	amd64, _ := SPDXAssetName("image-linux-amd64", digests.BuildVersion)
	arm64, _ := SPDXAssetName("image-linux-arm64", digests.BuildVersion)
	chart, _ := SPDXAssetName("chart", digests.BuildVersion)
	descriptor := "kubikles-accelerator-release-" + digests.BuildVersion + ".json"
	return []Attestation{
		{"image-index-provenance", ImageRepository, digests.ImageIndex, SLSAPredicate, "", true},
		{"chart-provenance", ChartRepository, digests.Chart, SLSAPredicate, "", true},
		{"descriptor-provenance", descriptor, digests.Descriptor, SLSAPredicate, "", false},
		{"image-amd64-spdx", ImageRepository, digests.ImageAMD64, SPDXPredicate, amd64, true},
		{"image-arm64-spdx", ImageRepository, digests.ImageARM64, SPDXPredicate, arm64, true},
		{"chart-spdx", ChartRepository, digests.Chart, SPDXPredicate, chart, true},
	}, nil
}

func ValidateVerifiedIdentity(identity VerifiedIdentity, version, commit string) error {
	if _, err := ValidateReleaseIdentity(version, commit); err != nil {
		return err
	}
	wantCertificate := fmt.Sprintf("https://github.com/%s@refs/tags/%s", ReleaseWorkflow, version)
	if identity.Issuer != OIDCIssuer || identity.Repository != Repository || identity.SignerRepository != Repository || identity.Workflow != ReleaseWorkflow || identity.Certificate != wantCertificate || identity.SourceRef != "refs/tags/"+version || identity.SourceCommit != commit || identity.SignerDigest != commit || identity.Runner != "github-hosted" {
		return errors.New("attestation identity mismatch")
	}
	return nil
}
