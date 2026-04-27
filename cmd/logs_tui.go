package cmd

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/logs"
)

// standaloneLogs wraps the embeddable logs.Model for standalone CLI
// execution. It intercepts embed.DoneMsg (which the inner model now
// emits in place of tea.Quit) and converts it back into tea.Quit so
// the bubbletea program exits cleanly. Everything else passes through.
type standaloneLogs struct {
	inner *logs.Model
}

func (s standaloneLogs) Init() tea.Cmd { return s.inner.Init() }

func (s standaloneLogs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(embed.DoneMsg); ok {
		return s, tea.Quit
	}
	model, cmd := s.inner.Update(msg)
	if m, ok := model.(*logs.Model); ok {
		s.inner = m
	}
	return s, cmd
}

func (s standaloneLogs) View() string { return s.inner.View() }

// runLogsTUI launches the interactive logs TUI as a standalone
// bubbletea program. It builds a logs.Config rooted at the workspaces
// passed in on the command line and captures them in a closure so the
// discovery goroutine can re-read the set on every tick (the CLI set
// is static, so the closure just returns the same slice).
func runLogsTUI(workspaces []*workspace.WorkspaceNode, follow bool, overrideOpts *logging.OverrideOptions, systemOnly, includeSystem, ecosystem bool) error {
	// Load logging config for component filtering, starting with defaults
	logCfg := logging.GetDefaultLoggingConfig()
	if cfg, err := config.LoadDefault(); err == nil {
		_ = cfg.UnmarshalExtension("logging", &logCfg)
	}

	ws := workspaces
	cfg := logs.Config{
		GetWorkspaces:  func() []*workspace.WorkspaceNode { return ws },
		Ecosystem:      ecosystem,
		SystemOnly:     systemOnly,
		IncludeSystem:  includeSystem,
		LogConfig:      &logCfg,
		OverrideOpts:   overrideOpts,
		Follow:         follow,
		ReplayExisting: true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inner := logs.New(ctx, cfg)
	defer inner.Close()

	p := tea.NewProgram(standaloneLogs{inner: inner}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}
	return nil
}
