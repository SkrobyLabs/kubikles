package rows

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(persistentVolumeClaimsSpec()) }

func pvcObj(r *Row) *corev1.PersistentVolumeClaim {
	o, _ := r.E.Obj.(*corev1.PersistentVolumeClaim)
	return o
}

// pvcStorage is the storage entry of a resource list, "" when absent.
func pvcStorage(l corev1.ResourceList) string {
	if q, ok := l[corev1.ResourceStorage]; ok {
		return q.String()
	}
	return ""
}

// pvcCapacity is status.capacity.storage || spec.resources.requests.storage || empty string.
func pvcCapacity(r *Row) string {
	if o := pvcObj(r); o != nil {
		return orDefault(pvcStorage(o.Status.Capacity), pvcStorage(o.Spec.Resources.Requests))
	}
	return ""
}

func pvcString(key string, f func(o *corev1.PersistentVolumeClaim) string) Column {
	return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
		Cell: func(r *Row) interface{} {
			if o := pvcObj(r); o != nil {
				return f(o)
			}
			return ""
		}}
}

func persistentVolumeClaimsSpec() *Spec {
	return &Spec{
		Type:       "persistentvolumeclaims",
		Namespaced: true,
		Needs:      store.ModeFull, // phase, capacity and spec fields are not metadata
		Columns: []Column{
			nameColumn(),
			namespaceColumn(),
			// status.phase, undefined (nil) when the claim has no status yet.
			{Key: "status", Kind: KindString, CellType: "string | null", FilterType: FilterSelect, Cell: func(r *Row) interface{} {
				if o := pvcObj(r); o != nil {
					return stringOrNil(string(o.Status.Phase))
				}
				return nil
			}},
			pvcString("volume", func(o *corev1.PersistentVolumeClaim) string { return o.Spec.VolumeName }),
			{Key: "capacity", Kind: KindString, CellType: "string", FilterType: FilterSelect, Cell: func(r *Row) interface{} { return pvcCapacity(r) }},
			pvcString("accessModes", func(o *corev1.PersistentVolumeClaim) string {
				modes := make([]string, len(o.Spec.AccessModes))
				for i, m := range o.Spec.AccessModes {
					modes[i] = string(m)
				}
				return strings.Join(modes, ", ")
			}),
			pvcString("storageClass", func(o *corev1.PersistentVolumeClaim) string {
				if o.Spec.StorageClassName != nil {
					return *o.Spec.StorageClassName
				}
				return ""
			}),
			ageColumn(creationAge),
			pvcString("volumeMode", func(o *corev1.PersistentVolumeClaim) string {
				if o.Spec.VolumeMode != nil {
					return orDefault(string(*o.Spec.VolumeMode), "Filesystem")
				}
				return "Filesystem"
			}),
			pvcString("requested", func(o *corev1.PersistentVolumeClaim) string { return pvcStorage(o.Spec.Resources.Requests) }),
			pvcString("selector", func(o *corev1.PersistentVolumeClaim) string {
				if o.Spec.Selector == nil {
					return ""
				}
				return labelPairs(o.Spec.Selector.MatchLabels, ",")
			}),
		},
		// The legacy list has no field table for claims: only the common fields are searchable.
		Search: commonSearchFields(),
	}
}
