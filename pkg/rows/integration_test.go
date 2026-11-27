package rows

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"

	"kubikles/pkg/store"
)

type fakeClients struct{ cs *fake.Clientset }

func (f fakeClients) Kube(string) (kubernetes.Interface, error) { return f.cs, nil }
func (f fakeClients) Dynamic(string) (dynamic.Interface, error) {
	return nil, fmt.Errorf("no dynamic client")
}
func (f fakeClients) Metadata(string) (metadata.Interface, error) { return nil, store.ErrNoMetadata }

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Views over the real store: a Table-needing type degrades to full objects on a fake clientset,
// and the secret spec must read type and key count from either representation.
func TestViewsOverRealStore(t *testing.T) {
	secret := func(ns, name string, typ corev1.SecretType, keys int) *corev1.Secret {
		s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}, Type: typ}
		if keys > 0 {
			s.Data = map[string][]byte{}
		}
		for i := 0; i < keys; i++ {
			s.Data[fmt.Sprintf("k%d", i)] = []byte("v")
		}
		return s
	}
	cs := fake.NewSimpleClientset(
		secret("default", "tls-a", corev1.SecretTypeTLS, 2),
		secret("default", "sh.helm.release.v1.x.v1", HelmReleaseSecretType, 1),
		secret("prod", "plain", corev1.SecretTypeOpaque, 0),
	)
	sm := store.New(fakeClients{cs}, store.Options{})
	t.Cleanup(sm.Close)
	m := NewManager(Config{Provider: StoreProvider{M: sm}, Context: func() string { return "c" }, NoJanitor: true})
	t.Cleanup(m.Close)

	res, err := m.Open(ViewSpec{ViewID: "s", ClientToken: "t", Type: "secrets", Namespaces: []string{"*"}, Hide: []string{"helm"}, Sort: SortSpec{Key: "name", Dir: "asc"}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "secrets view complete", func() bool {
		st, _ := m.State("s")
		return st.Phase == PhaseComplete && st.Total == 2
	})
	w, err := m.Window("s", 0, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Order) != 2 {
		t.Fatalf("order %v", w.Order)
	}
	if !w.State.Degraded {
		t.Fatalf("a Table shard on a fake clientset must report that it degraded to full objects: %+v", w.State)
	}
	idx := map[string]int{}
	for i, c := range res.Columns {
		idx[c] = i
	}
	for _, row := range w.Rows {
		switch row.Name {
		case "tls-a":
			if row.C[idx["type"]] != "kubernetes.io/tls" || row.C[idx["keys"]] != float64(2) {
				t.Errorf("tls-a cells %v", row.C)
			}
		case "plain":
			if row.C[idx["keys"]] != float64(0) {
				t.Errorf("plain cells %v", row.C)
			}
		default:
			t.Errorf("unexpected row %s (helm secrets must be hidden)", row.Name)
		}
	}

	// Live change flows through the real store into the open view.
	if _, err := cs.CoreV1().Secrets("default").Create(context.Background(), secret("default", "late", corev1.SecretTypeOpaque, 3), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "late secret", func() bool {
		st, _ := m.State("s")
		return st.Total == 3
	})
	if err := cs.CoreV1().Secrets("prod").Delete(context.Background(), "plain", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "delete applied", func() bool {
		st, _ := m.State("s")
		return st.Total == 2
	})

	// Search runs over the fields both representations carry.
	q := "type:tls"
	if _, err := m.Update("s", ViewPatch{Search: &q}); err != nil {
		t.Fatal(err)
	}
	w, _ = m.Window("s", 0, 10, 0)
	if len(w.Order) != 1 || len(w.Rows) != 1 {
		t.Fatalf("search order %v", w.Order)
	}
	for _, row := range w.Rows {
		if row.Name != "tls-a" {
			t.Fatalf("search matched %s", row.Name)
		}
	}

	// Closing releases the shard lease; the store then lingers it, and Reset closes everything.
	m.Reset("context")
	sm.Reset()
	if _, err := m.Window("s", 0, 1, 0); err != ErrViewNotFound {
		t.Fatalf("after reset: %v", err)
	}
}

func TestNamespacesViewOverRealStore(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating}},
	)
	sm := store.New(fakeClients{cs}, store.Options{})
	t.Cleanup(sm.Close)
	m := NewManager(Config{Provider: StoreProvider{M: sm}, Context: func() string { return "c" }, NoJanitor: true})
	t.Cleanup(m.Close)
	if _, err := m.Open(ViewSpec{ViewID: "n", ClientToken: "t", Type: "namespaces", Namespaces: []string{"ignored"}, Sort: SortSpec{Key: "name", Dir: "desc"}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "namespaces", func() bool { st, _ := m.State("n"); return st.Total == 2 && st.Phase == PhaseComplete })
	f, err := m.Facet("n", "status")
	if err != nil || !reflect.DeepEqual(f.Values, []string{"Active", "Terminating"}) {
		t.Fatalf("facet %+v %v", f, err)
	}
	refs, _, _ := m.Refs("n", 0, 0, 0)
	if len(refs) != 2 || refs[0].Name != "b" {
		t.Fatalf("refs %v", refs)
	}
}
