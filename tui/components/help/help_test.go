package help

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"

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
