package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// The fixture is the live machine's shape: a ~/.config/grove/tui.toml carrying
// a couple of hand-written raw-PTY panels, a plugin_order, and enough unrelated
// content around them that "the writer touched only what it said it would" is
// a claim with something to be false about.
const pluginTUIFixture = `# my treemux config
# hand-edited; do not let a tool eat this

[tui]
plugin_order = ["btop", "nfv"]
drawer_expanded = true

[tui.plugins.btop]
command = "btop"
icon = "T"
label = "btop"

[tui.plugins.nfv]
command = "nfv"
args = ["--watch"]
icon = "N"

[tui.panels.bindings]
alt-n = { command = "lazygit" }

[daemon]
git_interval = "1h"
`

func writePluginFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tui.toml")
	if err := os.WriteFile(path, []byte(pluginTUIFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readPluginFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The round trip: a UI edits one field of one entry, and everything else in the
// user's file — the leading comments, the other entry, the sibling tables, the
// keys of [tui] the writer never mentions — comes back byte-identical.
func TestWritePluginEntryPreservesEverythingElse(t *testing.T) {
	path := writePluginFixture(t)

	changed, err := WritePluginEntry(path, "btop", &PluginConfig{
		Command: "btop",
		Icon:    "T",
		Label:   "System monitor",
		Args:    []string{"--utf-force"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("WritePluginEntry reported no change after retitling the entry")
	}

	got := readPluginFixture(t, path)
	for _, want := range []string{
		"# my treemux config",
		"# hand-edited; do not let a tool eat this",
		`plugin_order = ["btop", "nfv"]`,
		"drawer_expanded = true",
		"[tui.plugins.nfv]",
		`args = ["--watch"]`,
		"[tui.panels.bindings]",
		`alt-n = { command = "lazygit" }`,
		"[daemon]",
		`git_interval = "1h"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the writer ate %q:\n%s", want, got)
		}
	}

	var cfg Config
	if err := toml.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("the file no longer parses: %v\n%s", err, got)
	}
	btop := cfg.TUI.Plugins["btop"]
	if btop.Label != "System monitor" {
		t.Errorf("label = %q, want the edited one", btop.Label)
	}
	if len(btop.Args) != 1 || btop.Args[0] != "--utf-force" {
		t.Errorf("args = %v, want the edited one", btop.Args)
	}
	// The unedited entry has to survive intact, not just textually: whole-entry
	// replacement means a writer that clipped one key off nfv would silently
	// change what nfv runs.
	nfv := cfg.TUI.Plugins["nfv"]
	if nfv.Command != "nfv" || len(nfv.Args) != 1 || nfv.Args[0] != "--watch" || nfv.Icon != "N" {
		t.Errorf("the neighbouring entry moved: %+v", nfv)
	}
}

// An empty field is omitted rather than written as `key = ""`, so an entry
// edited down to nothing looks like one that was never given the key.
func TestWritePluginEntryOmitsEmptyFields(t *testing.T) {
	path := writePluginFixture(t)
	if _, err := WritePluginEntry(path, "btop", &PluginConfig{Command: "btop"}); err != nil {
		t.Fatal(err)
	}
	got := readPluginFixture(t, path)
	if strings.Contains(got, `label = ""`) || strings.Contains(got, `icon = ""`) {
		t.Errorf("empty fields were written out:\n%s", got)
	}
	var cfg Config
	if err := toml.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.TUI.Plugins["btop"].Command != "btop" {
		t.Error("the command did not survive")
	}
}

// A file that declares the entry some other way — a dotted key, an inline
// table — is refused rather than appended to, because appending would produce a
// duplicate-key parse error at the next load and the user would have no way to
// connect it to the edit.
func TestWritePluginEntryRefusesANonTableDeclaration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.toml")
	if err := os.WriteFile(path, []byte("[tui]\nplugins.btop = { command = \"btop\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePluginEntry(path, "btop", &PluginConfig{Command: "btop"}); err == nil {
		t.Fatal("expected a refusal for an entry that is not declared as a table")
	} else if !strings.Contains(err.Error(), "edit the file by hand") {
		t.Errorf("the refusal must say what to do instead, got: %v", err)
	}
}

// Writing an unchanged entry is a no-op, so a UI that re-commits a field
// nobody edited does not churn the file's mtime.
func TestWritePluginEntryIsIdempotent(t *testing.T) {
	path := writePluginFixture(t)
	entry := &PluginConfig{Command: "btop", Icon: "T", Label: "btop"}
	if _, err := WritePluginEntry(path, "btop", entry); err != nil {
		t.Fatal(err)
	}
	first := readPluginFixture(t, path)
	changed, err := WritePluginEntry(path, "btop", entry)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("the second identical write reported a change")
	}
	if readPluginFixture(t, path) != first {
		t.Error("the second identical write moved bytes")
	}
}

func TestWritePluginOrderRewritesTheExistingKey(t *testing.T) {
	path := writePluginFixture(t)
	changed, err := WritePluginOrder(path, []string{"nfv", "btop"})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("WritePluginOrder reported no change")
	}
	got := readPluginFixture(t, path)
	if strings.Count(got, "plugin_order") != 1 {
		t.Errorf("plugin_order was duplicated rather than rewritten:\n%s", got)
	}
	if !strings.Contains(got, "drawer_expanded = true") {
		t.Errorf("the sibling key under [tui] was eaten:\n%s", got)
	}
	var cfg Config
	if err := toml.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.TUI.PluginOrder) != 2 || cfg.TUI.PluginOrder[0] != "nfv" {
		t.Errorf("plugin_order = %v, want the new order", cfg.TUI.PluginOrder)
	}
}

// A config that has never named an order gets the key inserted under its
// existing [tui] table rather than a second [tui] appended to the file.
func TestWritePluginOrderInsertsUnderAnExistingTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.toml")
	if err := os.WriteFile(path, []byte("[tui]\ndrawer_expanded = true\n\n[daemon]\ngit_interval = \"1h\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePluginOrder(path, []string{"btop"}); err != nil {
		t.Fatal(err)
	}
	got := readPluginFixture(t, path)
	if strings.Count(got, "[tui]") != 1 {
		t.Errorf("a second [tui] table was appended:\n%s", got)
	}
	var cfg Config
	if err := toml.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("the file no longer parses: %v\n%s", err, got)
	}
	if len(cfg.TUI.PluginOrder) != 1 || cfg.TUI.PluginOrder[0] != "btop" {
		t.Errorf("plugin_order = %v", cfg.TUI.PluginOrder)
	}
}

// Adopt: the hand entry is commented out, not deleted, and the commented text
// is handed back so the panel can show what it retired.
func TestCommentOutPluginEntry(t *testing.T) {
	path := writePluginFixture(t)
	removed, err := CommentOutPluginEntry(path, "btop")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(removed, `command = "btop"`) {
		t.Errorf("the returned diff does not show what was retired: %q", removed)
	}

	got := readPluginFixture(t, path)
	if !strings.Contains(got, `# command = "btop"`) {
		t.Errorf("the entry was not commented out:\n%s", got)
	}
	if !strings.Contains(got, "retired by the Plugins panel") {
		t.Errorf("nothing says why the block is commented:\n%s", got)
	}
	var cfg Config
	if err := toml.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("the file no longer parses: %v\n%s", err, got)
	}
	if _, still := cfg.TUI.Plugins["btop"]; still {
		t.Error("btop still declares an entry after being commented out")
	}
	// The neighbour is untouched — the whole risk of a line-range edit.
	if cfg.TUI.Plugins["nfv"] == nil || cfg.TUI.Plugins["nfv"].Command != "nfv" {
		t.Error("the neighbouring entry did not survive")
	}
}
