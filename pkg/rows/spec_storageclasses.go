package rows

import (
	storagev1 "k8s.io/api/storage/v1"

	"kubikles/pkg/store"
)

const storageDefaultAnnotation = "storageclass.kubernetes.io/is-default-class"

func init() { MustRegister(storageClassesSpec()) }

func storageClassObj(r *Row) *storagev1.StorageClass {
	sc, _ := r.E.Obj.(*storagev1.StorageClass)
	return sc
}

func storageClassProvisioner(r *Row) string {
	if sc := storageClassObj(r); sc != nil {
		return sc.Provisioner
	}
	return ""
}

func storageClassReclaim(r *Row) string {
	if sc := storageClassObj(r); sc != nil && sc.ReclaimPolicy != nil {
		return string(*sc.ReclaimPolicy)
	}
	return ""
}

func storageClassBinding(r *Row) string {
	if sc := storageClassObj(r); sc != nil && sc.VolumeBindingMode != nil {
		return string(*sc.VolumeBindingMode)
	}
	return ""
}

func storageClassExpansion(r *Row) string {
	if sc := storageClassObj(r); sc != nil && sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion {
		return "Yes"
	}
	return "No"
}

func storageClassesSpec() *Spec {
	str := func(key string, f func(*Row) string) Column {
		return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return f(r) }}
	}
	return &Spec{
		Type:       "storageclasses",
		Namespaced: false,
		Needs:      store.ModeFull, // provisioner and policies are not metadata
		Columns: []Column{
			nameColumn(),
			str("provisioner", storageClassProvisioner),
			str("reclaimPolicy", storageClassReclaim),
			str("volumeBindingMode", storageClassBinding),
			str("allowVolumeExpansion", storageClassExpansion),
			// Drives the default badge next to the name; hidden in the column picker by default.
			{Key: "isDefault", Kind: KindString, CellType: "string", FilterType: FilterNone,
				Cell: func(r *Row) interface{} {
					if r.M != nil && r.M.GetAnnotations()[storageDefaultAnnotation] == "true" {
						return "Yes"
					}
					return "No"
				}},
			ageColumn(creationAge),
		},
		// The legacy list has no field mapping for this type, so only the common fields apply.
		Search: commonSearchFields(),
	}
}
