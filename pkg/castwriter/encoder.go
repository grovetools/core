package castwriter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Theme is the terminal colour theme embedded in a cast header. Palette is a
// slice of "#rrggbb" strings (8 or 16 entries); the encoder joins it with ":"
// as the format requires. A nil Theme is omitted from the header.
type Theme struct {
	Fg      string
	Bg      string
	Palette []string
}

// Options configures a Writer and the header it emits.
type Options struct {
	// Version selects the cast format: 2 or 3. Zero defaults to 3.
	Version int

	// Cols and Rows are the initial terminal dimensions.
	Cols int
	Rows int

	// Term is the terminal type (e.g. "xterm-256color"). It becomes term.type
	// in v3; v2 has no dedicated field for it, so it is ignored there.
	Term string
	// TermVersion is the emulator version string (v3 term.version only).
	TermVersion string

	// Title, Command, and Env populate the corresponding optional header fields.
	Title   string
	Command string
	Env     map[string]string

	// Timestamp is the recording's wall-clock start. Zero omits the header
	// timestamp field. Only whole seconds are recorded (the format's unit).
	Timestamp time.Time

	// IdleTimeLimit sets the header idle_time_limit hint in seconds (a playback
	// hint the player uses to compress idle stretches). Zero omits it. This is
	// independent of IdleCap, which bakes clamping into the recorded timing.
	IdleTimeLimit float64

	// IdleCap clamps the gap between consecutive events at write time: any pause
	// longer than IdleCap is recorded as exactly IdleCap. Zero disables it.
	IdleCap time.Duration

	// StartTime is the reference instant for relative timing. When set, the
	// first event's time is measured from it (so a recording that opens with a
	// pause records that pause). When zero, the first event becomes t=0 and
	// timing is measured from it.
	StartTime time.Time

	// Theme is the optional colour theme embedded in the header.
	Theme *Theme

	// OutputFilter, if set, transforms every output payload before it is
	// encoded (e.g. Palettizer.Rewrite for down-conversion). It does not apply
	// to resize or marker events.
	OutputFilter func([]byte) []byte
}

// Writer streams asciicast events to an underlying io.Writer. It is not safe
// for concurrent use; serialize calls if events arrive from multiple
// goroutines. The header is written when the Writer is created.
type Writer struct {
	w       io.Writer
	version int
	idleCap time.Duration
	filter  func([]byte) []byte

	hasStart bool
	lastRaw  time.Time
	elapsed  float64 // clamped cumulative seconds since start (v2 timing)
	err      error
}

// NewWriter writes the cast header to w and returns a Writer ready for events.
func NewWriter(w io.Writer, opts Options) (*Writer, error) {
	version := opts.Version
	if version == 0 {
		version = 3
	}
	if version != 2 && version != 3 {
		return nil, fmt.Errorf("castwriter: unsupported version %d (want 2 or 3)", version)
	}
	hdr, err := marshalHeader(version, opts)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(append(hdr, '\n')); err != nil {
		return nil, err
	}
	cw := &Writer{
		w:       w,
		version: version,
		idleCap: opts.IdleCap,
		filter:  opts.OutputFilter,
	}
	if !opts.StartTime.IsZero() {
		cw.hasStart = true
		cw.lastRaw = opts.StartTime
	}
	return cw, nil
}

// WriteOutput records terminal output bytes produced at time t.
func (w *Writer) WriteOutput(t time.Time, data []byte) error {
	if w.filter != nil {
		data = w.filter(data)
	}
	return w.event(t, 'o', data)
}

// WriteInput records terminal input bytes (an "i" event) produced at time t.
func (w *Writer) WriteInput(t time.Time, data []byte) error {
	return w.event(t, 'i', data)
}

// WriteResize records a terminal resize to cols x rows at time t.
func (w *Writer) WriteResize(t time.Time, cols, rows int) error {
	return w.event(t, 'r', []byte(strconv.Itoa(cols)+"x"+strconv.Itoa(rows)))
}

// WriteMarker records a marker with the given label at time t.
func (w *Writer) WriteMarker(t time.Time, label string) error {
	return w.event(t, 'm', []byte(label))
}

// Err reports the first error encountered while writing, if any.
func (w *Writer) Err() error { return w.err }

// event computes the version-correct timing for t, then writes one event line.
func (w *Writer) event(t time.Time, code byte, payload []byte) error {
	if w.err != nil {
		return w.err
	}
	var gap float64
	if !w.hasStart {
		w.hasStart = true
		w.lastRaw = t
	} else {
		d := t.Sub(w.lastRaw)
		if d < 0 {
			d = 0 // never go backwards on out-of-order timestamps
		}
		if w.idleCap > 0 && d > w.idleCap {
			d = w.idleCap
		}
		gap = d.Seconds()
		w.lastRaw = t
	}
	w.elapsed += gap

	stamp := gap // v3: interval since previous event
	if w.version == 2 {
		stamp = w.elapsed // v2: absolute seconds since start
	}

	line := make([]byte, 0, len(payload)+24)
	line = append(line, '[')
	line = append(line, formatSeconds(stamp)...)
	line = append(line, ',', ' ', '"', code, '"', ',', ' ')
	line = appendJSONString(line, payload)
	line = append(line, ']', '\n')
	_, w.err = w.w.Write(line)
	return w.err
}

// formatSeconds renders a timestamp with up to microsecond precision, trailing
// zeros trimmed but always at least one fractional digit (e.g. "0.0", "0.155",
// "3.818") to match the shape of hand- and tool-produced casts.
func formatSeconds(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	s := strconv.FormatFloat(sec, 'f', 6, 64)
	s = strings.TrimRight(s, "0")
	if strings.HasSuffix(s, ".") {
		s += "0"
	}
	return s
}

// appendJSONString appends s as a JSON string literal (including the enclosing
// quotes). ESC and other C0 control bytes become \u00xx escapes, with \r \n \t
// \" \\ special-cased; valid multi-byte UTF-8 is copied verbatim; invalid UTF-8
// bytes are replaced with U+FFFD. HTML metacharacters are left unescaped, so
// the output matches asciinema's own encoding rather than Go's HTML-safe one.
func appendJSONString(dst, s []byte) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); {
		b := s[i]
		if b < 0x80 {
			switch b {
			case '"':
				dst = append(dst, '\\', '"')
			case '\\':
				dst = append(dst, '\\', '\\')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				if b < 0x20 {
					dst = append(dst, '\\', 'u', '0', '0', hexDigit(b>>4), hexDigit(b&0x0f))
				} else {
					dst = append(dst, b)
				}
			}
			i++
			continue
		}
		r, size := utf8.DecodeRune(s[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 byte: emit the replacement character (lossy).
			dst = append(dst, 0xEF, 0xBF, 0xBD)
			i++
			continue
		}
		dst = append(dst, s[i:i+size]...)
		i += size
	}
	return append(dst, '"')
}

func hexDigit(v byte) byte {
	if v < 10 {
		return '0' + v
	}
	return 'a' + (v - 10)
}

// --- header structs -------------------------------------------------------

type themeJSON struct {
	Fg      string `json:"fg,omitempty"`
	Bg      string `json:"bg,omitempty"`
	Palette string `json:"palette,omitempty"`
}

func (t *Theme) toJSON() *themeJSON {
	if t == nil {
		return nil
	}
	return &themeJSON{Fg: t.Fg, Bg: t.Bg, Palette: strings.Join(t.Palette, ":")}
}

type v2Header struct {
	Version       int               `json:"version"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	Timestamp     int64             `json:"timestamp,omitempty"`
	IdleTimeLimit float64           `json:"idle_time_limit,omitempty"`
	Command       string            `json:"command,omitempty"`
	Title         string            `json:"title,omitempty"`
	Theme         *themeJSON        `json:"theme,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
}

type v3Term struct {
	Cols    int        `json:"cols"`
	Rows    int        `json:"rows"`
	Type    string     `json:"type,omitempty"`
	Version string     `json:"version,omitempty"`
	Theme   *themeJSON `json:"theme,omitempty"`
}

type v3Header struct {
	Version       int               `json:"version"`
	Term          v3Term            `json:"term"`
	Timestamp     int64             `json:"timestamp,omitempty"`
	IdleTimeLimit float64           `json:"idle_time_limit,omitempty"`
	Command       string            `json:"command,omitempty"`
	Title         string            `json:"title,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
}

// marshalHeader renders the version-appropriate header object as compact JSON
// (no trailing newline). HTML escaping is disabled so payload-adjacent header
// fields keep their literal bytes.
func marshalHeader(version int, opts Options) ([]byte, error) {
	var ts int64
	if !opts.Timestamp.IsZero() {
		ts = opts.Timestamp.Unix()
	}

	var v any
	if version == 2 {
		v = v2Header{
			Version:       2,
			Width:         opts.Cols,
			Height:        opts.Rows,
			Timestamp:     ts,
			IdleTimeLimit: opts.IdleTimeLimit,
			Command:       opts.Command,
			Title:         opts.Title,
			Theme:         opts.Theme.toJSON(),
			Env:           opts.Env,
		}
	} else {
		v = v3Header{
			Version: 3,
			Term: v3Term{
				Cols:    opts.Cols,
				Rows:    opts.Rows,
				Type:    opts.Term,
				Version: opts.TermVersion,
				Theme:   opts.Theme.toJSON(),
			},
			Timestamp:     ts,
			IdleTimeLimit: opts.IdleTimeLimit,
			Command:       opts.Command,
			Title:         opts.Title,
			Env:           opts.Env,
		}
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
