package rows

import (
	"fmt"
	"strings"

	storagev1 "k8s.io/api/storage/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(csiNodesSpec()) }

func csiNodeDrivers(r *Row) []storagev1.CSINodeDriver {
	if n, _ := r.E.Obj.(*storagev1.CSINode); n != nil {
		return n.Spec.Drivers
	}
	return nil
}

// csiNodeDriverNames is getDriverNames of the legacy list.
func csiNodeDriverNames(r *Row) string {
	drivers := csiNodeDrivers(r)
	switch {
	case len(drivers) == 0:
		return "-"
	case len(drivers) <= 2:
		names := make([]string, len(drivers))
		for i, d := range drivers {
			names[i] = d.Name
		}
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s, +%d more", drivers[0].Name, len(drivers)-1)
}

func csiNodeJoin(r *Row, pick func(storagev1.CSINodeDriver) string) string {
	drivers := csiNodeDrivers(r)
	vals := make([]string, len(drivers))
	for i, d := range drivers {
		vals[i] = pick(d)
	}
	return strings.Join(vals, ",")
}

func csiNodesSpec() *Spec {
	return &Spec{
		Type:       "csinodes",
		Namespaced: false,
		Needs:      store.ModeFull, // drivers are in spec
		Columns: []Column{
			nameColumn(),
			{Key: "driverCount", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return numberCell(len(csiNodeDrivers(r))) }},
			{Key: "driverNames", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return csiNodeDriverNames(r) }},
			ageColumn(creationAge),
		},
		Search: append(commonSearchFields(),
			// "numDrivers" keeps its capital letter like the legacy mapping, so it is never
			// reachable through the lower-cased lookup there or here.
			SearchField{Name: "drivercount", Aliases: []string{"drivers", "numDrivers"}, Extract: func(r *Row) string { return itoa(len(csiNodeDrivers(r))) }},
			SearchField{Name: "drivernames", Aliases: []string{"driver"}, Extract: func(r *Row) string {
				return csiNodeJoin(r, func(d storagev1.CSINodeDriver) string { return d.Name })
			}},
			SearchField{Name: "nodeids", Aliases: []string{"nodeid"}, Extract: func(r *Row) string {
				return csiNodeJoin(r, func(d storagev1.CSINodeDriver) string { return d.NodeID })
			}},
		),
	}
}
