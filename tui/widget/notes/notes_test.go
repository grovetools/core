package notes

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/tui/theme"
	"github.com/grovetools/tuimux"
)

func press(p *Panel, key string) tea.Cmd {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if key == "enter" {
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	}
	if key == "tab" {
		msg = tea.KeyMsg{Type: tea.KeyTab}
	}
	_, cmd := p.Update(msg)
	return cmd
}

func TestEmptyListExplainsScopeAndCollapses(t *testing.T) {
	p := New(nil, func() Scope { return ScopeUnlinked })
	p.Resize(50, 12)
	view := p.View()
	if !strings.Contains(view, theme.IconInfo) || !strings.Contains(view, emptyReason) || !strings.Contains(view, "unlinked") {
		t.Fatalf("empty notes pane = %q", view)
	}
	if rows, flexible := p.PreferredHeightHint(); flexible || rows != 3 {
		t.Fatalf("hint=(%d,%v), want (3,false)", rows, flexible)
	}
}

func TestListRendersAndOpensSelectedNote(t *testing.T) {
	rows := []NoteRow{
		{ID: "plain", Title: "Plain note", Path: "/n/plain.md", Workspace: "ws"},
		{ID: "hot", Title: "Hot note", Path: "/n/hot.md", Workspace: "ws", Priority: "p0"},
	}
	p := New(func() []NoteRow { return rows }, func() Scope { return ScopePlan })
	p.Resize(40, 10)
	view := p.View()
	for _, want := range []string{"this plan", "Hot note", "Plain note", "P0"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	// Priority sorting puts Hot first.
	msg, ok := press(p, "enter")().(OpenNoteMsg)
	if !ok || msg.Path != "/n/hot.md" {
		t.Fatalf("enter = %#v", msg)
	}
	press(p, "j")
	if got := press(p, "enter")().(OpenNoteMsg).Path; got != "/n/plain.md" {
		t.Fatalf("second enter path=%q", got)
	}
}

func TestScopeCycleMessageAndLabels(t *testing.T) {
	if ScopePlan.Next() != ScopeNotespace || ScopeNotespace.Next() != ScopeUnlinked || ScopeUnlinked.Next() != ScopeAll || ScopeAll.Next() != ScopePlan {
		t.Fatal("scope cycle order changed")
	}
	p := New(nil, func() Scope { return ScopePlan })
	p.Resize(30, 5)
	if _, ok := press(p, "w")().(CycleScopeMsg); !ok {
		t.Fatalf("w emitted wrong message")
	}
}

var _ tuimux.SizeHintProvider = (*Panel)(nil)
