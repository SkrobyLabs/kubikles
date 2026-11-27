// Package watchevents holds the payload types of the watcher-status and
// watcher-error events. They are shared by the legacy watcher manager and the
// shared store so the frontend sees one shape whichever produced the event.
package watchevents

// Error is emitted as "watcher-error" when a watcher encounters an error.
type Error struct {
	ResourceType       string `json:"resourceType"`       // Resource type that failed
	Namespace          string `json:"namespace"`          // Namespace being watched
	Error              string `json:"error"`              // Error message
	Recoverable        bool   `json:"recoverable"`        // Whether the watcher will retry
	Premature          bool   `json:"premature"`          // Stream opened but closed unexpectedly
	ReceivedAny        bool   `json:"receivedAny"`        // Stream delivered an event before closing
	OpenDurationMillis int64  `json:"openDurationMillis"` // Time the stream remained open
	Context            string `json:"context"`            // K8s context this error belongs to
}

// Status is emitted as "watcher-status" when watcher status changes.
type Status struct {
	ResourceType string `json:"resourceType"` // Resource type
	Namespace    string `json:"namespace"`    // Namespace being watched
	Status       string `json:"status"`       // "connected", "reconnecting", "stopped"
	Context      string `json:"context"`      // K8s context this status belongs to
}
