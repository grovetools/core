package process

import (
	"os"
	"testing"
	"time"
)

func TestParseElapsed(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"00:07", 7 * time.Second},
		{"12:34", 12*time.Minute + 34*time.Second},
		{"01:02:03", time.Hour + 2*time.Minute + 3*time.Second},
		{"2-03:04:05", 2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second},
		{" 05:00 ", 5 * time.Minute},
		// Unparseable input must yield 0, which callers read as "unknown age"
		// and therefore refuse to sweep on — never as "brand new".
		{"", 0},
		{"?", 0},
		{"nonsense", 0},
		{"1:2:3:4", 0},
	}

	for _, tc := range cases {
		if got := ParseElapsed(tc.in); got != tc.want {
			t.Errorf("ParseElapsed(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestListFindsThisProcess(t *testing.T) {
	entries, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("List returned no processes")
	}

	self := os.Getpid()
	for _, e := range entries {
		if e.PID != self {
			continue
		}
		if e.Args == "" {
			t.Error("our own entry has an empty command line; the argv column was lost")
		}
		if e.PPID <= 0 {
			t.Errorf("our own entry has ppid %d", e.PPID)
		}
		return
	}
	t.Fatalf("List did not include our own pid %d", self)
}
