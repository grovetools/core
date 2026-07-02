package daemonstream

import (
	"testing"

	"github.com/grovetools/core/pkg/daemon"
)

func themeUpdate(name string) daemon.StateUpdate {
	return daemon.StateUpdate{
		UpdateType: daemon.UpdateTypeThemeChanged,
		Source:     "config",
		// Generic map payload mirrors what the SSE client decodes.
		Payload: map[string]interface{}{
			"name":   name,
			"family": name,
			"mode":   "hex",
			"dark":   map[string]interface{}{"name": name + "-dark", "appearance": "dark", "bg": "#1f1f28"},
		},
	}
}

func TestHandleUpdate_ThemeChangedEmitsMsg(t *testing.T) {
	// Pin the theme so SetTheme self-no-ops and the test never mutates the
	// package-global DefaultTheme.
	t.Setenv("GROVE_THEME", "kanagawa")

	cmd := HandleUpdate(themeUpdate("kanagawa"))
	if cmd == nil {
		t.Fatal("expected a command for theme_changed")
	}
	msg, ok := cmd().(ThemeChangedMsg)
	if !ok {
		t.Fatalf("expected ThemeChangedMsg, got %T", cmd())
	}
	if msg.Payload.Name != "kanagawa" {
		t.Errorf("expected payload name kanagawa, got %q", msg.Payload.Name)
	}
	if msg.Payload.Dark == nil || msg.Payload.Dark.Bg != "#1f1f28" {
		t.Errorf("dark palette did not survive decoding: %+v", msg.Payload.Dark)
	}
}

func TestHandleUpdate_ThemeChangedFromInitialSnapshot(t *testing.T) {
	t.Setenv("GROVE_THEME", "kanagawa")

	cmd := HandleUpdate(daemon.StateUpdate{
		UpdateType: "initial",
		Theme:      &daemon.ThemeChangedPayload{Name: "kanagawa", Family: "kanagawa", Mode: "hex"},
	})
	if cmd == nil {
		t.Fatal("expected a command for an initial snapshot carrying a theme")
	}
	msg, ok := cmd().(ThemeChangedMsg)
	if !ok || msg.Payload.Name != "kanagawa" {
		t.Fatalf("expected ThemeChangedMsg for initial snapshot, got %#v", cmd())
	}
}

func TestHandleUpdate_UnknownThemeIsIgnored(t *testing.T) {
	t.Setenv("GROVE_THEME", "") // unpinned: SetTheme runs and rejects the name

	if cmd := HandleUpdate(themeUpdate("definitely-not-a-theme")); cmd != nil {
		t.Error("expected no command when the theme is unknown to the registry")
	}
}

func TestHandleUpdate_UnrelatedUpdatesReturnNil(t *testing.T) {
	for _, updateType := range []string{"workspaces", "config_reload", "sessions", "initial"} {
		if cmd := HandleUpdate(daemon.StateUpdate{UpdateType: updateType}); cmd != nil {
			t.Errorf("expected nil command for %q", updateType)
		}
	}
}

func TestHandleUpdate_AttachAgentPaneStillWorks(t *testing.T) {
	cmd := HandleUpdate(daemon.StateUpdate{
		UpdateType: "attach_agent_pane",
		Payload: map[string]interface{}{
			"job_id": "job-1",
			"pty_id": "pty-1",
		},
	})
	if cmd == nil {
		t.Fatal("expected a command for attach_agent_pane")
	}
	msg, ok := cmd().(AttachAgentPaneMsg)
	if !ok || msg.JobID != "job-1" || msg.PtyID != "pty-1" {
		t.Fatalf("expected AttachAgentPaneMsg, got %#v", cmd())
	}
}
