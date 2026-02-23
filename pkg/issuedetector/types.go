package issuedetector

// Severity indicates how critical a finding is.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// Category groups findings by domain.
type Category string

const (
	CategoryNetworking  Category = "networking"
	CategoryWorkloads   Category = "workloads"
	CategoryStorage     Category = "storage"
	CategorySecurity    Category = "security"
	CategoryConfig      Category = "config"
	CategoryDeprecation Category = "deprecation"
)

// ResourceRef identifies a specific Kubernetes resource.
type ResourceRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// Finding represents a single detected issue.
type Finding struct {
	RuleID       string            `json:"ruleID"`
	RuleName     string            `json:"ruleName"`
	Severity     Severity          `json:"severity"`
	Category     Category          `json:"category"`
	Resource     ResourceRef       `json:"resource"`
	Description  string            `json:"description"`
	SuggestedFix string            `json:"suggestedFix,omitempty"`
	Details      map[string]string `json:"details,omitempty"`
	GroupKey     string            `json:"groupKey,omitempty"` // Optional sub-grouping key within a rule (e.g. "host/path")
}

// ScanRequest configures what to scan.
type ScanRequest struct {
	Namespaces    []string   `json:"namespaces"`
	Categories    []Category `json:"categories"`
	DisabledRules []string   `json:"disabledRules"`
	ClusterWide   bool       `json:"clusterWide"`
}

// ScanResult is the output of a scan.
type ScanResult struct {
	Findings         []Finding      `json:"findings"`
	RulesRun         int            `json:"rulesRun"`
	ResourcesFetched map[string]int `json:"resourcesFetched"`
	DurationMs       int64          `json:"durationMs"`
	Errors           []string       `json:"errors,omitempty"`
}

// ScanProgress reports scan progress to the frontend.
type ScanProgress struct {
	Phase       string  `json:"phase"`       // "fetching", "analyzing", "complete"
	Description string  `json:"description"` // Human-readable status
	Percent     float64 `json:"percent"`     // 0-100
}

// RuleInfo describes a rule for the frontend.
type RuleInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Severity    Severity `json:"severity"`
	Category    Category `json:"category"`
	IsBuiltin   bool     `json:"isBuiltin"`
	Requires    []string `json:"requires"`
}

// severityOrder returns a numeric order for sorting (critical first).
func severityOrder(s Severity) int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
	}
}
