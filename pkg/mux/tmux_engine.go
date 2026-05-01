package mux

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/tmux"
)

type TmuxEngine struct {
	client *tmux.Client
}

func NewTmuxEngine() (*TmuxEngine, error) {
	client, err := tmux.NewClient()
	if err != nil {
		return nil, err
	}
	return &TmuxEngine{client: client}, nil
}

// NewTmuxEngineWithSocket creates a TmuxEngine connected to a specific tmux socket.
func NewTmuxEngineWithSocket(socket string) (*TmuxEngine, error) {
	client, err := tmux.NewClientWithSocket(socket)
	if err != nil {
		return nil, err
	}
	return &TmuxEngine{client: client}, nil
}

func (e *TmuxEngine) Client() *tmux.Client {
	return e.client
}

func (e *TmuxEngine) CreateSession(ctx context.Context, name string, opts ...SessionOption) error {
	cfg := applySessionOptions(opts)
	args := []string{"new-session", "-d", "-s", name, "-n", "workspace"}
	if cfg.WorkDir != "" {
		args = append(args, "-c", cfg.WorkDir)
	}
	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux new-session failed: %w: %s", err, string(output))
	}
	return nil
}

func (e *TmuxEngine) KillSession(ctx context.Context, name string) error {
	return e.client.KillSession(ctx, name)
}

func (e *TmuxEngine) ListSessions(ctx context.Context) ([]SessionInfo, error) {
	names, err := e.client.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	sessions := make([]SessionInfo, len(names))
	for i, n := range names {
		sessions[i] = SessionInfo{Name: n}
	}
	return sessions, nil
}

func (e *TmuxEngine) SessionExists(ctx context.Context, name string) (bool, error) {
	return e.client.SessionExists(ctx, name)
}

func (e *TmuxEngine) SendKeys(ctx context.Context, target string, keys ...string) error {
	return e.client.SendKeys(ctx, target, keys...)
}

func (e *TmuxEngine) CapturePane(ctx context.Context, target string) (string, error) {
	return e.client.CapturePane(ctx, target)
}

func (e *TmuxEngine) WaitForIdle(ctx context.Context, target string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		output, err := e.CapturePane(ctx, target)
		if err != nil {
			return err
		}
		if isAgentIdleFromOutput(output) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timeout waiting for idle on %s", target)
}

func (e *TmuxEngine) WaitForText(ctx context.Context, target string, pattern string, timeout time.Duration) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		output, err := e.CapturePane(ctx, target)
		if err != nil {
			return "", err
		}
		if match := re.FindString(output); match != "" {
			return match, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("timeout waiting for pattern %q on %s", pattern, target)
}

func (e *TmuxEngine) Run(ctx context.Context, target string, command string, timeout time.Duration) (string, error) {
	if err := e.client.SendKeys(ctx, target, command, "C-m"); err != nil {
		return "", err
	}
	time.Sleep(500 * time.Millisecond)
	return e.CapturePane(ctx, target)
}

func (e *TmuxEngine) SplitWindow(ctx context.Context, target string, horizontal bool) (string, error) {
	return e.client.SplitWindow(ctx, target, horizontal, 0, "")
}

func (e *TmuxEngine) ListPanes(ctx context.Context, sessionName string) ([]PaneInfo, error) {
	output, err := e.client.Run(ctx, "list-panes", "-t", sessionName, "-F",
		`#{pane_id}:#{?pane_active,1,0}:#{pane_current_command}:#{pane_current_path}`)
	if err != nil {
		return nil, err
	}
	var panes []PaneInfo
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 4)
		if len(parts) < 4 {
			continue
		}
		panes = append(panes, PaneInfo{
			ID:                parts[0],
			Active:            parts[1] == "1",
			ForegroundProcess: parts[2],
			Cwd:               parts[3],
		})
	}
	return panes, nil
}

func (e *TmuxEngine) NewWindow(ctx context.Context, sessionName string, windowName string, workDir string, detached bool) error {
	return e.client.NewWindowWithOptions(ctx, tmux.NewWindowOptions{
		Target:     sessionName,
		WindowName: windowName,
		WorkingDir: workDir,
		Detached:   detached,
	})
}

func (e *TmuxEngine) GetSessionPID(ctx context.Context, sessionName string) (int, error) {
	return e.client.GetSessionPID(ctx, sessionName)
}

func (e *TmuxEngine) SwitchSession(ctx context.Context, name string) error {
	return e.client.SwitchClientToSession(ctx, name)
}

// MuxTUIEngine methods

func (e *TmuxEngine) OpenInEditorWindow(ctx context.Context, editorCmd, filePath, windowName string, windowIndex int, reset bool) error {
	return e.client.OpenInEditorWindow(ctx, editorCmd, filePath, windowName, windowIndex, reset)
}

func (e *TmuxEngine) FocusOrRunCommandInWindow(ctx context.Context, cmd, windowName string, windowIndex int) error {
	return e.client.FocusOrRunCommandInWindow(ctx, cmd, windowName, windowIndex)
}

func (e *TmuxEngine) ClosePopup(ctx context.Context) error {
	return e.client.ClosePopup(ctx)
}

func (e *TmuxEngine) IsPopup(ctx context.Context) (bool, error) {
	return e.client.IsPopup(ctx)
}

func isAgentIdleFromOutput(output string) bool {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	start := len(lines) - 10
	if start < 0 {
		start = 0
	}
	for _, l := range lines[start:] {
		if strings.Contains(l, "INSERT") || strings.Contains(l, "Tokens:") || strings.Contains(l, "❯") {
			return true
		}
	}
	return false
}

var (
	_ MuxEngine    = (*TmuxEngine)(nil)
	_ MuxTUIEngine = (*TmuxEngine)(nil)
)
