package castwriter

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// gtrc1Builder assembles a GTRC1 stream for tests.
type gtrc1Builder struct {
	buf bytes.Buffer
}

func newGTRC1() *gtrc1Builder {
	b := &gtrc1Builder{}
	b.buf.WriteString(gtrc1Magic)
	return b
}

func (b *gtrc1Builder) record(t time.Time, kind byte, payload []byte) {
	var head [13]byte
	binary.LittleEndian.PutUint64(head[0:8], uint64(t.UnixNano()))
	head[8] = kind
	binary.LittleEndian.PutUint32(head[9:13], uint32(len(payload)))
	b.buf.Write(head[:])
	b.buf.Write(payload)
}

func (b *gtrc1Builder) resize(t time.Time, cols, rows int) {
	var p [8]byte
	binary.LittleEndian.PutUint32(p[0:4], uint32(cols))
	binary.LittleEndian.PutUint32(p[4:8], uint32(rows))
	b.record(t, gtrc1KindResize, p[:])
}

func (b *gtrc1Builder) output(t time.Time, data []byte) {
	b.record(t, gtrc1KindBytes, data)
}

func TestTranscodeGTRC1(t *testing.T) {
	base := time.Unix(1700000000, 0)
	g := newGTRC1()
	g.resize(base, 100, 30) // leading resize seeds the header
	g.output(base.Add(100*time.Millisecond), []byte("hello"))
	g.resize(base.Add(200*time.Millisecond), 120, 40)
	g.output(base.Add(300*time.Millisecond), []byte{0x1b, 'X'})

	var out bytes.Buffer
	if err := TranscodeGTRC1(&g.buf, &out, Options{Cols: 24, Rows: 5, Term: "xterm-256color"}); err != nil {
		t.Fatal(err)
	}
	got := lines(t, &out)

	// Header takes its dimensions from the leading resize, not the defaults.
	wantHeader := `{"version":3,"term":{"cols":100,"rows":30,"type":"xterm-256color"},"timestamp":0}`
	// timestamp is 0 (omitted) since Options.Timestamp was zero:
	wantHeader = `{"version":3,"term":{"cols":100,"rows":30,"type":"xterm-256color"}}`
	if got[0] != wantHeader {
		t.Errorf("header:\n got: %s\nwant: %s", got[0], wantHeader)
	}

	wantEvents := []string{
		`[0.1, "o", "hello"]`,
		`[0.1, "r", "120x40"]`,
		`[0.1, "o", "\u001bX"]`,
	}
	if len(got)-1 != len(wantEvents) {
		t.Fatalf("event count: got %d want %d", len(got)-1, len(wantEvents))
	}
	for i, w := range wantEvents {
		if got[i+1] != w {
			t.Errorf("event %d:\n got: %s\nwant: %s", i, got[i+1], w)
		}
	}
}

func TestTranscodeGTRC1NoLeadingResize(t *testing.T) {
	base := time.Unix(1700000000, 0)
	g := newGTRC1()
	g.output(base, []byte("first"))
	g.output(base.Add(250*time.Millisecond), []byte("second"))

	var out bytes.Buffer
	if err := TranscodeGTRC1(&g.buf, &out, Options{Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	got := lines(t, &out)
	if got[0] != `{"version":3,"term":{"cols":80,"rows":24}}` {
		t.Errorf("header should use caller default dims: %s", got[0])
	}
	want := []string{`[0.0, "o", "first"]`, `[0.25, "o", "second"]`}
	for i, w := range want {
		if got[i+1] != w {
			t.Errorf("event %d:\n got: %s\nwant: %s", i, got[i+1], w)
		}
	}
}

func TestTranscodeGTRC1BadMagic(t *testing.T) {
	var out bytes.Buffer
	err := TranscodeGTRC1(bytes.NewReader([]byte("NOPE.")), &out, Options{Cols: 80, Rows: 24})
	if err == nil {
		t.Fatal("expected error on bad magic")
	}
}

func TestTranscodeGTRC1Truncated(t *testing.T) {
	base := time.Unix(1700000000, 0)
	g := newGTRC1()
	g.resize(base, 80, 24)
	g.output(base.Add(100*time.Millisecond), []byte("ok"))
	g.output(base.Add(200*time.Millisecond), []byte("more")) // this record gets chopped
	// Chop the stream mid-record to simulate a truncated capture-ring tail.
	full := g.buf.Bytes()
	truncated := full[:len(full)-3]

	var out bytes.Buffer
	if err := TranscodeGTRC1(bytes.NewReader(truncated), &out, Options{Cols: 80, Rows: 24}); err != nil {
		t.Fatalf("truncation should not error: %v", err)
	}
	got := lines(t, &out)
	if len(got) < 2 || got[1] != `[0.1, "o", "ok"]` {
		t.Errorf("expected the intact record to survive, got: %v", got)
	}
}

func TestTranscodeGTRC1WithPalettize(t *testing.T) {
	base := time.Unix(1700000000, 0)
	g := newGTRC1()
	g.resize(base, 80, 24)
	g.output(base.Add(10*time.Millisecond), []byte("\x1b[38;2;255;0;0mR"))

	pz := NewPalettizer(nil)
	var out bytes.Buffer
	err := TranscodeGTRC1(&g.buf, &out, Options{Cols: 80, Rows: 24, OutputFilter: pz.Rewrite})
	if err != nil {
		t.Fatal(err)
	}
	got := lines(t, &out)
	if got[1] != `[0.01, "o", "\u001b[91mR"]` {
		t.Errorf("palettized transcode:\n got: %s\nwant: %s", got[1], `[0.01, "o", "\u001b[91mR"]`)
	}
}
