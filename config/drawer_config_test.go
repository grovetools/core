package config

import (
	"encoding/json"
	"reflect"
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
