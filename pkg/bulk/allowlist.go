package bulk

import "strings"

// The call route only exposes read methods. A method is allowed when it
// matches one of the naming rules below or is in extraAllowed. The frontend
// mirrors these rules in frontend/src/lib/bulk/bulkMethods.ts and a vitest
// parity test parses this file, so keep the declarations below simple.

const listPrefix = "List"

// yamlSuffixes: Get<Kind>Yaml and Get<Kind>YAML.
var yamlSuffixes = []string{"Yaml", "YAML"}

// historySuffixes: Get<X>History and Get<X>HistoryRange.
var historySuffixes = []string{"History", "HistoryRange"}

// extraAllowed is the hand-written set of read methods that do not follow the
// naming rules.
var extraAllowed = []string{
	"GetPodMetrics",
	"GetNodeMetrics",
	"GetPodLogsBefore",
	"GetPodLogsAfter",
	"GetAllContainersLogsBefore",
	"GetAllContainersLogsAfter",
	"GetAllPodsLogsBefore",
	"GetAllPodsLogsAfter",
}

// denied lists methods that match a naming rule above but are not read-only
// (ListPodFiles execs in the pod and may add an ephemeral container). It is
// checked before every rule.
var denied = []string{
	"ListPodFiles",
}

// forbiddenPrefixes must never be reachable through the call route; the test
// asserts that no allowed name starts with one of them.
var forbiddenPrefixes = []string{
	"Create", "Delete", "Update", "Apply", "Restart", "Scale", "Evict", "Exec",
	"Set", "Save", "Import", "Install", "Uninstall", "Start", "Stop", "Cancel",
}

// IsAllowed reports whether name may be called through /_kubikles/v1/call.
func IsAllowed(name string) bool {
	if name == "" {
		return false
	}
	for _, d := range denied {
		if d == name {
			return false
		}
	}
	for _, p := range forbiddenPrefixes {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	if strings.HasPrefix(name, listPrefix) {
		return true
	}
	if strings.HasPrefix(name, "Get") {
		for _, s := range yamlSuffixes {
			if strings.HasSuffix(name, s) {
				return true
			}
		}
		for _, s := range historySuffixes {
			if strings.HasSuffix(name, s) {
				return true
			}
		}
	}
	for _, e := range extraAllowed {
		if e == name {
			return true
		}
	}
	return false
}

// ExtraAllowed returns the hand-written extras (for tests).
func ExtraAllowed() []string { return append([]string(nil), extraAllowed...) }
