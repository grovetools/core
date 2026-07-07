package castwriter

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// lines splits a cast buffer into its header line and event lines.
func lines(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	s := strings.TrimRight(buf.String(), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func TestV3Header(t *testing.T) {
	var buf bytes.Buffer
	_, err := NewWriter(&buf, Options{
		Cols:      80,
		Rows:      24,
		Term:      "xterm-256color",
		Timestamp: time.Unix(1700000000, 0),
		Env:       map[string]string{"SHELL": "/bin/sh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := lines(t, &buf)[0]
	want := `{"version":3,"term":{"cols":80,"rows":24,"type":"xterm-256color"},"timestamp":1700000000,"env":{"SHELL":"/bin/sh"}}`
	if got != want {
		t.Errorf("header mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestV2Header(t *testing.T) {
	var buf bytes.Buffer
	_, err := NewWriter(&buf, Options{
		Version:   2,
		Cols:      80,
		Rows:      24,
		Timestamp: time.Unix(1700000000, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := lines(t, &buf)[0]
	want := `{"version":2,"width":80,"height":24,"timestamp":1700000000}`
	if got != want {
		t.Errorf("header mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestV3HeaderTheme(t *testing.T) {
	var buf bytes.Buffer
	_, err := NewWriter(&buf, Options{
		Cols:      95,
		Rows:      25,
		Term:      "tmux-256color",
		Timestamp: time.Unix(1768594112, 0),
		Theme: &Theme{
			Fg:      "#c8c0a7",
			Bg:      "#0a0810",
			Palette: []string{"#14121f", "#8c3858"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := lines(t, &buf)[0]
	want := `{"version":3,"term":{"cols":95,"rows":25,"type":"tmux-256color","theme":{"fg":"#c8c0a7","bg":"#0a0810","palette":"#14121f:#8c3858"}},"timestamp":1768594112}`
	if got != want {
		t.Errorf("themed header mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestV3EventTiming(t *testing.T) {
	base := time.Unix(1700000000, 0)
	var buf bytes.Buffer
	w, err := NewWriter(&buf, Options{Cols: 80, Rows: 24, StartTime: base})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t,
		w.WriteOutput(base.Add(155*time.Millisecond), []byte("hi")),
		w.WriteOutput(base.Add(249*time.Millisecond), []byte("x")),
		w.WriteOutput(base.Add(249*time.Millisecond), []byte("y")),
		w.WriteResize(base.Add(249*time.Millisecond), 100, 30),
		w.WriteMarker(base.Add(1249*time.Millisecond), "chapter"),
	)
	want := []string{
		`[0.155, "o", "hi"]`,
		`[0.094, "o", "x"]`,
		`[0.0, "o", "y"]`,
		`[0.0, "r", "100x30"]`,
		`[1.0, "m", "chapter"]`,
	}
	assertEvents(t, &buf, want)
}

func TestV2EventTiming(t *testing.T) {
	base := time.Unix(1700000000, 0)
	var buf bytes.Buffer
	w, err := NewWriter(&buf, Options{Version: 2, Cols: 80, Rows: 24, StartTime: base})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t,
		w.WriteOutput(base.Add(155*time.Millisecond), []byte("hi")),
		w.WriteOutput(base.Add(249*time.Millisecond), []byte("x")),
		w.WriteOutput(base.Add(249*time.Millisecond), []byte("y")),
		w.WriteMarker(base.Add(1249*time.Millisecond), "chapter"),
	)
	want := []string{
		`[0.155, "o", "hi"]`,
		`[0.249, "o", "x"]`,
		`[0.249, "o", "y"]`,
		`[1.249, "m", "chapter"]`,
	}
	assertEvents(t, &buf, want)
}

func TestIdleCap(t *testing.T) {
	base := time.Unix(1700000000, 0)
	var buf bytes.Buffer
	w, err := NewWriter(&buf, Options{Cols: 80, Rows: 24, StartTime: base, IdleCap: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t,
		w.WriteOutput(base.Add(5*time.Second), []byte("a")),
		w.WriteOutput(base.Add(10*time.Second), []byte("b")),
	)
	want := []string{
		`[1.0, "o", "a"]`,
		`[1.0, "o", "b"]`,
	}
	assertEvents(t, &buf, want)
}

func TestOutputEscaping(t *testing.T) {
	base := time.Unix(1700000000, 0)
	var buf bytes.Buffer
	w, err := NewWriter(&buf, Options{Cols: 80, Rows: 24, StartTime: base})
	if err != nil {
		t.Fatal(err)
	}
	// ESC + CSI + CR + tab, then an <html>&amp; run, then an invalid UTF-8 byte.
	mustWrite(t,
		w.WriteOutput(base, append([]byte{0x1b}, []byte("[31m\r\t")...)),
		w.WriteOutput(base, []byte("<b>&")),
		w.WriteOutput(base, []byte{0xff}),
	)
	ev := lines(t, &buf)[1:]
	if ev[0] != `[0.0, "o", "\u001b[31m\r\t"]` {
		t.Errorf("escape mismatch: %q", ev[0])
	}
	if ev[1] != `[0.0, "o", "<b>&"]` {
		t.Errorf("html should not be escaped: %s", ev[1])
	}
	// Invalid byte becomes the UTF-8 replacement character U+FFFD (EF BF BD).
	if !strings.Contains(ev[2], "�") {
		t.Errorf("invalid UTF-8 not replaced: %q", ev[2])
	}
}

func TestLazyStart(t *testing.T) {
	// With no StartTime, the first event is t=0 regardless of its wall clock.
	base := time.Unix(1700000000, 0)
	var buf bytes.Buffer
	w, err := NewWriter(&buf, Options{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t,
		w.WriteOutput(base.Add(5*time.Second), []byte("a")),
		w.WriteOutput(base.Add(5500*time.Millisecond), []byte("b")),
	)
	assertEvents(t, &buf, []string{
		`[0.0, "o", "a"]`,
		`[0.5, "o", "b"]`,
	})
}

func mustWrite(t *testing.T, errs ...error) {
	t.Helper()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func assertEvents(t *testing.T, buf *bytes.Buffer, want []string) {
	t.Helper()
	got := lines(t, buf)
	if len(got) < 1 {
		t.Fatal("no output")
	}
	got = got[1:] // drop header
	if len(got) != len(want) {
		t.Fatalf("event count: got %d want %d\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d:\n got: %s\nwant: %s", i, got[i], want[i])
		}
	}
}
