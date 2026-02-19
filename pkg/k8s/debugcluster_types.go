package k8s

// DebugClusterConfig controls how many resources the debug cluster generates.
// All per-namespace counts are multiplied by the number of namespaces.
type DebugClusterConfig struct {
	Namespaces   int `json:"namespaces"`
	Pods         int `json:"pods"`         // per namespace
	Deployments  int `json:"deployments"`  // per namespace
	Services     int `json:"services"`     // per namespace
	ConfigMaps   int `json:"configMaps"`   // per namespace
	Secrets      int `json:"secrets"`      // per namespace
	Nodes        int `json:"nodes"`        // cluster-scoped
	StatefulSets int `json:"statefulSets"` // per namespace
	DaemonSets   int `json:"daemonSets"`   // per namespace
	Jobs         int `json:"jobs"`         // per namespace
	ReplicaSets  int `json:"replicaSets"`  // per namespace
}
