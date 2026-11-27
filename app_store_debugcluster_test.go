//go:build debugcluster

package main

import (
	"testing"
	"time"

	"kubikles/pkg/k8s"
	"kubikles/pkg/store"
)

// The debug cluster is a fake clientset without a REST client, so this runs the
// real reflector, the application client provider and the Table-to-full
// degradation end to end.
func TestStoreLoadsDebugClusterThroughAppProvider(t *testing.T) {
	a := newPerfApp()
	a.k8sClient = &k8s.Client{}

	for _, mode := range []store.Mode{store.ModeFull, store.ModeTable, store.ModeMetadata} {
		m := store.New(storeClients{a}, store.Options{})
		defer m.Close()
		lease, err := m.Acquire(store.Key{Context: k8s.DebugClusterContextName, Type: "pods"}, store.Hint{Mode: mode})
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for !lease.State().Synced && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		st := lease.State()
		if !st.Synced || st.Entries == 0 {
			t.Fatalf("%s: debug cluster did not load: %+v", mode, st)
		}
		t.Logf("%s: entries=%d effective=%s degraded=%v %s", mode, st.Entries, st.Mode, st.Degraded, st.DegradedReason)
		lease.Release()
	}
}
