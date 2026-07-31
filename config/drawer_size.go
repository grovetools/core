package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/invopop/jsonschema"
)

// DrawerSize is a drawer extent written either as an absolute cell count
// (`drawer_size = 35`) or as a percentage of the terminal along the drawer's
// axis (`drawer_size = "25%"`). It is stored as the raw string it was written
// as — resolution against a live terminal extent belongs to the TUI host, which
// is the only place that knows the terminal size and the per-orientation floor.
//
// The two spellings exist because they answer different questions. An absolute
// value is "give the drawer exactly this many columns", which is what a user
// who has tuned one layout on one machine wants. A percentage is "give the
// drawer this share of the screen", which is what survives moving between a
// laptop and an external display. Neither is a good default for the other, so
// the key accepts both rather than picking a winner.
type DrawerSize string

// drawerPercentRE matches the percentage spelling: an integer or decimal
// followed by a percent sign, with optional surrounding space. Anchored, so a
// value like "25% of width" is rejected rather than silently read as 25%.
var drawerPercentRE = regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?)\s*%\s*$`)

// UnmarshalText decodes a DrawerSize from any scalar TOML/YAML node. TOML
// hands the raw token bytes to encoding.TextUnmarshaler before it inspects the
// node kind, which is exactly what makes one key accept both `35` and `"25%"`
// without a bespoke decoder.
func (d *DrawerSize) UnmarshalText(text []byte) error {
	v := DrawerSize(strings.TrimSpace(string(text)))
	if err := v.Validate(); err != nil {
		return err
	}
	*d = v
	return nil
}

// UnmarshalJSON accepts the same two spellings from JSON, where a number is a
// distinct token type and would otherwise fail against a string-kinded field.
func (d *DrawerSize) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		return d.UnmarshalText([]byte(s))
	}
	return d.UnmarshalText([]byte(trimmed))
}

// Validate reports whether the value is one of the two accepted spellings.
// The empty value is valid and means "unset".
func (d DrawerSize) Validate() error {
	s := strings.TrimSpace(string(d))
	if s == "" {
		return nil
	}
	if m := drawerPercentRE.FindStringSubmatch(s); m != nil {
		pct, err := strconv.ParseFloat(m[1], 64)
		if err != nil || pct <= 0 {
			return fmt.Errorf("drawer size %q: percentage must be greater than 0", s)
		}
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("drawer size %q: want an integer (35) or a percentage (\"25%%\")", s)
	}
	if n <= 0 {
		return fmt.Errorf("drawer size %q: must be greater than 0", s)
	}
	return nil
}

// Resolve turns the value into a concrete cell count against extent — the
// terminal's width for a right drawer, its height for a bottom one. ok is false
// when the value is unset or unparseable, which is the caller's signal to fall
// back to its own default; no floor or clamp is applied here, because those are
// host policy and Resolve is the shared arithmetic.
//
// A percentage of a not-yet-known extent (extent <= 0) resolves to nothing
// rather than to zero: before the first window size there is no share to take.
func (d DrawerSize) Resolve(extent int) (int, bool) {
	s := strings.TrimSpace(string(d))
	if s == "" {
		return 0, false
	}
	if m := drawerPercentRE.FindStringSubmatch(s); m != nil {
		pct, err := strconv.ParseFloat(m[1], 64)
		if err != nil || pct <= 0 || extent <= 0 {
			return 0, false
		}
		resolved := int(float64(extent)*pct/100 + 0.5)
		if resolved < 1 {
			resolved = 1
		}
		return resolved, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// IsPercent reports whether the value is written as a share of the terminal.
// Percent sizes have to be recomputed when the terminal is resized; absolute
// ones never change, so hosts use this to skip needless relayouts.
func (d DrawerSize) IsPercent() bool {
	return drawerPercentRE.MatchString(strings.TrimSpace(string(d)))
}

// JSONSchema declares the union shape by hand: the reflector sees a string
// kind and would otherwise emit `"type": "string"`, which makes the load-path
// validator reject the integer spelling that TOML users reach for first.
//
// The string branch accepts a bare number as well as a percentage because both
// consumers need it. An editor validating grove.toml sees whichever spelling
// the user typed, while the load-path validator (config.validateAndWarn) sees
// the DECODED struct re-serialized to YAML — where an absolute size is always
// the string it was normalized to. A percent-only pattern would flag every
// config that sets `drawer_size = 35` the moment it was read back.
func (DrawerSize) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "integer", Minimum: json.Number("1")},
			{Type: "string", Pattern: `^\s*[0-9]+(\.[0-9]+)?\s*%?\s*$`},
		},
	}
}
