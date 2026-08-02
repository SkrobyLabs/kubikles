package acceleratorprovision

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	if err != nil || server.DeriveCreatorVerifier(parsed).Encoded() != credential.verifier {
		t.Fatal("credential did not use exact creator-token primitives")
	}
}

func TestCreatorAuthorizationLeaseIsOpaqueAndSerialized(t *testing.T) {
	entropy := make([]byte, 48)
	for index := range entropy {
		entropy[index] = byte(index)
	}
	credential, err := generateCreatorCredential(bytes.NewReader(entropy))
	if err != nil {
		t.Fatal(err)
	}
	const token = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if values := request.Header.Values("Authorization"); len(values) != 1 || values[0] != "Bearer "+token {
			t.Errorf("unexpected authorization cardinality")
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	entered := make(chan struct{})
	release := make(chan struct{})
	var escaped creatorAuthorizationLease
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- credential.withCreatorAuthorization(context.Background(), func(ctx context.Context, lease creatorAuthorizationLease) error {
			escaped = lease
			close(entered)
			<-release
			request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			if requestErr != nil {
				return requestErr
			}
			response, requestErr := lease.DoHTTP(server.Client(), request)
			if requestErr == nil {
				response.Body.Close()
			}
			if request.Header.Get("Authorization") != "" {
				return errors.New("authorization survived request")
			}
			return requestErr
		})
	}()
	<-entered
	waitCtx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err = credential.withCreatorAuthorization(waitCtx, func(context.Context, creatorAuthorizationLease) error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent lease did not honor cancellation: %v", err)
	}
	close(release)
	if err = <-firstDone; err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	if _, err = escaped.DoHTTP(server.Client(), request); err == nil {
		t.Fatal("escaped lease remained usable")
	}
	for _, output := range []string{fmt.Sprintf("%v", escaped), fmt.Sprintf("%#v", escaped), fmt.Sprintf("%+v", escaped)} {
		assertNoCredentialCorpus(t, output, []string{token, "Authorization", "Bearer"})
	}
}

func TestCredentialClosingWaitsLeasesAndClearsOwnedBytes(t *testing.T) {
	credential, err := generateCreatorCredential(bytes.NewReader(vectorEntropy()))
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- credential.withCreatorAuthorization(context.Background(), func(context.Context, creatorAuthorizationLease) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	credential.mu.Lock()
	closingTransition := credential.changed
	credential.mu.Unlock()
	destroyed := make(chan bool, 1)
	go func() { destroyed <- credential.closeAndDestroy(context.Background()) }()
	<-closingTransition
	select {
	case <-destroyed:
		t.Fatal("destroy returned while an authorization lease remained")
	default:
	}
	if err = credential.withCreatorAuthorization(context.Background(), func(context.Context, creatorAuthorizationLease) error { return nil }); err == nil {
		t.Fatal("closing credential granted a new authorization lease")
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if ok := <-destroyed; !ok {
		t.Fatal("credential was not destroyed")
	}
	for index, value := range credential.encoded {
		if value != 0 {
			t.Fatalf("owned token byte %d was not cleared", index)
		}
	}
	if credential.verifier != "" || credential.session != "" {
		t.Fatal("derived credential material was retained")
	}
	if !credential.closeAndDestroy(context.Background()) {
		t.Fatal("destroy was not idempotent")
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
