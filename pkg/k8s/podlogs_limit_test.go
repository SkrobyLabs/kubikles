package k8s

import (
	"math"
	"testing"
	"time"
)

// A caller supplied limit of MaxInt used to wrap limit+1 negative and panic.
func TestLogHistoryHugeLimitDoesNotPanic(t *testing.T) {
	srv := newStreamLogServer(50, time.Second)
	c := newAPIServerClient(t, srv)
	got, more, err := c.GetPodLogsBefore("ns", "pod", "", false, false, "2099-01-01T00:00:00Z", math.MaxInt)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(logLines(got)); n != 50 || more {
		t.Fatalf("lines=%d more=%v, want all 50 and no more", n, more)
	}
	if clampLogLimit(math.MaxInt) != maxLogLimit || clampLogLimit(-3) != defaultLogLimit {
		t.Fatal("clampLogLimit bounds wrong")
	}
}
