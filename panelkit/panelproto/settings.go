package panelproto

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// Settings is a panel's free-form [tui.plugins.<name>.settings] table. It
// arrives at two places carrying the same shape — Welcome.Settings at the
// handshake and Config.Settings on every later edit — so a panel writes one
// decode and calls it from both. Welcome.Config projects the first into the
// second so that "both" is one function taking one type.
//
// It is a map because the host does not interpret it: grove cannot know what a
// third-party panel's options mean, so the table crosses verbatim and the
// panel owns validation of its own keys. The methods below are what a panel
// does about that, and they exist because the obvious hand-rolled version is
// wrong in a way that never announces itself.
//
// # Every number is a float64
//
// The table reaches a panel as JSON, whatever the manifest declared it as. So
// `work_minutes = 25` in TOML arrives as float64(25), and
//
//	if v, ok := s["work_minutes"].(int); ok { … }
//
// is false for a value the user set correctly. The panel then runs on its
// default forever, the user re-reads their config and finds nothing wrong with
// it, and nothing anywhere says why. Decode and the typed accessors below both
// close that trap: Decode goes through encoding/json, which converts by
// destination type, and the accessors take any numeric form the map can hold.
//
// # Which one to use
//
// Decode for a panel with more than a key or two: it names its options once, in
// a struct, and REPORTS a type mismatch instead of silently defaulting. The
// accessors for a panel with one or two knobs, or for reading a key whose
// presence is itself the question — they take a default and never fail, which
// is convenient and is also the thing to be careful about.
type Settings map[string]any

// Decode unmarshals the whole table into dst, a pointer to the panel's own
// settings struct, matching keys by `json` tag.
//
// SEED dst WITH YOUR DEFAULTS before calling. A key the user did not set is
// left untouched, so settings only ever OVERRIDE — which is the same rule that
// lets a panel run standalone with no host, no welcome and no table at all:
//
//	s := settings{WorkMinutes: m.workMin, BreakMinutes: m.breakMin}
//	if err := cfg.Settings.Decode(&s); err != nil {
//	    m.settingsErr = err.Error()   // report it in your own pane
//	    return
//	}
//
// Unknown keys are ignored, deliberately: a user may be carrying config for a
// newer build of the panel, and the protocol is additive everywhere else too.
// A type mismatch is NOT ignored — it is the error, naming the offending key —
// because that is exactly the case a hand-rolled assertion swallows. Range and
// meaning are still yours: Decode says work_minutes is an int, not that -3 is
// a sane one.
//
// On an error, dst may have been partially written (encoding/json keeps going
// past a bad key), so decode into a local seeded from your live values and
// commit it only on success, as the snippet above does.
func (s Settings) Decode(dst any) error {
	if len(s) == 0 {
		return nil
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("panelproto: settings: %w", err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			// json's own message names the Go struct field and reads like a
			// compiler diagnostic. The user edited a TOML key; say that.
			return fmt.Errorf("panelproto: settings: %s: cannot use %s as %s",
				typeErr.Field, typeErr.Value, typeErr.Type)
		}
		return fmt.Errorf("panelproto: settings: %w", err)
	}
	return nil
}

// Has reports whether the key is present at all, which is the one question a
// default cannot answer: `enabled = false` and an absent `enabled` are the same
// value and different intentions.
func (s Settings) Has(key string) bool {
	_, ok := s[key]
	return ok
}

// Int returns key as an int, or def if it is absent, not a number, or not a
// whole one. A fractional value is a miss rather than a truncation: silently
// turning 0.5 into 0 is how a panel ends up disabled by a setting the user
// thought they were tuning.
func (s Settings) Int(key string, def int) int {
	f, ok := s.number(key)
	if !ok || f != math.Trunc(f) || f > math.MaxInt64 || f < math.MinInt64 {
		return def
	}
	return int(f)
}

// Float returns key as a float64, or def if it is absent or not a number.
func (s Settings) Float(key string, def float64) float64 {
	if f, ok := s.number(key); ok {
		return f
	}
	return def
}

// String returns key as a string, or def if it is absent or not one. A number
// is not coerced to its text: a panel asking for a string wants a string, and
// "25" arriving where 25 was written is a config bug worth seeing.
func (s Settings) String(key, def string) string {
	if v, ok := s[key].(string); ok {
		return v
	}
	return def
}

// Bool returns key as a bool, or def if it is absent or not one.
func (s Settings) Bool(key string, def bool) bool {
	if v, ok := s[key].(bool); ok {
		return v
	}
	return def
}

// Strings returns key as a []string, or def if it is absent, not a list, or
// holds anything that is not a string. A TOML array crosses the wire as a
// []any, so the element-wise check is the whole reason this exists.
func (s Settings) Strings(key string, def []string) []string {
	switch v := s[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			str, ok := e.(string)
			if !ok {
				return def
			}
			out = append(out, str)
		}
		return out
	}
	return def
}

// Table returns a nested table — [tui.plugins.<name>.settings.<key>] — as
// Settings, so the accessors work at any depth. A missing or non-table key
// yields nil, which is a usable empty Settings rather than a panic.
func (s Settings) Table(key string) Settings {
	switch v := s[key].(type) {
	case Settings:
		return v
	case map[string]any:
		return v
	}
	return nil
}

// number reads any numeric form the map can hold.
//
// float64 is what the wire delivers and covers a panel talking to a host. The
// integer forms cover a table built in-process — a test, a standalone panel
// reading its own file — where nothing forced it through JSON. json.Number
// covers a decoder configured with UseNumber.
func (s Settings) number(key string) (float64, bool) {
	switch v := s[key].(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	}
	return 0, false
}
