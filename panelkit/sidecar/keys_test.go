package sidecar

import (
	"io"
	"strings"
	"testing"
)

func decodeAll(t *testing.T, in string) []string {
	t.Helper()
	d := NewDecoder(strings.NewReader(in))
	var names []string
	for {
		k, err := d.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatalf("Next() = %v", err)
		}
		names = append(names, k.Name)
	}
}

func TestDecodePrintable(t *testing.T) {
	if got, want := decodeAll(t, "abc"), []string{"a", "b", "c"}; !equal(got, want) {
		t.Errorf("decoded %v, want %v", got, want)
	}
}

// The names have to be the ecosystem's spelling, because they are matched
// against claims the host granted. A decoder inventing "C-f" cannot check
// itself against a grant of "ctrl+f".
func TestDecodeControlChords(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"\x06", "ctrl+f"},
		{"\x01", "ctrl+a"},
		{"\x1a", "ctrl+z"},
		{"\x00", "ctrl+space"},
		{"\r", "enter"},
		{"\n", "enter"},
		{"\t", "tab"},
		{" ", "space"},
		{"\x7f", "backspace"},
	}
	for _, tt := range tests {
		if got := decodeAll(t, tt.in); len(got) != 1 || got[0] != tt.want {
			t.Errorf("decode(%q) = %v, want [%s]", tt.in, got, tt.want)
		}
	}
}

func TestDecodeArrowsAndEditingKeys(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"csi up", "\x1b[A", "up"},
		{"csi down", "\x1b[B", "down"},
		{"csi right", "\x1b[C", "right"},
		{"csi left", "\x1b[D", "left"},
		{"csi home", "\x1b[H", "home"},
		{"csi end", "\x1b[F", "end"},
		{"shift+tab", "\x1b[Z", "shift+tab"},
		{"ss3 up (application mode)", "\x1bOA", "up"},
		{"ss3 f1", "\x1bOP", "f1"},
		{"delete", "\x1b[3~", "delete"},
		{"page up", "\x1b[5~", "pgup"},
		{"page down", "\x1b[6~", "pgdown"},
		{"f5", "\x1b[15~", "f5"},
		{"f12", "\x1b[24~", "f12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeAll(t, tt.in); len(got) != 1 || got[0] != tt.want {
				t.Errorf("decode(%q) = %v, want [%s]", tt.in, got, tt.want)
			}
		})
	}
}

func TestDecodeModifiedArrows(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"\x1b[1;5C", "ctrl+right"},
		{"\x1b[1;2D", "shift+left"},
		{"\x1b[1;3A", "alt+up"},
		{"\x1b[1;6B", "shift+ctrl+down"},
	}
	for _, tt := range tests {
		if got := decodeAll(t, tt.in); len(got) != 1 || got[0] != tt.want {
			t.Errorf("decode(%q) = %v, want [%s]", tt.in, got, tt.want)
		}
	}
}

func TestDecodeAltPrefix(t *testing.T) {
	if got := decodeAll(t, "\x1bx"); len(got) != 1 || got[0] != "alt+x" {
		t.Errorf("decode(esc x) = %v, want [alt+x]", got)
	}
	// alt+ctrl+a: the recursion means modifier stacking needs no special case.
	if got := decodeAll(t, "\x1b\x01"); len(got) != 1 || got[0] != "alt+ctrl+a" {
		t.Errorf("decode(esc ^A) = %v, want [alt+ctrl+a]", got)
	}
}

// A lone ESC and the start of a sequence are the same byte; they are told
// apart by whether anything follows immediately.
func TestDecodeBareEscape(t *testing.T) {
	if got := decodeAll(t, "\x1b"); len(got) != 1 || got[0] != "esc" {
		t.Errorf("decode(esc) = %v, want [esc]", got)
	}
}

func TestDecodeMultibyteRune(t *testing.T) {
	got := decodeAll(t, "é→")
	if len(got) != 2 || got[0] != "é" || got[1] != "→" {
		t.Errorf("decode of multibyte runes = %v, want [é →]", got)
	}
}

func TestDecodeUnknownSequenceIsNotFatal(t *testing.T) {
	// An unrecognised CSI must be consumed whole and reported as unknown, not
	// leak its parameter bytes into the stream as keystrokes.
	got := decodeAll(t, "\x1b[200~a")
	if len(got) != 2 {
		t.Fatalf("decoded %v, want two keys", got)
	}
	if got[0] != "unknown" {
		t.Errorf("first key = %q, want unknown", got[0])
	}
	if got[1] != "a" {
		t.Errorf("second key = %q, want the key after the sequence", got[1])
	}
}

func TestDecodeMixedStream(t *testing.T) {
	got := decodeAll(t, "j\x1b[Bk\x06\r")
	want := []string{"j", "down", "k", "ctrl+f", "enter"}
	if !equal(got, want) {
		t.Errorf("decoded %v, want %v", got, want)
	}
}

func TestKeyCarriesRawBytes(t *testing.T) {
	d := NewDecoder(strings.NewReader("\x1b[A"))
	k, err := d.Next()
	if err != nil {
		t.Fatalf("Next() = %v", err)
	}
	if string(k.Bytes) != "\x1b[A" {
		t.Errorf("Bytes = %q, want the raw sequence", k.Bytes)
	}
	if k.String() != "up" {
		t.Errorf("String() = %q, want up", k.String())
	}
}

func TestKeysChannelClosesAtEOF(t *testing.T) {
	d := NewDecoder(strings.NewReader("ab"))
	var got []string
	for k := range d.Keys() {
		got = append(got, k.Name)
	}
	if !equal(got, []string{"a", "b"}) {
		t.Errorf("streamed %v, want [a b]", got)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
