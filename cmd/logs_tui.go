package cmd

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/daemon"
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
// bubbletea program. It connects to the daemon's aggregated log
// stream instead of doing local file tailing.
func runLogsTUI(workspaces []*workspace.WorkspaceNode, follow bool, overrideOpts *logging.OverrideOptions, scope string, includeSystem bool, level string) error {
	logCfg := logging.GetDefaultLoggingConfig()
	if cfg, err := config.LoadDefault(); err == nil {
		_ = cfg.UnmarshalExtension("logging", &logCfg)
	}

	var initialPath string
	if len(workspaces) > 0 && workspaces[0] != nil {
		initialPath = workspaces[0].Path
	}

	cwd, _ := os.Getwd()
	daemonClient := daemon.NewWithAutoStart(cwd)

	cfg := logs.Config{
		DaemonClient:         daemonClient,
		InitialScope:         scope,
		IncludeSystem:        includeSystem,
		LogConfig:            &logCfg,
		OverrideOpts:         overrideOpts,
		Follow:               follow,
		InitialWorkspacePath: initialPath,
		Replay:               500,
		InitialLevel:         level,
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
