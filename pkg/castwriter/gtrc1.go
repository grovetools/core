package castwriter

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// GTRC1 is the ecosystem's binary terminal-capture format, written by
// GROVE_TTY_CAPTURE (compositor/ext) and GROVE_PTY_CAPTURE (compositor/ghostty).
// Layout: the 5-byte magic "GTRC1", then a sequence of records, each
//
//	[8B unix-nanos LE][1B kind][4B payload-len LE][payload]
//
// kind 0 = output bytes (payload is the raw stream); kind 1 = resize (payload
// is [4B cols LE][4B rows LE]).
const gtrc1Magic = "GTRC1"

const (
	gtrc1KindBytes  = 0
	gtrc1KindResize = 1
)

// GTRC1Record is one decoded record from a GTRC1 stream. For output records,
// Data holds the bytes; for resize records, Cols and Rows hold the dimensions.
type GTRC1Record struct {
	Time time.Time
	Kind byte
	Data []byte
	Cols int
	Rows int
}

// ReadGTRC1 is the streaming primitive: it validates the magic, then decodes
// records one at a time and invokes fn for each. A truncated tail (a common
// artifact of the fixed-size capture ring) stops iteration cleanly rather than
// erroring. If fn returns an error, iteration stops and that error is returned.
func ReadGTRC1(r io.Reader, fn func(GTRC1Record) error) error {
	br := bufio.NewReader(r)

	magic := make([]byte, len(gtrc1Magic))
	if _, err := io.ReadFull(br, magic); err != nil {
		return fmt.Errorf("castwriter: reading GTRC1 magic: %w", err)
	}
	if string(magic) != gtrc1Magic {
		return fmt.Errorf("castwriter: bad GTRC1 magic %q", magic)
	}

	head := make([]byte, 13)
	for {
		if _, err := io.ReadFull(br, head); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil // clean or truncated end of stream
			}
			return err
		}
		ns := binary.LittleEndian.Uint64(head[0:8])
		kind := head[8]
		n := binary.LittleEndian.Uint32(head[9:13])

		payload := make([]byte, n)
		if _, err := io.ReadFull(br, payload); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil // truncated final record
			}
			return err
		}

		rec := GTRC1Record{Time: time.Unix(0, int64(ns)), Kind: kind}
		switch kind {
		case gtrc1KindResize:
			if len(payload) >= 8 {
				rec.Cols = int(binary.LittleEndian.Uint32(payload[0:4]))
				rec.Rows = int(binary.LittleEndian.Uint32(payload[4:8]))
			}
		default:
			rec.Data = payload
		}
		if err := fn(rec); err != nil {
			return err
		}
	}
}

// TranscodeGTRC1 decodes a GTRC1 stream from r and writes an asciicast to w
// using opts. The initial terminal size is taken from the stream's first record
// if it is a resize; otherwise opts.Cols/Rows are used. Timing is anchored to
// the first record's timestamp (opts.StartTime, if unset, is derived from it),
// so pauses before the first output are preserved. opts.Version, Theme, Env,
// IdleCap, OutputFilter, etc. all apply as usual.
func TranscodeGTRC1(r io.Reader, w io.Writer, opts Options) error {
	var cw *Writer

	err := ReadGTRC1(r, func(rec GTRC1Record) error {
		if cw == nil {
			// Seed the header from the first record.
			o := opts
			if o.StartTime.IsZero() {
				o.StartTime = rec.Time
			}
			if rec.Kind == gtrc1KindResize {
				o.Cols = rec.Cols
				o.Rows = rec.Rows
			}
			var e error
			if cw, e = NewWriter(w, o); e != nil {
				return e
			}
			if rec.Kind == gtrc1KindResize {
				// The leading resize seeded the header; it is not an event.
				return nil
			}
			return cw.WriteOutput(rec.Time, rec.Data)
		}

		switch rec.Kind {
		case gtrc1KindResize:
			return cw.WriteResize(rec.Time, rec.Cols, rec.Rows)
		default:
			return cw.WriteOutput(rec.Time, rec.Data)
		}
	})
	if err != nil {
		return err
	}

	if cw == nil {
		// Empty stream: still emit a valid (event-less) cast header.
		_, err = NewWriter(w, opts)
	}
	return err
}
