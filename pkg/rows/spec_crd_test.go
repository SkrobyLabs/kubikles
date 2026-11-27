package rows

import (
	"errors"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"

	"kubikles/pkg/store"
)

var certDesc = CRDDescriptor{Group: "cert.io", Version: "v1", Resource: "certs", Namespaced: true}

func certObject(name, ns string, ready string, replicas interface{}, enabled interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert.io/v1", "kind": "Cert",
		"metadata": map[string]interface{}{"name": name, "namespace": ns, "uid": "u-" + name, "resourceVersion": "1",
			"creationTimestamp": "2024-01-02T03:04:05Z"},
		"spec": map[string]interface{}{"replicas": replicas, "enabled": enabled, "notAfter": "2030-01-01T00:00:00Z"},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Issued", "status": "False"},
			map[string]interface{}{"type": "Ready", "status": ready},
		}},
	}}
}

func rowOf(obj *unstructured.Unstructured) Row {
	e := &store.Entry{UID: string(obj.GetUID()), Namespace: obj.GetNamespace(), Name: obj.GetName(), Obj: obj}
	return NewRow(e, nil, time.Now())
}

func cellOf(t *testing.T, spec *Spec, key string, r Row) interface{} {
	t.Helper()
	c := spec.Column(key)
	if c == nil {
		t.Fatalf("no column %q in %v", key, columnKeys(spec))
	}
	return c.Cell(&r)
}

func columnKeys(s *Spec) []string {
	var out []string
	for _, c := range s.Columns {
		out = append(out, c.Key)
	}
	return out
}

func TestGenericCRDSpecColumnOrderAndSlugs(t *testing.T) {
	spec := GenericCRDSpec(certDesc, []PrinterColumn{
		{Name: "Ready", Type: "string", JSONPath: `.status.conditions[?(@.type=="Ready")].status`},
		{Name: "Not After", Type: "date", JSONPath: ".spec.notAfter"},
		{Name: "Name", Type: "string", JSONPath: ".metadata.name"},
		{Name: "Age", Type: "date", JSONPath: ".metadata.creationTimestamp"},
		{Name: "Created", Type: "date", JSONPath: ".metadata.creationTimestamp"},
		{Name: "Replicas", Type: "integer", JSONPath: ".spec.replicas", Priority: 1},
	})
	want := []string{"name", "namespace", "ready", "not_after", "replicas", "age"}
	if got := columnKeys(spec); !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	if !spec.Column("replicas").DefaultHidden || spec.Column("ready").DefaultHidden {
		t.Fatal("only priority > 0 columns are hidden by default")
	}
	if spec.Type != "crd:cert.io/v1/certs" || !spec.Namespaced {
		t.Fatalf("spec = %s namespaced=%v", spec.Type, spec.Namespaced)
	}
	if got := PrinterColumnKey("  Not   After "); got != "_not_after_" {
		t.Fatalf("slug = %q", got)
	}
}

func TestGenericCRDSpecKindsAndFilters(t *testing.T) {
	spec := GenericCRDSpec(certDesc, []PrinterColumn{
		{Name: "S", Type: "string", JSONPath: ".spec.s"},
		{Name: "I", Type: "integer", JSONPath: ".spec.replicas"},
		{Name: "N", Type: "number", JSONPath: ".spec.replicas"},
		{Name: "B", Type: "boolean", JSONPath: ".spec.enabled"},
		{Name: "D", Type: "date", JSONPath: ".spec.notAfter"},
	})
	cases := map[string]struct {
		kind Kind
		ft   FilterType
	}{
		"name": {KindString, FilterRegex}, "namespace": {KindString, FilterRegex},
		"s": {KindString, FilterSelect}, "i": {KindNumber, FilterNumeric}, "n": {KindNumber, FilterNumeric},
		"b": {KindBool, FilterSelect}, "d": {KindTime, FilterNone}, "age": {KindTime, FilterNumeric},
	}
	for key, c := range cases {
		col := spec.Column(key)
		if col == nil || col.Kind != c.kind || col.FilterType != c.ft {
			t.Errorf("%s = %+v, want kind %s filter %s", key, col, c.kind, c.ft)
		}
	}
	if err := validate(spec); err != nil {
		t.Fatal(err)
	}
}

func TestGenericCRDSpecValuesFromObjects(t *testing.T) {
	spec := GenericCRDSpec(certDesc, []PrinterColumn{
		{Name: "Ready", Type: "string", JSONPath: `.status.conditions[?(@.type=="Ready")].status`},
		{Name: "Replicas", Type: "integer", JSONPath: ".spec.replicas"},
		{Name: "Enabled", Type: "boolean", JSONPath: ".spec.enabled"},
		{Name: "Missing", Type: "string", JSONPath: ".spec.nothing.here"},
		{Name: "Conditions", Type: "string", JSONPath: ".status.conditions[0]"},
	})
	r := rowOf(certObject("a", "ns", "True", int64(3), true))
	if got := cellOf(t, spec, "ready", r); got != "True" {
		t.Errorf("ready = %v", got)
	}
	if got := cellOf(t, spec, "replicas", r); got != float64(3) {
		t.Errorf("replicas = %#v", got)
	}
	if got := cellOf(t, spec, "enabled", r); got != true {
		t.Errorf("enabled = %#v", got)
	}
	if got := cellOf(t, spec, "missing", r); got != nil {
		t.Errorf("missing = %#v", got)
	}
	if got := cellOf(t, spec, "conditions", r); got != `{"status":"False","type":"Issued"}` {
		t.Errorf("conditions = %#v", got)
	}
	// Scalars arriving as strings.
	r2 := rowOf(certObject("b", "ns", "False", "7", "true"))
	if got := cellOf(t, spec, "replicas", r2); got != float64(7) {
		t.Errorf("string replicas = %#v", got)
	}
	if got := cellOf(t, spec, "enabled", r2); got != true {
		t.Errorf("string enabled = %#v", got)
	}
	if got := cellOf(t, spec, "age", r2); got != "2024-01-02T03:04:05Z" {
		t.Errorf("age = %#v", got)
	}
}

func TestGenericCRDSpecNeedsAndPrune(t *testing.T) {
	none := GenericCRDSpec(certDesc, nil)
	if none.Needs != store.ModeMetadata || none.FullPrune != nil {
		t.Fatalf("no columns: needs %v prune %v", none.Needs, none.FullPrune)
	}
	if got := columnKeys(none); !reflect.DeepEqual(got, []string{"name", "namespace", "age"}) {
		t.Fatalf("columns = %v", got)
	}
	table := GenericCRDSpec(certDesc, []PrinterColumn{
		{Name: "Ready", Type: "string", JSONPath: ".status.phase"},
		{Name: "R", Type: "integer", JSONPath: ".spec.replicas"},
		{Name: "R2", Type: "integer", JSONPath: ".spec.replicas2"},
	})
	if table.Needs != store.ModeTable || !reflect.DeepEqual(table.FullPrune, []string{"status", "spec"}) {
		t.Fatalf("table: needs %v prune %v", table.Needs, table.FullPrune)
	}
	date := GenericCRDSpec(certDesc, []PrinterColumn{{Name: "Expires", Type: "date", JSONPath: ".spec.notAfter"}})
	if date.Needs != store.ModeFull {
		t.Fatalf("date column needs %v, want full objects", date.Needs)
	}
	unprunable := GenericCRDSpec(certDesc, []PrinterColumn{{Name: "X", Type: "string", JSONPath: `{['odd key']}`}})
	if unprunable.FullPrune != nil {
		t.Fatalf("a path without a plain root must disable pruning, got %v", unprunable.FullPrune)
	}
	cluster := GenericCRDSpec(CRDDescriptor{Group: "g", Version: "v1", Resource: "things"}, nil)
	if got := columnKeys(cluster); !reflect.DeepEqual(got, []string{"name", "age"}) || cluster.Namespaced {
		t.Fatalf("cluster scoped columns = %v", got)
	}
}

func TestGenericCRDSpecSearchFields(t *testing.T) {
	spec := GenericCRDSpec(certDesc, []PrinterColumn{
		{Name: "Ready", Type: "string", JSONPath: `.status.conditions[?(@.type=="Ready")].status`},
		{Name: "Labels", Type: "string", JSONPath: ".spec.x"},
	})
	names := map[string]bool{}
	for _, f := range spec.Search {
		if names[f.Name] {
			t.Fatalf("duplicate search field %q", f.Name)
		}
		names[f.Name] = true
	}
	for _, n := range []string{"name", "namespace", "uid", "labels", "annotations", "ready"} {
		if !names[n] {
			t.Errorf("missing search field %q in %v", n, names)
		}
	}
	match := CompileSearch(spec, "ready:True")
	r := rowOf(certObject("a", "ns", "True", int64(1), true))
	r2 := rowOf(certObject("b", "ns", "False", int64(1), true))
	if !match(&r) || match(&r2) {
		t.Fatal("ready:True should match only the ready object")
	}
}

// Table cells and JSONPath evaluation of the same object must agree: that is the contract that
// lets the store switch between Table, metadata-plus-Table and pruned full objects.
func TestGenericCRDSpecTableParity(t *testing.T) {
	printer := []PrinterColumn{
		{Name: "Ready", Type: "string", JSONPath: `.status.conditions[?(@.type=="Ready")].status`},
		{Name: "Replicas", Type: "integer", JSONPath: ".spec.replicas"},
		{Name: "Enabled", Type: "boolean", JSONPath: ".spec.enabled"},
	}
	spec := GenericCRDSpec(certDesc, printer)
	obj := certObject("a", "ns", "True", int64(3), true)
	full := rowOf(obj)

	pm := &metav1.PartialObjectMetadata{}
	pm.Name, pm.Namespace, pm.UID = "a", "ns", "u-a"
	pm.CreationTimestamp = metav1.NewTime(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
	tableRow := NewRow(&store.Entry{UID: "u-a", Namespace: "ns", Name: "a", Obj: pm,
		Cells: []interface{}{"a", "True", float64(3), true, "5d"}},
		map[string]int{"Name": 0, "Ready": 1, "Replicas": 2, "Enabled": 3, "Age": 4}, time.Now())

	for _, key := range []string{"name", "namespace", "ready", "replicas", "enabled", "age"} {
		a, b := cellOf(t, spec, key, full), cellOf(t, spec, key, tableRow)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: full %#v vs table %#v", key, a, b)
		}
	}
	// A Table row with a null cell reads as missing.
	nullRow := NewRow(&store.Entry{Name: "n", Obj: pm, Cells: []interface{}{"n", nil, nil, nil, "1d"}},
		map[string]int{"Name": 0, "Ready": 1, "Replicas": 2, "Enabled": 3, "Age": 4}, time.Now())
	if cellOf(t, spec, "replicas", nullRow) != nil || cellOf(t, spec, "ready", nullRow) != nil {
		t.Fatal("null Table cells must read as nil")
	}
}

func TestSchemaOfDynamicSpec(t *testing.T) {
	spec := GenericCRDSpec(certDesc, []PrinterColumn{{Name: "Replicas", Type: "integer", JSONPath: ".spec.replicas", Priority: 2}})
	s := spec.Schema()
	if s.Type != spec.Type || s.SchemaVersion != spec.SchemaVersion() || !s.Namespaced {
		t.Fatalf("schema = %+v", s)
	}
	var hidden []string
	for _, c := range s.Columns {
		if c.Hidden {
			hidden = append(hidden, c.Key)
		}
		if c.Predefined == nil {
			t.Fatal("predefined must marshal as an array")
		}
	}
	if !reflect.DeepEqual(hidden, []string{"replicas"}) {
		t.Fatalf("hidden = %v", hidden)
	}
	other := GenericCRDSpec(certDesc, []PrinterColumn{{Name: "Replicas", Type: "string", JSONPath: ".spec.replicas"}})
	if other.SchemaVersion() == spec.SchemaVersion() {
		t.Fatal("a different column type must change the schema version")
	}
}

// --- OpenView over the real store -------------------------------------------------------------

type crdClients struct {
	dyn dynamic.Interface
}

func (c crdClients) Kube(string) (kubernetes.Interface, error) { return fake.NewSimpleClientset(), nil }
func (c crdClients) Dynamic(string) (dynamic.Interface, error) { return c.dyn, nil }
func (c crdClients) Metadata(string) (metadata.Interface, error) {
	return nil, store.ErrNoMetadata
}

type staticResolver struct {
	info CRDInfo
	err  error
}

func (s staticResolver) ResolveCRD(string, CRDDescriptor) (CRDInfo, error) { return s.info, s.err }

func certDynamic(objs ...*unstructured.Unstructured) dynamic.Interface {
	var rt []runtime.Object
	for _, o := range objs {
		rt = append(rt, o)
	}
	gvr := schema.GroupVersionResource{Group: "cert.io", Version: "v1", Resource: "certs"}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "CertList"}, rt...)
}

func TestOpenViewOverCustomResource(t *testing.T) {
	dyn := certDynamic(
		certObject("a", "ns", "True", int64(3), true),
		certObject("b", "ns", "False", int64(5), false),
	)
	sm := store.New(crdClients{dyn: dyn}, store.Options{})
	t.Cleanup(sm.Close)
	m := NewManager(Config{
		Provider: StoreProvider{M: sm}, Context: func() string { return "c" }, NoJanitor: true,
		CRD: staticResolver{info: CRDInfo{Namespaced: true, Columns: []PrinterColumn{
			// A date column forces full objects, which the fake dynamic client serves.
			{Name: "Not After", Type: "date", JSONPath: ".spec.notAfter"},
			{Name: "Ready", Type: "string", JSONPath: `.status.conditions[?(@.type=="Ready")].status`},
			{Name: "Replicas", Type: "integer", JSONPath: ".spec.replicas"},
		}}},
	})
	t.Cleanup(m.Close)

	res, err := m.Open(ViewSpec{ViewID: "v1", Type: "crd:cert.io/v1/certs", Namespaces: []string{"*"},
		Sort: SortSpec{Key: "replicas", Dir: "desc"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Schema == nil || res.Schema.Type != "crd:cert.io/v1/certs" || res.SchemaVersion != res.Schema.SchemaVersion {
		t.Fatalf("schema = %+v", res.Schema)
	}
	want := []string{"name", "namespace", "not_after", "ready", "replicas", "age"}
	if !reflect.DeepEqual(res.Columns, want) {
		t.Fatalf("columns = %v", res.Columns)
	}
	waitFor(t, "rows", func() bool {
		st, _ := m.State("v1")
		return st.Total == 2 && st.Phase == "complete"
	})
	w, err := m.Window("v1", 0, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Order) != 2 || w.Order[0] != "u-b" {
		t.Fatalf("order = %v (replicas desc puts b first)", w.Order)
	}
	row := w.Rows["u-b"]
	if row.C[3] != "False" || row.C[4] != float64(5) || row.C[2] != "2030-01-01T00:00:00Z" {
		t.Fatalf("cells = %#v", row.C)
	}

	// Column filter on a dynamic numeric column.
	if _, err := m.Update("v1", ViewPatch{ColumnFilters: &[]ColumnFilterSpec{{Col: "replicas", Type: "numeric", Operator: ">", Value: floatPtr(4)}}}); err != nil {
		t.Fatal(err)
	}
	st, _ := m.State("v1")
	if st.Filtered != 1 {
		t.Fatalf("filtered = %d", st.Filtered)
	}
}

func floatPtr(f float64) *float64 { return &f }

func TestOpenViewUnknownCustomResourceOpensWithBaseColumns(t *testing.T) {
	m := NewManager(Config{
		Provider: StoreProvider{M: store.New(crdClients{dyn: certDynamic()}, store.Options{})},
		Context:  func() string { return "c" }, NoJanitor: true,
		CRD: staticResolver{err: errors.New("customresourcedefinitions.apiextensions.k8s.io \"nothing.example.io\" not found")},
	})
	t.Cleanup(m.Close)
	res, err := m.Open(ViewSpec{ViewID: "v2", Type: "crd:example.io/v1/nothing", Namespaces: []string{"*"},
		CRD: &CRDDescriptor{Group: "example.io", Version: "v1", Resource: "nothing", Namespaced: false}})
	if err != nil {
		t.Fatalf("an unresolved resource must open, got %v", err)
	}
	if !reflect.DeepEqual(res.Columns, []string{"name", "age"}) {
		t.Fatalf("columns = %v", res.Columns)
	}
	if res.Schema == nil || res.Schema.Namespaced {
		t.Fatalf("schema = %+v", res.Schema)
	}
	if _, err := m.Open(ViewSpec{ViewID: "v3", Type: "crd:broken"}); err == nil {
		t.Fatal("a malformed crd type must be rejected")
	}
}

func TestViewStatsCountWindowsAndDoNotTouchTheView(t *testing.T) {
	dyn := certDynamic(certObject("a", "ns", "True", int64(3), true), certObject("b", "ns", "False", int64(5), false))
	sm := store.New(crdClients{dyn: dyn}, store.Options{})
	t.Cleanup(sm.Close)
	clock := &manualClockForStats{now: time.Unix(1000, 0)}
	m := NewManager(Config{
		Provider: StoreProvider{M: sm}, Context: func() string { return "c" }, NoJanitor: true, Clock: clock,
		CRD: staticResolver{info: CRDInfo{Namespaced: true, Columns: []PrinterColumn{{Name: "Ready", Type: "string", JSONPath: ".status.conditions[0].status"}}}},
	})
	t.Cleanup(m.Close)
	if _, err := m.Open(ViewSpec{ViewID: "vs", Type: "crd:cert.io/v1/certs", Namespaces: []string{"*"}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "rows", func() bool { st, _ := m.State("vs"); return st.Total == 2 })
	for i := 0; i < 2; i++ {
		if _, err := m.Window("vs", 0, 10, 0); err != nil {
			t.Fatal(err)
		}
	}
	clock.now = clock.now.Add(40 * time.Second)
	stats := m.Stats()
	if len(stats) != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	s := stats[0]
	if s.ID != "vs" || s.Type != "crd:cert.io/v1/certs" || s.Total != 2 || s.Filtered != 2 || s.Windows != 2 || s.WindowRows != 2 || s.RowsServed != 4 || s.BytesSent == 0 || !s.Active {
		t.Fatalf("stats = %+v", s)
	}
	if s.IdleSecs < 39 {
		t.Fatalf("idle = %v: reading stats must not touch the view", s.IdleSecs)
	}
	if again := m.Stats()[0]; again.IdleSecs < 39 {
		t.Fatalf("a second reading touched the view: idle = %v", again.IdleSecs)
	}
}

type manualClockForStats struct{ now time.Time }

func (c *manualClockForStats) Now() time.Time { return c.now }
