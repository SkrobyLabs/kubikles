// Package hosts provides cross-platform management of the system hosts file
// for adding and removing hostname entries used by ingress forwarding.
package hosts

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	// Marker comments used to identify kubikles-managed entries
	BeginMarker = "# BEGIN kubikles-managed"
	EndMarker   = "# END kubikles-managed"
)

// Entry represents a single hosts file entry
type Entry struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

// hostnameRegex validates RFC 1123 hostnames
// Allows: a-z, A-Z, 0-9, hyphen, dot
// Max 253 chars total, max 63 chars per label
var hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

// ValidateHostname checks if a hostname is safe for use in shell commands.
// Returns error if hostname contains characters that could enable command injection.
func ValidateHostname(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	if len(hostname) > 253 {
		return fmt.Errorf("hostname exceeds maximum length of 253 characters")
	}
	if !hostnameRegex.MatchString(hostname) {
		return fmt.Errorf("hostname contains invalid characters: %q", hostname)
	}
	return nil
}

// ValidateEntries validates all entries and returns an error if any hostname is invalid.
// This MUST be called before passing entries to AddEntries or AddEntriesWithPortRedirect.
func ValidateEntries(entries []Entry) error {
	for _, e := range entries {
		if err := ValidateHostname(e.Hostname); err != nil {
			return err
		}
	}
	return nil
}

// Manager handles hosts file operations
type Manager struct {
	hostsPath string
}

// NewManager creates a new hosts file manager
func NewManager() *Manager {
	return &Manager{
		hostsPath: getHostsPath(),
	}
}

// GetManagedEntries returns the current kubikles-managed entries from the hosts file
func (m *Manager) GetManagedEntries() ([]Entry, error) {
	content, err := os.ReadFile(m.hostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read hosts file: %w", err)
	}

	return parseManagedEntries(string(content)), nil
}

// parseManagedEntries extracts kubikles-managed entries from hosts file content
func parseManagedEntries(content string) []Entry {
	var entries []Entry
	var inManagedBlock bool

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, BeginMarker) {
			inManagedBlock = true
			continue
		}
		if strings.HasPrefix(line, EndMarker) {
			inManagedBlock = false
			continue
		}

		if inManagedBlock && line != "" && !strings.HasPrefix(line, "#") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				entries = append(entries, Entry{
					IP:       parts[0],
					Hostname: parts[1],
				})
			}
		}
	}

	return entries
}

// buildEntriesContent creates the content block for hosts file entries
func buildEntriesContent(entries []Entry) string {
	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(BeginMarker)
	sb.WriteString("\n")
	for _, e := range entries {
		sb.WriteString(e.IP)
		sb.WriteByte(' ')
		sb.WriteString(e.Hostname)
		sb.WriteByte('\n')
	}
	sb.WriteString(EndMarker)
	sb.WriteString("\n")
	return sb.String()
}

// removeExistingManagedBlock removes the kubikles-managed block from hosts content
func removeExistingManagedBlock(content string) string {
	var result strings.Builder
	var inManagedBlock bool

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, BeginMarker) {
			inManagedBlock = true
			continue
		}
		if strings.HasPrefix(trimmed, EndMarker) {
			inManagedBlock = false
			continue
		}

		if !inManagedBlock {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	// Trim trailing newlines but keep one
	resultStr := strings.TrimRight(result.String(), "\n")
	if resultStr != "" {
		resultStr += "\n"
	}
	return resultStr
}

// CheckPortAvailable checks if a port is available for binding
func CheckPortAvailable(port int) bool {
	return checkPortAvailable(port)
}
