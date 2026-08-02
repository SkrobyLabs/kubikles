package acceleratorprovision

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"kubikles/pkg/server"
)

func TestGenerateCreatorCredentialKnownVector(t *testing.T) {
	entropy := make([]byte, 48)
	for i := range entropy {
		entropy[i] = byte(i)
	}
	credential, err := generateCreatorCredential(bytes.NewReader(entropy))
	if err != nil {
		t.Fatal(err)
	}
	const token = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	const verifier = "w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM"
	const session = "202122232425262728292a2b2c2d2e2f"
	if credential.verifier != verifier || credential.session != session || credential.releaseName() != "kubikles-accelerator-"+session {
		t.Fatalf("known vector mismatch: verifier=%q session=%q release=%q", credential.verifier, credential.session, credential.releaseName())
	}
	parsed, err := server.ParseCreatorToken(token)
	if err != nil || credential.token != parsed || !server.DeriveCreatorVerifier(credential.token).Matches(credential.token) {
		t.Fatal("credential did not use exact creator-token primitives")
	}
}

func TestCredentialEntropyFailures(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty"},
		{name: "short token", data: make([]byte, 31)},
		{name: "missing session", data: make([]byte, 32)},
		{name: "short session", data: make([]byte, 47)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credential, err := generateCreatorCredential(bytes.NewReader(test.data))
			if credential != nil || !errors.Is(err, errEntropy) || err.Error() != "accelerator credential entropy unavailable" {
				t.Fatalf("credential=%#v error=%v", credential, err)
			}
		})
	}
}

func TestCredentialAndHandleFormattingRedacts(t *testing.T) {
	entropy := make([]byte, 48)
	for i := range entropy {
		entropy[i] = byte(i)
	}
	credential, err := generateCreatorCredential(bytes.NewReader(entropy))
	if err != nil {
		t.Fatal(err)
	}
	workload := &ProvisionedWorkload{
		ContextName: "ctx", ReleaseNamespace: "default", ReleaseName: credential.releaseName(),
		WorkloadSessionID: credential.session, Job: ObjectIdentity{Name: "job", UID: "job-uid"},
		Pod: ObjectIdentity{Name: "pod", UID: "pod-uid"}, BuildVersion: "v1.2.3",
		ImageDigest: "sha256:" + strings.Repeat("a", 64), ChartDigest: "sha256:" + strings.Repeat("b", 64), credential: credential,
	}
	result := available(workload)
	secrets := []string{
		"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
		"w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM",
		"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
	}
	for _, value := range []any{credential, *credential, workload, *workload, result} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
			assertNoCredentialCorpus(t, fmt.Sprintf(verb, value), secrets)
		}
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		assertNoCredentialCorpus(t, string(encoded), secrets)
	}
	if _, ok := any(credential).(encoding.TextMarshaler); ok {
		t.Fatal("credential exposes text marshaling")
	}
	if _, ok := any(credential).(json.Marshaler); ok {
		t.Fatal("credential exposes JSON marshaling")
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "credential") || strings.Contains(string(encoded), "verifier") {
		t.Fatalf("unsafe result JSON: %s (%v)", encoded, err)
	}
}

func assertNoCredentialCorpus(t *testing.T, output string, secrets []string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(output, secret) {
			t.Fatalf("credential corpus escaped: %q", output)
		}
	}
}
