//go:build !debugcluster

package k8s

import (
	"errors"

	"k8s.io/client-go/kubernetes"
)

// ErrDebugClusterDisabled is returned when debug cluster is not compiled in.
var ErrDebugClusterDisabled = errors.New("debug cluster not enabled")

// DebugClusterContextName is empty in production builds, causing all
// IsDebugClusterContext checks to be eliminated by the compiler.
const DebugClusterContextName = ""

// IsDebugClusterContext always returns false in production builds.
func IsDebugClusterContext(_ string) bool { return false }

// GetDebugClusterClientset is a no-op stub (unreachable in production builds).
func GetDebugClusterClientset() (kubernetes.Interface, error) { return nil, ErrDebugClusterDisabled }

// switchToDebugCluster is a no-op stub (unreachable in production builds).
func (c *Client) switchToDebugCluster() error { return nil }

// GetDebugClusterConfig is a no-op stub for production builds.
func GetDebugClusterConfig() DebugClusterConfig { return DebugClusterConfig{} }

// RegenerateDebugCluster is a no-op stub for production builds.
func RegenerateDebugCluster(_ DebugClusterConfig) error { return nil }

// debugNodeMetrics and debugPodMetrics: production builds have no debug cluster, so the metrics
// calls never reach them; they answer like a cluster without metrics.
func (c *Client) debugNodeMetrics() (*NodeMetricsResult, error) {
	return &NodeMetricsResult{Available: false, Error: "metrics not available on debug cluster"}, nil
}

func (c *Client) debugPodMetrics() (*PodMetricsResult, error) {
	return &PodMetricsResult{Available: false, Error: "metrics not available on debug cluster"}, nil
}
