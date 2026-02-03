package tools

// Truncation limits for AI tool output.
// These limits balance providing useful information while staying within
// typical AI context window constraints.
const (
	// MaxPodLogChars is the maximum characters for pod log output.
	MaxPodLogChars = 8000

	// MaxYAMLChars is the maximum characters for YAML manifest output.
	MaxYAMLChars = 12000

	// MaxDescribeChars is the maximum characters for resource description output.
	MaxDescribeChars = 12000

	// MaxMetricsChars is the maximum characters for metrics output.
	MaxMetricsChars = 10000

	// MaxPodMetricsChars is the maximum characters for pod metrics output.
	MaxPodMetricsChars = 10000

	// MaxDependenciesChars is the maximum characters for dependency graph output.
	MaxDependenciesChars = 6000

	// MaxCRDListChars is the maximum characters for CRD list output.
	MaxCRDListChars = 12000

	// MaxCustomResourceListChars is the maximum characters for custom resource list output.
	MaxCustomResourceListChars = 12000

	// MaxToolOutputChars is the default maximum characters for general tool output.
	MaxToolOutputChars = 10000
)
