//go:build !windows

package hosts

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

// getHostsPath returns the path to the hosts file on Unix systems
func getHostsPath() string {
	return "/etc/hosts"
}

// AddEntriesWithPortRedirect adds hostname entries and sets up port redirection (443->8443, 80->8080)
func (m *Manager) AddEntriesWithPortRedirect(entries []Entry, httpsPort, httpPort int) error {
	if len(entries) == 0 {
		return nil
	}

	// First, read current content and remove any existing managed block
	content, err := os.ReadFile(m.hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	cleanedContent := removeExistingManagedBlock(string(content))
	newContent := cleanedContent + buildEntriesContent(entries)

	// Escape content for AppleScript
	escapedContent := escapeForAppleScript(newContent)

	// Build pfctl rules for port redirection
	// This allows binding to non-privileged ports while users access standard ports
	var pfctlRules strings.Builder
	if httpsPort > 0 && httpsPort != 443 {
		pfctlRules.WriteString(fmt.Sprintf("rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 443 -> 127.0.0.1 port %d\\n", httpsPort))
	}
	if httpPort > 0 && httpPort != 80 {
		pfctlRules.WriteString(fmt.Sprintf("rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 80 -> 127.0.0.1 port %d\\n", httpPort))
	}

	// Combined shell command: update hosts file AND set up pfctl port redirection
	var shellCmd string
	if pfctlRules.Len() > 0 {
		shellCmd = fmt.Sprintf(
			"printf '%%s' '%s' > /etc/hosts && echo '%s' | pfctl -ef -",
			escapedContent,
			pfctlRules.String(),
		)
	} else {
		shellCmd = fmt.Sprintf("printf '%%s' '%s' > /etc/hosts", escapedContent)
	}

	script := fmt.Sprintf(`do shell script "%s" with administrator privileges`, shellCmd)

	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update hosts/pfctl (user may have cancelled): %w, output: %s", err, string(output))
	}

	return nil
}

// AddEntries adds hostname entries to the hosts file using osascript for privilege escalation
func (m *Manager) AddEntries(entries []Entry) error {
	return m.AddEntriesWithPortRedirect(entries, 0, 0)
}

// RemoveEntriesWithPortRedirect removes hosts entries and disables pfctl port redirection
func (m *Manager) RemoveEntriesWithPortRedirect() error {
	// Read current content
	content, err := os.ReadFile(m.hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	// Remove managed block
	cleanedContent := removeExistingManagedBlock(string(content))

	// Check if there's actually anything to remove
	if string(content) == cleanedContent {
		// Still need to disable pfctl
		script := `do shell script "pfctl -d 2>/dev/null || true" with administrator privileges`
		cmd := exec.Command("osascript", "-e", script)
		cmd.Run() // Ignore errors - pfctl might not be enabled
		return nil
	}

	// Escape content for AppleScript
	escapedContent := escapeForAppleScript(cleanedContent)

	// Combined: update hosts file AND disable pfctl
	shellCmd := fmt.Sprintf(
		"printf '%%s' '%s' > /etc/hosts && pfctl -d 2>/dev/null || true",
		escapedContent,
	)
	script := fmt.Sprintf(`do shell script "%s" with administrator privileges`, shellCmd)

	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update hosts file (user may have cancelled): %w, output: %s", err, string(output))
	}

	return nil
}

// RemoveEntries removes all kubikles-managed entries from the hosts file
func (m *Manager) RemoveEntries() error {
	return m.RemoveEntriesWithPortRedirect()
}

// escapeForAppleScript escapes a string for use in AppleScript
func escapeForAppleScript(s string) string {
	// Escape backslashes first, then single quotes
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "'\\''")
	return s
}

// checkPortAvailable checks if a port is available on Unix
func checkPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
