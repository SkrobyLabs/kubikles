package store

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"kubikles/pkg/issuedetector"
	"kubikles/pkg/k8s"
	"kubikles/pkg/tools"
)

// Behaviour of the non-UI readers of the warm store: with a synced shard they issue no LIST for
// that type and return what the live path returns; with a cold store they list live. Readers
// never start shards.

const consumerCtx = "ctx"

func consumerCluster() *fake.Clientset {
	pod := func(name, ns string, phase v1.PodPhase) *v1.Pod {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID("uid-" + name), ResourceVersion: "1", Labels: map[string]string{"app": "web"}},
			Status:     v1.PodStatus{Phase: phase},
		}
	}
	return fake.NewSimpleClientset(
		pod("web-1", "prod", v1.PodRunning), pod("web-2", "prod", v1.PodRunning),
		pod("done", "prod", v1.PodSucceeded), pod("other", "dev", v1.PodPending),
		&v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "prod", UID: "uid-svc"}, Spec: v1.ServiceSpec{Selector: map[string]string{"app": "web"}}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "prod", UID: "uid-dep"}},
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", UID: "uid-n1"}},
	)
}

type consumerEnv struct {
	cs     *fake.Clientset
	m      *Manager
	client *k8s.Client
}

func newConsumerEnv(t *testing.T) *consumerEnv {
	t.Helper()
	cs := consumerCluster()
	m := New(&fakeProvider{cs: cs}, Options{})
	t.Cleanup(m.Close)
	client := k8s.NewClientWithClientset(cs, consumerCtx)
	client.SetStoreReader(Reader{M: m})
	return &consumerEnv{cs: cs, m: m, client: client}
}

// warm starts and syncs a shard the way a view would.
func (e *consumerEnv) warm(t *testing.T, typ, ns string) {
	t.Helper()
	l, err := e.m.Acquire(Key{Context: consumerCtx, Type: typ, Namespace: ns}, Hint{Mode: ModeFull})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	waitSynced(t, l)
}

// lists counts LIST actions on a resource since the last ClearActions.
func (e *consumerEnv) lists(resource string) int {
	n := 0
	for _, a := range e.cs.Actions() {
		if a.Matches("list", resource) {
			n++
		}
	}
	return n
}

func TestStoreListWarmAndCold(t *testing.T) {
	e := newConsumerEnv(t)
	ctx := k8s.WithStoreRead(context.Background())

	if _, ok := k8s.StoreList[v1.Pod](ctx, e.client, consumerCtx, "pods", ""); ok {
		t.Fatal("a cold store must not answer")
	}
	if len(e.m.Stats().Shards) != 0 {
		t.Fatal("a read started a shard")
	}

	e.warm(t, "pods", "")
	pods, ok := k8s.StoreList[v1.Pod](ctx, e.client, consumerCtx, "pods", "")
	if !ok || len(pods) != 4 {
		t.Fatalf("warm cluster-wide read = %d pods, ok=%v", len(pods), ok)
	}
	prod, ok := k8s.StoreList[v1.Pod](ctx, e.client, consumerCtx, "pods", "prod")
	if !ok || len(prod) != 3 {
		t.Fatalf("a cluster-wide shard must serve a namespace: %d pods, ok=%v", len(prod), ok)
	}
	// Without the context option the store is never consulted: UI lists stay on their own path.
	if _, ok := k8s.StoreList[v1.Pod](context.Background(), e.client, consumerCtx, "pods", ""); ok {
		t.Fatal("a context without WithStoreRead must not read the store")
	}
	// A different type or a type mismatch is a miss.
	if _, ok := k8s.StoreList[v1.Service](ctx, e.client, consumerCtx, "pods", ""); ok {
		t.Fatal("type mismatch must be a miss")
	}
}

func TestStoreReaderNamespacedShardNeverServesClusterWide(t *testing.T) {
	e := newConsumerEnv(t)
	e.warm(t, "pods", "prod")
	ctx := k8s.WithStoreRead(context.Background())
	if _, ok := k8s.StoreList[v1.Pod](ctx, e.client, consumerCtx, "pods", ""); ok {
		t.Fatal("a namespaced shard must not answer a cluster-wide read")
	}
	if pods, ok := k8s.StoreList[v1.Pod](ctx, e.client, consumerCtx, "pods", "prod"); !ok || len(pods) != 3 {
		t.Fatalf("namespaced read = %d, ok=%v", len(pods), ok)
	}
}

func dependencyJSON(t *testing.T, g *k8s.DependencyGraph, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(g)
	return string(b)
}

func TestDependencyGraphWarmVersusCold(t *testing.T) {
	e := newConsumerEnv(t)
	g, err := e.client.GetResourceDependencies(consumerCtx, "pod", "prod", "web-1")
	coldJSON := dependencyJSON(t, g, err)
	if e.lists("services") == 0 || e.lists("ingresses") == 0 {
		t.Fatalf("cold run must list live, actions: %v", e.cs.Actions())
	}
	g, err = e.client.GetResourceDependencies(consumerCtx, "deployment", "prod", "web")
	coldDep := dependencyJSON(t, g, err)

	e.warm(t, "pods", "")
	e.warm(t, "services", "")
	e.warm(t, "ingresses", "")
	e.warm(t, "replicasets", "")
	e.warm(t, "jobs", "")
	e.cs.ClearActions()
	g, err = e.client.GetResourceDependencies(consumerCtx, "pod", "prod", "web-1")
	warmJSON := dependencyJSON(t, g, err)
	for _, r := range []string{"pods", "services", "ingresses", "replicasets", "jobs"} {
		if n := e.lists(r); n != 0 {
			t.Errorf("warm run listed %s %d times", r, n)
		}
	}
	g, err = e.client.GetResourceDependencies(consumerCtx, "deployment", "prod", "web")
	warmDep := dependencyJSON(t, g, err)
	if n := e.lists("pods") + e.lists("replicasets"); n != 0 {
		t.Errorf("warm deployment graph listed pods/replicasets %d times", n)
	}
	if warmDep != coldDep {
		t.Fatalf("deployment graphs differ\nwarm: %s\ncold: %s", warmDep, coldDep)
	}
	if warmJSON != coldJSON {
		t.Fatalf("graphs differ\nwarm: %s\ncold: %s", warmJSON, coldJSON)
	}
	if !strings.Contains(warmJSON, "web") {
		t.Fatalf("graph looks empty: %s", warmJSON)
	}
}

func TestIssueDetectorCacheWarmVersusCold(t *testing.T) {
	e := newConsumerEnv(t)
	fetch := func(namespaces []string) *issuedetector.ResourceCache {
		rc := issuedetector.NewResourceCache(e.client, namespaces)
		if err := rc.Fetch(context.Background(), []string{"pods", "services", "deployments", "nodes"}); err != nil {
			t.Fatal(err)
		}
		if errs := rc.Errors(); len(errs) > 0 {
			t.Fatalf("errors: %v", errs)
		}
		return rc
	}
	names := func(rc *issuedetector.ResourceCache) []string {
		var out []string
		for _, p := range rc.Pods() {
			out = append(out, p.Namespace+"/"+p.Name)
		}
		return out
	}
	sortedEqual := func(a, b []string) bool {
		m := map[string]int{}
		for _, x := range a {
			m[x]++
		}
		for _, x := range b {
			m[x]--
		}
		for _, n := range m {
			if n != 0 {
				return false
			}
		}
		return true
	}

	cold := fetch(nil)
	if e.lists("pods") == 0 {
		t.Fatal("cold scan must list live")
	}
	e.warm(t, "pods", "")
	e.warm(t, "services", "")
	e.warm(t, "deployments", "")
	e.warm(t, "nodes", "")
	e.cs.ClearActions()
	warm := fetch(nil)
	for _, r := range []string{"pods", "services", "deployments", "nodes"} {
		if n := e.lists(r); n != 0 {
			t.Errorf("warm scan listed %s %d times", r, n)
		}
	}
	if !sortedEqual(names(cold), names(warm)) {
		t.Fatalf("pods differ: warm %v cold %v", names(warm), names(cold))
	}
	for _, k := range []string{"pods", "services", "deployments", "nodes"} {
		if cold.ResourceCount(k) != warm.ResourceCount(k) {
			t.Errorf("%s count: cold %d warm %d", k, cold.ResourceCount(k), warm.ResourceCount(k))
		}
	}

	// A namespace-scoped scan is served by the cluster-wide shard too.
	e.cs.ClearActions()
	scoped := fetch([]string{"prod"})
	if n := e.lists("pods"); n != 0 {
		t.Errorf("namespaced warm scan listed pods %d times", n)
	}
	if len(scoped.Pods()) != 3 {
		t.Fatalf("scoped pods = %d", len(scoped.Pods()))
	}

	// One cold kind among the requested ones falls back for that kind only.
	e.cs.ClearActions()
	rc := issuedetector.NewResourceCache(e.client, nil)
	if err := rc.Fetch(context.Background(), []string{"pods", "configmaps"}); err != nil {
		t.Fatal(err)
	}
	if e.lists("pods") != 0 || e.lists("configmaps") == 0 {
		t.Fatalf("pods list=%d configmaps list=%d", e.lists("pods"), e.lists("configmaps"))
	}
	if rc.Pods() == nil || reflect.TypeOf(rc.Pods()).Kind() != reflect.Slice {
		t.Fatal("typed getters must return slices")
	}
}

func TestToolsListResourcesWarmVersusCold(t *testing.T) {
	e := newConsumerEnv(t)
	args := map[string]interface{}{"kind": "pods", "namespace": "prod"}
	coldOut, isErr := tools.CallTool(e.client, "list_resources", args)
	if isErr {
		t.Fatal(coldOut)
	}
	if e.lists("pods") == 0 {
		t.Fatal("cold tool call must list live")
	}
	e.warm(t, "pods", "")
	e.cs.ClearActions()
	warmOut, isErr := tools.CallTool(e.client, "list_resources", args)
	if isErr {
		t.Fatal(warmOut)
	}
	if n := e.lists("pods"); n != 0 {
		t.Fatalf("warm tool call listed pods %d times", n)
	}
	// The AGE column can tick between the two calls; compare the identity columns.
	strip := func(s string) []string {
		var rows []string
		for _, l := range strings.Split(s, "\n")[1:] {
			f := strings.Fields(l)
			if len(f) >= 3 {
				rows = append(rows, strings.Join(f[:3], " "))
			}
		}
		return rows
	}
	if !reflect.DeepEqual(strip(coldOut), strip(warmOut)) {
		t.Fatalf("tool output differs\nwarm: %s\ncold: %s", warmOut, coldOut)
	}
}

func TestNamespaceCountsWarmVersusCold(t *testing.T) {
	e := newConsumerEnv(t)
	cold, err := e.client.GetNamespaceResourceCounts("prod")
	if err != nil {
		t.Fatal(err)
	}
	e.warm(t, "pods", "")
	e.warm(t, "deployments", "")
	e.cs.ClearActions()
	warm, err := e.client.GetNamespaceResourceCounts("prod")
	if err != nil {
		t.Fatal(err)
	}
	if e.lists("pods") != 0 || e.lists("deployments") != 0 {
		t.Fatalf("warm counts listed pods=%d deployments=%d", e.lists("pods"), e.lists("deployments"))
	}
	if e.lists("services") == 0 {
		t.Fatal("a cold type must still be counted live")
	}
	// The fake clientset ignores the field selector of the live pod count, so compare through
	// the terminal-phase rule: 2 running pods in prod (the Succeeded one is excluded).
	if warm.Pods != 2 {
		t.Fatalf("warm pod count = %d, want 2", warm.Pods)
	}
	if cold.Deployments != warm.Deployments || cold.Services != warm.Services {
		t.Fatalf("cold %+v warm %+v", cold, warm)
	}
}

func TestReadersNeverStartShards(t *testing.T) {
	e := newConsumerEnv(t)
	_, _ = e.client.GetResourceDependencies(consumerCtx, "pod", "prod", "web-1")
	_, _ = e.client.GetNamespaceResourceCounts("prod")
	tools.CallTool(e.client, "list_resources", map[string]interface{}{"kind": "nodes"})
	rc := issuedetector.NewResourceCache(e.client, nil)
	_ = rc.Fetch(context.Background(), []string{"pods"})
	if n := len(e.m.Stats().Shards); n != 0 {
		t.Fatalf("readers started %d shards", n)
	}
	var sawWatch bool
	for _, a := range e.cs.Actions() {
		if a.GetVerb() == "watch" {
			sawWatch = true
		}
	}
	if sawWatch {
		t.Fatal("readers opened a watch")
	}
}
