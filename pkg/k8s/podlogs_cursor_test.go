package k8s

import "testing"

func TestNextLogSinceTimeAdvancesOneNanosecond(t *testing.T) {
	got := nextLogSinceTime("2026-01-01T00:00:00.123456789Z")
	want := "2026-01-01T00:00:00.12345679Z"
	if got != want {
		t.Fatalf("nextLogSinceTime() = %q, want %q", got, want)
	}
}

func TestNextLogSinceTimeCarriesIntoNextSecond(t *testing.T) {
	got := nextLogSinceTime("2026-01-01T00:00:00.999999999Z")
	want := "2026-01-01T00:00:01Z"
	if got != want {
		t.Fatalf("nextLogSinceTime() = %q, want %q", got, want)
	}
}

func TestNextLogSinceTimeIsExclusiveForIdenticalTimestamps(t *testing.T) {
	cursor := "2026-01-01T00:00:00.123456789Z"
	if got := nextLogSinceTime(cursor); got == cursor {
		t.Fatal("cursor must advance to prevent duplicate pages; identical-timestamp lines beyond one page are an API limitation")
	}
}
