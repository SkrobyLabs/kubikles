package rows

import (
	"strconv"
	"strings"

	"kubikles/pkg/rows/query"
)

// Shared pieces of the type specs: the metadata columns and the search fields every resource
// has (fieldMappings/common.ts).

func nameColumn() Column {
	return Column{
		Key: "name", Kind: KindString, CellType: "string", FilterType: FilterRegex,
		Cell: func(r *Row) interface{} { return r.Name() },
	}
}

func namespaceColumn() Column {
	return Column{
		Key: "namespace", Kind: KindString, CellType: "string | null", FilterType: FilterRegex,
		Cell: func(r *Row) interface{} { return stringOrNil(r.Namespace()) },
	}
}

func ageColumn(cell func(r *Row) interface{}) Column {
	return Column{Key: "age", Kind: KindTime, CellType: "string | null", FilterType: FilterNumeric, Cell: cell}
}

func creationAge(r *Row) interface{} { return stringOrNil(r.CreationTimestamp()) }

func stringOrNil(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func numberCell(n int) interface{} { return float64(n) }

// kvText is Object.entries(map).map(([k, v]) => `${k}=${v}`).join(' ').
func kvText(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := jsKeys(m)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + m[k]
	}
	return strings.Join(parts, " ")
}

func labelsField() SearchField {
	return SearchField{Name: "labels", Aliases: []string{"label", "l"}, Extract: func(r *Row) string {
		if r.M == nil {
			return ""
		}
		return kvText(r.M.GetLabels())
	}}
}

func annotationsField() SearchField {
	return SearchField{Name: "annotations", Aliases: []string{"annotation", "a"}, Extract: func(r *Row) string {
		if r.M == nil {
			return ""
		}
		return kvText(r.M.GetAnnotations())
	}}
}

func uidField() SearchField {
	return SearchField{Name: "uid", Extract: func(r *Row) string { return r.UID() }}
}

func nameField() SearchField {
	return SearchField{Name: "name", Aliases: []string{"n"}, Extract: func(r *Row) string { return r.Name() }}
}

func namespaceField() SearchField {
	return SearchField{Name: "namespace", Aliases: []string{"ns"}, Extract: func(r *Row) string { return r.Namespace() }}
}

// commonSearchFields is commonFields in fieldMappings/common.ts.
func commonSearchFields() []SearchField {
	return []SearchField{nameField(), namespaceField(), labelsField(), annotationsField(), uidField()}
}

// SearchName is the text plain search terms match: the object name.
func SearchName(r *Row) string { return r.Name() }

// CompileSearch parses and compiles a search query for a spec.
func CompileSearch(spec *Spec, q string) func(*Row) bool {
	return query.CompileString(q, spec.Search, SearchName)
}

func itoa(n int) string { return strconv.Itoa(n) }
