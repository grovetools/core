package sidecar

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Key is one decoded keystroke.
//
// Name is the chord in the ecosystem's spelling — the same strings a plugin
// manifest declares, the host grants, and Client.Granted matches against.
// Getting that vocabulary right is the whole point: a sidecar that decodes
// 0x06 into its own idea of "C-f" cannot check it against a claim the host
// granted as "ctrl+f".
type Key struct {
	// Name is the chord: "a", "ctrl+f", "alt+x", "enter", "esc", "up",
	// "shift+tab", "f1".
	//
	// Two names are terminal EVENTS rather than keystrokes: "focus" and "blur",
	// the mode-1004 reports the host writes into the pane's PTY once the panel
	// has enabled them with CSI ? 1004 h. They come down the same stream keys
	// do, which is why they are decoded here.
	Name string
	// Rune is the character for a plain printable key, or 0.
	Rune rune
	// Bytes is the raw input this key was decoded from, for a panel that wants
	// to pass an unrecognised sequence through.
	Bytes []byte
}

// String returns the chord name, so a Key formats as what you would bind.
func (k Key) String() string { return k.Name }

// Decoder turns a raw input stream into keystrokes.
//
// It handles what a panel actually meets on stdin: control characters,
// CSI and SS3 escape sequences for the arrows, function and editing keys, the
// alt-prefix form, and UTF-8 for everything else. It is deliberately not a
// full terminfo implementation — a panel that needs mouse reporting, bracketed
// paste or the kitty protocol should use bubbletea through Run, which has all
// of it.
type Decoder struct {
	r *bufio.Reader
}

// NewDecoder reads keystrokes from r, normally Terminal.In().
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReaderSize(r, 256)}
}

// Next returns the next keystroke, or io.EOF when the stream ends.
func (d *Decoder) Next() (Key, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return Key{}, err
	}

	switch {
	case b == 0x1b:
		return d.escape()
	case b == 0x7f, b == 0x08:
		return Key{Name: "backspace", Bytes: []byte{b}}, nil
	case b == '\r', b == '\n':
		return Key{Name: "enter", Bytes: []byte{b}}, nil
	case b == '\t':
		return Key{Name: "tab", Bytes: []byte{b}}, nil
	case b == ' ':
		return Key{Name: "space", Rune: ' ', Bytes: []byte{b}}, nil
	case b < 0x20:
		// C0 controls are ctrl+<letter>: 0x01 is ctrl+a. 0x00 is ctrl+@ by
		// the same arithmetic, which is how a terminal sends ctrl+space.
		if b == 0 {
			return Key{Name: "ctrl+space", Bytes: []byte{b}}, nil
		}
		return Key{Name: "ctrl+" + string(rune('a'+b-1)), Bytes: []byte{b}}, nil
	case b < 0x80:
		return Key{Name: string(rune(b)), Rune: rune(b), Bytes: []byte{b}}, nil
	}

	// A multi-byte UTF-8 rune. Put the lead byte back and let the reader
	// assemble it, rather than decoding continuation bytes by hand.
	if err := d.r.UnreadByte(); err != nil {
		return Key{}, err
	}
	r, size, err := d.r.ReadRune()
	if err != nil {
		return Key{}, err
	}
	buf := make([]byte, size)
	copy(buf, string(r))
	return Key{Name: string(r), Rune: r, Bytes: buf}, nil
}

// escape decodes what follows an ESC byte.
//
// A bare ESC and the start of a sequence are the same byte, so the two are
// told apart by whether anything is buffered behind it. That is the standard
// ambiguity and the standard resolution; it is why a panel occasionally sees
// alt+<key> when a user typed ESC then the key very fast.
func (d *Decoder) escape() (Key, error) {
	if d.r.Buffered() == 0 {
		return Key{Name: "esc", Bytes: []byte{0x1b}}, nil
	}
	next, err := d.r.ReadByte()
	if err != nil {
		return Key{Name: "esc", Bytes: []byte{0x1b}}, nil
	}

	switch next {
	case '[':
		return d.csi()
	case 'O':
		// SS3: the application-mode arrows and F1-F4.
		final, err := d.r.ReadByte()
		if err != nil {
			return Key{Name: "esc", Bytes: []byte{0x1b, next}}, nil
		}
		if name, ok := ss3Names[final]; ok {
			return Key{Name: name, Bytes: []byte{0x1b, next, final}}, nil
		}
		return Key{Name: "unknown", Bytes: []byte{0x1b, next, final}}, nil
	default:
		// alt+<key>. Recurse so alt+ctrl+a and alt+<multibyte rune> decode
		// through the same path as their unprefixed forms.
		if err := d.r.UnreadByte(); err != nil {
			return Key{}, err
		}
		inner, err := d.Next()
		if err != nil {
			return Key{}, err
		}
		return Key{
			Name:  "alt+" + inner.Name,
			Rune:  inner.Rune,
			Bytes: append([]byte{0x1b}, inner.Bytes...),
		}, nil
	}
}

// csi decodes a CSI sequence: ESC [ params final.
func (d *Decoder) csi() (Key, error) {
	raw := []byte{0x1b, '['}
	var params strings.Builder
	for {
		b, err := d.r.ReadByte()
		if err != nil {
			return Key{Name: "unknown", Bytes: raw}, nil
		}
		raw = append(raw, b)
		// Parameter and intermediate bytes precede a final byte in 0x40-0x7e.
		if b >= 0x40 && b <= 0x7e {
			return Key{Name: csiName(params.String(), b), Bytes: raw}, nil
		}
		params.WriteByte(b)
	}
}

// csiName maps a CSI sequence to a chord name.
func csiName(params string, final byte) string {
	// Modifiers ride in a second parameter: "1;5C" is ctrl+right. The
	// modifier is a bitmask offset by one — 2 is shift, 3 alt, 5 ctrl.
	base, mod := params, ""
	if i := strings.IndexByte(params, ';'); i >= 0 {
		base, mod = params[:i], modifierPrefix(params[i+1:])
	}

	if name, ok := csiFinalNames[final]; ok {
		return mod + name
	}
	if final == '~' {
		if name, ok := csiTildeNames[base]; ok {
			return mod + name
		}
	}
	return "unknown"
}

// modifierPrefix turns a CSI modifier parameter into a chord prefix.
func modifierPrefix(param string) string {
	var n int
	if _, err := fmt.Sscanf(param, "%d", &n); err != nil || n < 2 {
		return ""
	}
	bits := n - 1
	var prefix string
	if bits&1 != 0 {
		prefix += "shift+"
	}
	if bits&2 != 0 {
		prefix += "alt+"
	}
	if bits&4 != 0 {
		prefix += "ctrl+"
	}
	return prefix
}

var (
	ss3Names = map[byte]string{
		'A': "up", 'B': "down", 'C': "right", 'D': "left",
		'H': "home", 'F': "end",
		'P': "f1", 'Q': "f2", 'R': "f3", 'S': "f4",
	}
	csiFinalNames = map[byte]string{
		'A': "up", 'B': "down", 'C': "right", 'D': "left",
		'H': "home", 'F': "end", 'Z': "shift+tab",
		// Not keystrokes: CSI I / CSI O are the mode-1004 focus reports, and
		// the host now emits them into every pane's PTY. They arrive on the
		// same stream as keys because that is where a terminal puts them, so
		// they are decoded here rather than left to fall out as "unknown" —
		// and they are the reason a panel does not need the control plane's
		// deprecated focus/blur frames. Enable them with CSI ? 1004 h.
		'I': "focus", 'O': "blur",
	}
	csiTildeNames = map[string]string{
		"1": "home", "2": "insert", "3": "delete", "4": "end",
		"5": "pgup", "6": "pgdown", "7": "home", "8": "end",
		"11": "f1", "12": "f2", "13": "f3", "14": "f4", "15": "f5",
		"17": "f6", "18": "f7", "19": "f8", "20": "f9", "21": "f10",
		"23": "f11", "24": "f12",
	}
)

// Keys streams decoded keystrokes on a channel, closing it when the input
// ends. Convenient for a select loop that also watches resizes and host
// events.
func (d *Decoder) Keys() <-chan Key {
	out := make(chan Key)
	go func() {
		defer close(out)
		for {
			k, err := d.Next()
			if err != nil {
				return
			}
			out <- k
		}
	}()
	return out
}
