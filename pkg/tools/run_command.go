package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultAllowedCommandPrefixes lists safe, read-only command prefixes
// that are allowed by default when no custom allowlist is configured.
var DefaultAllowedCommandPrefixes = []string{
	"kubectl get",
	"kubectl describe",
	"kubectl logs",
	"kubectl top",
	"kubectl explain",
	"kubectl api-resources",
	"kubectl api-versions",
	"kubectl version",
	"kubectl config get-contexts",
	"kubectl config current-context",
	"helm list",
	"helm status",
	"helm get",
	"helm history",
	"helm search",
	"helm show",
}

// AllowedCommandPrefixes is the active set of allowed command prefixes.
// Set by the MCP server at startup from CLI flags or config.
// If nil, DefaultAllowedCommandPrefixes is used.
var AllowedCommandPrefixes []string

// ValidateCommand checks whether the given command tokens match any of the
// allowed command prefixes. Each prefix is split into tokens and compared
// against the leading tokens of the command.
func ValidateCommand(tokens []string, prefixes []string) bool {
	for _, prefix := range prefixes {
		prefixTokens := strings.Fields(prefix)
		if len(prefixTokens) == 0 {
			continue
		}
		if len(tokens) < len(prefixTokens) {
			continue
		}
		match := true
		for i, pt := range prefixTokens {
			if tokens[i] != pt {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// toolRunCommand validates the command against the allowlist and executes it
// directly via exec.Command (no shell interpretation) with a 30-second timeout.
// Commands not matching any allowed prefix are rejected.
func toolRunCommand(args map[string]interface{}, prefixes []string) (string, bool) {
	command := strArg(args, "command")
	if command == "" {
		return "command is required", true
	}

	tokens := strings.Fields(command)
	if len(tokens) == 0 {
		return "command is empty", true
	}

	if !ValidateCommand(tokens, prefixes) {
		// Build the prefix that would need to be allowed (first 1-2 tokens)
		hint := tokens[0]
		if len(tokens) > 1 {
			hint = tokens[0] + " " + tokens[1]
		}
		if len(prefixes) == 0 {
			return fmt.Sprintf("Command %q is not allowed. No command prefixes are in the allowlist. "+
				"The user needs to enable the %q prefix in their command allowlist settings.",
				strings.Join(tokens, " "), hint), true
		}
		return fmt.Sprintf("Command %q is not allowed. The prefix %q is not in the allowlist. "+
			"Currently allowed prefixes: %s",
			strings.Join(tokens, " "), hint,
			strings.Join(prefixes, ", ")), true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, tokens[0], tokens[1:]...) //nolint:gosec // command is validated against the allowlist above

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("STDERR:\n")
		result.WriteString(stderr.String())
	}

	output := truncate(result.String(), MaxCommandOutputChars)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("Command timed out after 30s.\n%s", output), true
		}
		if output == "" {
			return fmt.Sprintf("Command failed: %v", err), true
		}
		return fmt.Sprintf("Command exited with error: %v\n%s", err, output), true
	}

	if output == "" {
		return "(no output)", false
	}
	return output, false
}
