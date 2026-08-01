package agent

// PrincipalID identifies the authenticated principal that made a call.
type PrincipalID string

// SessionID identifies the authenticated session that made a call.
type SessionID string

// AuthenticatedCallContext carries server-issued identity for one method call.
type AuthenticatedCallContext struct {
	PrincipalID PrincipalID
	SessionID   SessionID
}

// IsAuthenticated reports whether both server-issued identity values are present.
func (c AuthenticatedCallContext) IsAuthenticated() bool {
	return c.PrincipalID != "" && c.SessionID != ""
}

// LocalCallContext returns the unauthenticated context used by ordinary server calls.
func LocalCallContext() AuthenticatedCallContext { return AuthenticatedCallContext{} }
