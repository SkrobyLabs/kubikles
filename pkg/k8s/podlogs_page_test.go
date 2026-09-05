package k8s

import (
	"errors"
	"strings"
	"testing"
)

func TestReadBoundedLogPage(t *testing.T) {
	t.Run("stops after complete lookahead line", func(t *testing.T) {
		page, err := readBoundedLogPage(strings.NewReader("one\ntwo\nthree\n"+strings.Repeat("x", 100)), 20, 3)
		if err != nil || page != "one\ntwo\nthree\n" {
			t.Fatalf("page=%q error=%v", page, err)
		}
	})
	t.Run("never returns truncated cursor line", func(t *testing.T) {
		page, err := readBoundedLogPage(strings.NewReader("one\n"+strings.Repeat("x", 100)), 20, 3)
		if !errors.Is(err, errLogPageTooLarge) || page != "" {
			t.Fatalf("page=%q error=%v", page, err)
		}
	})
	t.Run("accepts EOF without trailing newline", func(t *testing.T) {
		page, err := readBoundedLogPage(strings.NewReader("one\ntwo"), 7, 3)
		if err != nil || page != "one\ntwo" {
			t.Fatalf("page=%q error=%v", page, err)
		}
	})
}
