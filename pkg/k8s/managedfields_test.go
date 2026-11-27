package k8s

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func podWithManagedFields(name string) corev1.Pod {
	p := newRichPod(1)
	p.Name = name
	return *p
}

func TestStripManagedFieldsTyped(t *testing.T) {
	items := []corev1.Pod{podWithManagedFields("a"), podWithManagedFields("b")}
	if len(items[0].ManagedFields) == 0 {
		t.Fatal("fixture should carry managedFields")
	}
	stripManagedFields(items)
	for _, p := range items {
		if p.ManagedFields != nil {
			t.Fatalf("%s still has managedFields", p.Name)
		}
		if p.Labels["app"] != "web" || p.Spec.NodeName == "" {
			t.Fatalf("%s lost other data", p.Name)
		}
	}
}

func TestStripManagedFieldsUnstructuredMaps(t *testing.T) {
	items := []map[string]interface{}{
		{"metadata": map[string]interface{}{"name": "a", "managedFields": []interface{}{map[string]interface{}{"manager": "x"}}}},
		{"metadata": map[string]interface{}{"name": "b"}},
		{"spec": map[string]interface{}{}}, // no metadata at all
	}
	stripManagedFields(items)
	if _, ok := items[0]["metadata"].(map[string]interface{})["managedFields"]; ok {
		t.Fatal("managedFields not removed from custom resource map")
	}
	if items[0]["metadata"].(map[string]interface{})["name"] != "a" {
		t.Fatal("name was lost")
	}
}

func TestStripManagedFieldsIgnoresNonObjects(t *testing.T) {
	// Must not panic for element types without metadata.
	stripManagedFields([]string{"x"})
	stripManagedFields([]int(nil))
}

// managedFieldsPodServer serves a pod list whose pods carry managedFields.
func managedFieldsPodServer(total int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list := corev1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"}}
		if r.URL.Query().Get("fieldSelector") != "" || r.URL.Path == "/api/v1/pods" {
			for i := 0; i < total; i++ {
				list.Items = append(list.Items, podWithManagedFields("pod"))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})
}

func TestListPodsStripsManagedFieldsFastAndPaginated(t *testing.T) {
	c := newAPIServerClient(t, managedFieldsPodServer(3))

	fast, err := c.ListPodsWithContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	paged, err := c.ListPodsWithContext(context.Background(), "", func(loaded, total int) {})
	if err != nil {
		t.Fatal(err)
	}
	for name, pods := range map[string][]corev1.Pod{"fast": fast, "paginated": paged} {
		if len(pods) != 3 {
			t.Fatalf("%s: got %d pods", name, len(pods))
		}
		for _, p := range pods {
			if p.ManagedFields != nil {
				t.Fatalf("%s: managedFields leaked into list result", name)
			}
		}
	}
}

func TestListPodsForNodeStripsManagedFields(t *testing.T) {
	c := newAPIServerClient(t, managedFieldsPodServer(2))
	pods, err := c.ListPodsForNode("node-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 2 {
		t.Fatalf("got %d pods", len(pods))
	}
	for _, p := range pods {
		if p.ManagedFields != nil {
			t.Fatal("managedFields leaked from ListPodsForNode")
		}
	}
}

func TestPaginatedListObserverPagesAreStripped(t *testing.T) {
	// Streamed pages carry the same items that the final result does.
	pages := 0
	ctx := WithListPageObserver(context.Background(), func(p ListPage) {
		pages++
		for _, pod := range p.Items.([]corev1.Pod) {
			if pod.ManagedFields != nil {
				t.Fatal("observer page carries managedFields")
			}
		}
	})
	if _, err := paginatedList(ctx, "pods", 2, func(ctx context.Context, opts metav1.ListOptions) ([]corev1.Pod, string, *int64, error) {
		items := []corev1.Pod{podWithManagedFields("x")}
		cont := ""
		if opts.Continue == "" {
			cont = "next"
		}
		return items, cont, nil, nil
	}, func(loaded, total int) {}); err != nil {
		t.Fatal(err)
	}
	if pages == 0 {
		t.Fatal("observer was never called")
	}
}

func TestEncodeWatchObjectTypedStripsAndMatchesTypedJSON(t *testing.T) {
	pod := newRichPod(3)
	want := pod.DeepCopy()
	want.ManagedFields = nil

	info, raw, err := EncodeWatchObject(pod)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != pod.Name || info.Namespace != "production" || info.ResourceVersion != "1000003" {
		t.Fatalf("info = %+v", info)
	}
	if strings.Contains(string(raw), "managedFields") {
		t.Fatal("managedFields present in encoded watch object")
	}
	if got, exp := canonicalJSON(t, json.RawMessage(raw)), canonicalJSON(t, want); got != exp {
		t.Fatalf("encoded JSON differs from typed marshal:\n got %s\nwant %s", got, exp)
	}
}

func TestEncodeWatchObjectUnstructured(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "example.com/v1",
		"kind":       "Widget",
		"metadata": map[string]interface{}{
			"name": "w", "namespace": "ns", "resourceVersion": "9",
			"managedFields": []interface{}{map[string]interface{}{"manager": "kubectl"}},
		},
		"spec": map[string]interface{}{"replicas": int64(3)},
	}}
	info, raw, err := EncodeWatchObject(u)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "w" || info.Namespace != "ns" || info.ResourceVersion != "9" {
		t.Fatalf("info = %+v", info)
	}
	if strings.Contains(string(raw), "managedFields") {
		t.Fatal("managedFields present")
	}
	if strings.HasSuffix(string(raw), "\n") {
		t.Fatal("raw message must not carry a trailing newline")
	}
	var back map[string]interface{}
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back["spec"].(map[string]interface{})["replicas"] != float64(3) || back["kind"] != "Widget" {
		t.Fatalf("content changed: %v", back)
	}
}

func TestEncodeWatchObjectRejectsObjectsWithoutMetadata(t *testing.T) {
	if _, _, err := EncodeWatchObject(&metav1.Status{}); err == nil {
		t.Fatal("expected an error for an object without ObjectMeta")
	}
	if _, err := WatchObjectInfo(&metav1.Status{}); err == nil {
		t.Fatal("expected an error from WatchObjectInfo")
	}
}

func TestWatchObjectInfoBookmark(t *testing.T) {
	// A bookmark carries only a resourceVersion.
	bm := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "777"}}
	info, err := WatchObjectInfo(bm)
	if err != nil || info.ResourceVersion != "777" || info.Name != "" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}
