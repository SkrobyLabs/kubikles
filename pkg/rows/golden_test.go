package rows

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"kubikles/pkg/store"
)

// The golden files are written by frontend/src/test/golden/generateGolden.test.ts from the real
// legacy list code. Everything below asserts the Go engine reproduces them.

type golden struct {
	Type         string                            `json:"type"`
	Now          int64                             `json:"now"`
	Columns      []string                          `json:"columns"`
	FilterTypes  map[string]interface{}            `json:"filterTypes"`
	Fixtures     []json.RawMessage                 `json:"fixtures"`
	NoSort       []string                          `json:"noSort"`
	Cells        map[string]map[string]interface{} `json:"cells"`
	FilterValues map[string]map[string]string      `json:"filterValues"`
	Numeric      map[string]map[string]interface{} `json:"numeric"`
	SortOrders   map[string]map[string][]string    `json:"sortOrders"`
	SearchCases  []struct {
		Query   string   `json:"query"`
		Matches []string `json:"matches"`
	} `json:"searchCases"`
	ColumnFilterCases []struct {
		Name    string                      `json:"name"`
		Filters map[string]ColumnFilterSpec `json:"filters"`
		Matches []string                    `json:"matches"`
	} `json:"columnFilterCases"`
	UniqueValues map[string]*struct {
		Values []string       `json:"values"`
		Counts map[string]int `json:"counts"`
	} `json:"uniqueValues"`
	Overlay *goldenOverlay `json:"overlay"`
	Derived *goldenDerived `json:"derived"`
}

func loadGolden(t testing.TB, typ string) *golden {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "golden", typ+".json"))
	if err != nil {
		t.Fatalf("golden file: %v (run GOLDEN=1 npx vitest run src/test/golden in frontend)", err)
	}
	var g golden
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	return &g
}

// variant is one way of holding the fixtures in the store.
type variant struct {
	name    string
	entries []*store.Entry
	tcols   map[string]int
	// skip lists columns whose values intentionally differ from the legacy list.
	skip map[string]string
}

func entryOf(obj runtime.Object, m metav1.Object) *store.Entry {
	return &store.Entry{UID: string(m.GetUID()), Namespace: m.GetNamespace(), Name: m.GetName(), RV: "1", Obj: obj}
}

// clusterObject is a typed object that is both a runtime.Object and has a metadata accessor.
type clusterObject interface {
	runtime.Object
	metav1.Object
}

func variantsFor(t testing.TB, g *golden) []variant {
	t.Helper()
	switch g.Type {
	case "namespaces", "configmaps", "events", "priorityclasses", "ingressclasses", "storageclasses",
		"validatingwebhookconfigurations", "mutatingwebhookconfigurations", "csidrivers", "csinodes",
		"persistentvolumes", "clusterroles", "clusterrolebindings",
		"hpas", "pdbs", "resourcequotas", "limitranges", "leases", "networkpolicies", "serviceaccounts",
		"roles", "rolebindings", "endpoints", "endpointslices", "services", "ingresses",
		"replicasets", "jobs", "daemonsets", "persistentvolumeclaims", "pods", "nodes", "deployments", "statefulsets":
		newObj := map[string]func() clusterObject{
			"priorityclasses":                 func() clusterObject { return &schedulingv1.PriorityClass{} },
			"ingressclasses":                  func() clusterObject { return &networkingv1.IngressClass{} },
			"storageclasses":                  func() clusterObject { return &storagev1.StorageClass{} },
			"namespaces":                      func() clusterObject { return &corev1.Namespace{} },
			"configmaps":                      func() clusterObject { return &corev1.ConfigMap{} },
			"events":                          func() clusterObject { return &corev1.Event{} },
			"validatingwebhookconfigurations": func() clusterObject { return &admissionv1.ValidatingWebhookConfiguration{} },
			"mutatingwebhookconfigurations":   func() clusterObject { return &admissionv1.MutatingWebhookConfiguration{} },
			"csidrivers":                      func() clusterObject { return &storagev1.CSIDriver{} },
			"csinodes":                        func() clusterObject { return &storagev1.CSINode{} },
			"persistentvolumes":               func() clusterObject { return &corev1.PersistentVolume{} },
			"clusterroles":                    func() clusterObject { return &rbacv1.ClusterRole{} },
			"clusterrolebindings":             func() clusterObject { return &rbacv1.ClusterRoleBinding{} },
			"hpas":                            func() clusterObject { return &autoscalingv2.HorizontalPodAutoscaler{} },
			"pdbs":                            func() clusterObject { return &policyv1.PodDisruptionBudget{} },
			"resourcequotas":                  func() clusterObject { return &corev1.ResourceQuota{} },
			"limitranges":                     func() clusterObject { return &corev1.LimitRange{} },
			"leases":                          func() clusterObject { return &coordinationv1.Lease{} },
			"networkpolicies":                 func() clusterObject { return &networkingv1.NetworkPolicy{} },
			"serviceaccounts":                 func() clusterObject { return &corev1.ServiceAccount{} },
			"roles":                           func() clusterObject { return &rbacv1.Role{} },
			"rolebindings":                    func() clusterObject { return &rbacv1.RoleBinding{} },
			"endpoints":                       func() clusterObject { return &corev1.Endpoints{} },
			"endpointslices":                  func() clusterObject { return &discoveryv1.EndpointSlice{} },
			"services":                        func() clusterObject { return &corev1.Service{} },
			"ingresses":                       func() clusterObject { return &networkingv1.Ingress{} },
			"replicasets":                     func() clusterObject { return &appsv1.ReplicaSet{} },
			"jobs":                            func() clusterObject { return &batchv1.Job{} },
			"daemonsets":                      func() clusterObject { return &appsv1.DaemonSet{} },
			"deployments":                     func() clusterObject { return &appsv1.Deployment{} },
			"statefulsets":                    func() clusterObject { return &appsv1.StatefulSet{} },
			"persistentvolumeclaims":          func() clusterObject { return &corev1.PersistentVolumeClaim{} },
			"pods":                            func() clusterObject { return &corev1.Pod{} },
			"nodes":                           func() clusterObject { return &corev1.Node{} },
		}[g.Type]
		var entries []*store.Entry
		for _, raw := range g.Fixtures {
			o := newObj()
			must(t, json.Unmarshal(raw, o))
			entries = append(entries, entryOf(o, o))
		}
		return []variant{{name: "full", entries: entries}}
	case "secrets":
		var tableEntries, fullEntries []*store.Entry
		for _, raw := range g.Fixtures {
			var item struct {
				Metadata metav1.ObjectMeta `json:"metadata"`
				Type     string            `json:"type"`
				DataKeys int               `json:"dataKeys"`
			}
			must(t, json.Unmarshal(raw, &item))
			pm := &metav1.PartialObjectMetadata{ObjectMeta: item.Metadata}
			te := entryOf(pm, pm)
			te.Cells = []interface{}{item.Metadata.Name, item.Type, float64(item.DataKeys), "1d"}
			tableEntries = append(tableEntries, te)

			sec := &corev1.Secret{ObjectMeta: item.Metadata, Type: corev1.SecretType(item.Type)}
			if item.DataKeys > 0 {
				sec.Data = map[string][]byte{}
				for i := 0; i < item.DataKeys; i++ {
					sec.Data[fmt.Sprintf("k%d", i)] = []byte("x")
				}
			}
			fullEntries = append(fullEntries, entryOf(sec, sec))
		}
		skip := map[string]string{
			"keys": "intentional: count instead of key names (spec_secrets.go)",
			"size": "intentional: dropped from the view path (spec_secrets.go)",
		}
		return []variant{
			{name: "table", entries: tableEntries, tcols: map[string]int{"Name": 0, "Type": 1, "Data": 2, "Age": 3}, skip: skip},
			{name: "full-degraded", entries: fullEntries, skip: skip},
		}
	}
	t.Fatalf("no fixtures for %s", g.Type)
	return nil
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func jsonOf(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func goldenNow(g *golden) time.Time { return time.UnixMilli(g.Now) }

func TestGoldenCells(t *testing.T) {
	for _, typ := range Types() {
		g := loadGolden(t, typ)
		spec, _ := Lookup(typ)
		for _, v := range variantsFor(t, g) {
			t.Run(typ+"/"+v.name, func(t *testing.T) {
				for _, e := range v.entries {
					r := NewRow(e, v.tcols, goldenNow(g))
					for _, key := range g.Columns {
						if _, skip := v.skip[key]; skip {
							continue
						}
						col := spec.Column(key)
						if col == nil {
							t.Fatalf("column %s missing from spec", key)
						}
						want := g.Cells[e.UID][key]
						if got := col.Cell(&r); jsonOf(got) != jsonOf(want) {
							t.Errorf("%s cell %s = %s, want %s", e.UID, key, jsonOf(got), jsonOf(want))
						}
						if got, want := col.FilterValue(&r), g.FilterValues[e.UID][key]; got != want {
							t.Errorf("%s filter value %s = %q, want %q", e.UID, key, got, want)
						}
						wantNum := g.Numeric[e.UID][key]
						gotNum, ok := col.NumericValue(&r)
						switch {
						case wantNum == nil && ok:
							t.Errorf("%s numeric %s = %v, want NaN", e.UID, key, gotNum)
						case wantNum != nil && !ok:
							t.Errorf("%s numeric %s = NaN, want %v", e.UID, key, wantNum)
						case wantNum != nil && math.Abs(gotNum-wantNum.(float64)) > 1e-9:
							t.Errorf("%s numeric %s = %v, want %v", e.UID, key, gotNum, wantNum)
						}
					}
				}
			})
		}
	}
}

func TestGoldenSchemaMatchesLegacyColumns(t *testing.T) {
	for _, typ := range Types() {
		g := loadGolden(t, typ)
		spec, _ := Lookup(typ)
		for _, key := range g.Columns {
			col := spec.Column(key)
			if col == nil && droppedColumns[typ+"/"+key] {
				continue
			}
			if col == nil {
				t.Errorf("%s: legacy column %s has no spec column", typ, key)
				continue
			}
			want := fmt.Sprint(g.FilterTypes[key])
			if want == "false" {
				want = "none"
			}
			if droppedColumns[typ+"/"+key] {
				continue
			}
			if string(col.FilterType) != want {
				t.Errorf("%s.%s filter type %s, legacy %s", typ, key, col.FilterType, want)
			}
		}
		// Every spec column is either a legacy column or documented as new.
		legacy := map[string]bool{}
		for _, k := range g.Columns {
			legacy[k] = true
		}
		for _, c := range spec.Columns {
			if !legacy[c.Key] {
				t.Errorf("%s.%s is not a legacy column; add it to the semantics module or remove it", typ, c.Key)
			}
		}
	}
}

// droppedColumns and changedColumns document the intentional differences from the legacy list.
var droppedColumns = map[string]bool{"secrets/size": true, "secrets/keys": true}

func testCfg(t testing.TB, spec *Spec, vs ViewSpec) *compiled {
	t.Helper()
	vs.Type = spec.Type
	cfg, err := compileView(vs, spec)
	must(t, err)
	return cfg
}

func TestGoldenSortOrders(t *testing.T) {
	for _, typ := range Types() {
		g := loadGolden(t, typ)
		spec, _ := Lookup(typ)
		excluded := map[string]bool{}
		for _, u := range g.NoSort {
			excluded[u] = true
		}
		for _, v := range variantsFor(t, g) {
			var entries []*store.Entry
			for _, e := range v.entries {
				if !excluded[e.UID] {
					entries = append(entries, e)
				}
			}
			for _, key := range g.Columns {
				if _, skip := v.skip[key]; skip {
					continue
				}
				for _, dir := range []string{"asc", "desc"} {
					cfg := testCfg(t, spec, ViewSpec{Namespaces: []string{"*"}, Sort: SortSpec{Key: key, Dir: dir}})
					c := buildCore(cfg, entries, v.tcols, goldenNow(g), 1)
					got := make([]string, len(c.index))
					for i, r := range c.index {
						got[i] = r.id
					}
					if want := g.SortOrders[key][dir]; !reflect.DeepEqual(got, want) {
						t.Errorf("%s/%s sort %s %s:\n got  %v\n want %v", typ, v.name, key, dir, got, want)
					}
				}
			}
		}
	}
}

func TestGoldenSearch(t *testing.T) {
	for _, typ := range Types() {
		g := loadGolden(t, typ)
		spec, _ := Lookup(typ)
		for _, v := range variantsFor(t, g) {
			for _, c := range g.SearchCases {
				pred := CompileSearch(spec, c.Query)
				var got []string
				for _, e := range v.entries {
					r := NewRow(e, v.tcols, goldenNow(g))
					if pred(&r) {
						got = append(got, e.UID)
					}
				}
				// Secrets: only the search fields that exist in both representations are compared.
				if !reflect.DeepEqual(got, c.Matches) && !(len(got) == 0 && len(c.Matches) == 0) {
					t.Errorf("%s/%s search %q:\n got  %v\n want %v", typ, v.name, c.Query, got, c.Matches)
				}
			}
		}
	}
}

func TestGoldenColumnFilters(t *testing.T) {
	for _, typ := range Types() {
		g := loadGolden(t, typ)
		spec, _ := Lookup(typ)
		for _, v := range variantsFor(t, g) {
			for _, c := range g.ColumnFilterCases {
				var filters []ColumnFilterSpec
				keys := make([]string, 0, len(c.Filters))
				for k := range c.Filters {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				skipCase := false
				for _, k := range keys {
					f := c.Filters[k]
					f.Col = k
					if _, skip := v.skip[k]; skip {
						skipCase = true
					}
					filters = append(filters, f)
				}
				if skipCase {
					continue
				}
				pred, _, err := compileColumnFilters(spec, filters)
				must(t, err)
				var got []string
				for _, e := range v.entries {
					r := NewRow(e, v.tcols, goldenNow(g))
					if pred == nil || pred(&r) {
						got = append(got, e.UID)
					}
				}
				if !reflect.DeepEqual(got, c.Matches) && !(len(got) == 0 && len(c.Matches) == 0) {
					t.Errorf("%s/%s column filter %q:\n got  %v\n want %v", typ, v.name, c.Name, got, c.Matches)
				}
			}
		}
	}
}

func TestGoldenUniqueValues(t *testing.T) {
	for _, typ := range Types() {
		g := loadGolden(t, typ)
		spec, _ := Lookup(typ)
		for _, v := range variantsFor(t, g) {
			cfg := testCfg(t, spec, ViewSpec{Namespaces: []string{"*"}})
			c := buildCore(cfg, v.entries, v.tcols, goldenNow(g), 1)
			vw := &View{core: c}
			for key, want := range g.UniqueValues {
				if _, skip := v.skip[key]; skip {
					continue
				}
				var f *facet
				for _, ff := range c.facets {
					if ff.col.Key == key {
						f = ff
					}
				}
				if f == nil {
					t.Fatalf("%s: no facet for %s", typ, key)
				}
				got := vw.facetResultLocked(f)
				if want == nil {
					if !got.Suppressed {
						t.Errorf("%s.%s: expected suppressed facet, got %v", typ, key, got.Values)
					}
					continue
				}
				if got.Suppressed || !reflect.DeepEqual(got.Values, want.Values) || !reflect.DeepEqual(got.Counts, want.Counts) {
					t.Errorf("%s/%s unique values %s:\n got  %v %v\n want %v %v", typ, v.name, key, got.Values, got.Counts, want.Values, want.Counts)
				}
			}
		}
	}
}

// The CellType declared in a spec (which becomes the generated TypeScript type) must describe the
// values the column really produces.
func TestDeclaredCellTypesMatchValues(t *testing.T) {
	for _, typ := range Types() {
		g := loadGolden(t, typ)
		spec, _ := Lookup(typ)
		for _, v := range variantsFor(t, g) {
			for _, e := range v.entries {
				r := NewRow(e, v.tcols, goldenNow(g))
				for _, col := range spec.Columns {
					got := col.Cell(&r)
					var kind string
					switch got.(type) {
					case nil:
						kind = "null"
					case string:
						kind = "string"
					case float64:
						kind = "number"
					case bool:
						kind = "boolean"
					default:
						t.Fatalf("%s.%s produced %T", typ, col.Key, got)
					}
					allowed := map[string]bool{}
					for _, part := range strings.Split(col.CellType, "|") {
						allowed[strings.TrimSpace(part)] = true
					}
					if !allowed[kind] {
						t.Errorf("%s.%s declares %q but produced %s (%v)", typ, col.Key, col.CellType, kind, got)
					}
				}
			}
		}
	}
}
