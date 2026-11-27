package rows

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// SortSpec is the list sort: a column key and "asc" or "desc". An empty key means unsorted.
type SortSpec struct {
	Key string `json:"key"`
	Dir string `json:"dir"`
}

// ViewSpec describes one list view. Everything a client retains in order to re-open the view
// after a failure lives here.
type ViewSpec struct {
	// ViewID is chosen by the client (a UUID). Opening an id that exists replaces that view.
	ViewID string `json:"viewId"`
	// ClientToken groups the views of one client for caps and lease reaping.
	ClientToken string `json:"clientToken"`
	// ClientID is the WebSocket client id in server mode; empty on desktop.
	ClientID string `json:"clientId,omitempty"`
	Type     string `json:"type"`
	// Namespaces is the namespace selection: [] selects nothing, ["*"] everything, otherwise the
	// namespace names. It is ignored for cluster-scoped types.
	Namespaces    []string           `json:"namespaces"`
	Search        string             `json:"search"`
	ColumnFilters []ColumnFilterSpec `json:"columnFilters"`
	Sort          SortSpec           `json:"sort"`
	// Columns are the cells to project, in order. Empty means every column of the type.
	Columns []string `json:"columns"`
	// Locale is the client locale used for name collation.
	Locale string `json:"locale"`
	// Hide names the type's presets that remove rows from the scope ("helm" on secrets).
	Hide []string `json:"hide"`
	// CRD describes a custom resource when Type is "crd:group/version/resource". It is optional:
	// without it the backend resolves the resource itself.
	CRD *CRDDescriptor `json:"crd,omitempty"`
	// NotifyOn asks the view to report transitions of its objects in notices besides the counts:
	// "pods" (pod views only) reports restarts of visible pods, pods that disappeared and pods
	// that appeared, which the pod notification bell turns into messages.
	NotifyOn []string `json:"notifyOn,omitempty"`
}

// ViewPatch changes parts of a view. Nil fields stay as they are.
type ViewPatch struct {
	Namespaces    *[]string           `json:"namespaces,omitempty"`
	Search        *string             `json:"search,omitempty"`
	ColumnFilters *[]ColumnFilterSpec `json:"columnFilters,omitempty"`
	Sort          *SortSpec           `json:"sort,omitempty"`
	Columns       *[]string           `json:"columns,omitempty"`
	Locale        *string             `json:"locale,omitempty"`
	Hide          *[]string           `json:"hide,omitempty"`
	NotifyOn      *[]string           `json:"notifyOn,omitempty"`
}

// Load phases of a view, mapped one to one onto the legacy load state.
const (
	PhaseLoading    = "loading"
	PhaseRefreshing = "refreshing"
	PhaseComplete   = "complete"
	PhaseIncomplete = "incomplete"
)

// ViewState is the cheap description of a view that travels with every response and notice.
type ViewState struct {
	Version uint64 `json:"version"`
	// Total is the number of rows in the namespace scope, Filtered the number passing search and
	// column filters.
	Total     int    `json:"total"`
	Filtered  int    `json:"filtered"`
	Phase     string `json:"phase"`
	Loaded    int    `json:"loaded"`
	Expected  int    `json:"expected"`
	Error     string `json:"error,omitempty"`
	Forbidden bool   `json:"forbidden,omitempty"`
	// Degraded is set when the shard fell back from Table or metadata to full objects.
	Degraded bool `json:"degraded,omitempty"`
	// Filterable says which columns currently offer a filter (select columns need at least one
	// and at most MaxSelectFilterValues distinct values).
	Filterable map[string]bool        `json:"filterable,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
	// Overlays reports each overlay source of the type (pod metrics, node metrics): whether it
	// could answer yet (null while unknown). It carries the "metrics available" state of the
	// legacy lists.
	Overlays map[string]OverlayInfo `json:"overlays,omitempty"`
}

// ViewOpenResult is returned when a view opens.
type ViewOpenResult struct {
	ViewID string `json:"viewId"`
	// Columns lists the projected columns; cells in windows follow this order.
	Columns       []string        `json:"columns"`
	SchemaVersion string          `json:"schemaVersion"`
	State         ViewState       `json:"state"`
	Filterable    map[string]bool `json:"filterable"`
	// Schema is the row schema of a dynamic type (custom resources), whose columns come from
	// the CRD and so are not in the generated schema file. Static types leave it nil.
	Schema *Schema `json:"schema,omitempty"`
}

// WindowRow is one projected row.
type WindowRow struct {
	Seq  uint64        `json:"seq"`
	NS   string        `json:"ns"`
	Name string        `json:"name"`
	TS   string        `json:"ts"`
	Del  bool          `json:"del"`
	C    []interface{} `json:"c"`
}

// ViewWindow is a slice of the filtered, sorted list. Order lists every uid of the window; Rows
// holds only rows the client may not have yet (see View.Window).
type ViewWindow struct {
	Version  uint64               `json:"version"`
	Total    int                  `json:"total"`
	Filtered int                  `json:"filtered"`
	Offset   int                  `json:"offset"`
	Order    []string             `json:"order"`
	Rows     map[string]WindowRow `json:"rows"`
	State    ViewState            `json:"state"`
}

// RowRef identifies one row without its cells.
type RowRef struct {
	UID  string `json:"uid"`
	NS   string `json:"ns"`
	Name string `json:"name"`
}

// Facet is the value list of a select column: predefined values first in declared order, then the
// dynamic ones sorted by the collator, each with its count over the whole scope. Suppressed is
// set when the column has no values or more than MaxSelectFilterValues.
type Facet struct {
	Column     string         `json:"column"`
	Values     []string       `json:"values"`
	Counts     map[string]int `json:"counts"`
	Suppressed bool           `json:"suppressed"`
}

// SelectionAck answers SetViewSelection.
type SelectionAck struct {
	// Selected is the number of selected uids still present in the view's scope.
	Selected int `json:"selected"`
	// Stale lists selected uids no longer in the scope.
	Stale []string `json:"stale"`
}

// ViewNotice is the tiny update the notifier sends. It never carries rows.
type ViewNotice struct {
	V        string `json:"v"`
	Version  uint64 `json:"version"`
	Total    int    `json:"total"`
	Filtered int    `json:"filtered"`
	Phase    string `json:"phase"`
	Loaded   int    `json:"loaded"`
	Expected int    `json:"expected"`
	// Closed is set when the server closed the view (reason "context", "expired", "client").
	Closed string `json:"closed,omitempty"`
	// SelectionStale lists (up to a cap) selected uids that left the scope.
	SelectionStale []string `json:"selectionStale,omitempty"`
	// Overlays repeats ViewState.Overlays so availability changes reach the client without a window fetch.
	Overlays map[string]OverlayInfo `json:"overlays,omitempty"`
	// Transitions are the pod events since the previous notice (ViewSpec.NotifyOn).
	Transitions []Transition `json:"transitions,omitempty"`
	// WindowChanged says the client's last window is affected (rows or order); without it only
	// the counts moved.
	WindowChanged bool `json:"windowChanged,omitempty"`
}

// ObjectRef addresses one object for GetResourceObject.
type ObjectRef struct {
	Type      string `json:"type"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
}

// Errors the bindings and routes map to wire statuses.
var (
	// ErrViewNotFound is returned for an unknown, expired or closed view (HTTP 410).
	ErrViewNotFound = errors.New("view not found")
	// ErrViewChanged means the list changed since the client last saw it, so row indexes the
	// client holds no longer address the rows it displays.
	ErrViewChanged = errors.New("view changed: refresh and retry")
	// ErrTooManyViews is returned when a per-client or global cap is reached (HTTP 429).
	ErrTooManyViews = errors.New("too many open views")
)

// UnknownTypeError is returned when a type has no registered spec.
type UnknownTypeError struct{ Type string }

func (e *UnknownTypeError) Error() string { return fmt.Sprintf("no row spec for type %q", e.Type) }

// UnknownColumnError is returned for a column key the type does not declare.
type UnknownColumnError struct{ Type, Column string }

func (e *UnknownColumnError) Error() string {
	return fmt.Sprintf("type %q has no column %q", e.Type, e.Column)
}

// UnknownPresetError is returned for an unknown Hide preset.
type UnknownPresetError struct{ Type, Preset string }

func (e *UnknownPresetError) Error() string {
	return fmt.Sprintf("type %q has no preset %q", e.Type, e.Preset)
}

// UnsupportedNotifyError is returned for a NotifyOn value the type does not support.
type UnsupportedNotifyError struct{ Type, Kind string }

func (e *UnsupportedNotifyError) Error() string {
	return fmt.Sprintf("type %q cannot notify on %q", e.Type, e.Kind)
}

// overlaySig is a comparable rendering of the overlay availability for change detection.
func overlaySig(m map[string]OverlayInfo) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		switch a := m[k].Available; {
		case a == nil:
			b.WriteString("=?;")
		case *a:
			b.WriteString("=1;")
		default:
			b.WriteString("=0;")
		}
	}
	return b.String()
}
