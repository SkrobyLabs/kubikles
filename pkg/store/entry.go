package store

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// Entry is one stored object. It is immutable once published: a change to the
// object produces a new Entry with a higher Seq, so a reader that holds an
// *Entry can use it without locking or copying.
type Entry struct {
	UID       string
	Namespace string
	Name      string
	RV        string
	// Seq is the shard-wide change counter value at which this entry was stored.
	Seq uint64
	// Obj is the typed object, *unstructured.Unstructured, or
	// *metav1.PartialObjectMetadata (Table and metadata modes). Managed fields
	// are already stripped. Treat it as read-only.
	Obj runtime.Object
	// Cells holds the server-printed Table cells in the shard's column order.
	// Nil outside Table mode.
	Cells []interface{}
}

// ID is the key an entry is stored under: the UID, or namespace/name for the
// objects (fake clientsets, some aggregated APIs) that have none.
func (e *Entry) ID() string {
	if e.UID != "" {
		return e.UID
	}
	return e.Namespace + "/" + e.Name
}

// ChangeType classifies a Change.
type ChangeType int

const (
	ChangeAdded ChangeType = iota
	ChangeModified
	ChangeDeleted
	// ChangeReset tells a subscriber it missed changes (queue overflow) or the
	// shard was cleared (mode switch); it must re-read Snapshot. Old and New
	// are nil.
	ChangeReset
)

func (t ChangeType) String() string {
	switch t {
	case ChangeAdded:
		return "Added"
	case ChangeModified:
		return "Modified"
	case ChangeDeleted:
		return "Deleted"
	case ChangeReset:
		return "Reset"
	}
	return "Unknown"
}

// Change is one stored change. Added has only New, Deleted only Old, Modified
// both. Seq is the shard counter value of the change, so a subscriber that
// re-read a snapshot at sequence S ignores changes with Seq <= S.
type Change struct {
	Type ChangeType
	Old  *Entry
	New  *Entry
	Seq  uint64
}
