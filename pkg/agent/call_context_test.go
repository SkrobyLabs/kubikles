package agent

import "testing"

func TestAuthenticatedCallContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  AuthenticatedCallContext
		want bool
	}{
		{name: "zero"},
		{name: "principal only", ctx: AuthenticatedCallContext{PrincipalID: "principal"}},
		{name: "session only", ctx: AuthenticatedCallContext{SessionID: "session"}},
		{name: "both", ctx: AuthenticatedCallContext{PrincipalID: "principal", SessionID: "session"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.IsAuthenticated(); got != tt.want {
				t.Fatalf("IsAuthenticated() = %v, want %v", got, tt.want)
			}
		})
	}
	_ = map[AuthenticatedCallContext]bool{}
}

func TestLocalCallContextIsZero(t *testing.T) {
	ctx := LocalCallContext()
	if ctx != (AuthenticatedCallContext{}) {
		t.Fatalf("LocalCallContext() = %#v, want zero value", ctx)
	}
	if ctx.IsAuthenticated() {
		t.Fatal("LocalCallContext() must be unauthenticated")
	}
}
