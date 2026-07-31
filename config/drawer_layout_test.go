package config

import (
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestDrawerNodeMinimumsRoundTripThroughTOML(t *testing.T) {
	input := `
[tui.drawer.pages.review.layout]
split = "vertical"
ratio = 0.5
first = { pane = "changes", min_width = 48 }
second = { pane = "notes", min_width = 30, min_height = 8 }
`
	var cfg Config
	if err := toml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal drawer layout: %v", err)
	}
	layout := cfg.TUI.Drawer.Pages["review"].Layout
	if got := layout.First.MinWidth; got != 48 {
		t.Fatalf("first min_width = %d, want 48", got)
	}
	if got, want := layout.Second.MinWidth, 30; got != want {
		t.Fatalf("second min_width = %d, want %d", got, want)
	}
	if got, want := layout.Second.MinHeight, 8; got != want {
		t.Fatalf("second min_height = %d, want %d", got, want)
	}
	// Unset stays unset, so a node that states nothing inherits the host's
	// default rather than pinning itself to zero.
	if layout.MinWidth != 0 || layout.MinHeight != 0 {
		t.Fatalf("split node invented minimums: %d/%d", layout.MinWidth, layout.MinHeight)
	}

	// The clone every merge goes through must carry them, or a layered config
	// would silently lose the minimums the deepest layer set.
	cloned := cloneDrawerNode(layout)
	if cloned.First.MinWidth != 48 || cloned.Second.MinHeight != 8 {
		t.Fatalf("clone dropped minimums: %#v / %#v", cloned.First, cloned.Second)
	}
}

func TestEffectiveRatioClampsToTheAcceptedRange(t *testing.T) {
	for _, tc := range []struct {
		name  string
		node  *DrawerNodeConfig
		want  float64
		about string
	}{
		{"unset", &DrawerNodeConfig{}, .5, "an unwritten ratio is an even split"},
		{"in range", &DrawerNodeConfig{Ratio: .3}, .3, "a stated ratio is honored"},
		{"under", &DrawerNodeConfig{Ratio: .01}, .1, "a child never gets less than a tenth"},
		{"over", &DrawerNodeConfig{Ratio: 12}, .9, "nor more than nine tenths"},
		{"nil", nil, .5, "a missing node still answers"},
	} {
		if got := tc.node.EffectiveRatio(); got != tc.want {
			t.Fatalf("%s: EffectiveRatio() = %v, want %v (%s)", tc.name, got, tc.want, tc.about)
		}
	}
}
