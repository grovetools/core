package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestDrawerConfigTOMLRoundTripNestedLayout(t *testing.T) {
	input := `
[tui.drawer]
cycle_key = "D"
default_page = "review"
page_order = ["sessions", "review"]

[tui.drawer.pages.review]
key = "V"
icon = "note"

[tui.drawer.pages.review.layout]
split = "horizontal"
ratio = 0.6
first = { pane = "queue" }
second = { split = "vertical", ratio = 0.4, first = { pane = "notes" }, second = { pane = "agents" } }
`

	var cfg Config
	if err := toml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal drawer config: %v", err)
	}
	if cfg.TUI == nil || cfg.TUI.Drawer == nil {
		t.Fatal("expected tui.drawer to be populated")
	}

	encoded, err := toml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal drawer config: %v", err)
	}
	var roundTripped Config
	if err := toml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped drawer config: %v", err)
	}
	if !reflect.DeepEqual(cfg.TUI.Drawer, roundTripped.TUI.Drawer) {
		t.Fatalf("drawer changed across TOML round trip:\nwant: %#v\n got: %#v", cfg.TUI.Drawer, roundTripped.TUI.Drawer)
	}

	review := roundTripped.TUI.Drawer.Pages["review"]
	if review == nil || review.Layout == nil || review.Layout.Second == nil || review.Layout.Second.Second == nil {
		t.Fatal("nested drawer layout was not preserved")
	}
	if got := review.Layout.Second.Second.Pane; got != "agents" {
		t.Fatalf("nested pane = %q, want agents", got)
	}
}

func TestDrawerConfigMerge(t *testing.T) {
	layout := func(pane string) *DrawerNodeConfig { return &DrawerNodeConfig{Pane: pane} }

	t.Run("overlay one page keeps others", func(t *testing.T) {
		base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{Pages: map[string]*DrawerPageConfig{
			"sessions": {Key: "S", Layout: layout("queue")},
			"review":   {Key: "R", Layout: layout("notes")},
		}}}}
		override := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{Pages: map[string]*DrawerPageConfig{
			"review": {Icon: "note", Layout: layout("agents")},
		}}}}

		got := mergeConfigs(base, override).TUI.Drawer
		if len(got.Pages) != 2 || got.Pages["sessions"] == nil {
			t.Fatalf("pages = %#v, want sessions preserved and review replaced", got.Pages)
		}
		if got.Pages["review"].Layout.Pane != "agents" {
			t.Fatalf("review page was not replaced: %#v", got.Pages["review"])
		}
	})

	t.Run("whole page replacement drops earlier fields", func(t *testing.T) {
		base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{Pages: map[string]*DrawerPageConfig{
			"sessions": {Key: "S", Icon: "dashboard", Layout: layout("queue")},
		}}}}
		overridePage := &DrawerPageConfig{Icon: "note"}
		override := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{Pages: map[string]*DrawerPageConfig{
			"sessions": overridePage,
		}}}}

		got := mergeConfigs(base, override).TUI.Drawer.Pages["sessions"]
		if got == overridePage {
			t.Fatal("merged page aliases override definition")
		}
		if got.Key != "" || got.Layout != nil {
			t.Fatalf("whole-page replacement retained base fields: %#v", got)
		}
	})

	t.Run("delete propagates", func(t *testing.T) {
		base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{Pages: map[string]*DrawerPageConfig{
			"sessions": {Key: "S"},
		}}}}
		override := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{Pages: map[string]*DrawerPageConfig{
			"sessions": {Delete: true},
		}}}}

		got := mergeConfigs(base, override).TUI.Drawer.Pages["sessions"]
		if got == nil || !got.Delete {
			t.Fatalf("delete marker did not propagate: %#v", got)
		}
	})

	t.Run("scalar and order replacement rules", func(t *testing.T) {
		base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
			CycleKey: "D", DefaultPage: "sessions", PageOrder: []string{"sessions", "review"},
		}}}
		empty := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{}}}
		got := mergeConfigs(base, empty).TUI.Drawer
		if got.CycleKey != "D" || got.DefaultPage != "sessions" || !reflect.DeepEqual(got.PageOrder, []string{"sessions", "review"}) {
			t.Fatalf("empty override replaced drawer values: %#v", got)
		}

		override := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
			CycleKey: "C", DefaultPage: "review", PageOrder: []string{},
		}}}
		got = mergeConfigs(base, override).TUI.Drawer
		if got.CycleKey != "C" || got.DefaultPage != "review" {
			t.Fatalf("last non-empty scalar did not win: %#v", got)
		}
		if got.PageOrder == nil || len(got.PageOrder) != 0 {
			t.Fatalf("non-nil empty page_order did not replace base: %#v", got.PageOrder)
		}
	})

	t.Run("merged drawer owns retained and replacement data", func(t *testing.T) {
		base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
			PageOrder: []string{"base"},
			Pages: map[string]*DrawerPageConfig{
				"base": {Key: "B", Layout: &DrawerNodeConfig{Split: "horizontal", First: layout("left"), Second: layout("right")}},
			},
		}}}
		override := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
			PageOrder: []string{"override"},
			Pages: map[string]*DrawerPageConfig{
				"override": {Key: "O", Layout: &DrawerNodeConfig{Split: "vertical", First: layout("top"), Second: layout("bottom")}},
			},
		}}}

		got := mergeConfigs(base, override).TUI.Drawer
		got.PageOrder[0] = "merged"
		got.Pages["base"].Layout.First.Pane = "merged-left"
		got.Pages["override"].Layout.Second.Pane = "merged-bottom"
		if override.TUI.Drawer.PageOrder[0] != "override" || base.TUI.Drawer.Pages["base"].Layout.First.Pane != "left" || override.TUI.Drawer.Pages["override"].Layout.Second.Pane != "bottom" {
			t.Fatal("mutating merged drawer changed an input")
		}

		base.TUI.Drawer.Pages["base"].Layout.Second.Pane = "input-right"
		override.TUI.Drawer.Pages["override"].Layout.First.Pane = "input-top"
		if got.Pages["base"].Layout.Second.Pane != "right" || got.Pages["override"].Layout.First.Pane != "top" {
			t.Fatal("mutating an input changed the merged drawer")
		}

		retainedOrder := mergeConfigs(base, &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{}}}).TUI.Drawer.PageOrder
		retainedOrder[0] = "retained-merged"
		if base.TUI.Drawer.PageOrder[0] != "base" {
			t.Fatal("retained base page order aliases merged result")
		}
	})

	t.Run("later page replaces delete and nil pages survive", func(t *testing.T) {
		base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{Pages: map[string]*DrawerPageConfig{
			"restored": {Delete: true}, "invalid": nil,
		}}}}
		override := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{Pages: map[string]*DrawerPageConfig{
			"restored": {Key: "R"},
		}}}}
		got := mergeConfigs(base, override).TUI.Drawer.Pages
		if got["restored"] == nil || got["restored"].Delete || got["restored"].Key != "R" {
			t.Fatalf("later page did not replace delete marker: %#v", got["restored"])
		}
		if page, ok := got["invalid"]; !ok || page != nil {
			t.Fatalf("nil page was not retained: %#v", got)
		}
	})
}

// Pane settings live beside the pages and merge field-wise, so a layer naming
// only the files view neither restates a page nor aliases the layer it came from.
func TestDrawerFilesViewMergesFieldWise(t *testing.T) {
	var cfg Config
	input := "[tui.drawer.files]\nview = \"tree\"\n"
	if err := toml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal files view: %v", err)
	}
	if cfg.TUI == nil || cfg.TUI.Drawer == nil || cfg.TUI.Drawer.Files == nil {
		t.Fatalf("files view did not parse: %#v", cfg.TUI)
	}
	if got := cfg.TUI.Drawer.Files.View; got != DrawerFilesViewTree {
		t.Fatalf("view = %q, want %q", got, DrawerFilesViewTree)
	}

	base := &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
		CycleKey: "D", Files: &DrawerFilesConfig{View: DrawerFilesViewTree},
	}}}
	// An override that says nothing about the pane keeps the base's view...
	got := mergeConfigs(base, &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{DefaultPage: "git"}}}).TUI.Drawer
	if got.Files == nil || got.Files.View != DrawerFilesViewTree {
		t.Fatalf("empty override dropped the files view: %#v", got.Files)
	}
	// ...and the merged result owns its copy.
	got.Files.View = DrawerFilesViewFlat
	if base.TUI.Drawer.Files.View != DrawerFilesViewTree {
		t.Fatal("merged files config aliases the base layer")
	}

	got = mergeConfigs(base, &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
		Files: &DrawerFilesConfig{View: DrawerFilesViewFlat},
	}}}).TUI.Drawer
	if got.Files.View != DrawerFilesViewFlat {
		t.Fatalf("last non-empty view did not win: %#v", got.Files)
	}
}

func TestDrawerConfigAbsentRemainsNil(t *testing.T) {
	var cfg Config
	if err := toml.Unmarshal([]byte("[tui]\ntheme = \"dark\"\n"), &cfg); err != nil {
		t.Fatalf("unmarshal config without drawer: %v", err)
	}
	if cfg.TUI == nil {
		t.Fatal("expected tui config")
	}
	if cfg.TUI.Drawer != nil {
		t.Fatalf("drawer = %#v, want nil", cfg.TUI.Drawer)
	}
}

func TestDrawerConfigAbsentRemainsNilThroughMerge(t *testing.T) {
	got := mergeConfigs(&Config{}, &Config{})
	if got.TUI != nil {
		t.Fatalf("untouched merged TUI = %#v, want nil", got.TUI)
	}
}

func TestDrawerConfigRecursiveJSONSchema(t *testing.T) {
	schema, err := GenerateSchema()
	if err != nil {
		t.Fatalf("generate schema with recursive drawer nodes: %v", err)
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
	tuiProps := mapAt(mapAt(mapAt(defs, "TUIConfig"), "properties"), "drawer")
	if tuiProps["$ref"] != "#/$defs/DrawerViewsConfig" {
		t.Fatalf("drawer ref = %#v", tuiProps["$ref"])
	}
	drawerProps := mapAt(mapAt(defs, "DrawerViewsConfig"), "properties")
	pages := mapAt(drawerProps, "pages")
	if pages["type"] != "object" || mapAt(pages, "additionalProperties")["$ref"] != "#/$defs/DrawerPageConfig" {
		t.Fatalf("pages does not accept arbitrary DrawerPageConfig values: %#v", pages)
	}
	pageProps := mapAt(mapAt(defs, "DrawerPageConfig"), "properties")
	if mapAt(pageProps, "layout")["$ref"] != "#/$defs/DrawerNodeConfig" {
		t.Fatalf("layout ref = %#v", pageProps["layout"])
	}
	nodeProps := mapAt(mapAt(defs, "DrawerNodeConfig"), "properties")
	for _, child := range []string{"first", "second"} {
		if mapAt(nodeProps, child)["$ref"] != "#/$defs/DrawerNodeConfig" {
			t.Fatalf("%s is not recursive: %#v", child, nodeProps[child])
		}
	}
	if got, want := mapAt(nodeProps, "split")["enum"], []interface{}{"auto", "horizontal", "vertical"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("split enum = %#v, want %#v", got, want)
	}
}

func TestDrawerPageScopeRoundTripsAndValidates(t *testing.T) {
	input := `
[tui.drawer.pages.review]
scope = "worktree"
layout = { pane = "changes" }

[tui.drawer.pages.watch]
layout = { pane = "toc" }
`
	var cfg Config
	if err := toml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal drawer scope: %v", err)
	}
	encoded, err := toml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal drawer scope: %v", err)
	}
	var roundTripped Config
	if err := toml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped drawer scope: %v", err)
	}
	if got := roundTripped.TUI.Drawer.Pages["review"].Scope; got != DrawerScopeWorktree {
		t.Errorf("scope after round trip = %q, want %q", got, DrawerScopeWorktree)
	}
	// Unset stays unset on the wire — omitempty keeps a page that declared no
	// scope from acquiring an explicit "mixed" it never asked for.
	if got := roundTripped.TUI.Drawer.Pages["watch"].Scope; got != "" {
		t.Errorf("unset scope round-tripped as %q, want empty", got)
	}
	if got := roundTripped.TUI.Drawer.Pages["watch"].Scope.Resolved(); got != DrawerScopeMixed {
		t.Errorf("unset scope resolved to %q, want %q", got, DrawerScopeMixed)
	}

	// JSON is the shape the schema validator sees.
	encodedJSON, err := json.Marshal(cfg.TUI.Drawer.Pages["review"])
	if err != nil {
		t.Fatalf("marshal page as JSON: %v", err)
	}
	if !strings.Contains(string(encodedJSON), `"scope":"worktree"`) {
		t.Errorf("scope missing from JSON encoding: %s", encodedJSON)
	}
}

func TestDrawerPageScopeValidateNamesTheAcceptedSet(t *testing.T) {
	for _, ok := range append([]DrawerPageScope{""}, drawerPageScopes...) {
		if err := ok.Validate(); err != nil {
			t.Errorf("scope %q rejected: %v", ok, err)
		}
	}
	err := DrawerPageScope("wortree").Validate()
	if err == nil {
		t.Fatal("misspelled scope accepted")
	}
	// The message has to carry both what was wrong and what is allowed: a lint
	// line that only says "invalid" sends the user to the source to find out
	// what the six words are.
	msg := err.Error()
	if !strings.Contains(msg, `"wortree"`) {
		t.Errorf("error does not quote the bad value: %q", msg)
	}
	for _, want := range drawerPageScopes {
		if !strings.Contains(msg, string(want)) {
			t.Errorf("error does not list %q: %q", want, msg)
		}
	}
	// A bad value costs the grouping, never the page.
	if got := DrawerPageScope("wortree").Resolved(); got != DrawerScopeMixed {
		t.Errorf("invalid scope resolved to %q, want %q", got, DrawerScopeMixed)
	}
}

// TestDrawerPageScopeIsSchemaEnforced pins the lint half of the contract: the
// embedded schema the config load path validates against carries the enum, so
// a misspelled scope is reported rather than silently ignored.
func TestDrawerPageScopeIsSchemaEnforced(t *testing.T) {
	validator, err := NewSchemaValidator()
	if err != nil {
		t.Skipf("schema validator unavailable: %v", err)
	}
	page := func(scope string) *Config {
		return &Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
			Pages: map[string]*DrawerPageConfig{
				"review": {Scope: DrawerPageScope(scope), Layout: &DrawerNodeConfig{Pane: "changes"}},
			},
		}}}
	}
	for _, scope := range drawerPageScopes {
		if err := validator.Validate(page(string(scope))); err != nil {
			t.Errorf("schema rejected valid scope %q: %v", scope, err)
		}
	}
	if err := validator.Validate(page("wortree")); err == nil {
		t.Error("schema accepted a misspelled scope")
	} else if !strings.Contains(err.Error(), "scope") {
		t.Errorf("schema violation does not point at scope: %v", err)
	}
}

// The composition grammar rides the existing `pane` key, so it needs nothing
// from the encoder — which is the point of spelling it as a prefix. This pins
// that: a composed page survives a TOML round trip unchanged, and the layered
// clone every merge goes through carries the references too.
func TestDrawerConfigTOMLRoundTripComposedPage(t *testing.T) {
	input := `
[tui.drawer.pages.wide]
key = "W"
size = "40%"

[tui.drawer.pages.wide.layout]
split = "vertical"
first = { pane = "page:sessions" }
second = { pane = "page:git", min_width = 40 }
`

	var cfg Config
	if err := toml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal composed drawer config: %v", err)
	}
	encoded, err := toml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal composed drawer config: %v", err)
	}
	var roundTripped Config
	if err := toml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped composed drawer config: %v", err)
	}
	if !reflect.DeepEqual(cfg.TUI.Drawer, roundTripped.TUI.Drawer) {
		t.Fatalf("composed drawer changed across TOML round trip:\nwant: %#v\n got: %#v", cfg.TUI.Drawer, roundTripped.TUI.Drawer)
	}

	wide := roundTripped.TUI.Drawer.Pages["wide"]
	if wide == nil || wide.Layout == nil {
		t.Fatal("composed page was not preserved")
	}
	first, ok := DrawerPageRef(wide.Layout.First.Pane)
	if !ok || first != "sessions" {
		t.Fatalf("first child = %q, want a reference to sessions", wide.Layout.First.Pane)
	}
	second, ok := DrawerPageRef(wide.Layout.Second.Pane)
	if !ok || second != "git" || wide.Layout.Second.MinWidth != 40 {
		t.Fatalf("second child = %#v, want a reference to git minding 40 columns", wide.Layout.Second)
	}
	if cloned := cloneDrawerViews(roundTripped.TUI.Drawer); cloned.Pages["wide"].Layout.First.Pane != "page:sessions" {
		t.Fatalf("clone dropped the reference: %#v", cloned.Pages["wide"].Layout)
	}
}

// TestDrawerViewAccessorsTreatAbsentConfigAndAbsentKeyAlike pins the one rule
// the three [tui.drawer] booleans share: hosts ask the accessor, never the
// field, so "no config at all", "no key" and "the key set to its default" are
// one answer written in one place.
//
// The DEFAULTS are what differ, and each is a decision:
//   - responsive is ON. It has baked; a pane handing its unused rows to a
//     sibling is what a reader wants often enough not to have to ask for.
//   - hide_inapplicable_pages is OFF. The page map's premise is a fixed shape
//     you learn by looking at it, and a page that comes and goes teaches less
//     than one that is always there greyed out.
//   - page_map_long_form is OFF. The compact glyph strip is what fits a narrow
//     drawer, which is the drawer most people have.
func TestDrawerViewAccessorsTreatAbsentConfigAndAbsentKeyAlike(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name                       string
		cfg                        *DrawerViewsConfig
		responsive, hide, longForm bool
	}{
		{"nil config", nil, true, false, false},
		{"no keys set", &DrawerViewsConfig{}, true, false, false},
		{"every key off", &DrawerViewsConfig{Responsive: &no, HideInapplicablePages: &no, PageMapLongForm: &no}, false, false, false},
		{"every key on", &DrawerViewsConfig{Responsive: &yes, HideInapplicablePages: &yes, PageMapLongForm: &yes}, true, true, true},
		// Each key answers only for itself: they share a table, not a meaning.
		{"hide alone", &DrawerViewsConfig{HideInapplicablePages: &yes}, true, true, false},
		{"long form alone", &DrawerViewsConfig{PageMapLongForm: &yes}, true, false, true},
		{"responsive off alone", &DrawerViewsConfig{Responsive: &no}, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.ResponsiveDrawer(); got != tc.responsive {
				t.Errorf("ResponsiveDrawer() = %v, want %v", got, tc.responsive)
			}
			if got := tc.cfg.HideInapplicableDrawerPages(); got != tc.hide {
				t.Errorf("HideInapplicableDrawerPages() = %v, want %v", got, tc.hide)
			}
			if got := tc.cfg.LongFormPageMap(); got != tc.longForm {
				t.Errorf("LongFormPageMap() = %v, want %v", got, tc.longForm)
			}
		})
	}
}

// TestDrawerViewBooleansRoundTripAsThreeStateKeys: each is a *bool so that
// "unset" stays distinguishable from "explicitly false" across a TOML round
// trip — which is the whole reason the accessors can own the default.
func TestDrawerViewBooleansRoundTripAsThreeStateKeys(t *testing.T) {
	input := `
[tui.drawer]
responsive = false
hide_inapplicable_pages = true
page_map_long_form = true
`
	var cfg Config
	if err := toml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal drawer booleans: %v", err)
	}
	d := cfg.TUI.Drawer
	if d == nil || d.Responsive == nil || *d.Responsive {
		t.Fatalf("responsive = %#v, want an explicit false", d)
	}
	if !d.HideInapplicableDrawerPages() || !d.LongFormPageMap() {
		t.Fatalf("drawer booleans did not survive the parse: %#v", d)
	}

	encoded, err := toml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal drawer booleans: %v", err)
	}
	var roundTripped Config
	if err := toml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped drawer booleans: %v", err)
	}
	if !reflect.DeepEqual(cfg.TUI.Drawer, roundTripped.TUI.Drawer) {
		t.Fatalf("drawer booleans changed across a TOML round trip:\nwant: %#v\n got: %#v", cfg.TUI.Drawer, roundTripped.TUI.Drawer)
	}

	// An unset key is omitted entirely rather than written as false, so a
	// config file never pins a default the accessor is free to change.
	empty, err := toml.Marshal(&Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{}}})
	if err != nil {
		t.Fatalf("marshal empty drawer: %v", err)
	}
	for _, key := range []string{"responsive", "hide_inapplicable_pages", "page_map_long_form"} {
		if strings.Contains(string(empty), key) {
			t.Errorf("unset key %q was written out: %s", key, empty)
		}
	}
}

// TestDrawerSettingsMergeFromAnyLayer covers the five keys that used to reach
// the host only from the GLOBAL config, because mergeConfigs had no clause for
// them and every non-global layer arrives here as an override.
//
// The failure this pins was silent and total: a user writing
// `hide_inapplicable_pages = true` in an ecosystem grove.toml got no warning, no
// error, and no effect. Their x-layer=global schema tag is a hint to the config
// editor about where a key usually belongs, not a statement about where it
// works — drawer_size carries the same tag and has always merged from anywhere.
func TestDrawerSettingsMergeFromAnyLayer(t *testing.T) {
	t.Run("orientation and expanded", func(t *testing.T) {
		base := &Config{TUI: &TUIConfig{DrawerOrientation: "right"}}
		override := &Config{TUI: &TUIConfig{DrawerOrientation: "bottom", DrawerExpanded: true}}
		got := mergeConfigs(base, override).TUI
		if got.DrawerOrientation != "bottom" {
			t.Errorf("orientation = %q, want bottom", got.DrawerOrientation)
		}
		if !got.DrawerExpanded {
			t.Error("drawer_expanded did not survive the merge")
		}
	})

	t.Run("an override that says nothing changes nothing", func(t *testing.T) {
		base := &Config{TUI: &TUIConfig{DrawerOrientation: "bottom", DrawerExpanded: true}}
		got := mergeConfigs(base, &Config{TUI: &TUIConfig{}}).TUI
		if got.DrawerOrientation != "bottom" || !got.DrawerExpanded {
			t.Errorf("silent override clobbered the base: orientation=%q expanded=%v",
				got.DrawerOrientation, got.DrawerExpanded)
		}
	})

	t.Run("three-state booleans", func(t *testing.T) {
		yes, no := true, false
		for _, tc := range []struct {
			name           string
			base, override *bool
			want           *bool
		}{
			{"override sets it", nil, &yes, &yes},
			// The case an or-style bool merge cannot express, and the reason
			// these three are pointers: a layer turning an inherited true back
			// OFF has no other way to say so.
			{"override can turn it off again", &yes, &no, &no},
			{"silence inherits", &yes, nil, &yes},
			{"silence over silence", nil, nil, nil},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got := mergeConfigs(
					&Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
						Responsive: tc.base, HideInapplicablePages: tc.base, PageMapLongForm: tc.base,
					}}},
					&Config{TUI: &TUIConfig{Drawer: &DrawerViewsConfig{
						Responsive: tc.override, HideInapplicablePages: tc.override, PageMapLongForm: tc.override,
					}}},
				).TUI.Drawer

				for name, p := range map[string]*bool{
					"responsive":              got.Responsive,
					"hide_inapplicable_pages": got.HideInapplicablePages,
					"page_map_long_form":      got.PageMapLongForm,
				} {
					switch {
					case tc.want == nil && p != nil:
						t.Errorf("%s = %v, want unset", name, *p)
					case tc.want != nil && p == nil:
						t.Errorf("%s is unset, want %v", name, *tc.want)
					case tc.want != nil && p != nil && *p != *tc.want:
						t.Errorf("%s = %v, want %v", name, *p, *tc.want)
					}
				}
			})
		}
	})
}
