//go:build linux

package hosts

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

// getHostsPath returns the path to the hosts file on Linux systems
func getHostsPath() string {
	return "/etc/hosts"
}

// findPrivilegeEscalator returns the available privilege escalation command
// Prefers pkexec (PolicyKit) for GUI environments, falls back to sudo
func findPrivilegeEscalator() (string, []string) {
	// Check for pkexec (PolicyKit) - works well in GUI environments
	if _, err := exec.LookPath("pkexec"); err == nil {
		return "pkexec", nil
	}
	// Fall back to sudo
	if _, err := exec.LookPath("sudo"); err == nil {
		return "sudo", []string{"-n"} // -n for non-interactive (will fail if password needed)
	}
	return "", nil
}

// runPrivileged runs a command with privilege escalation (or directly if already root)
func runPrivileged(command string, args ...string) ([]byte, error) {
	// If already running as root, run directly
	if os.Geteuid() == 0 {
		cmd := exec.Command(command, args...)
		return cmd.CombinedOutput()
	}

	escalator, escalatorArgs := findPrivilegeEscalator()
	if escalator == "" {
		return nil, fmt.Errorf("no privilege escalation method available (need pkexec or sudo)")
	}

	fullArgs := append(escalatorArgs, command)
	fullArgs = append(fullArgs, args...)

	cmd := exec.Command(escalator, fullArgs...)
	return cmd.CombinedOutput()
}

// AddEntriesWithPortRedirect adds hostname entries and sets up port redirection using iptables
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

	// Write to a temp file first
	tmpFile, err := os.CreateTemp("", "hosts-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(newContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Copy temp file to /etc/hosts with elevated privileges
	output, err := runPrivileged("cp", tmpPath, "/etc/hosts")
	if err != nil {
		return fmt.Errorf("failed to update hosts file (privilege escalation may have been cancelled): %w, output: %s", err, string(output))
	}

	// Set up iptables port redirection if needed
	if httpsPort > 0 && httpsPort != 443 {
		if err := setupIptablesRedirect(443, httpsPort); err != nil {
			// Log but don't fail - hosts file update succeeded
			fmt.Printf("Warning: failed to set up HTTPS port redirection: %v\n", err)
		}
	}
	if httpPort > 0 && httpPort != 80 {
		if err := setupIptablesRedirect(80, httpPort); err != nil {
			fmt.Printf("Warning: failed to set up HTTP port redirection: %v\n", err)
		}
	}

	return nil
}

// setupIptablesRedirect sets up port redirection using iptables
func setupIptablesRedirect(fromPort, toPort int) error {
	// Check if iptables is available
	if _, err := exec.LookPath("iptables"); err != nil {
		return fmt.Errorf("iptables not found")
	}

	// Add OUTPUT chain rule for localhost traffic
	// iptables -t nat -A OUTPUT -p tcp --dport 443 -j REDIRECT --to-port 8443
	rule := []string{
		"-t", "nat",
		"-A", "OUTPUT",
		"-o", "lo",
		"-p", "tcp",
		"--dport", fmt.Sprintf("%d", fromPort),
		"-j", "REDIRECT",
		"--to-port", fmt.Sprintf("%d", toPort),
	}

	output, err := runPrivileged("iptables", rule...)
	if err != nil {
		return fmt.Errorf("failed to add iptables rule: %w, output: %s", err, string(output))
	}

	return nil
}

// removeIptablesRedirect removes port redirection rules
func removeIptablesRedirect(fromPort, toPort int) error {
	if _, err := exec.LookPath("iptables"); err != nil {
		return nil // iptables not available, nothing to remove
	}

	// Remove OUTPUT chain rule
	rule := []string{
		"-t", "nat",
		"-D", "OUTPUT",
		"-o", "lo",
		"-p", "tcp",
		"--dport", fmt.Sprintf("%d", fromPort),
		"-j", "REDIRECT",
		"--to-port", fmt.Sprintf("%d", toPort),
	}

	// Ignore errors - rule might not exist
	runPrivileged("iptables", rule...)
	return nil
}

// AddEntries adds hostname entries to the hosts file
func (m *Manager) AddEntries(entries []Entry) error {
	return m.AddEntriesWithPortRedirect(entries, 0, 0)
}

// RemoveEntriesWithPortRedirect removes hosts entries and disables iptables port redirection
func (m *Manager) RemoveEntriesWithPortRedirect() error {
	// Read current content
	content, err := os.ReadFile(m.hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	// Remove managed block
	cleanedContent := removeExistingManagedBlock(string(content))

	// Remove iptables rules (best effort)
	removeIptablesRedirect(443, 8443)
	removeIptablesRedirect(80, 8080)

	// Check if there's actually anything to remove from hosts
	if string(content) == cleanedContent {
		return nil
	}

	// Write to a temp file first
	tmpFile, err := os.CreateTemp("", "hosts-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(cleanedContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Copy temp file to /etc/hosts with elevated privileges
	output, err := runPrivileged("cp", tmpPath, "/etc/hosts")
	if err != nil {
		return fmt.Errorf("failed to update hosts file (privilege escalation may have been cancelled): %w, output: %s", err, string(output))
	}

	return nil
}

// RemoveEntries removes all kubikles-managed entries from the hosts file
func (m *Manager) RemoveEntries() error {
	return m.RemoveEntriesWithPortRedirect()
}

// escapeForShell escapes a string for shell use (not needed for Linux approach but kept for compatibility)
func escapeForShell(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "'\\''")
	return s
}

// checkPortAvailable checks if a port is available on Linux
func checkPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
