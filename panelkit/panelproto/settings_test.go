package panelproto

import (
	"encoding/json"
	"strings"
	"testing"
)

type timerSettings struct {
	WorkMinutes  int     `json:"work_minutes"`
	BreakMinutes int     `json:"break_minutes"`
	Label        string  `json:"label"`
	Loud         bool    `json:"loud"`
	Scale        float64 `json:"scale"`
}

// The whole reason this helper exists: a number written as an integer in TOML
// crosses the wire as a JSON number and lands in the map as a float64, so the
// obvious `.(int)` assertion misses a value the user set correctly. Decode goes
// through encoding/json, which converts by destination type.
func TestDecodeConvertsWireFloatsToInts(t *testing.T) {
	raw := []byte(`{"label":"Focus","settings":{"work_minutes":50,"break_minutes":10}}`)
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if _, ok := cfg.Settings["work_minutes"].(int); ok {
		t.Fatal("work_minutes asserted as int — the trap this helper exists for is gone, so the test is wrong")
	}

	s := timerSettings{WorkMinutes: 25, BreakMinutes: 5}
	if err := cfg.Settings.Decode(&s); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if s.WorkMinutes != 50 || s.BreakMinutes != 10 {
		t.Fatalf("decoded = %d/%d, want 50/10", s.WorkMinutes, s.BreakMinutes)
	}
}

// Seeded defaults survive every key the user did not set, which is what lets a
// panel run standalone: settings only ever override.
func TestDecodeLeavesUnsetKeysAlone(t *testing.T) {
	tests := []struct {
		name string
		s    Settings
	}{
		{"partial table", Settings{"work_minutes": 50.0}},
		{"empty table", Settings{}},
		{"nil table", nil},
		{"only unknown keys", Settings{"future_option": "from a newer build"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := timerSettings{WorkMinutes: 25, BreakMinutes: 5, Label: "Breaktimer"}
			if err := tt.s.Decode(&s); err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if s.BreakMinutes != 5 || s.Label != "Breaktimer" {
				t.Fatalf("unset keys were clobbered: %+v", s)
			}
		})
	}
}

// A type mismatch is the one thing Decode must NOT swallow — it is precisely
// what a hand-rolled assertion turns into a silent default. The message names
// the user's key, not the Go field.
func TestDecodeReportsTypeMismatchByKey(t *testing.T) {
	tests := []struct {
		name string
		s    Settings
		want string
	}{
		{"string where a number belongs", Settings{"work_minutes": "ten"}, "work_minutes"},
		{"fraction where a whole number belongs", Settings{"break_minutes": 2.5}, "break_minutes"},
		{"number where a string belongs", Settings{"label": 3.0}, "label"},
		{"string where a bool belongs", Settings{"loud": "yes"}, "loud"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := timerSettings{WorkMinutes: 25, BreakMinutes: 5}
			err := tt.s.Decode(&s)
			if err == nil {
				t.Fatalf("Decode(%v) returned no error", tt.s)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not name the offending key %q", err, tt.want)
			}
			if s.WorkMinutes != 25 || s.BreakMinutes != 5 {
				t.Fatalf("a rejected table still moved the defaults: %+v", s)
			}
		})
	}
}

func TestAccessorsTakeAnyNumericForm(t *testing.T) {
	s := Settings{
		"wire":    50.0, // as JSON delivers it
		"native":  50,   // as an in-process table holds it
		"wide":    int64(50),
		"decoded": json.Number("50"),
	}
	for _, key := range []string{"wire", "native", "wide", "decoded"} {
		if got := s.Int(key, 25); got != 50 {
			t.Fatalf("Int(%q) = %d, want 50", key, got)
		}
	}
}

func TestAccessorsFallBackToDefaults(t *testing.T) {
	s := Settings{
		"work_minutes": 50.0,
		"fraction":     0.5,
		"text":         "hello",
		"loud":         true,
		"number":       7.0,
		"tags":         []any{"a", "b"},
		"mixed":        []any{"a", 2.0},
		"nested":       map[string]any{"inner": 3.0},
	}

	if got := s.Int("work_minutes", 25); got != 50 {
		t.Fatalf("Int hit = %d, want 50", got)
	}
	if got := s.Int("absent", 25); got != 25 {
		t.Fatalf("Int miss = %d, want the default 25", got)
	}
	// A fraction is a miss, not a truncation: 0.5 quietly becoming 0 is how a
	// panel ends up disabled by a setting the user thought they were tuning.
	if got := s.Int("fraction", 25); got != 25 {
		t.Fatalf("Int on a fraction = %d, want the default 25", got)
	}
	if got := s.Int("text", 25); got != 25 {
		t.Fatalf("Int on a string = %d, want the default 25", got)
	}
	if got := s.Float("fraction", 1); got != 0.5 {
		t.Fatalf("Float = %v, want 0.5", got)
	}
	if got := s.String("text", "def"); got != "hello" {
		t.Fatalf("String = %q, want hello", got)
	}
	// A number is not coerced to its text; asking for a string means one.
	if got := s.String("number", "def"); got != "def" {
		t.Fatalf("String on a number = %q, want the default", got)
	}
	if got := s.Bool("loud", false); !got {
		t.Fatal("Bool = false, want true")
	}
	if got := s.Bool("text", true); !got {
		t.Fatal("Bool on a string did not fall back to the default")
	}
	if got := s.Strings("tags", nil); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("Strings = %v, want [a b]", got)
	}
	if got := s.Strings("mixed", []string{"def"}); len(got) != 1 || got[0] != "def" {
		t.Fatalf("Strings on a mixed list = %v, want the default", got)
	}
	if got := s.Table("nested").Int("inner", 0); got != 3 {
		t.Fatalf("Table().Int = %d, want 3", got)
	}
	// A missing nested table is usable, not a panic.
	if got := s.Table("absent").Int("inner", 9); got != 9 {
		t.Fatalf("Table on a missing key = %d, want the default 9", got)
	}
	if !s.Has("loud") || s.Has("absent") {
		t.Fatal("Has does not distinguish present from absent")
	}
}

// Welcome and config_changed carry the same two facts, so a panel writes one
// apply function taking one type.
func TestWelcomeConfigProjectsTheSameShape(t *testing.T) {
	w := &Welcome{Label: "Breaktimer", Settings: Settings{"work_minutes": 50.0}}
	cfg := w.Config()
	if cfg.Label != "Breaktimer" {
		t.Fatalf("Label = %q, want Breaktimer", cfg.Label)
	}
	if cfg.Settings.Int("work_minutes", 25) != 50 {
		t.Fatalf("Settings did not survive the projection: %v", cfg.Settings)
	}

	// The no-host case: a nil welcome projects an empty Config, so a panel's
	// apply function keeps its defaults instead of panicking.
	var missing *Welcome
	empty := missing.Config()
	if empty.Label != "" || empty.Settings != nil {
		t.Fatalf("nil Welcome projected %+v, want the zero Config", empty)
	}
	if got := empty.Settings.Int("work_minutes", 25); got != 25 {
		t.Fatalf("accessor on a nil table = %d, want the default 25", got)
	}
	s := timerSettings{WorkMinutes: 25}
	if err := empty.Settings.Decode(&s); err != nil || s.WorkMinutes != 25 {
		t.Fatalf("Decode of a nil table: err=%v settings=%+v", err, s)
	}
}

// The named type must stay wire-identical: a plain JSON object, omitted when
// empty, and readable by a panel built against the map.
func TestSettingsRoundTripsOnTheWire(t *testing.T) {
	b, err := json.Marshal(Config{Label: "Breaktimer", Settings: Settings{"work_minutes": 50}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(b), `{"label":"Breaktimer","settings":{"work_minutes":50}}`; got != want {
		t.Fatalf("wire form = %s, want %s", got, want)
	}
	if b, err = json.Marshal(Config{Label: "Breaktimer"}); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(b), `{"label":"Breaktimer"}`; got != want {
		t.Fatalf("empty settings = %s, want %s", got, want)
	}

	// And it is still a map[string]any everywhere it needs to be: the host
	// assigns core/config's table straight into the field.
	var fromConfig map[string]any = map[string]any{"work_minutes": 50.0}
	cfg := Config{Settings: fromConfig}
	var back map[string]any = cfg.Settings
	if back["work_minutes"] != 50.0 {
		t.Fatalf("round trip through map[string]any lost the value: %v", back)
	}
}
