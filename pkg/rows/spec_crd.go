package rows

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"

	"kubikles/pkg/store"
)

// CRDDescriptor names a custom resource a view lists. It travels with ViewSpec so the backend
// can build the row spec at open time.
type CRDDescriptor struct {
	Group      string `json:"group"`
	Version    string `json:"version"`
	Resource   string `json:"resource"`
	Namespaced bool   `json:"namespaced"`
}

// Type is the store type string of the descriptor.
func (d CRDDescriptor) Type() string { return store.CRDType(d.Group, d.Version, d.Resource) }

// PrinterColumn is one additionalPrinterColumns entry of a CRD version.
type PrinterColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	JSONPath string `json:"jsonPath"`
	Priority int32  `json:"priority"`
}

// CRDInfo is what the resolver knows about a custom resource.
type CRDInfo struct {
	Namespaced bool
	Columns    []PrinterColumn
}

// CRDResolver looks up a custom resource for OpenView. A not-found error makes the view open
// with the base columns; the shard then reports the failure through ViewState.error.
type CRDResolver interface {
	ResolveCRD(contextName string, d CRDDescriptor) (CRDInfo, error)
}

var slugSpaces = regexp.MustCompile(`\s+`)

// PrinterColumnKey is the column key CustomResourceList.tsx derives from a printer column name:
// lower case with whitespace runs replaced by an underscore.
func PrinterColumnKey(name string) string {
	return slugSpaces.ReplaceAllString(strings.ToLower(name), "_")
}

// crdColumn is a printer column that became a view column.
type crdColumn struct {
	key  string
	pc   PrinterColumn
	path *compiledPath
}

// GenericCRDSpec builds the row spec of a custom resource from its printer columns. It is not
// registered: a spec for a custom resource lives as long as the view that opened it.
//
// Columns are name, namespace (namespaced resources), one per printer column (skipping name,
// age, namespace and the creationTimestamp column, as the legacy list does) and age. Needs is
// Table when the server can print every column, metadata when there are none, and full objects
// (pruned to the JSONPath roots) when a date column needs the raw timestamp, because the server
// prints dates as ages.
func GenericCRDSpec(d CRDDescriptor, printer []PrinterColumn) *Spec {
	spec := &Spec{Type: d.Type(), Namespaced: d.Namespaced, Needs: store.ModeMetadata}
	spec.Columns = append(spec.Columns, nameColumn())
	if d.Namespaced {
		spec.Columns = append(spec.Columns, namespaceColumn())
	}
	search := commonSearchFields()
	if !d.Namespaced {
		search = withoutField(search, "namespace")
	}
	taken := map[string]bool{"name": true, "age": true, "namespace": true}
	var roots []string
	rootSeen := map[string]bool{}
	pruneAll := false
	hasDate := false
	printerCount := 0
	for _, pc := range printer {
		key := PrinterColumnKey(pc.Name)
		if key == "" || taken[key] || pc.JSONPath == ".metadata.creationTimestamp" {
			continue
		}
		taken[key] = true
		printerCount++
		path := compilePath(pc.JSONPath)
		cc := &crdColumn{key: key, pc: pc, path: path}
		spec.Columns = append(spec.Columns, crdPrinterColumn(cc))
		if pc.Type == "date" {
			hasDate = true
		}
		if root := jsonPathRoot(pc.JSONPath); root == "" {
			pruneAll = true
		} else if !rootSeen[root] {
			rootSeen[root] = true
			roots = append(roots, root)
		}
		fieldName := key
		for _, f := range search {
			if f.Name == fieldName {
				fieldName = ""
			}
		}
		if fieldName != "" {
			col := spec.Columns[len(spec.Columns)-1]
			search = append(search, SearchField{Name: fieldName, Extract: func(r *Row) string {
				return cellText(col.Cell(r))
			}})
		}
	}
	spec.Columns = append(spec.Columns, ageColumn(creationAge))
	spec.Search = search
	if printerCount > 0 {
		spec.Needs = store.ModeTable
	}
	if hasDate {
		spec.Needs = store.ModeFull
	}
	if !pruneAll && len(roots) > 0 {
		spec.FullPrune = roots
	}
	for i := range spec.Columns {
		switch spec.Columns[i].Key {
		case "name":
			spec.Columns[i].Label = "Name"
		case "namespace":
			spec.Columns[i].Label = "Namespace"
		case "age":
			spec.Columns[i].Label = "Age"
		}
	}
	spec.Notes = "Generated from the custom resource's additionalPrinterColumns."
	return spec
}

func withoutField(fields []SearchField, name string) []SearchField {
	out := fields[:0:0]
	for _, f := range fields {
		if f.Name != name {
			out = append(out, f)
		}
	}
	return out
}

// jsonPathRoot is the first field of a printer-column JSONPath ("status" for
// ".status.conditions[?(@.type==\"Ready\")].status"), or "" when the path does not start with a
// plain field and so cannot be pruned safely.
var jsonPathRootRe = regexp.MustCompile(`^\{?\$?\.([A-Za-z0-9_\-]+)`)

func jsonPathRoot(p string) string {
	m := jsonPathRootRe.FindStringSubmatch(strings.TrimSpace(p))
	if m == nil {
		return ""
	}
	return m[1]
}

// compiledPath evaluates a JSONPath against unstructured content. client-go's evaluator mutates
// state while ranging, so one path is guarded by a mutex.
type compiledPath struct {
	mu sync.Mutex
	jp *jsonpath.JSONPath
}

func compilePath(expr string) *compiledPath {
	e := strings.TrimSpace(expr)
	if e == "" {
		return nil
	}
	if !strings.HasPrefix(e, "{") {
		e = "{" + e + "}"
	}
	jp := jsonpath.New("printer").AllowMissingKeys(true)
	if err := jp.Parse(e); err != nil {
		return nil
	}
	return &compiledPath{jp: jp}
}

// eval returns the first value the path selects, or nil.
func (p *compiledPath) eval(obj map[string]interface{}) interface{} {
	if p == nil || obj == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	results, err := p.jp.FindResults(obj)
	if err != nil || len(results) == 0 || len(results[0]) == 0 {
		return nil
	}
	v := results[0][0]
	if !v.IsValid() || !v.CanInterface() {
		return nil
	}
	return v.Interface()
}

func crdPrinterColumn(cc *crdColumn) Column {
	col := Column{Key: cc.key, DefaultHidden: cc.pc.Priority > 0, Label: cc.pc.Name}
	raw := func(r *Row) interface{} {
		if v, ok := r.TableCell(cc.pc.Name); ok {
			return v
		}
		if u, ok := r.E.Obj.(*unstructured.Unstructured); ok {
			return cc.path.eval(u.Object)
		}
		return nil
	}
	switch cc.pc.Type {
	case "integer", "number":
		col.Kind, col.CellType, col.FilterType = KindNumber, "number | null", FilterNumeric
		col.Cell = func(r *Row) interface{} { return toNumberCell(raw(r)) }
	case "boolean":
		col.Kind, col.CellType, col.FilterType = KindBool, "boolean | null", FilterSelect
		col.Cell = func(r *Row) interface{} { return toBoolCell(raw(r)) }
		col.Filter = func(r *Row) string { return cellText(col.Cell(r)) }
	case "date":
		col.Kind, col.CellType, col.FilterType = KindTime, "string | null", FilterNone
		col.Cell = func(r *Row) interface{} { return toStringCell(raw(r)) }
	default:
		col.Kind, col.CellType, col.FilterType = KindString, "string | null", FilterSelect
		col.Cell = func(r *Row) interface{} { return toStringCell(raw(r)) }
	}
	return col
}

func toNumberCell(v interface{}) interface{} {
	switch n := v.(type) {
	case nil:
		return nil
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			return f
		}
	}
	return nil
}

func toBoolCell(v interface{}) interface{} {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		switch strings.ToLower(b) {
		case "true":
			return true
		case "false":
			return false
		}
	}
	return nil
}

// toStringCell is String(value) for scalars and compact JSON for objects and arrays (where the
// legacy list printed "[object Object]").
func toStringCell(v interface{}) interface{} {
	switch s := v.(type) {
	case nil:
		return nil
	case string:
		return s
	case bool, float64, float32, int, int32, int64, json.Number:
		return fmt.Sprint(s)
	default:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return fmt.Sprint(v)
		}
		return strings.TrimSpace(buf.String())
	}
}

// cellText is how a cell reads in a search field or select filter: "" for nil.
func cellText(v interface{}) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	case bool:
		return strconv.FormatBool(s)
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}
