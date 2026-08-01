package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"kubikles/pkg/agent"
)

const creatorTokenDomain = "kubikles/accelerator/creator-token/v1\x00"

var (
	ErrInvalidCreatorToken    = errors.New("invalid creator token")
	ErrInvalidCreatorVerifier = errors.New("invalid creator verifier")
)

type CreatorToken struct{ value [32]byte }
type CreatorVerifier struct{ value [32]byte }

func parseCreatorValue(text string, target *[32]byte, sentinel error) error {
	if len(text) != 43 {
		return sentinel
	}
	decoded, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil || len(decoded) != len(target) || base64.RawURLEncoding.EncodeToString(decoded) != text {
		return sentinel
	}
	copy(target[:], decoded)
	return nil
}

func ParseCreatorToken(text string) (CreatorToken, error) {
	var token CreatorToken
	if err := parseCreatorValue(text, &token.value, ErrInvalidCreatorToken); err != nil {
		return CreatorToken{}, err
	}
	return token, nil
}
func ParseCreatorVerifier(text string) (CreatorVerifier, error) {
	var verifier CreatorVerifier
	if err := parseCreatorValue(text, &verifier.value, ErrInvalidCreatorVerifier); err != nil {
		return CreatorVerifier{}, err
	}
	return verifier, nil
}
func DeriveCreatorVerifier(token CreatorToken) CreatorVerifier {
	return CreatorVerifier{value: sha256.Sum256(append([]byte(creatorTokenDomain), token.value[:]...))}
}
func (v CreatorVerifier) String() string             { return "<redacted>" }
func (t CreatorToken) String() string                { return "<redacted>" }
func (v CreatorVerifier) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "<redacted>") }
func (t CreatorToken) Format(s fmt.State, _ rune)    { _, _ = io.WriteString(s, "<redacted>") }
func (v CreatorVerifier) Encoded() string            { return base64.RawURLEncoding.EncodeToString(v.value[:]) }
func (v CreatorVerifier) Matches(token CreatorToken) bool {
	derived := DeriveCreatorVerifier(token)
	return subtle.ConstantTimeCompare(v.value[:], derived.value[:]) == 1
}

type creatorContextKey struct{}
type credentialKindContextKey struct{}
type credentialKind uint8

const (
	credentialKindCreator credentialKind = iota + 1
	credentialKindBrowser
)

func authenticatedCreatorContext(ctx context.Context) (agent.AuthenticatedCallContext, bool) {
	value, ok := ctx.Value(creatorContextKey{}).(agent.AuthenticatedCallContext)
	return value, ok && value.IsAuthenticated()
}
func authenticatedContext(ctx context.Context, call agent.AuthenticatedCallContext, kind credentialKind) context.Context {
	ctx = context.WithValue(ctx, creatorContextKey{}, call)
	return context.WithValue(ctx, credentialKindContextKey{}, kind)
}
func authenticatedCredentialKind(ctx context.Context) credentialKind {
	kind, _ := ctx.Value(credentialKindContextKey{}).(credentialKind)
	return kind
}

type CreatorAuthenticator struct {
	verifier CreatorVerifier
	context  agent.AuthenticatedCallContext
}

func (a CreatorAuthenticator) String() string             { return "<redacted>" }
func (a CreatorAuthenticator) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "<redacted>") }

func NewCreatorAuthenticator(encodedVerifier string) (*CreatorAuthenticator, error) {
	return newCreatorAuthenticator(encodedVerifier, rand.Reader)
}
func newCreatorAuthenticator(encodedVerifier string, entropy io.Reader) (*CreatorAuthenticator, error) {
	verifier, err := ParseCreatorVerifier(encodedVerifier)
	if err != nil {
		return nil, ErrInvalidCreatorVerifier
	}
	principal, err := creatorID(entropy, "creator-")
	if err != nil {
		return nil, err
	}
	session, err := creatorID(entropy, "http-")
	if err != nil {
		return nil, err
	}
	return &CreatorAuthenticator{verifier: verifier, context: agent.AuthenticatedCallContext{PrincipalID: agent.PrincipalID(principal), SessionID: agent.SessionID(session)}}, nil
}
func creatorID(entropy io.Reader, prefix string) (string, error) {
	var raw [16]byte
	if _, err := io.ReadFull(entropy, raw[:]); err != nil {
		return "", errors.New("creator identity entropy unavailable")
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
func newAcceleratorInstanceID(entropy io.Reader) (string, error) { return creatorID(entropy, "accel-") }

// NewAcceleratorInstanceID creates the safe public process instance identifier.
func NewAcceleratorInstanceID() (string, error) { return newAcceleratorInstanceID(rand.Reader) }

func (a *CreatorAuthenticator) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		credential, ok := strictBearer(r)
		if !ok || a == nil || !a.matchesBearer(credential) {
			writeAcceleratorError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(authenticatedContext(r.Context(), a.context, credentialKindCreator)))
	})
}
func (a *CreatorAuthenticator) authenticate(r *http.Request) bool {
	credential, ok := strictBearer(r)
	return ok && a.matchesBearer(credential)
}
func strictBearer(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") || strings.Count(values[0], " ") != 1 {
		return "", false
	}
	return strings.TrimPrefix(values[0], "Bearer "), true
}
func (a *CreatorAuthenticator) matchesBearer(credential string) bool {
	token, err := ParseCreatorToken(credential)
	return err == nil && a.verifier.Matches(token)
}
