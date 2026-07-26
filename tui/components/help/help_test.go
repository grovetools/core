package help

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/keymap"
)

// sectionedKeys implements keymap.SectionedKeyMap.
type sectionedKeys struct{}

func (sectionedKeys) Sections() []keymap.Section {
	disabled := key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "disabled action"),
	)
	disabled.SetEnabled(false)
	return []keymap.Section{
		keymap.NewSection("Testing",
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "alpha action")),
			disabled,
			key.NewBinding(key.WithKeys("ctrl+q"), key.WithHelp("ctrl+q", "")), // empty desc
		),
	}
}

// legacyKeys implements only the legacy FullHelp interface.
type legacyKeys struct{}

func (legacyKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "legacy action"))},
	}
}

// noKeys implements neither interface.
type noKeys struct{}

func fullHelpView(t *testing.T, keys interface{}) string {
	t.Helper()
	m := New(keys)
	m.SetSize(100, 40)
	m.Toggle() // show full help, renders viewport content
	return m.View()
}

func TestHelp_SectionsPath(t *testing.T) {
	out := fullHelpView(t, sectionedKeys{})
	if !strings.Contains(out, "alpha action") {
		t.Errorf("expected sectioned binding desc in view, got:\n%s", out)
	}
	if !strings.Contains(out, "Testing") {
		t.Errorf("expected section name in view, got:\n%s", out)
	}
}

func TestHelp_LegacyFullHelpPath(t *testing.T) {
	out := fullHelpView(t, legacyKeys{})
	if !strings.Contains(out, "legacy action") {
		t.Errorf("expected legacy binding desc in view, got:\n%s", out)
	}
}

func TestHelp_NoKeymapFallback(t *testing.T) {
	out := fullHelpView(t, noKeys{})
	if !strings.Contains(out, "(no keymap registered)") {
		t.Errorf("expected fallback message in view, got:\n%s", out)
	}
}

func TestHelp_NilKeysFallback(t *testing.T) {
	out := fullHelpView(t, nil)
	if !strings.Contains(out, "(no keymap registered)") {
		t.Errorf("expected fallback message for nil keys, got:\n%s", out)
	}
}

func TestHelp_ExcludesDisabledBindings(t *testing.T) {
	out := fullHelpView(t, sectionedKeys{})
	if strings.Contains(out, "disabled action") {
		t.Errorf("disabled binding must not render, got:\n%s", out)
	}
}

func TestHelp_ExcludesEmptyDescBindings(t *testing.T) {
	out := fullHelpView(t, sectionedKeys{})
	if strings.Contains(out, "ctrl+q") {
		t.Errorf("binding with empty help desc must not render, got:\n%s", out)
	}
}

func TestHelp_SearchHighlightsAndScrollsToBinding(t *testing.T) {
	bindings := make([]key.Binding, 0, 30)
	for i := 0; i < 29; i++ {
		bindings = append(bindings, key.NewBinding(
			key.WithKeys(fmt.Sprintf("k%d", i)),
			key.WithHelp(fmt.Sprintf("k%d", i), fmt.Sprintf("action %d", i)),
		))
	}
	bindings = append(bindings, key.NewBinding(
		key.WithKeys("cp"), key.WithHelp("cp", "set provider"),
	))

	m := New(legacySearchKeys{bindings: bindings})
	m.SetSize(50, 12)
	m.Toggle()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.search.Focused() {
		t.Fatal("slash should focus help search")
	}
	for _, r := range "provider" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if m.matchCount != 1 {
		t.Fatalf("expected one matching binding, got %d", m.matchCount)
	}
	if m.viewport.YOffset == 0 {
		t.Fatal("expected search to scroll the off-screen match into view")
	}
	if out := m.View(); !strings.Contains(out, "1 matches") || !strings.Contains(out, "set provider") {
		t.Fatalf("expected search status and matching row in view, got:\n%s", out)
	}
}

type legacySearchKeys struct{ bindings []key.Binding }

func (k legacySearchKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.bindings} }

func TestHelp_SearchEscapePreservesThenClears(t *testing.T) {
	m := New(legacyKeys{})
	m.SetSize(80, 20)
	m.Toggle()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("legacy")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.search.Focused() || m.search.Value() != "legacy" || !m.ShowAll {
		t.Fatal("first escape should blur search while preserving its query")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.search.Value() != "" || !m.ShowAll {
		t.Fatal("second escape should clear search without closing help")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.ShowAll {
		t.Fatal("escape with no active search should close help")
	}
}

func TestHelp_CustomHelpWithoutKeymap(t *testing.T) {
	// CustomHelp alone should render, not the fallback message.
	m := New(noKeys{})
	m.SetSize(100, 40)
	m.SetCustomHelp([][]key.Binding{
		{key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "custom action"))},
	})
	m.Toggle()
	out := m.View()
	if !strings.Contains(out, "custom action") {
		t.Errorf("expected custom help to render, got:\n%s", out)
	}
	if strings.Contains(out, "(no keymap registered)") {
		t.Errorf("fallback message must not render when custom help exists, got:\n%s", out)
	}
}
