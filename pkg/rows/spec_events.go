package rows

import (
	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(eventsSpec()) }

func eventObj(r *Row) *corev1.Event {
	ev, _ := r.E.Obj.(*corev1.Event)
	return ev
}

func eventStr(r *Row, f func(*corev1.Event) string) string {
	if ev := eventObj(r); ev != nil {
		return f(ev)
	}
	return ""
}

func eventCount(ev *corev1.Event) int {
	if ev.Count == 0 {
		return 1
	}
	return int(ev.Count)
}

func eventAge(r *Row) interface{} {
	if ev := eventObj(r); ev != nil && !ev.FirstTimestamp.IsZero() {
		return TimeString(ev.FirstTimestamp)
	}
	return creationAge(r)
}

func eventLast(r *Row) interface{} {
	if ev := eventObj(r); ev != nil && !ev.LastTimestamp.IsZero() {
		return TimeString(ev.LastTimestamp)
	}
	return creationAge(r)
}

// eventSourceComponent is `item.source?.component || item.reportingController || empty string`. The legacy
// code reads a reportingController property that corev1.Event never serialises (its
// ReportingController field is written as reportingComponent), so only the component ever
// counts; that is reproduced here.
func eventSourceComponent(r *Row) string {
	return eventStr(r, func(ev *corev1.Event) string { return ev.Source.Component })
}

// involvedObjectText is `${obj.kind}/${obj.name}` as JavaScript renders it: a missing kind or
// name reads "undefined" (encoding/json omits empty ObjectReference fields).
func involvedObjectText(ev *corev1.Event) string {
	kind, name := ev.InvolvedObject.Kind, ev.InvolvedObject.Name
	if kind == "" {
		kind = "undefined"
	}
	if name == "" {
		name = "undefined"
	}
	return kind + "/" + name
}

func eventsSpec() *Spec {
	return &Spec{
		Type:       "events",
		Namespaced: true,
		Needs:      store.ModeFull,
		Columns: []Column{
			{Key: "type", Kind: KindString, CellType: "string", FilterType: FilterSelect, Predefined: []string{"Normal", "Warning"},
				Cell: func(r *Row) interface{} { return eventStr(r, func(ev *corev1.Event) string { return ev.Type }) }},
			namespaceColumn(),
			{Key: "involvedObject", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} {
					return eventStr(r, involvedObjectText)
				}},
			{Key: "message", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return eventStr(r, func(ev *corev1.Event) string { return ev.Message }) }},
			{Key: "count", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} {
					if ev := eventObj(r); ev != nil {
						return numberCell(eventCount(ev))
					}
					return numberCell(1)
				}},
			ageColumn(eventAge),
			{Key: "last", Kind: KindTime, CellType: "string | null", FilterType: FilterSelect, Cell: eventLast},
			{Key: "reason", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return eventStr(r, func(ev *corev1.Event) string { return ev.Reason }) }},
			{Key: "source", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} { return eventSourceComponent(r) }},
			{Key: "objectKind", Kind: KindString, CellType: "string", FilterType: FilterSelect,
				Cell: func(r *Row) interface{} {
					return eventStr(r, func(ev *corev1.Event) string { return ev.InvolvedObject.Kind })
				}},
		},
		Search: []SearchField{
			nameField(),
			namespaceField(),
			{Name: "type", Aliases: []string{"t", "kind"}, Extract: func(r *Row) string { return eventStr(r, func(ev *corev1.Event) string { return ev.Type }) }},
			{Name: "reason", Aliases: []string{"r"}, Extract: func(r *Row) string { return eventStr(r, func(ev *corev1.Event) string { return ev.Reason }) }},
			{Name: "message", Aliases: []string{"msg", "m"}, Extract: func(r *Row) string { return eventStr(r, func(ev *corev1.Event) string { return ev.Message }) }},
			{Name: "object", Aliases: []string{"involvedobject", "involved", "obj"}, Extract: func(r *Row) string {
				return eventStr(r, involvedObjectText)
			}},
			{Name: "objectkind", Aliases: []string{"objkind", "ok"}, Extract: func(r *Row) string {
				return eventStr(r, func(ev *corev1.Event) string { return ev.InvolvedObject.Kind })
			}},
			{Name: "objectname", Aliases: []string{"objname", "on"}, Extract: func(r *Row) string {
				return eventStr(r, func(ev *corev1.Event) string { return ev.InvolvedObject.Name })
			}},
			{Name: "source", Aliases: []string{"src"}, Extract: func(r *Row) string {
				return eventStr(r, func(ev *corev1.Event) string { return ev.Source.Component + "/" + ev.Source.Host })
			}},
			{Name: "count", Aliases: []string{"c"}, Extract: func(r *Row) string {
				if ev := eventObj(r); ev != nil {
					return itoa(eventCount(ev))
				}
				return "1"
			}},
			uidField(),
			labelsField(),
			annotationsField(),
		},
	}
}
