// Package rows is the Go half of the windowed resource list. A Spec declares, once per resource
// type, the columns a list shows, how each cell is derived from a stored object, how it sorts
// and filters, and which fields the search language can address. The view engine in this
// package keeps a sorted, filtered index over a shared store shard and serves windows of it.
//
// Every semantic here is proven bug-for-bug against the legacy TypeScript list code through
// golden files (testdata/golden, produced by frontend/src/test/golden/generateGolden.test.ts).
package rows

import (
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubikles/pkg/rows/query"
	"kubikles/pkg/store"
)

// Kind is the value kind of a column; it decides how the generated schema types the cell.
type Kind string

const (
	KindString Kind = "string"
	KindNumber Kind = "number"
	KindTime   Kind = "time"
	KindBool   Kind = "bool"
)

// FilterType is the column filter a list offers: the string values match detectColumnFilterTypes
// in the frontend, with "none" for columns that cannot be filtered.
type FilterType string

const (
	FilterSelect  FilterType = "select"
	FilterRegex   FilterType = "regex"
	FilterNumeric FilterType = "numeric"
	FilterNone    FilterType = "none"
)

// MaxSelectFilterValues is the largest number of distinct values a select filter offers; a
// column with more gets no filter (MAX_SELECT_FILTER_VALUES in resourceListPipeline.ts).
const MaxSelectFilterValues = 200

// Row is one stored object as seen by column and search functions.
type Row struct {
	E *store.Entry
	// M is the object's metadata accessor (typed objects and PartialObjectMetadata alike).
	M metav1.Object
	// TableCols maps server Table column names to cell indexes. It is nil outside Table mode
	// (including a Table shard degraded to full objects, where Cells is nil too).
	TableCols map[string]int
	// Now is the evaluation time used by age-dependent numeric filters.
	Now time.Time
	// OV holds the current snapshot of each overlay source the view is subscribed to. It is nil
	// for rows built outside a view; overlay lookups then find nothing.
	OV map[string]*OverlaySnapshot

	// pods, podSel and the memo behind Pods: the derived-column source of a view of a workload
	// type. Nil outside a view (or in a type without a derived column).
	pods     *podIndex
	podSel   func(*Row) map[string]string
	podsMemo []PodRef
	podsDone bool
}

// Pods returns the pods the row's workload selects (see Derived). It is nil outside a view and
// for a workload without matchLabels. The result is computed once per Row value.
func (r *Row) Pods() []PodRef {
	if r.pods == nil || r.podSel == nil {
		return nil
	}
	if !r.podsDone {
		r.podsMemo = r.pods.match(r.Namespace(), r.podSel(r))
		r.podsDone = true
	}
	return r.podsMemo
}

// Overlay returns the snapshot of an overlay source, nil when there is none.
func (r *Row) Overlay(name string) *OverlaySnapshot {
	if r.OV == nil {
		return nil
	}
	return r.OV[name]
}

// NewRow wraps an entry. Entries whose object has no metadata accessor yield a Row with a nil M;
// Name and friends then return empty strings.
func NewRow(e *store.Entry, tableCols map[string]int, now time.Time) Row {
	m, _ := meta.Accessor(e.Obj)
	return Row{E: e, M: m, TableCols: tableCols, Now: now}
}

func (r *Row) Name() string {
	if r.M == nil {
		return r.E.Name
	}
	return r.M.GetName()
}

func (r *Row) Namespace() string {
	if r.M == nil {
		return r.E.Namespace
	}
	return r.M.GetNamespace()
}

func (r *Row) UID() string {
	if r.M == nil {
		return r.E.UID
	}
	return string(r.M.GetUID())
}

// CreationTimestamp is metadata.creationTimestamp as the legacy list saw it: an RFC 3339 UTC
// string, or "" when the object has none (JSON null).
func (r *Row) CreationTimestamp() string {
	if r.M == nil {
		return ""
	}
	return TimeString(r.M.GetCreationTimestamp())
}

// Deleting reports whether metadata.deletionTimestamp is set.
func (r *Row) Deleting() bool { return r.M != nil && r.M.GetDeletionTimestamp() != nil }

// TableCell returns the server-printed cell for a Table column name.
func (r *Row) TableCell(column string) (interface{}, bool) {
	if r.TableCols == nil || r.E.Cells == nil {
		return nil, false
	}
	i, ok := r.TableCols[column]
	if !ok || i >= len(r.E.Cells) {
		return nil, false
	}
	return r.E.Cells[i], true
}

// TimeString formats a metav1.Time the way encoding/json does, "" for the zero time.
func TimeString(t metav1.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// Column declares one list column.
type Column struct {
	// Key is identical to the key of the TypeScript column.
	Key  string
	Kind Kind
	// CellType is the TypeScript type of the cell in the generated schema, e.g. "string",
	// "number" or "string | null".
	CellType string
	// Cell is the column's getValue: the projected cell and the primary sort key. A nil
	// result stands for a JavaScript undefined or null.
	Cell func(r *Row) interface{}
	// Filter overrides the string select and regex filters see. When nil the legacy default
	// applies: the name and namespace columns use metadata, any other column its Cell when that
	// is a string and "" otherwise.
	Filter func(r *Row) string
	// Numeric overrides the number numeric filters see. When nil the legacy default applies: the
	// age column is hours since metadata.creationTimestamp, any other column its Cell when that
	// is a number. ok=false means NaN, which never matches.
	Numeric func(r *Row) (float64, bool)
	// FilterType is the filter this column offers.
	FilterType FilterType
	// Predefined lists select values shown with count 0 even when no row has them.
	Predefined []string
	// Volatile marks a column whose value changes without the object changing (it reads an
	// overlay). A view re-sorts only while it is sorted by a volatile column and re-evaluates
	// filters only while a filter targets one; other columns keep their position on a tick.
	Volatile bool
	// DefaultHidden marks a column the list hides until the user shows it (printer columns
	// with a priority above zero). Only dynamic schemas carry it.
	DefaultHidden bool
	// Label is the header text for columns of dynamic schemas (a printer column's name).
	Label string
}

// Derived declares columns computed from another resource type: the Pods column of a workload
// shows the pods its selector matches. The view keeps an index of the source type next to its own
// rows (podindex.go) and re-derives only the workloads a changed pod can belong to. Columns read
// the derived value through Row.Pods. It is validated by Register.
type Derived struct {
	// Columns are the columns fed by the derived value; they are marked volatile.
	Columns []string
	// From is the resource type the value is derived from. Only "pods" is supported.
	From string
	// Selector returns the matchLabels of the row's workload, nil or empty when it selects nothing.
	Selector func(r *Row) map[string]string
}

// Overlay declares columns fed by a periodic out-of-band source (pod and node metrics). The
// source publishes records through an OverlayHub under Name; Key maps an object to the record
// key. Every column that reads the record (cells, sort values, numeric filter values) is listed
// in Columns, so a tick re-derives only those.
type Overlay struct {
	Name    string
	Columns []string
	// Key returns the record key of the object with the given namespace and name.
	Key func(namespace, name string) string
}

// Preset is a named predicate that removes rows from the view's scope (the list toggles such as
// "Hide Helm" on secrets). It acts before counts and facets, like filtering the data handed to
// the legacy list.
type Preset struct {
	Name string
	Hide func(r *Row) bool
}

// SearchField is one field of the search language.
type SearchField = query.Field[*Row]

// Spec declares one resource type.
type Spec struct {
	// Type is the store/watcher type string ("namespaces", "configmaps").
	Type       string
	Namespaced bool
	// Needs is the lightest store representation the columns and search fields can be derived
	// from. A Table or metadata shard that degrades to full objects must still work, so cell
	// functions handle both.
	Needs    store.Mode
	Columns  []Column
	Search   []SearchField
	Derived  []Derived
	Overlays []Overlay
	Presets  []Preset
	// FullPrune, when set, is the list of top-level fields a full-object shard keeps besides
	// metadata (apiVersion and kind are kept too). It bounds memory for custom resources whose
	// columns read a few fields of large objects.
	FullPrune []string
	// Notes records intentional differences from the legacy list.
	Notes string
}

// Column returns the column with the key, or nil.
func (s *Spec) Column(key string) *Column {
	for i := range s.Columns {
		if s.Columns[i].Key == key {
			return &s.Columns[i]
		}
	}
	return nil
}

func (s *Spec) preset(name string) *Preset {
	for i := range s.Presets {
		if s.Presets[i].Name == name {
			return &s.Presets[i]
		}
	}
	return nil
}

// SchemaVersion identifies the column layout. The generated TypeScript schema embeds the same
// value, so a stale frontend can tell it no longer matches the backend.
func (s *Spec) SchemaVersion() string {
	h := fnv32a()
	write := func(parts ...string) {
		for _, p := range parts {
			h.add(p)
			h.add("\x00")
		}
	}
	write(s.Type, fmt.Sprint(s.Namespaced))
	for _, c := range s.Columns {
		write(c.Key, string(c.Kind), c.CellType, string(c.FilterType), fmt.Sprint(c.Volatile))
		for _, p := range c.Predefined {
			write(p)
		}
	}
	for _, f := range s.Search {
		write(f.Name)
		al := append([]string(nil), f.Aliases...)
		sort.Strings(al)
		write(al...)
	}
	return fmt.Sprintf("%08x", h.sum())
}

type fnv struct{ h uint32 }

func fnv32a() *fnv { return &fnv{h: 2166136261} }
func (f *fnv) add(s string) {
	for i := 0; i < len(s); i++ {
		f.h ^= uint32(s[i])
		f.h *= 16777619
	}
}
func (f *fnv) sum() uint32 { return f.h }

// SchemaColumn is a column of a Schema.
type SchemaColumn struct {
	Key        string   `json:"key"`
	Kind       string   `json:"kind"`
	FilterType string   `json:"filterType"`
	Predefined []string `json:"predefined"`
	Volatile   bool     `json:"volatile"`
	Hidden     bool     `json:"hidden,omitempty"`
	Label      string   `json:"label,omitempty"`
}

// SchemaSearchField is a search field of a Schema.
type SchemaSearchField struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

// Schema is the wire form of a spec, the same shape as an entry of the generated
// rowSchemas.ts, for specs built at runtime.
type Schema struct {
	Type          string              `json:"type"`
	Namespaced    bool                `json:"namespaced"`
	SchemaVersion string              `json:"schemaVersion"`
	Migrated      bool                `json:"migrated"`
	Columns       []SchemaColumn      `json:"columns"`
	Search        []SchemaSearchField `json:"search"`
	Presets       []string            `json:"presets"`
}

// Schema renders the spec as a Schema.
func (s *Spec) Schema() *Schema {
	out := &Schema{Type: s.Type, Namespaced: s.Namespaced, SchemaVersion: s.SchemaVersion(), Migrated: true,
		Columns: []SchemaColumn{}, Search: []SchemaSearchField{}, Presets: []string{}}
	for _, c := range s.Columns {
		pre := c.Predefined
		if pre == nil {
			pre = []string{}
		}
		out.Columns = append(out.Columns, SchemaColumn{Key: c.Key, Kind: string(c.Kind), FilterType: string(c.FilterType),
			Predefined: pre, Volatile: c.Volatile, Hidden: c.DefaultHidden, Label: c.Label})
	}
	for _, f := range s.Search {
		al := f.Aliases
		if al == nil {
			al = []string{}
		}
		out.Search = append(out.Search, SchemaSearchField{Name: f.Name, Aliases: al})
	}
	for _, p := range s.Presets {
		out.Presets = append(out.Presets, p.Name)
	}
	return out
}
