package tree

import (
	"strings"
	"testing"
)

func TestPrefixes(t *testing.T) {
	tests := []struct {
		name   string
		depths []int
		want   []string
	}{
		{"no entries", nil, nil},
		{
			"a single root is the last of its group",
			[]int{0},
			[]string{"└─ "},
		},
		{
			"siblings continue until the last one",
			[]int{0, 0, 0},
			[]string{"├─ ", "├─ ", "└─ "},
		},
		{
			"a child under a continuing parent gets the vertical line",
			[]int{0, 1, 0},
			[]string{"├─ ", "│  └─ ", "└─ "},
		},
		{
			"a child under the last parent gets a blank instead of a line",
			[]int{0, 1},
			[]string{"└─ ", "   └─ "},
		},
		{
			"the line only continues past a parent with more siblings coming",
			[]int{0, 1, 1, 0},
			[]string{"├─ ", "│  ├─ ", "│  └─ ", "└─ "},
		},
		{
			"three levels track each ancestor independently",
			[]int{0, 1, 2, 2, 1, 0},
			[]string{
				"├─ ",
				"│  ├─ ",
				"│  │  ├─ ",
				"│  │  └─ ",
				"│  └─ ",
				"└─ ",
			},
		},
		{
			"a deep subtree does not leave its lines behind for the next one",
			[]int{0, 1, 2, 1, 2},
			[]string{
				"└─ ",
				"   ├─ ",
				"   │  └─ ",
				"   └─ ",
				"      └─ ",
			},
		},
		{
			"negative depths are treated as roots rather than panicking",
			[]int{-1, 0},
			[]string{"├─ ", "└─ "},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Prefixes(tt.depths)
			if len(got) != len(tt.want) {
				t.Fatalf("Prefixes() returned %d prefixes, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("prefix %d = %q, want %q\nfull tree:\n%s", i, got[i], tt.want[i], strings.Join(got, "\n"))
				}
			}
		})
	}
}

func TestPrefixesWithIndent(t *testing.T) {
	got := PrefixesWith([]int{0, 1}, "  ", DefaultGlyphs)
	want := []string{"  └─ ", "     └─ "}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("prefix %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPrefixesWithCustomGlyphs(t *testing.T) {
	ascii := Glyphs{Branch: "|- ", LastBranch: "`- ", Vertical: "|  ", Space: "   "}
	got := PrefixesWith([]int{0, 1, 0}, "", ascii)
	want := []string{"|- ", "|  `- ", "`- "}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("prefix %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// A node type for the fold tests. IDs are stable strings, which is the whole
// point of keying folds on them.
type node struct {
	id       string
	depth    int
	foldable bool
}

func (n node) ID() string     { return n.id }
func (n node) Depth() int     { return n.depth }
func (n node) Foldable() bool { return n.foldable }

func TestFoldStateZeroValueIsExpanded(t *testing.T) {
	var f FoldState
	if f.Collapsed("anything") {
		t.Error("the zero FoldState collapsed a node; it should default to expanded")
	}
}

func TestFoldStateNilIsSafe(t *testing.T) {
	var f *FoldState
	if f.Collapsed("anything") {
		t.Error("a nil FoldState collapsed a node")
	}
}

func TestFoldStateOverrideBeatsDefault(t *testing.T) {
	f := NewFoldState(func(string) bool { return true }) // collapse everything

	if !f.Collapsed("a") {
		t.Error("a node with no override should follow the default")
	}
	f.Set("a", false)
	if f.Collapsed("a") {
		t.Error("an explicit override should beat the default")
	}
}

// The reason the two are split: a node the user has not touched keeps
// following the policy as its state changes underneath, without anyone
// writing a fold entry for it.
func TestFoldStateDefaultTracksChangingPolicy(t *testing.T) {
	running := map[string]bool{"job-1": true}
	f := NewFoldState(func(id string) bool { return !running[id] })

	if f.Collapsed("job-1") {
		t.Error("a running job should default to expanded")
	}
	running["job-1"] = false
	if !f.Collapsed("job-1") {
		t.Error("the job stopped running and should now default to collapsed")
	}
}

func TestFoldStateToggleFlipsWhatTheUserSees(t *testing.T) {
	f := NewFoldState(func(string) bool { return true })

	// The node is collapsed by policy with no override. The first toggle must
	// expand it — toggling relative to a zero-value override would be a no-op
	// and the key would appear dead.
	f.Toggle("a")
	if f.Collapsed("a") {
		t.Error("the first toggle of a policy-collapsed node should expand it")
	}
	f.Toggle("a")
	if !f.Collapsed("a") {
		t.Error("toggling back should collapse it again")
	}
}

func TestFoldStateClearReturnsToPolicy(t *testing.T) {
	f := NewFoldState(func(string) bool { return true })
	f.Set("a", false)
	f.Clear("a")
	if !f.Collapsed("a") {
		t.Error("clearing an override should return the node to the default")
	}
}

func TestFoldStateResetDropsEveryOverride(t *testing.T) {
	f := NewFoldState(func(string) bool { return false })
	f.Set("a", true)
	f.Set("b", true)
	f.Reset()

	if f.Collapsed("a") || f.Collapsed("b") {
		t.Error("Reset should return every node to the default")
	}
	if f.Overrides() != nil {
		t.Errorf("Overrides() after Reset = %v, want nil", f.Overrides())
	}
}

func TestFoldStateOverridesRoundTrip(t *testing.T) {
	f := NewFoldState(nil)
	f.Set("a", true)
	f.Set("b", false)

	restored := NewFoldState(nil)
	restored.SetOverrides(f.Overrides())

	if !restored.Collapsed("a") || restored.Collapsed("b") {
		t.Error("overrides did not survive a save/restore round trip")
	}
}

func TestFoldStateOverridesIsACopy(t *testing.T) {
	f := NewFoldState(nil)
	f.Set("a", true)

	saved := f.Overrides()
	saved["a"] = false

	if !f.Collapsed("a") {
		t.Error("mutating the map from Overrides() changed the live state")
	}
}

func TestVisible(t *testing.T) {
	// root
	//   child-a        (foldable)
	//     grandchild
	//   child-b
	nodes := []node{
		{"root", 0, true},
		{"child-a", 1, true},
		{"grandchild", 2, false},
		{"child-b", 1, false},
	}

	t.Run("everything expanded shows everything", func(t *testing.T) {
		f := NewFoldState(nil)
		if got := len(Visible(nodes, f)); got != 4 {
			t.Errorf("visible = %d nodes, want 4", got)
		}
	})

	t.Run("a collapsed node stays visible but hides its descendants", func(t *testing.T) {
		f := NewFoldState(nil)
		f.Set("child-a", true)

		got := ids(Visible(nodes, f))
		want := []string{"root", "child-a", "child-b"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("visible = %v, want %v", got, want)
		}
	})

	t.Run("collapsing the root hides the whole subtree", func(t *testing.T) {
		f := NewFoldState(nil)
		f.Set("root", true)

		got := ids(Visible(nodes, f))
		if strings.Join(got, ",") != "root" {
			t.Errorf("visible = %v, want just the root", got)
		}
	})

	t.Run("a collapsed leaf is not treated as foldable", func(t *testing.T) {
		f := NewFoldState(func(string) bool { return true }) // collapse everything

		// child-b is not foldable, so it cannot hide anything; root and
		// child-a are, so only they survive.
		got := ids(Visible(nodes, f))
		if strings.Join(got, ",") != "root" {
			t.Errorf("visible = %v, want just the root", got)
		}
	})
}

func TestVisibleWithSiblingSubtrees(t *testing.T) {
	nodes := []node{
		{"a", 0, true},
		{"a1", 1, false},
		{"b", 0, true},
		{"b1", 1, false},
	}
	f := NewFoldState(nil)
	f.Set("a", true)

	// Collapsing a must not swallow b's children: the hidden run ends at the
	// next node at or above a's depth.
	got := ids(Visible(nodes, f))
	want := "a,b,b1"
	if strings.Join(got, ",") != want {
		t.Errorf("visible = %v, want %s", got, want)
	}
}

func TestDepths(t *testing.T) {
	nodes := []node{{"a", 0, false}, {"b", 2, false}}
	got := Depths(nodes)
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("Depths() = %v, want [0 2]", got)
	}
}

func ids(nodes []node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.id
	}
	return out
}
