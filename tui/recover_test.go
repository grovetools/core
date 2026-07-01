package tui

import (
	"strings"
	"testing"
)

func TestRecoverView(t *testing.T) {
	crash := func() (out string) {
		defer RecoverView(&out)
		panic("boom")
	}

	got := crash()
	if !strings.Contains(got, "panel crashed") {
		t.Fatalf("expected crash text to contain the crash message, got %q", got)
	}
	if !strings.Contains(got, "boom") {
		t.Fatalf("expected crash text to contain panic value, got %q", got)
	}
}
