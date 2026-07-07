// Package castwriter encodes asciicast recordings (asciinema's .cast format)
// and transcodes the ecosystem's GTRC1 binary capture streams into them.
//
// It is a leaf package: standard library only, no grove dependencies, so it
// can be linked into tuimux, treemux, the daemon, tend, or a standalone CLI
// without dragging the rest of core along.
//
// # What it provides
//
//   - Writer — an asciicast v2 or v3 encoder over an io.Writer. Callers push
//     timestamped output/resize/marker events; the Writer converts wall-clock
//     times into the version-correct timing (v2 = absolute seconds since the
//     recording start; v3 = interval since the previous event) and JSON-encodes
//     the payloads so arbitrary terminal byte streams round-trip.
//
//   - TranscodeGTRC1 / ReadGTRC1 — decode a GTRC1 capture (the format written
//     by GROVE_TTY_CAPTURE and GROVE_PTY_CAPTURE) and emit it as a playable
//     cast. Every existing capture dump and soak-session artifact becomes a
//     cast for free.
//
//   - Palettizer — an opt-in stream transform that rewrites truecolor and
//     256-color SGR sequences down to the ANSI-16 palette, using an exact-match
//     map for a supplied theme and nearest-color fallback for everything else.
//
// # Format notes (verified against real casts)
//
// The header is a single JSON object on line one; every subsequent line is a
// JSON array event. The two versions differ in header shape and event timing:
//
//	v2 header: {"version":2,"width":W,"height":H,"timestamp":T,...}
//	v3 header: {"version":3,"term":{"cols":C,"rows":R,"type":..,"theme":{..}},"timestamp":T,...}
//	events:    [time,"o",data]  [time,"r","COLSxROWS"]  [time,"m","label"]
//
// In v2 the event time is seconds since the recording start (monotonically
// increasing); in v3 it is the interval since the previous event. The v3 theme
// lives inside the term object; in v2 it is a top-level "theme" key. The theme
// palette is a colon-joined list of "#rrggbb" colors (8 or 16 entries).
//
// Output payloads are JSON strings: ESC (0x1b) is escaped as the six-byte
// sequence backslash-u-0-0-1-b, CR/LF/TAB as backslash r/n/t, and valid
// multi-byte UTF-8 passes through raw (matching the real casts). HTML
// metacharacters are NOT escaped. Invalid UTF-8 bytes are replaced with
// U+FFFD, the same lossy handling asciinema applies.
package castwriter
