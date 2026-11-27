package store

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
)

func TestLookupFindsWarmObjectsOnly(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("5", pod("default", "a", "1"), pod("other", "b", "2"))
	key := Key{Context: "c", Type: "pods"}

	if _, _, ok := f.m.Lookup(key, "default", "a", ""); ok {
		t.Fatal("a cold shard must not be looked up (and must not be started)")
	}
	l := f.acquire(t, key, Hint{})
	waitSynced(t, l)
	obj, rv, ok := f.m.Lookup(key, "default", "a", "")
	if !ok || rv == "" || obj == nil {
		t.Fatalf("lookup by name: %v %q %v", obj, rv, ok)
	}
	if _, _, ok := f.m.Lookup(key, "default", "zzz", ""); ok {
		t.Fatal("missing object")
	}
	// A namespaced key is served by the warm cluster-wide shard, only for its namespace.
	if _, _, ok := f.m.Lookup(Key{Context: "c", Type: "pods", Namespace: "other"}, "other", "b", ""); !ok {
		t.Fatal("namespaced lookup through the cluster-wide shard")
	}
	if _, _, ok := f.m.Lookup(Key{Context: "c", Type: "pods", Namespace: "other"}, "default", "a", ""); ok {
		t.Fatal("namespaced lookup must not leak other namespaces")
	}
}

// A lookup that names a uid means that object. When it is gone, another object
// that has taken its name must not be returned in its place.
func TestLookupByUIDDoesNotFallBackToName(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("5", pod("default", "web-0", "1"))
	key := Key{Context: "c", Type: "pods"}
	l := f.acquire(t, key, Hint{})
	waitSynced(t, l)

	obj, _, ok := f.m.Lookup(key, "default", "web-0", "")
	if !ok {
		t.Fatal("lookup by name")
	}
	acc, err := meta.Accessor(obj)
	if err != nil {
		t.Fatal(err)
	}
	uid := string(acc.GetUID())
	if !IsRealUID(uid) {
		t.Skipf("fixture pods carry no uid (%q); the uid path cannot be exercised", uid)
	}
	if _, _, ok := f.m.Lookup(key, "default", "web-0", uid); !ok {
		t.Fatal("lookup by its own uid")
	}
	if _, _, ok := f.m.Lookup(key, "default", "web-0", "uid-of-the-pod-that-was-deleted"); ok {
		t.Fatal("a uid that is gone must not resolve to the object now holding the name")
	}
	if _, _, ok := f.m.Lookup(key, "default", "web-0", "default/web-0"); !ok {
		t.Fatal("a namespace/name row key is not a uid and still goes by name")
	}
}
