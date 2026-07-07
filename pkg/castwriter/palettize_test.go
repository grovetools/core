package castwriter

import (
	"bytes"
	"testing"
)

func TestPalettizeTruecolorFg(t *testing.T) {
	pz := NewPalettizer(nil)
	got := pz.Rewrite([]byte("\x1b[38;2;255;0;0mX"))
	want := []byte("\x1b[91mX") // pure red == default slot 9 (bright red) -> fg 91
	if !bytes.Equal(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestPalettizeTruecolorBg(t *testing.T) {
	pz := NewPalettizer(nil)
	got := pz.Rewrite([]byte("\x1b[48;2;0;0;255mX"))
	want := []byte("\x1b[104mX") // pure blue == default slot 12 -> bg 104
	if !bytes.Equal(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestPalettize256Cube(t *testing.T) {
	pz := NewPalettizer(nil)
	got := pz.Rewrite([]byte("\x1b[38;5;196m"))
	want := []byte("\x1b[91m") // xterm 196 == (255,0,0) -> slot 9 -> fg 91
	if !bytes.Equal(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestPalettize256LowIndex(t *testing.T) {
	pz := NewPalettizer(nil)
	got := pz.Rewrite([]byte("\x1b[38;5;4m"))
	want := []byte("\x1b[34m") // index 4 maps straight to slot 4 -> fg 34
	if !bytes.Equal(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestPalettizePreservesOtherParams(t *testing.T) {
	pz := NewPalettizer(nil)
	got := pz.Rewrite([]byte("\x1b[1;38;2;255;0;0;4mX"))
	want := []byte("\x1b[1;91;4mX") // bold + colour + underline; only the colour changes
	if !bytes.Equal(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestPalettizeSplitAcrossChunks(t *testing.T) {
	pz := NewPalettizer(nil)
	first := pz.Rewrite([]byte("\x1b[38;2;255;")) // partial CSI: buffered, nothing emitted
	if len(first) != 0 {
		t.Fatalf("expected nothing on partial chunk, got %q", first)
	}
	second := pz.Rewrite([]byte("0;0mX")) // completes the sequence
	want := []byte("\x1b[91mX")
	if !bytes.Equal(second, want) {
		t.Errorf("got %q want %q", second, want)
	}
}

func TestPalettizePassThrough(t *testing.T) {
	pz := NewPalettizer(nil)
	// A non-SGR CSI (clear screen) and plain text must survive untouched.
	in := []byte("\x1b[2Jhello world")
	got := pz.Rewrite(in)
	if !bytes.Equal(got, in) {
		t.Errorf("pass-through altered stream: got %q want %q", got, in)
	}
}

func TestPalettizeExactThemeMatch(t *testing.T) {
	// Slot 5 set to #abcdef; that exact truecolor must map to slot 5 (fg 35).
	pal := []string{"", "", "", "", "", "#abcdef"}
	pz := NewPalettizer(pal)
	got := pz.Rewrite([]byte("\x1b[38;2;171;205;239mX")) // 0xab,0xcd,0xef
	want := []byte("\x1b[35mX")
	if !bytes.Equal(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestPalettizeFlush(t *testing.T) {
	pz := NewPalettizer(nil)
	pz.Rewrite([]byte("\x1b[38;2;255;")) // buffered partial
	got := pz.Flush()
	want := []byte("\x1b[38;2;255;")
	if !bytes.Equal(got, want) {
		t.Errorf("flush got %q want %q", got, want)
	}
	if len(pz.Flush()) != 0 {
		t.Error("second flush should be empty")
	}
}
