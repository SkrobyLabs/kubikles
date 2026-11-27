package main

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"kubikles/pkg/rows"
)

type fakeNotices struct {
	mu      sync.Mutex
	pending []rows.ViewNotice
	takes   int
}

func (f *fakeNotices) TakeNotices() []rows.ViewNotice {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.takes++
	out := f.pending
	f.pending = nil
	return out
}

func (f *fakeNotices) add(n rows.ViewNotice) {
	f.mu.Lock()
	f.pending = append(f.pending, n)
	f.mu.Unlock()
}

type emitted struct {
	mu   sync.Mutex
	name []string
	data []interface{}
}

func (e *emitted) emit(name string, data ...interface{}) {
	e.mu.Lock()
	e.name = append(e.name, name)
	e.data = append(e.data, data[0])
	e.mu.Unlock()
}

func (e *emitted) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.name)
}

func TestViewNotifierBatchesOneEventPerFrame(t *testing.T) {
	src := &fakeNotices{}
	out := &emitted{}
	n := newViewNotifier(func() noticeSource { return src }, func() time.Duration { return 20 * time.Millisecond }, out.emit)
	t.Cleanup(n.Stop)

	src.add(rows.ViewNotice{V: "a", Version: 1})
	n.Mark()
	n.Mark() // armed already: no second timer
	src.add(rows.ViewNotice{V: "b", Version: 2})
	n.Mark()
	time.Sleep(80 * time.Millisecond)
	if out.count() != 1 {
		t.Fatalf("expected one batched event, got %d", out.count())
	}
	batch, ok := out.data[0].([]rows.ViewNotice)
	if !ok || len(batch) != 2 || out.name[0] != viewUpdatesEvent {
		t.Fatalf("event %q payload %#v", out.name[0], out.data[0])
	}
	// Nothing pending: marking again emits nothing.
	n.Mark()
	time.Sleep(60 * time.Millisecond)
	if out.count() != 1 {
		t.Fatalf("empty flush must not emit, got %d events", out.count())
	}
	// The payload is a few bytes per view and never carries rows.
	b, _ := json.Marshal(batch)
	if strings.Contains(string(b), "rows") {
		t.Fatalf("notice payload mentions rows: %s", b)
	}
}

func TestViewNotifierFollowsBackgroundAndFlushesOnForeground(t *testing.T) {
	app := &App{}
	app.eventCoalescer = NewEventCoalescer(app, 10*time.Millisecond)
	src := &fakeNotices{}
	out := &emitted{}
	n := newViewNotifier(func() noticeSource { return src }, app.eventCoalescer.FrameDelay, out.emit)
	t.Cleanup(n.Stop)
	app.viewNotifier = n
	app.hookViewForeground()

	app.eventCoalescer.SetBackground(true)
	if d := app.eventCoalescer.FrameDelay(); d < backgroundFrameInterval {
		t.Fatalf("background delay %v", d)
	}
	src.add(rows.ViewNotice{V: "a"})
	n.Mark()
	time.Sleep(60 * time.Millisecond)
	if out.count() != 0 {
		t.Fatal("a hidden window must not receive notices at the foreground cadence")
	}
	app.eventCoalescer.SetBackground(false)
	if out.count() != 1 {
		t.Fatalf("leaving background must flush at once, got %d", out.count())
	}
}

func TestViewBindingsWithoutManager(t *testing.T) {
	app := &App{}
	if _, err := app.OpenView(rows.ViewSpec{Type: "configmaps"}); err == nil {
		t.Fatal("OpenView without a manager must fail")
	}
	if _, err := app.GetViewWindow("x", 0, 10, 0); err == nil {
		t.Fatal("GetViewWindow without a manager must fail")
	}
	app.CloseView("x")
	app.SetViewActive("x", true)
	if info := app.GetViewsInfo(); info.Enabled || len(info.Types) != len(rows.Types()) {
		t.Fatalf("info %+v", info)
	}
}

func TestViewsKillSwitch(t *testing.T) {
	for _, v := range []string{"0", "false", "off"} {
		t.Setenv(envDisableViews, v)
		if viewsEnabled() {
			t.Errorf("KUBIKLES_VIEWS=%s must disable views", v)
		}
	}
	t.Setenv(envDisableViews, "1")
	if !viewsEnabled() {
		t.Error("KUBIKLES_VIEWS=1 keeps views on")
	}
	t.Setenv(envDisableViews, "")
	if !viewsEnabled() {
		t.Error("views are on by default")
	}
}

func TestEncodeObjectDoesNotMutateSharedEntries(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: "a", Namespace: "d", ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl"}},
	}, Data: map[string]string{"k": "v"}}
	raw, err := encodeObject(cm)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["kind"] != "ConfigMap" || m["apiVersion"] != "v1" {
		t.Fatalf("type meta %v", m)
	}
	if md := m["metadata"].(map[string]interface{}); md["managedFields"] != nil {
		t.Fatalf("managed fields leaked: %v", md)
	}
	if cm.Kind != "" || len(cm.ManagedFields) != 1 {
		t.Fatal("the shared store entry must not be mutated")
	}

	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "example.com/v1", "kind": "Widget",
		"metadata": map[string]interface{}{"name": "w", "managedFields": []interface{}{map[string]interface{}{"manager": "x"}}},
	}}
	raw, err = encodeObject(u)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "managedFields") {
		t.Fatalf("unstructured managed fields leaked: %s", raw)
	}
	if _, ok := u.Object["metadata"].(map[string]interface{})["managedFields"]; !ok {
		t.Fatal("the original unstructured object must be left alone")
	}
}

func TestResourceForResolvesBuiltinsAndCRDs(t *testing.T) {
	gvr, namespaced, err := resourceFor("configmaps")
	if err != nil || gvr.Resource != "configmaps" || !namespaced {
		t.Fatalf("configmaps: %v %v %v", gvr, namespaced, err)
	}
	gvr, _, err = resourceFor("crd:example.com/v1/widgets")
	if err != nil || gvr.Group != "example.com" || gvr.Resource != "widgets" {
		t.Fatalf("crd: %v %v", gvr, err)
	}
	if _, _, err := resourceFor("nonsense"); err == nil {
		t.Fatal("unknown type")
	}
}
