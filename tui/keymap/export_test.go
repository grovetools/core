package keymap

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

// exportFixture has two bindings with the SAME help description but different
// keys, to prove export identity comes from field names, not descriptions.
type exportFixture struct {
	Base
	ViewGit  key.Binding
	ViewDiff key.Binding
	CopyLine key.Binding
}

func newExportFixture() exportFixture {
	return exportFixture{
		Base: DefaultVim(),
		ViewGit: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "view"), // duplicate description...
		),
		ViewDiff: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "view"), // ...same description, different field
		),
		CopyLine: key.NewBinding(
			key.WithKeys("Y"),
			key.WithHelp("Y", "copy current line"),
		),
	}
}

func (f exportFixture) Sections() []Section {
	return []Section{
		f.NavigationSection(),
		NewSection("Custom", f.ViewGit, f.ViewDiff, f.CopyLine),
		f.SystemSection(),
	}
}

func findBinding(t *testing.T, info TUIInfo, section, keys0 string) BindingInfo {
	t.Helper()
	for _, s := range info.Sections {
		if s.Name != section {
			continue
		}
		for _, b := range s.Bindings {
			if len(b.Keys) > 0 && b.Keys[0] == keys0 {
				return b
			}
		}
	}
	t.Fatalf("binding with key %q not found in section %q", keys0, section)
	return BindingInfo{}
}

func TestMakeTUIInfo_DuplicateDescriptionsGetDistinctIdentity(t *testing.T) {
	info := MakeTUIInfo("test-tui", "test", "test fixture", newExportFixture())

	git := findBinding(t, info, "Custom", "g")
	diff := findBinding(t, info, "Custom", "D")

	if git.Name == diff.Name {
		t.Errorf("bindings with duplicate descriptions must get distinct Names, both got %q", git.Name)
	}
	if git.ConfigKey == diff.ConfigKey {
		t.Errorf("bindings with duplicate descriptions must get distinct ConfigKeys, both got %q", git.ConfigKey)
	}
	if git.Name != "ViewGit" {
		t.Errorf("expected Name=ViewGit, got %q", git.Name)
	}
	if diff.Name != "ViewDiff" {
		t.Errorf("expected Name=ViewDiff, got %q", diff.Name)
	}
	// Descriptions are preserved as Help().Desc.
	if git.Description != "view" || diff.Description != "view" {
		t.Errorf("descriptions must remain Help().Desc, got %q / %q", git.Description, diff.Description)
	}
}

func TestMakeTUIInfo_ConfigKeyIsSnakeCaseFieldName(t *testing.T) {
	info := MakeTUIInfo("test-tui", "test", "test fixture", newExportFixture())

	tests := []struct {
		section   string
		key0      string
		name      string
		configKey string
	}{
		{"Custom", "g", "ViewGit", "view_git"},
		{"Custom", "D", "ViewDiff", "view_diff"},
		{"Custom", "Y", "CopyLine", "copy_line"},
		{"Navigation", "ctrl+u", "PageUp", "page_up"}, // embedded Base field
		{"Navigation", "home", "Home", "home"},        // Task-1 addition present in export
		{"Navigation", "gg", "Top", "top"},
	}
	for _, tt := range tests {
		b := findBinding(t, info, tt.section, tt.key0)
		if b.Name != tt.name {
			t.Errorf("key %q: expected Name=%q, got %q", tt.key0, tt.name, b.Name)
		}
		if b.ConfigKey != tt.configKey {
			t.Errorf("key %q: expected ConfigKey=%q, got %q", tt.key0, tt.configKey, b.ConfigKey)
		}
	}
}

// TestMakeTUIInfo_ConfigKeyRoundTripsWithOverrides asserts the exported
// ConfigKey uses the same conversion as the overrides system, so a user can
// paste a registry ConfigKey into grove.toml and ApplyOverrides will honor it.
func TestMakeTUIInfo_ConfigKeyRoundTripsWithOverrides(t *testing.T) {
	info := MakeTUIInfo("test-tui", "test", "test fixture", newExportFixture())
	b := findBinding(t, info, "Custom", "g")

	if got := camelToSnake("ViewGit"); b.ConfigKey != got {
		t.Errorf("ConfigKey %q does not match overrides camelToSnake %q", b.ConfigKey, got)
	}
}

// TestMakeTUIInfo_FallbackForInlineBindings covers bindings not backed by a
// struct field: they keep the legacy description-derived identity.
func TestMakeTUIInfo_FallbackForInlineBindings(t *testing.T) {
	info := MakeTUIInfo("test-tui", "test", "test fixture", inlineFixture{Base: DefaultVim()})

	b := findBinding(t, info, "Inline", "x")
	if b.Name != "inline action" {
		t.Errorf("expected fallback Name=%q, got %q", "inline action", b.Name)
	}
	if b.ConfigKey != "inline_action" {
		t.Errorf("expected fallback ConfigKey=%q, got %q", "inline_action", b.ConfigKey)
	}
}

type inlineFixture struct {
	Base
}

func (f inlineFixture) Sections() []Section {
	return []Section{
		NewSection("Inline", key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "inline action"),
		)),
	}
}
