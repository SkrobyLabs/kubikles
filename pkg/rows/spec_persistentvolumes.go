package rows

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(persistentVolumesSpec()) }

func pvObj(r *Row) *corev1.PersistentVolume {
	pv, _ := r.E.Obj.(*corev1.PersistentVolume)
	return pv
}

// pvCapacity is spec.capacity.storage as the API serialises it.
func pvCapacity(r *Row) string {
	if pv := pvObj(r); pv != nil {
		if q, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
			return q.String()
		}
	}
	return ""
}

func pvAccessModes(r *Row) string {
	pv := pvObj(r)
	if pv == nil {
		return ""
	}
	modes := make([]string, len(pv.Spec.AccessModes))
	for i, m := range pv.Spec.AccessModes {
		modes[i] = string(m)
	}
	return strings.Join(modes, ", ")
}

func pvClaim(r *Row) string {
	if pv := pvObj(r); pv != nil && pv.Spec.ClaimRef != nil {
		return pv.Spec.ClaimRef.Namespace + "/" + pv.Spec.ClaimRef.Name
	}
	return ""
}

func persistentVolumesSpec() *Spec {
	str := func(key string, f func(*Row) string) Column {
		return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return f(r) }}
	}
	return &Spec{
		Type:       "persistentvolumes",
		Namespaced: false,
		Needs:      store.ModeFull, // spec and status are not metadata
		Columns: []Column{
			nameColumn(),
			str("capacity", pvCapacity),
			str("accessModes", pvAccessModes),
			str("reclaimPolicy", func(r *Row) string {
				if pv := pvObj(r); pv != nil {
					return string(pv.Spec.PersistentVolumeReclaimPolicy)
				}
				return ""
			}),
			// item.status?.phase: undefined (null here) until the controller sets a phase.
			{Key: "status", Kind: KindString, CellType: "string | null", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} {
					if pv := pvObj(r); pv != nil {
						return stringOrNil(string(pv.Status.Phase))
					}
					return nil
				}},
			str("claim", pvClaim),
			str("storageClass", func(r *Row) string {
				if pv := pvObj(r); pv != nil {
					return pv.Spec.StorageClassName
				}
				return ""
			}),
			ageColumn(creationAge),
		},
		// The legacy list has no field mapping for this type, so only the common fields apply.
		Search: commonSearchFields(),
		Notes:  "The list's ResourceList type is \"pvs\"; the view schema is looked up through an alias.",
	}
}
