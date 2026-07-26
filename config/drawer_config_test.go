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
		if got != overridePage {
			t.Fatalf("page replacement did not retain the override definition")
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

func TestDrawerConfigRecursiveJSONSchemaSmoke(t *testing.T) {
	schema, err := GenerateSchema()
	if err != nil {
		t.Fatalf("generate schema with recursive drawer nodes: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("generated schema is invalid JSON: %v", err)
	}
	text := string(schema)
	for _, want := range []string{`"drawer"`, `"DrawerNodeConfig"`, `"first"`, `"second"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated schema missing recursive drawer marker %s", want)
		}
	}
}
