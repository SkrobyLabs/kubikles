package rows

import (
	"strings"

	storagev1 "k8s.io/api/storage/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(csiDriversSpec()) }

func csiDriverObj(r *Row) *storagev1.CSIDriver {
	d, _ := r.E.Obj.(*storagev1.CSIDriver)
	return d
}

// csiDriverBool is `spec.<field> ?? fallback` for an optional boolean.
func csiDriverBool(r *Row, field func(*storagev1.CSIDriverSpec) *bool, fallback bool) bool {
	if d := csiDriverObj(r); d != nil {
		if v := field(&d.Spec); v != nil {
			return *v
		}
	}
	return fallback
}

func csiDriverAttachRequired(r *Row) bool {
	return csiDriverBool(r, func(s *storagev1.CSIDriverSpec) *bool { return s.AttachRequired }, true)
}

func csiDriverPodInfoOnMount(r *Row) bool {
	return csiDriverBool(r, func(s *storagev1.CSIDriverSpec) *bool { return s.PodInfoOnMount }, false)
}

func csiDriverStorageCapacity(r *Row) bool {
	return csiDriverBool(r, func(s *storagev1.CSIDriverSpec) *bool { return s.StorageCapacity }, false)
}

func csiDriverModes(r *Row, sep string) string {
	d := csiDriverObj(r)
	if d == nil {
		return ""
	}
	modes := make([]string, len(d.Spec.VolumeLifecycleModes))
	for i, m := range d.Spec.VolumeLifecycleModes {
		modes[i] = string(m)
	}
	return strings.Join(modes, sep)
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func trueFalse(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func csiDriversSpec() *Spec {
	yn := func(key string, f func(*Row) bool) Column {
		return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return yesNo(f(r)) }}
	}
	return &Spec{
		Type:       "csidrivers",
		Namespaced: false,
		Needs:      store.ModeFull, // everything shown is in spec
		Columns: []Column{
			nameColumn(),
			yn("attachRequired", csiDriverAttachRequired),
			yn("podInfoOnMount", csiDriverPodInfoOnMount),
			yn("storageCapacity", csiDriverStorageCapacity),
			{Key: "volumeModes", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return orDefault(csiDriverModes(r, ", "), "-") }},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			SearchField{Name: "attachrequired", Aliases: []string{"attach", "attachreq"}, Extract: func(r *Row) string { return trueFalse(csiDriverAttachRequired(r)) }},
			SearchField{Name: "podinfoonmount", Aliases: []string{"podinfo"}, Extract: func(r *Row) string { return trueFalse(csiDriverPodInfoOnMount(r)) }},
			SearchField{Name: "storagecapacity", Aliases: []string{"capacity"}, Extract: func(r *Row) string { return trueFalse(csiDriverStorageCapacity(r)) }},
			SearchField{Name: "volumemodes", Aliases: []string{"volumelifecyclemodes", "modes"}, Extract: func(r *Row) string { return csiDriverModes(r, ",") }},
			SearchField{Name: "fsgrouppolicy", Aliases: []string{"fsgroup", "fspolicy"}, Extract: func(r *Row) string {
				if d := csiDriverObj(r); d != nil && d.Spec.FSGroupPolicy != nil {
					return string(*d.Spec.FSGroupPolicy)
				}
				return ""
			}},
		),
	}
}
