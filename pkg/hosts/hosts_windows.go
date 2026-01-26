//go:build windows

package hosts

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// getHostsPath returns the path to the hosts file on Windows
func getHostsPath() string {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	return filepath.Join(systemRoot, "System32", "drivers", "etc", "hosts")
}

// AddEntriesWithPortRedirect adds hostname entries and sets up port redirection using netsh portproxy
func (m *Manager) AddEntriesWithPortRedirect(entries []Entry, httpsPort, httpPort int) error {
	if len(entries) == 0 {
		return nil
	}

	// SECURITY: Validate all hostnames before using in shell commands
	if err := ValidateEntries(entries); err != nil {
		return fmt.Errorf("invalid hostname: %w", err)
	}

	// First, read current content and remove any existing managed block
	content, err := os.ReadFile(m.hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	cleanedContent := removeExistingManagedBlock(string(content))
	newContent := cleanedContent + buildEntriesContent(entries)

	// Write hosts content to temp file
	tmpFile, err := os.CreateTemp("", "kubikles-hosts-*.txt")
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

	// Build PowerShell script that:
	// 1. Updates hosts file
	// 2. Sets up netsh portproxy rules for port redirection
	hostsPathEscaped := strings.ReplaceAll(m.hostsPath, `\`, `\\`)
	tmpPathEscaped := strings.ReplaceAll(tmpPath, `\`, `\\`)

	var scriptBuilder strings.Builder
	scriptBuilder.WriteString(fmt.Sprintf("Copy-Item -Path '%s' -Destination '%s' -Force; ", tmpPathEscaped, hostsPathEscaped))

	// Add port proxy rules (443->httpsPort, 80->httpPort)
	if httpsPort > 0 && httpsPort != 443 {
		scriptBuilder.WriteString(fmt.Sprintf(
			"netsh interface portproxy add v4tov4 listenport=443 listenaddress=127.0.0.1 connectport=%d connectaddress=127.0.0.1; ",
			httpsPort,
		))
	}
	if httpPort > 0 && httpPort != 80 {
		scriptBuilder.WriteString(fmt.Sprintf(
			"netsh interface portproxy add v4tov4 listenport=80 listenaddress=127.0.0.1 connectport=%d connectaddress=127.0.0.1; ",
			httpPort,
		))
	}

	psScript := scriptBuilder.String()
	psCommand := fmt.Sprintf(
		`Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile', '-Command', '%s'`,
		strings.ReplaceAll(psScript, "'", "''"),
	)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update hosts/portproxy (user may have cancelled UAC): %w, output: %s", err, string(output))
	}

	return nil
}

// AddEntries adds hostname entries to the hosts file using PowerShell with UAC elevation
func (m *Manager) AddEntries(entries []Entry) error {
	return m.addEntriesInternal(entries)
}

func (m *Manager) addEntriesInternal(entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}

	// SECURITY: Validate all hostnames before using in shell commands
	if err := ValidateEntries(entries); err != nil {
		return fmt.Errorf("invalid hostname: %w", err)
	}

	// First, read current content and remove any existing managed block
	content, err := os.ReadFile(m.hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	cleanedContent := removeExistingManagedBlock(string(content))
	newContent := cleanedContent + buildEntriesContent(entries)

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "kubikles-hosts-*.txt")
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

	// PowerShell command to copy with elevation
	// We use Start-Process with -Verb RunAs to trigger UAC
	hostsPathEscaped := strings.ReplaceAll(m.hostsPath, `\`, `\\`)
	tmpPathEscaped := strings.ReplaceAll(tmpPath, `\`, `\\`)

	psScript := fmt.Sprintf(
		`Copy-Item -Path '%s' -Destination '%s' -Force`,
		tmpPathEscaped, hostsPathEscaped,
	)

	// Encode the command for passing to elevated PowerShell
	psCommand := fmt.Sprintf(
		`Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile', '-Command', '%s'`,
		strings.ReplaceAll(psScript, "'", "''"),
	)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update hosts file (user may have cancelled UAC): %w, output: %s", err, string(output))
	}

	return nil
}

// RemoveEntries removes all Kubikles-managed entries from the hosts file and cleans up portproxy rules
func (m *Manager) RemoveEntries() error {
	// Read current content
	content, err := os.ReadFile(m.hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	// Remove managed block
	cleanedContent := removeExistingManagedBlock(string(content))

	// Build PowerShell script to:
	// 1. Update hosts file (if needed)
	// 2. Remove netsh portproxy rules
	var scriptBuilder strings.Builder

	// Only update hosts file if there's something to remove
	if string(content) != cleanedContent {
		// Write to temp file
		tmpFile, err := os.CreateTemp("", "kubikles-hosts-*.txt")
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

		hostsPathEscaped := strings.ReplaceAll(m.hostsPath, `\`, `\\`)
		tmpPathEscaped := strings.ReplaceAll(tmpPath, `\`, `\\`)
		scriptBuilder.WriteString(fmt.Sprintf("Copy-Item -Path '%s' -Destination '%s' -Force; ", tmpPathEscaped, hostsPathEscaped))
	}

	// Always try to remove portproxy rules (ignore errors if they don't exist)
	scriptBuilder.WriteString("netsh interface portproxy delete v4tov4 listenport=443 listenaddress=127.0.0.1 2>$null; ")
	scriptBuilder.WriteString("netsh interface portproxy delete v4tov4 listenport=80 listenaddress=127.0.0.1 2>$null; ")

	psScript := scriptBuilder.String()
	if psScript == "" {
		return nil // Nothing to do
	}

	psCommand := fmt.Sprintf(
		`Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile', '-Command', '%s'`,
		strings.ReplaceAll(psScript, "'", "''"),
	)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to cleanup hosts/portproxy (user may have cancelled UAC): %w, output: %s", err, string(output))
	}

	return nil
}

// checkPortAvailable checks if a port is available on Windows
func checkPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
