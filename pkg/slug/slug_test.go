package slug

import (
	"strings"
	"testing"
)

func TestCanonical(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "lowercases and hyphenates", in: "Fix Shared Slugger", want: "fix-shared-slugger"},
		{name: "collapses and trims", in: " -- Fix   shared---slugger -- ", want: "fix-shared-slugger"},
		{name: "keeps ASCII letters and numbers", in: "Phase 2 ABC-123", want: "phase-2-abc-123"},
		{name: "discards punctuation", in: `Fix: foo/bar, #1 & (again)`, want: "fix-foobar-1-again"},
		{name: "discards non ASCII", in: "Café 東京 test", want: "caf-test"},
		{name: "handles other ASCII whitespace", in: "one\ttwo\nthree", want: "one-two-three"},
		{name: "only unsupported characters", in: "東京!!!", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Canonical(tt.in); got != tt.want {
				t.Fatalf("Canonical(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCanonicalCapsAndTrims(t *testing.T) {
	if got := Canonical(strings.Repeat("a", MaxLength+10)); len(got) != MaxLength {
		t.Fatalf("Canonical(long text) length = %d, want %d", len(got), MaxLength)
	}

	input := strings.Repeat("a", MaxLength-1) + "-tail"
	want := strings.Repeat("a", MaxLength-1)
	if got := Canonical(input); got != want {
		t.Fatalf("Canonical(trailing hyphen at cap) = %q, want %q", got, want)
	}
}

func TestStripNotePrefix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "20260703-fix-foo", want: "fix-foo"},
		{in: "20260703-142530-fix-foo", want: "fix-foo"},
		{in: "20260703-", want: ""},
		{in: "2026070-fix-foo", want: "2026070-fix-foo"},
		{in: "abcdefgh-fix-foo", want: "abcdefgh-fix-foo"},
		{in: "fix-foo", want: "fix-foo"},
	}
	for _, tt := range tests {
		if got := StripNotePrefix(tt.in); got != tt.want {
			t.Errorf("StripNotePrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStripJobPrefix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "18-fix-foo", want: "fix-foo"},
		{in: "01-fix-foo", want: "fix-foo"},
		{in: "123-fix-foo", want: "fix-foo"},
		{in: "18-", want: ""},
		{in: "-fix-foo", want: "-fix-foo"},
		{in: "job-fix-foo", want: "job-fix-foo"},
		{in: "18", want: "18"},
	}
	for _, tt := range tests {
		if got := StripJobPrefix(tt.in); got != tt.want {
			t.Errorf("StripJobPrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
