package store

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	"kubikles/pkg/k8s"
)

// Mode is how much of each object a shard holds.
type Mode int

const (
	// ModeFull holds complete typed (or unstructured) objects.
	ModeFull Mode = iota
	// ModeTable holds the server-printed columns plus object metadata.
	ModeTable
	// ModeMetadata holds only object metadata.
	ModeMetadata
)

func (m Mode) String() string {
	switch m {
	case ModeFull:
		return "full"
	case ModeTable:
		return "table"
	case ModeMetadata:
		return "metadata"
	}
	return "unknown"
}

// strength orders modes by how much data they carry, so a shard in a stronger
// mode can serve a weaker request.
func (m Mode) strength() int {
	switch m {
	case ModeMetadata:
		return 1
	case ModeTable:
		return 2
	default:
		return 3
	}
}

// ErrModeUnsupported is returned by a Table or metadata source when the server
// (or a proxy or aggregated API) cannot answer in that representation. The
// shard reacts by switching to full objects.
var ErrModeUnsupported = k8s.ErrTableUnsupported

// ListOpts parameterises Source.List.
type ListOpts struct {
	// PageSize is the LIST limit. Zero means one unbounded LIST, which keeps the
	// request eligible for the API server watch cache.
	PageSize  int64
	Namespace string
}

// Page is one batch of listed objects.
type Page struct {
	Items []runtime.Object
	// Loaded is the cumulative number of items delivered so far.
	Loaded int
	// Remaining is the server-reported number of items still to come, nil when
	// unknown.
	Remaining *int64
}

// Source lists and watches one resource type in one namespace.
type Source interface {
	Mode() Mode
	// List delivers every page to onPage in order and returns the list
	// resourceVersion (the snapshot version of the first page).
	List(ctx context.Context, opts ListOpts, onPage func(Page)) (rv string, err error)
	// Watch opens a watch from rv. Events carry the same object representation
	// List produces.
	Watch(ctx context.Context, rv string) (watch.Interface, error)
}

// TableRow is the object a Table source produces for one row. It carries the
// row's metadata and cells plus the column definitions in force.
type TableRow struct {
	Meta    *metav1.PartialObjectMetadata
	Cells   []interface{}
	Columns []metav1.TableColumnDefinition
}

func (r *TableRow) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }

// GetObjectMeta lets meta.Accessor read identity from a row.
func (r *TableRow) GetObjectMeta() metav1.Object { return &r.Meta.ObjectMeta }

func (r *TableRow) DeepCopyObject() runtime.Object {
	if r == nil {
		return nil
	}
	out := &TableRow{Columns: r.Columns}
	if r.Meta != nil {
		out.Meta = r.Meta.DeepCopy()
	}
	if r.Cells != nil {
		out.Cells = append([]interface{}(nil), r.Cells...)
	}
	return out
}

func isModeUnsupported(err error) bool {
	return err != nil && errors.Is(err, ErrModeUnsupported)
}
