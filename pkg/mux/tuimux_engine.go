package mux

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/grovetools/tuimux"
)

type TuimuxEngine struct {
	api        *tuimux.ApiClient
	socketPath string
}

func NewTuimuxEngine() (*TuimuxEngine, error) {
	socketPath := GetTuimuxSocketPath()
	api, err := tuimux.EnsureDaemon(socketPath)
	if err != nil {
		return nil, fmt.Errorf("tuimux daemon: %w", err)
	}
	return &TuimuxEngine{api: api, socketPath: socketPath}, nil
}

// NewTuimuxEngineWithSocket connects to an existing tuimux daemon at the given socket path.
// Unlike NewTuimuxEngine, it does not attempt to start a daemon.
func NewTuimuxEngineWithSocket(socketPath string) (*TuimuxEngine, error) {
	api := tuimux.NewApiClient(socketPath)
	if err := api.Ping(); err != nil {
		return nil, fmt.Errorf("tuimux daemon not reachable at %s: %w", socketPath, err)
	}
	return &TuimuxEngine{api: api, socketPath: socketPath}, nil
}

func (e *TuimuxEngine) StartServer(ctx context.Context, name string, opts ...SessionOption) error {
	cfg := applySessionOptions(opts)

	if err := e.api.CreateServer(name); err != nil {
		return fmt.Errorf("create tuimux server: %w", err)
	}

	args := []string{"new", "-s", name, "-d"}
	cmd := exec.CommandContext(ctx, "tuimux", args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start tuimux primary: %w", err)
	}
	_ = cmd.Process.Release()

	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		servers, err := e.api.ListServers()
		if err != nil {
			continue
		}
		for _, s := range servers {
			if s.Name == name && s.ClientCount > 0 {
				return nil
			}
		}
	}
	return nil
}

func (e *TuimuxEngine) KillServer(ctx context.Context, name string) error {
	return e.api.DeleteServer(name)
}

func (e *TuimuxEngine) ListServers(ctx context.Context) ([]ServerInfo, error) {
	servers, err := e.api.ListServers()
	if err != nil {
		return nil, err
	}
	result := make([]ServerInfo, len(servers))
	for i, s := range servers {
		result[i] = ServerInfo{
			Name:        s.Name,
			ClientCount: s.ClientCount,
		}
	}
	return result, nil
}

func (e *TuimuxEngine) resolveServerName() string {
	if name := os.Getenv(EnvTuimuxSession); name != "" {
		return name
	}
	servers, err := e.api.ListServers()
	if err == nil && len(servers) > 0 {
		return servers[0].Name
	}
	return ""
}

func (e *TuimuxEngine) CreateSession(ctx context.Context, name string, opts ...SessionOption) error {
	serverName := e.resolveServerName()
	if serverName == "" {
		return fmt.Errorf("no tuimux server available")
	}
	cfg := applySessionOptions(opts)
	args := []string{"new-session", "-t", name}
	if cfg.WorkDir != "" {
		args = append(args, "-c", cfg.WorkDir)
	}
	result, err := e.api.Execute(serverName, args)
	if err != nil {
		return fmt.Errorf("create tuimux session: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("create tuimux session: %s", result.Error)
	}
	return nil
}

func (e *TuimuxEngine) KillSession(ctx context.Context, name string) error {
	serverName := e.resolveServerName()
	if serverName == "" {
		return fmt.Errorf("no tuimux server available")
	}
	result, err := e.api.Execute(serverName, []string{"kill-session", "-t", name})
	if err != nil {
		return fmt.Errorf("kill tuimux session: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("kill tuimux session: %s", result.Error)
	}
	return nil
}

func (e *TuimuxEngine) ListSessions(ctx context.Context) ([]SessionInfo, error) {
	serverName := e.resolveServerName()
	if serverName == "" {
		return nil, fmt.Errorf("no tuimux server available")
	}
	result, err := e.api.Execute(serverName, []string{"list-sessions"})
	if err != nil {
		return nil, fmt.Errorf("list tuimux sessions: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("list tuimux sessions: %s", result.Error)
	}
	var names []string
	if err := json.Unmarshal([]byte(result.Output), &names); err != nil {
		return nil, fmt.Errorf("parse session list: %w", err)
	}
	sessions := make([]SessionInfo, len(names))
	for i, n := range names {
		sessions[i] = SessionInfo{Name: n}
	}
	return sessions, nil
}

func (e *TuimuxEngine) SessionExists(ctx context.Context, name string) (bool, error) {
	sessions, err := e.ListSessions(ctx)
	if err != nil {
		return false, err
	}
	for _, s := range sessions {
		if s.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (e *TuimuxEngine) SendKeys(ctx context.Context, target string, keys ...string) error {
	session, pane := splitTarget(target)
	args := []string{"send-keys"}
	if pane != "" {
		args = append(args, "-t", pane)
	}
	args = append(args, keys...)
	result, err := e.api.Execute(session, args)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("send-keys failed: %s", result.Error)
	}
	return nil
}

func (e *TuimuxEngine) CapturePane(ctx context.Context, target string) (string, error) {
	session, pane := splitTarget(target)
	args := []string{"capture-pane", "-p"}
	if pane != "" {
		args = append(args, "-t", pane)
	}
	result, err := e.api.Execute(session, args)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("capture-pane failed: %s", result.Error)
	}
	return result.Output, nil
}

func (e *TuimuxEngine) WaitForIdle(ctx context.Context, target string, timeout time.Duration) error {
	session, _ := splitTarget(target)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		result, err := e.api.Execute(session, []string{"list-panes", "--json"})
		if err == nil && result.ExitCode == 0 {
			var panes []struct {
				Idle bool `json:"idle"`
			}
			if json.Unmarshal([]byte(result.Output), &panes) == nil {
				allIdle := len(panes) > 0
				for _, p := range panes {
					if !p.Idle {
						allIdle = false
						break
					}
				}
				if allIdle {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timeout waiting for idle on %s", target)
}

func (e *TuimuxEngine) WaitForText(ctx context.Context, target string, pattern string, timeout time.Duration) (string, error) {
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

func (e *TuimuxEngine) Run(ctx context.Context, target string, command string, timeout time.Duration) (string, error) {
	session, _ := splitTarget(target)
	result, err := e.api.Execute(session, []string{"run", command})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return result.Output, fmt.Errorf("run failed (exit %d): %s", result.ExitCode, result.Error)
	}
	return result.Output, nil
}

func (e *TuimuxEngine) SplitWindow(ctx context.Context, target string, horizontal bool) (string, error) {
	session, pane := splitTarget(target)
	args := []string{"split-window"}
	if horizontal {
		args = append(args, "-h")
	}
	if pane != "" {
		args = append(args, "-t", pane)
	}
	result, err := e.api.Execute(session, args)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("split-window failed: %s", result.Error)
	}
	return "", nil
}

func (e *TuimuxEngine) ListPanes(ctx context.Context, sessionName string) ([]PaneInfo, error) {
	result, err := e.api.Execute(sessionName, []string{"list-panes", "--json"})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("list-panes failed: %s", result.Error)
	}
	var raw []struct {
		ID                string `json:"id"`
		Active            bool   `json:"active"`
		Idle              bool   `json:"idle"`
		ForegroundProcess string `json:"foreground_process"`
		Cwd               string `json:"cwd"`
		PID               int    `json:"pid"`
	}
	if err := json.Unmarshal([]byte(result.Output), &raw); err != nil {
		return nil, fmt.Errorf("parse panes: %w", err)
	}
	panes := make([]PaneInfo, len(raw))
	for i, r := range raw {
		panes[i] = PaneInfo{
			ID:                r.ID,
			Active:            r.Active,
			Idle:              r.Idle,
			ForegroundProcess: r.ForegroundProcess,
			Cwd:               r.Cwd,
			PID:               r.PID,
		}
	}
	return panes, nil
}

func (e *TuimuxEngine) NewWindow(ctx context.Context, sessionName string, windowName string, workDir string, detached bool) error {
	args := []string{"new-window", "-n", windowName}
	if workDir != "" {
		args = append(args, "-c", workDir)
	}
	if detached {
		args = append(args, "-d")
	}
	result, err := e.api.Execute(sessionName, args)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("new-window failed: %s", result.Error)
	}
	return nil
}

func (e *TuimuxEngine) GetSessionPID(ctx context.Context, sessionName string) (int, error) {
	pid := os.Getpid()
	return pid, nil
}

func (e *TuimuxEngine) SwitchSession(ctx context.Context, name string, cwd string) error {
	currentSession := os.Getenv(EnvTuimuxSession)
	if currentSession == "" {
		return fmt.Errorf("not inside a tuimux session (TUIMUX_SESSION not set)")
	}
	args := []string{"switch-session", "-t", name}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	result, err := e.api.Execute(currentSession, args)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("switch-session failed: %s", result.Error)
	}
	return nil
}

func (e *TuimuxEngine) GetPanePID(ctx context.Context, target string) (int, error) {
	session, paneID := splitTarget(target)
	if session == "" {
		session = e.resolveServerName()
	}
	panes, err := e.ListPanes(ctx, session)
	if err != nil {
		return 0, err
	}
	for _, p := range panes {
		if paneID == "" && p.Active {
			return p.PID, nil
		}
		if p.ID == paneID {
			return p.PID, nil
		}
	}
	return 0, fmt.Errorf("pane %q not found", target)
}

func (e *TuimuxEngine) GetCurrentSession(ctx context.Context) (string, error) {
	name := os.Getenv(EnvTuimuxSession)
	if name == "" {
		return "", fmt.Errorf("not inside a tuimux session (TUIMUX_SESSION not set)")
	}
	return name, nil
}

func (e *TuimuxEngine) SelectWindow(ctx context.Context, target string) error {
	return e.SwitchSession(ctx, target, "")
}

func (e *TuimuxEngine) KillWindow(ctx context.Context, target string) error {
	return ErrNotImplemented
}

func (e *TuimuxEngine) ListWindows(ctx context.Context, session string) ([]WindowInfo, error) {
	sessions, err := e.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	currentSession := os.Getenv(EnvTuimuxSession)
	windows := make([]WindowInfo, len(sessions))
	for i, s := range sessions {
		windows[i] = WindowInfo{
			ID:       s.Name,
			Index:    i,
			Name:     s.Name,
			IsActive: s.Name == currentSession,
		}
	}
	return windows, nil
}

func (e *TuimuxEngine) PaneExists(ctx context.Context, target string) (bool, error) {
	session, paneID := splitTarget(target)
	if session == "" {
		session = e.resolveServerName()
	}
	panes, err := e.ListPanes(ctx, session)
	if err != nil {
		return false, nil
	}
	if paneID == "" {
		return len(panes) > 0, nil
	}
	for _, p := range panes {
		if p.ID == paneID {
			return true, nil
		}
	}
	return false, nil
}

func (e *TuimuxEngine) GetPaneCommand(ctx context.Context, target string) (string, error) {
	session, paneID := splitTarget(target)
	if session == "" {
		session = e.resolveServerName()
	}
	panes, err := e.ListPanes(ctx, session)
	if err != nil {
		return "", err
	}
	for _, p := range panes {
		if paneID == "" && p.Active {
			return p.ForegroundProcess, nil
		}
		if p.ID == paneID {
			return p.ForegroundProcess, nil
		}
	}
	return "", fmt.Errorf("pane %q not found", target)
}

func (e *TuimuxEngine) GetSessionPath(ctx context.Context, session string) (string, error) {
	panes, err := e.ListPanes(ctx, session)
	if err != nil {
		return "", err
	}
	for _, p := range panes {
		if p.Active {
			return p.Cwd, nil
		}
	}
	if len(panes) > 0 {
		return panes[0].Cwd, nil
	}
	return "", fmt.Errorf("no panes found in session %q", session)
}

func (e *TuimuxEngine) WaitForSessionClose(ctx context.Context, session string, interval time.Duration) error {
	for {
		exists, err := e.SessionExists(ctx, session)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (e *TuimuxEngine) GetCursorPosition(ctx context.Context, target string) (int, int, error) {
	return 0, 0, ErrNotImplemented
}

func (e *TuimuxEngine) Launch(ctx context.Context, opts LaunchOptions) error {
	if opts.SessionName == "" {
		return fmt.Errorf("session name is required")
	}

	var createOpts []SessionOption
	if opts.WorkingDirectory != "" {
		createOpts = append(createOpts, WithWorkDir(opts.WorkingDirectory))
	}
	if err := e.CreateSession(ctx, opts.SessionName, createOpts...); err != nil {
		return fmt.Errorf("launch: create session: %w", err)
	}

	for i, pane := range opts.Panes {
		target := opts.SessionName
		if i > 0 {
			_, err := e.SplitWindow(ctx, target, false)
			if err != nil {
				return fmt.Errorf("launch: split pane %d: %w", i, err)
			}
			target = opts.SessionName + ":" + strconv.Itoa(i)
		}
		if len(pane.Env) > 0 {
			for k, v := range pane.Env {
				_ = e.SendKeys(ctx, target, fmt.Sprintf("export %s=%q", k, v), "C-m")
			}
		}
		if pane.Command != "" {
			if err := e.SendKeys(ctx, target, pane.Command, "C-m"); err != nil {
				return fmt.Errorf("launch: send command to pane %d: %w", i, err)
			}
		}
		if pane.SendKeys != "" {
			if err := e.SendKeys(ctx, target, pane.SendKeys, "C-m"); err != nil {
				return fmt.Errorf("launch: send keys to pane %d: %w", i, err)
			}
		}
	}

	return nil
}

// MuxTUIEngine methods — not yet implemented for tuimux.

func (e *TuimuxEngine) OpenInEditorWindow(ctx context.Context, editorCmd, filePath, windowName string, windowIndex int, reset bool) error {
	return ErrNotImplemented
}

func (e *TuimuxEngine) FocusOrRunCommandInWindow(ctx context.Context, cmd, windowName string, windowIndex int) error {
	return ErrNotImplemented
}

func (e *TuimuxEngine) ClosePopup(ctx context.Context) error {
	return ErrNotImplemented
}

func (e *TuimuxEngine) IsPopup(ctx context.Context) (bool, error) {
	return false, nil
}

func splitTarget(target string) (session, pane string) {
	if i := strings.Index(target, ":"); i >= 0 {
		return target[:i], target[i+1:]
	}
	return target, ""
}

var (
	_ MuxEngine    = (*TuimuxEngine)(nil)
	_ MuxTUIEngine = (*TuimuxEngine)(nil)
)
