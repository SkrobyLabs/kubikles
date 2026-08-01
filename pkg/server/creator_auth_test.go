package server

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kubikles/pkg/agent"
)

const creatorTestToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestCreatorVerifierKnownVector(t *testing.T) {
	const encodedToken = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	const encodedVerifier = "w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM"
	const verifierHex = "c3680bad734d20b1832e6443cb39b6b009af46c76efdf6d0a739abc8f2be8653"

	token, err := ParseCreatorToken(encodedToken)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := ParseCreatorVerifier(encodedVerifier)
	if err != nil {
		t.Fatal(err)
	}
	derived := DeriveCreatorVerifier(token)
	if derived != verifier || derived.Encoded() != encodedVerifier || hex.EncodeToString(derived.value[:]) != verifierHex || !verifier.Matches(token) {
		t.Fatalf("known vector mismatch: encoded=%q digest=%x matches=%v", derived.Encoded(), derived.value, verifier.Matches(token))
	}
	rawDigest := sha256.Sum256(token.value[:])
	if rawDigest == derived.value {
		t.Fatal("domain-separated verifier unexpectedly equals raw token hash")
	}
}

func TestParseCreatorTokenAndVerifierCanonical(t *testing.T) {
	validRaw := make([]byte, 32)
	for i := range validRaw {
		validRaw[i] = byte(i)
	}
	valid := base64.RawURLEncoding.EncodeToString(validRaw)
	standardAlphabetRaw := bytes.Repeat([]byte{0xff}, 32)
	standardAlphabet := base64.RawStdEncoding.EncodeToString(standardAlphabetRaw)
	nonCanonical := valid[:len(valid)-1] + "B"
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "leading whitespace", value: " " + valid},
		{name: "trailing whitespace", value: valid + "\n"},
		{name: "padded", value: valid + "="},
		{name: "standard alphabet", value: standardAlphabet},
		{name: "malformed", value: strings.Repeat("!", 43)},
		{name: "31 bytes", value: base64.RawURLEncoding.EncodeToString(make([]byte, 31))},
		{name: "33 bytes", value: base64.RawURLEncoding.EncodeToString(make([]byte, 33))},
		{name: "prefix", value: "x" + valid},
		{name: "suffix", value: valid + "x"},
		{name: "non-canonical trailing bits", value: nonCanonical},
	}
	parsers := []struct {
		name     string
		parse    func(string) error
		sentinel error
	}{
		{name: "token", sentinel: ErrInvalidCreatorToken, parse: func(value string) error { _, err := ParseCreatorToken(value); return err }},
		{name: "verifier", sentinel: ErrInvalidCreatorVerifier, parse: func(value string) error { _, err := ParseCreatorVerifier(value); return err }},
	}
	for _, parser := range parsers {
		t.Run(parser.name+" accepts canonical", func(t *testing.T) {
			if err := parser.parse(valid); err != nil {
				t.Fatalf("canonical value rejected: %v", err)
			}
		})
		for _, test := range tests {
			t.Run(parser.name+"/"+test.name, func(t *testing.T) {
				err := parser.parse(test.value)
				if !errors.Is(err, parser.sentinel) {
					t.Fatalf("error = %v, want %v", err, parser.sentinel)
				}
				if test.value != "" && strings.Contains(err.Error(), test.value) {
					t.Fatalf("error exposed rejected input: %q", err)
				}
			})
		}
	}
}

func TestVerifierCannotAuthenticateAsToken(t *testing.T) {
	token, err := ParseCreatorToken(creatorTestToken)
	if err != nil {
		t.Fatal(err)
	}
	verifier := DeriveCreatorVerifier(token)
	auth, err := NewCreatorAuthenticator(verifier.Encoded())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/call", nil)
	request.Header.Set("Authorization", "Bearer "+verifier.Encoded())
	response := httptest.NewRecorder()
	auth.Guard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("verifier authenticated as token") })).ServeHTTP(response, request)
	assertUnauthorizedResponse(t, response)
}

func TestCreatorCredentialsAlwaysFormatRedacted(t *testing.T) {
	token, err := ParseCreatorToken("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8")
	if err != nil {
		t.Fatal(err)
	}
	verifier := DeriveCreatorVerifier(token)
	secrets := []string{
		base64.RawURLEncoding.EncodeToString(token.value[:]),
		verifier.Encoded(),
		hex.EncodeToString(token.value[:]),
		hex.EncodeToString(verifier.value[:]),
	}
	for _, value := range []any{token, verifier} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
			if got := fmt.Sprintf(verb, value); got != "<redacted>" {
				t.Fatalf("%s = %q", verb, got)
			}
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range secrets {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("JSON exposed credential material: %s", encoded)
			}
		}
		if _, ok := value.(json.Marshaler); ok {
			t.Fatal("credential unexpectedly implements json.Marshaler")
		}
		if _, ok := value.(encoding.TextMarshaler); ok {
			t.Fatal("credential unexpectedly implements encoding.TextMarshaler")
		}
	}
}

func TestCreatorAuthenticatorAlwaysFormatsRedacted(t *testing.T) {
	token, err := ParseCreatorToken("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := newCreatorAuthenticator(DeriveCreatorVerifier(token).Encoded(), bytes.NewReader(append(bytes.Repeat([]byte{0x41}, 16), bytes.Repeat([]byte{0x42}, 16)...)))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{*auth, auth} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
			got := fmt.Sprintf(verb, value)
			if got != "<redacted>" {
				t.Fatalf("%T %s = %q", value, verb, got)
			}
			for _, secret := range []string{auth.verifier.Encoded(), string(auth.context.PrincipalID), string(auth.context.SessionID)} {
				if strings.Contains(got, secret) {
					t.Fatalf("%T %s exposed %q", value, verb, secret)
				}
			}
		}
	}
}

type recordingBody struct {
	reads int
	data  *strings.Reader
}

func (b *recordingBody) Read(p []byte) (int, error) {
	b.reads++
	return b.data.Read(p)
}

func (*recordingBody) Close() error { return nil }

func TestCreatorAuthenticatorAuthorizationMatrix(t *testing.T) {
	token, err := ParseCreatorToken(creatorTestToken)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := newCreatorAuthenticator(DeriveCreatorVerifier(token).Encoded(), bytes.NewReader(bytes.Repeat([]byte{0x11}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	wrong := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x7f}, 32))
	tests := []struct {
		name  string
		apply func(*http.Request)
	}{
		{name: "missing"},
		{name: "empty", apply: func(r *http.Request) { r.Header["Authorization"] = []string{""} }},
		{name: "duplicate", apply: func(r *http.Request) {
			r.Header["Authorization"] = []string{"Bearer " + creatorTestToken, "Bearer " + creatorTestToken}
		}},
		{name: "comma joined", apply: func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+creatorTestToken+", Bearer "+creatorTestToken)
		}},
		{name: "alternate scheme case", apply: func(r *http.Request) { r.Header.Set("Authorization", "bearer "+creatorTestToken) }},
		{name: "alternate scheme", apply: func(r *http.Request) { r.Header.Set("Authorization", "Basic "+creatorTestToken) }},
		{name: "extra leading whitespace", apply: func(r *http.Request) { r.Header.Set("Authorization", " Bearer "+creatorTestToken) }},
		{name: "extra separator whitespace", apply: func(r *http.Request) { r.Header.Set("Authorization", "Bearer  "+creatorTestToken) }},
		{name: "trailing whitespace", apply: func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+creatorTestToken+" ") }},
		{name: "malformed token", apply: func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+strings.Repeat("!", 43)) }},
		{name: "wrong token", apply: func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+wrong) }},
		{name: "query token", apply: func(r *http.Request) { r.URL.RawQuery = "token=" + creatorTestToken }},
		{name: "cookie token", apply: func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "token", Value: creatorTestToken}) }},
		{name: "custom header token", apply: func(r *http.Request) { r.Header.Set("X-Creator-Token", creatorTestToken) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &recordingBody{data: strings.NewReader("sensitive body")}
			request := httptest.NewRequest(http.MethodPost, "/api/call", nil)
			request.Body = body
			if test.apply != nil {
				test.apply(request)
			}
			called := false
			response := httptest.NewRecorder()
			auth.Guard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(response, request)
			assertUnauthorizedResponse(t, response)
			if called || body.reads != 0 {
				t.Fatalf("invalid auth reached downstream: called=%v body reads=%d", called, body.reads)
			}
		})
	}

	request := httptest.NewRequest(http.MethodPost, "/api/call", nil)
	request.Header["Authorization"] = []string{"Bearer " + creatorTestToken}
	response := httptest.NewRecorder()
	called := false
	auth.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if _, ok := authenticatedCreatorContext(r.Context()); !ok {
			t.Fatal("valid authentication did not install trusted context")
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("valid bearer rejected: status=%d called=%v", response.Code, called)
	}
}

func TestCreatorAuthenticatorInjectsStableTrustedContext(t *testing.T) {
	token, err := ParseCreatorToken(creatorTestToken)
	if err != nil {
		t.Fatal(err)
	}
	verifier := DeriveCreatorVerifier(token).Encoded()
	firstEntropy := append(bytes.Repeat([]byte{0x21}, 16), bytes.Repeat([]byte{0x42}, 16)...)
	first, err := newCreatorAuthenticator(verifier, bytes.NewReader(firstEntropy))
	if err != nil {
		t.Fatal(err)
	}
	second, err := newCreatorAuthenticator(verifier, bytes.NewReader(bytes.Repeat([]byte{0x84}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if first.context == second.context {
		t.Fatal("independent authenticators reused identity")
	}
	assertRandomID(t, string(first.context.PrincipalID), "creator-", bytes.Repeat([]byte{0x21}, 16))
	assertRandomID(t, string(first.context.SessionID), "http-", bytes.Repeat([]byte{0x42}, 16))

	var observed []agent.AuthenticatedCallContext
	handler := first.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := authenticatedCreatorContext(r.Context())
		if !ok {
			t.Fatal("trusted context missing")
		}
		observed = append(observed, got)
		w.WriteHeader(http.StatusNoContent)
	}))
	for i := 0; i < 2; i++ {
		request := httptest.NewRequest(http.MethodPost, "/api/call?PrincipalID=forged&SessionID=forged", strings.NewReader(`{"PrincipalID":"forged","SessionID":"forged"}`))
		request.Header.Set("Authorization", "Bearer "+creatorTestToken)
		request.Header.Set("X-Principal-ID", "forged")
		request.Header.Set("X-Session-ID", "forged")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("request %d status = %d", i, response.Code)
		}
	}
	if len(observed) != 2 || observed[0] != first.context || observed[1] != first.context {
		t.Fatalf("context was unstable or forgeable: %#v", observed)
	}

	instance, err := newAcceleratorInstanceID(bytes.NewReader(bytes.Repeat([]byte{0x63}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	assertRandomID(t, instance, "accel-", bytes.Repeat([]byte{0x63}, 16))
	for _, test := range []struct {
		name    string
		entropy io.Reader
	}{
		{name: "short identity entropy", entropy: bytes.NewReader(make([]byte, 31))},
		{name: "identity entropy error", entropy: io.MultiReader(bytes.NewReader(make([]byte, 16)), errReader{})},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got, err := newCreatorAuthenticator(verifier, test.entropy); err == nil || got != nil || strings.Contains(err.Error(), verifier) {
				t.Fatalf("constructor = %#v, %v", got, err)
			}
		})
	}
	for _, test := range []struct {
		name    string
		entropy io.Reader
	}{
		{name: "short instance entropy", entropy: bytes.NewReader(make([]byte, 15))},
		{name: "instance entropy error", entropy: errReader{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got, err := newAcceleratorInstanceID(test.entropy); err == nil || got != "" {
				t.Fatalf("instance constructor = %q, %v", got, err)
			}
		})
	}
}

func TestCreatorGuardMarksCreatorCredentialKind(t *testing.T) {
	token, err := ParseCreatorToken(creatorTestToken)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := newCreatorAuthenticator(DeriveCreatorVerifier(token).Encoded(), bytes.NewReader(bytes.Repeat([]byte{0x91}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/call?PrincipalID=forged&SessionID=forged", strings.NewReader(`{"PrincipalID":"forged","SessionID":"forged"}`))
	request.Header.Set("Authorization", "Bearer "+creatorTestToken)
	request.Header.Set("X-Credential-Kind", "browser")
	request = request.WithContext(authenticatedContext(request.Context(), agent.AuthenticatedCallContext{PrincipalID: "forged", SessionID: "forged"}, credentialKindBrowser))
	response := httptest.NewRecorder()
	auth.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call, ok := authenticatedCreatorContext(r.Context())
		if !ok || call != auth.context || authenticatedCredentialKind(r.Context()) != credentialKindCreator {
			t.Fatalf("trusted creator context/kind = %#v/%v/%v", call, authenticatedCredentialKind(r.Context()), ok)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%q", response.Code, response.Body.String())
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("entropy source detail") }

func assertRandomID(t *testing.T, value, prefix string, want []byte) {
	t.Helper()
	if !strings.HasPrefix(value, prefix) {
		t.Fatalf("ID %q missing prefix %q", value, prefix)
	}
	encoded := strings.TrimPrefix(value, prefix)
	if len(encoded) != 22 || strings.Contains(encoded, "=") {
		t.Fatalf("ID %q is not fixed canonical 128-bit base64url", value)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || !bytes.Equal(decoded, want) {
		t.Fatalf("ID %q decoded to %x, %v; want %x", value, decoded, err, want)
	}
}

func assertUnauthorizedResponse(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"error\":\"unauthorized\"}\n" || response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unauthorized response = %d %q %#v", response.Code, response.Body.String(), response.Header())
	}
}
