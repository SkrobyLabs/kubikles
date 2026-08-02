package acceleratorprovision

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"kubikles/pkg/server"
)

var errEntropy = errors.New("accelerator credential entropy unavailable")

type creatorCredential struct {
	token    server.CreatorToken
	verifier string
	session  string
}

func (c creatorCredential) String() string             { return "<redacted>" }
func (c creatorCredential) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "<redacted>") }
func generateCreatorCredential(entropy io.Reader) (*creatorCredential, error) {
	var tokenBytes [32]byte
	if _, err := io.ReadFull(entropy, tokenBytes[:]); err != nil {
		return nil, errEntropy
	}
	token, err := server.ParseCreatorToken(base64.RawURLEncoding.EncodeToString(tokenBytes[:]))
	if err != nil {
		return nil, errEntropy
	}
	var sessionBytes [16]byte
	if _, err = io.ReadFull(entropy, sessionBytes[:]); err != nil {
		return nil, errEntropy
	}
	return &creatorCredential{token: token, verifier: server.DeriveCreatorVerifier(token).Encoded(), session: hex.EncodeToString(sessionBytes[:])}, nil
}
func (c *creatorCredential) releaseName() string {
	if c == nil {
		return ""
	}
	return "kubikles-accelerator-" + c.session
}
