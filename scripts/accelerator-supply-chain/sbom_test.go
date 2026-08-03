package supplychain

import (
	"encoding/json"
	"strings"
	"testing"
)

func validSPDXFixture() []byte {
	return []byte(`{
  "spdxVersion":"SPDX-2.3",
  "dataLicense":"CC0-1.0",
  "SPDXID":"SPDXRef-DOCUMENT",
  "name":"nondeterministic",
  "documentNamespace":"https://example.invalid/random",
  "creationInfo":{"created":"2020-01-01T00:00:00Z","creators":["Tool: old"]},
  "documentDescribes":["SPDXRef-Package-a"],
  "packages":[{"SPDXID":"SPDXRef-Package-a","name":"a","versionInfo":"1.0","downloadLocation":"NOASSERTION","filesAnalyzed":false,"externalRefs":[{"referenceCategory":"PACKAGE-MANAGER","referenceType":"purl","referenceLocator":"pkg:generic/a@1.0?arch=amd64"},{"referenceCategory":"SECURITY","referenceType":"cpe23Type","referenceLocator":"cpe:2.3:a:a:a:1.0\\+build:*:*:*:*:*:*:*"}]}],
  "relationships":[{"spdxElementId":"SPDXRef-DOCUMENT","relationshipType":"DESCRIBES","relatedSpdxElement":"SPDXRef-Package-a"}]
}`)
}

func TestNormalizeSPDX23Deterministically(t *testing.T) {
	commit := strings.Repeat("a", 40)
	digest := "sha256:" + strings.Repeat("b", 64)
	first, err := NormalizeSPDX(validSPDXFixture(), "accelerator-amd64", digest, commit, 1700000000)
	if err != nil {
		t.Fatal("normalize SPDX")
	}
	second, err := NormalizeSPDX(first, "accelerator-amd64", digest, commit, 1700000000)
	if err != nil || string(first) != string(second) {
		t.Fatal("SPDX bytes are not deterministic")
	}
	var document SPDXDocument
	if json.Unmarshal(first, &document) != nil || document.CreationInfo.Created != "2023-11-14T22:13:20Z" || document.CreationInfo.Creators[0] != "Tool: syft-1.44.0" || !strings.HasSuffix(document.DocumentNamespace, strings.Repeat("b", 64)) {
		t.Fatal("normalized SPDX identity")
	}
	for _, mutation := range []string{
		strings.Replace(string(validSPDXFixture()), "SPDX-2.3", "SPDX-2.2", 1),
		strings.Replace(string(validSPDXFixture()), "NOASSERTION", "/tmp/runner-secret", 1),
		strings.Replace(string(validSPDXFixture()), "pkg:generic/a@1.0?arch=amd64", "pkg:generic/%2Ftmp%2Fsecret@1.0", 1),
		strings.Replace(string(validSPDXFixture()), "pkg:generic/a@1.0?arch=amd64", "pkg:generic/*", 1),
		strings.Replace(string(validSPDXFixture()), `"packages":[`, `"unknown":true,"packages":[`, 1),
	} {
		if _, err := NormalizeSPDX([]byte(mutation), "accelerator-amd64", digest, commit, 1700000000); err == nil {
			t.Fatal("accepted invalid SPDX mutation")
		}
	}
}
