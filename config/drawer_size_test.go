package config

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// Both spellings have to survive the same key, which is the whole reason
// DrawerSize exists as a type rather than an int or a string.
func TestDrawerSizeAcceptsIntAndPercentFromTOML(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  DrawerSize
	}{
		{"absolute int", "[tui]\ndrawer_size = 35\n", "35"},
		{"percent string", "[tui]\ndrawer_size = \"25%\"\n", "25%"},
		{"quoted int", "[tui]\ndrawer_size = \"48\"\n", "48"},
		{"fractional percent", "[tui]\ndrawer_size = \"33.5%\"\n", "33.5%"},
		{"unset", "[tui]\ntheme = \"dark\"\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg Config
			if err := toml.Unmarshal([]byte(tc.input), &cfg); err != nil {
				t.Fatalf("unmarshal %q: %v", tc.input, err)
			}
			if cfg.TUI == nil {
				t.Fatal("expected tui config")
			}
			if got := cfg.TUI.DrawerSize; got != tc.want {
				t.Fatalf("drawer_size = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDrawerSizeRejectsNonsense(t *testing.T) {
	for _, input := range []string{
		"[tui]\ndrawer_size = \"wide\"\n",
		"[tui]\ndrawer_size = \"25% of width\"\n",
		"[tui]\ndrawer_size = 0\n",
		"[tui]\ndrawer_size = -10\n",
		"[tui]\ndrawer_size = \"0%\"\n",
	} {
		var cfg Config
		if err := toml.Unmarshal([]byte(input), &cfg); err == nil {
			t.Fatalf("%q parsed to %q, want an error", input, cfg.TUI.DrawerSize)
		}
	}
}

// A page override reads the same syntax as the shared key, and survives a TOML
// round trip alongside the rest of the page definition.
func TestDrawerPageSizeTOMLRoundTrip(t *testing.T) {
	input := `
[tui]
drawer_size = "25%"

[tui.drawer.pages.review]
key = "V"
size = 70
layout = { pane = "changes" }
`
	var cfg Config
	if err := toml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal page size: %v", err)
	}
	if got := cfg.TUI.Drawer.Pages["review"].Size; got != "70" {
		t.Fatalf("page size = %q, want 70", got)
	}

	encoded, err := toml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal page size: %v", err)
	}
	var roundTripped Config
	if err := toml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped page size: %v", err)
	}
	if got := roundTripped.TUI.DrawerSize; got != "25%" {
		t.Fatalf("drawer_size after round trip = %q, want 25%%", got)
	}
	if got := roundTripped.TUI.Drawer.Pages["review"].Size; got != "70" {
		t.Fatalf("page size after round trip = %q, want 70", got)
	}
}

func TestDrawerSizeJSONRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  DrawerSize
	}{
		{`{"drawer_size": 35}`, "35"},
		{`{"drawer_size": "25%"}`, "25%"},
		{`{"drawer_size": null}`, ""},
		{`{}`, ""},
	} {
		var tui TUIConfig
		if err := json.Unmarshal([]byte(tc.input), &tui); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.input, err)
		}
		if tui.DrawerSize != tc.want {
			t.Fatalf("%s -> %q, want %q", tc.input, tui.DrawerSize, tc.want)
		}
	}
}

func TestDrawerSizeResolve(t *testing.T) {
	for _, tc := range []struct {
		size   DrawerSize
		extent int
		want   int
		ok     bool
	}{
		{"35", 200, 35, true},
		{"35", 0, 35, true}, // absolute sizes never need the extent
		{"25%", 200, 50, true},
		{"25%", 81, 20, true}, // 20.25 rounds to 20
		{"25%", 82, 21, true}, // 20.5 rounds to 21
		{"0.5%", 100, 1, true},
		{"25%", 0, 0, false}, // no terminal yet: no share to take
		{"", 200, 0, false},
		{"garbage", 200, 0, false},
	} {
		got, ok := tc.size.Resolve(tc.extent)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("%q.Resolve(%d) = (%d, %v), want (%d, %v)", tc.size, tc.extent, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDrawerSizeIsPercent(t *testing.T) {
	for size, want := range map[DrawerSize]bool{
		"25%": true, " 25 % ": true, "33.5%": true, "35": false, "": false, "junk": false,
	} {
		if got := size.IsPercent(); got != want {
			t.Fatalf("%q.IsPercent() = %v, want %v", size, got, want)
		}
	}
}

// drawer_size is an ordinary last-non-empty-wins scalar: an override that says
// nothing about it must not reset the layer below to the built-in default.
func TestDrawerSizeMerge(t *testing.T) {
	base := &Config{TUI: &TUIConfig{DrawerSize: "25%"}}

	got := mergeConfigs(base, &Config{TUI: &TUIConfig{Theme: "dark"}})
	if got.TUI.DrawerSize != "25%" {
		t.Fatalf("empty override reset drawer_size to %q", got.TUI.DrawerSize)
	}

	got = mergeConfigs(base, &Config{TUI: &TUIConfig{DrawerSize: "48"}})
	if got.TUI.DrawerSize != "48" {
		t.Fatalf("last non-empty drawer_size did not win: %q", got.TUI.DrawerSize)
	}
	if base.TUI.DrawerSize != "25%" {
		t.Fatal("merge mutated the base layer")
	}
}

// A per-page size rides whole-page replacement like every other page field: a
// later layer that restates the page without a size drops it.
func TestDrawerPageSizeMerge(t *testing.T) {
	base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{Pages: map[string]*DrawerPageConfig{
		"review": {Key: "V", Size: "70", Layout: &DrawerNodeConfig{Pane: "changes"}},
	}}}}

	got := mergeConfigs(base, &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{DefaultPage: "review"}}})
	if got.TUI.Drawer.Pages["review"].Size != "70" {
		t.Fatalf("untouched page lost its size: %#v", got.TUI.Drawer.Pages["review"])
	}

	got = mergeConfigs(base, &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{Pages: map[string]*DrawerPageConfig{
		"review": {Icon: "note"},
	}}}})
	if got.TUI.Drawer.Pages["review"].Size != "" {
		t.Fatalf("whole-page replacement retained the base size: %#v", got.TUI.Drawer.Pages["review"])
	}
}

// The generated schema has to accept both spellings, at the shared key and at
// the page override, or a config that parses fine still warns on every load.
func TestDrawerSizeJSONSchemaUnion(t *testing.T) {
	schema, err := GenerateSchema()
	if err != nil {
		t.Fatalf("generate schema: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("generated schema is invalid JSON: %v", err)
	}
	mapAt := func(parent map[string]interface{}, key string) map[string]interface{} {
		t.Helper()
		value, ok := parent[key].(map[string]interface{})
		if !ok {
			t.Fatalf("schema key %q = %#v, want object", key, parent[key])
		}
		return value
	}
	defs := mapAt(decoded, "$defs")
	for _, where := range []struct{ def, prop string }{
		{"TUIConfig", "drawer_size"},
		{"DrawerPageConfig", "size"},
	} {
		prop := mapAt(mapAt(mapAt(defs, where.def), "properties"), where.prop)
		if prop["$ref"] != "#/$defs/DrawerSize" {
			t.Fatalf("%s.%s ref = %#v, want the DrawerSize union", where.def, where.prop, prop["$ref"])
		}
	}
	union, ok := mapAt(defs, "DrawerSize")["oneOf"].([]interface{})
	if !ok || len(union) != 2 {
		t.Fatalf("DrawerSize is not a two-branch union: %#v", mapAt(defs, "DrawerSize"))
	}
	branches := make([]string, 0, len(union))
	for _, b := range union {
		branch, _ := b.(map[string]interface{})
		branches = append(branches, branch["type"].(string))
	}
	if !reflect.DeepEqual(branches, []string{"integer", "string"}) {
		t.Fatalf("DrawerSize branches = %#v, want integer then string", branches)
	}
}

// The load path validates the DECODED struct, not the file — so both spellings
// have to survive being read into a DrawerSize and written back out, or every
// load of a perfectly valid config emits a schema warning.
func TestDrawerSizeValidatesAfterDecode(t *testing.T) {
	validator, err := getSharedValidator()
	if err != nil {
		t.Skipf("schema validator unavailable: %v", err)
	}
	for _, input := range []string{
		"[tui]\ndrawer_size = 35\n",
		"[tui]\ndrawer_size = \"25%\"\n",
		"[tui.drawer.pages.review]\nsize = 70\nlayout = { pane = \"changes\" }\n",
	} {
		var cfg Config
		if err := toml.Unmarshal([]byte(input), &cfg); err != nil {
			t.Fatalf("unmarshal %q: %v", input, err)
		}
		if err := validator.Validate(&cfg); err != nil {
			t.Fatalf("decoded %q failed schema validation: %v", input, err)
		}
	}
}
