package castwriter

import (
	"strconv"
	"strings"
)

// Palettizer down-converts truecolor and 256-colour SGR sequences in a terminal
// byte stream to the ANSI-16 palette. It is opt-in and composes with a Writer:
// pass Palettizer.Rewrite as Options.OutputFilter, or call Rewrite on the bytes
// you hand to Writer.WriteOutput.
//
// It rewrites the colour selectors inside CSI ... m (SGR) sequences:
//
//   - 38;2;r;g;b / 48;2;r;g;b (24-bit truecolor) -> nearest ANSI-16 slot
//   - 38;5;n / 48;5;n (256-colour) -> slot n directly for n < 16, otherwise the
//     xterm cube/greyscale colour is resolved and mapped to the nearest slot
//
// Matching is exact when the colour equals one of the supplied theme palette
// entries; otherwise the nearest slot by squared RGB distance is chosen. All
// other SGR parameters (bold, reset, the 30-37/40-47 basic colours, etc.) pass
// through untouched, as do non-SGR escape sequences and plain text.
//
// A partial escape sequence split across a chunk boundary is buffered
// internally and prepended to the next Rewrite call, so streaming works. Call
// Flush at end-of-stream to emit any buffered partial sequence verbatim.
//
// Limitations (this is a spike-quality filter):
//   - Only CSI-terminated-by-'m' sequences are inspected. A truecolor selector
//     embedded inside some other escape (e.g. an OSC string) is not rewritten.
//   - Nearest-colour matching uses plain squared RGB distance, not a
//     perceptual metric, so arbitrary colourschemes (e.g. an nvim theme) may
//     quantise imperfectly. Known grove theme colours map exactly.
//   - The internal palette assumes a dark-ish default when no theme is given.
type Palettizer struct {
	slots []rgb       // ANSI slots 0..15, RGB
	exact map[rgb]int // exact colour -> slot index
	carry []byte      // buffered partial escape across chunk boundaries
}

type rgb struct{ r, g, b uint8 }

// defaultANSI16 is a standard xterm 16-colour palette, used to fill any slot
// the caller's theme does not supply and as the nearest-match target set.
var defaultANSI16 = []rgb{
	{0x00, 0x00, 0x00}, // 0  black
	{0x80, 0x00, 0x00}, // 1  red
	{0x00, 0x80, 0x00}, // 2  green
	{0x80, 0x80, 0x00}, // 3  yellow
	{0x00, 0x00, 0x80}, // 4  blue
	{0x80, 0x00, 0x80}, // 5  magenta
	{0x00, 0x80, 0x80}, // 6  cyan
	{0xc0, 0xc0, 0xc0}, // 7  white
	{0x80, 0x80, 0x80}, // 8  bright black
	{0xff, 0x00, 0x00}, // 9  bright red
	{0x00, 0xff, 0x00}, // 10 bright green
	{0xff, 0xff, 0x00}, // 11 bright yellow
	{0x00, 0x00, 0xff}, // 12 bright blue
	{0xff, 0x00, 0xff}, // 13 bright magenta
	{0x00, 0xff, 0xff}, // 14 bright cyan
	{0xff, 0xff, 0xff}, // 15 bright white
}

// NewPalettizer builds a Palettizer. paletteHex is an optional theme palette of
// "#rrggbb" strings (up to 16); supplied entries override the corresponding
// default slot and become exact-match targets. A nil/short palette leaves the
// remaining slots at their default xterm values.
func NewPalettizer(paletteHex []string) *Palettizer {
	p := &Palettizer{
		slots: append([]rgb(nil), defaultANSI16...),
		exact: make(map[rgb]int, 16),
	}
	for i, h := range paletteHex {
		if i >= 16 {
			break
		}
		if c, ok := parseHexColor(h); ok {
			p.slots[i] = c
		}
	}
	// Build the exact-match map; lower slot indices win on duplicates.
	for i, c := range p.slots {
		if _, dup := p.exact[c]; !dup {
			p.exact[c] = i
		}
	}
	return p
}

// Rewrite returns data with truecolor/256-colour SGR selectors down-converted
// to ANSI-16. A trailing partial escape sequence is buffered and prepended to
// the next call.
func (p *Palettizer) Rewrite(data []byte) []byte {
	buf := data
	if len(p.carry) > 0 {
		buf = append(append([]byte(nil), p.carry...), data...)
		p.carry = p.carry[:0]
	}

	out := make([]byte, 0, len(buf))
	i := 0
	for i < len(buf) {
		b := buf[i]
		if b != 0x1b { // ESC
			out = append(out, b)
			i++
			continue
		}
		if i+1 >= len(buf) {
			p.carry = append(p.carry[:0], buf[i:]...) // lone trailing ESC
			return out
		}
		if buf[i+1] != '[' {
			out = append(out, b) // not a CSI; pass ESC through
			i++
			continue
		}
		// CSI: scan to the final byte (0x40..0x7e).
		j := i + 2
		for j < len(buf) && !(buf[j] >= 0x40 && buf[j] <= 0x7e) {
			j++
		}
		if j >= len(buf) {
			p.carry = append(p.carry[:0], buf[i:]...) // incomplete CSI
			return out
		}
		seq := buf[i : j+1]
		if buf[j] == 'm' {
			out = append(out, p.rewriteSGR(seq)...)
		} else {
			out = append(out, seq...)
		}
		i = j + 1
	}
	return out
}

// Flush emits any buffered partial escape sequence verbatim and clears it.
// Call at end-of-stream.
func (p *Palettizer) Flush() []byte {
	if len(p.carry) == 0 {
		return nil
	}
	out := append([]byte(nil), p.carry...)
	p.carry = p.carry[:0]
	return out
}

// rewriteSGR rewrites the colour selectors inside one CSI...m sequence,
// returning seq unchanged when nothing matched.
func (p *Palettizer) rewriteSGR(seq []byte) []byte {
	params := string(seq[2 : len(seq)-1]) // strip leading "ESC[" and trailing "m"
	if params == "" {
		return seq // ESC[m is a reset; nothing to convert
	}
	toks := strings.Split(params, ";")
	out := make([]string, 0, len(toks))
	changed := false
	for k := 0; k < len(toks); {
		t := toks[k]
		if t == "38" || t == "48" {
			fg := t == "38"
			if k+1 < len(toks) && toks[k+1] == "2" && k+4 < len(toks) {
				c := rgb{clampByte(toks[k+2]), clampByte(toks[k+3]), clampByte(toks[k+4])}
				out = append(out, ansiSGR(p.nearest(c), fg))
				changed = true
				k += 5
				continue
			}
			if k+1 < len(toks) && toks[k+1] == "5" && k+2 < len(toks) {
				out = append(out, ansiSGR(p.index256(atoi(toks[k+2])), fg))
				changed = true
				k += 3
				continue
			}
		}
		out = append(out, t)
		k++
	}
	if !changed {
		return seq
	}
	res := make([]byte, 0, len(seq))
	res = append(res, 0x1b, '[')
	res = append(res, strings.Join(out, ";")...)
	return append(res, 'm')
}

// index256 maps a 256-colour index to an ANSI-16 slot: indices below 16 are
// slots directly; the cube (16..231) and greyscale ramp (232..255) resolve to
// RGB and take the nearest slot.
func (p *Palettizer) index256(n int) int {
	if n < 0 {
		n = 0
	}
	if n < 16 {
		return n
	}
	if n > 255 {
		n = 255
	}
	return p.nearest(xterm256RGB(n))
}

// nearest returns the palette slot exactly matching c, or the slot minimising
// squared RGB distance.
func (p *Palettizer) nearest(c rgb) int {
	if i, ok := p.exact[c]; ok {
		return i
	}
	best, bestDist := 0, int64(1)<<62
	for i, s := range p.slots {
		dr := int64(c.r) - int64(s.r)
		dg := int64(c.g) - int64(s.g)
		db := int64(c.b) - int64(s.b)
		d := dr*dr + dg*dg + db*db
		if d < bestDist {
			bestDist, best = d, i
		}
	}
	return best
}

// xterm256RGB converts a 256-colour index (16..255) to its RGB value using the
// standard xterm 6x6x6 cube and 24-step greyscale ramp.
func xterm256RGB(n int) rgb {
	if n >= 232 {
		v := uint8(8 + (n-232)*10)
		return rgb{v, v, v}
	}
	n -= 16
	cube := func(x int) uint8 {
		if x == 0 {
			return 0
		}
		return uint8(55 + x*40)
	}
	return rgb{cube(n / 36), cube((n % 36) / 6), cube(n % 6)}
}

// ansiSGR renders an SGR parameter selecting ANSI slot idx (0..15) as a
// foreground (fg) or background colour.
func ansiSGR(idx int, fg bool) string {
	if idx < 8 {
		if fg {
			return strconv.Itoa(30 + idx)
		}
		return strconv.Itoa(40 + idx)
	}
	if fg {
		return strconv.Itoa(90 + (idx - 8))
	}
	return strconv.Itoa(100 + (idx - 8))
}

func parseHexColor(s string) (rgb, bool) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return rgb{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return rgb{}, false
	}
	return rgb{uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func clampByte(s string) uint8 {
	n := atoi(s)
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return uint8(n)
}
